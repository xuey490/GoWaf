// Package config 负责 YAML 配置的加载、默认值与校验。
//
// 设计要点：
//  1. 所有时长字段用自定义 Duration 类型，支持 "5s" / "2h" 这类字符串；
//  2. 校验失败直接拒绝启动，绝不带病运行（WAF 配错比不装更危险）；
//  3. 动态规则不在这里，配置只承载"启动后不易变"的基础设施参数。
package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration 是自定义时长类型：让 YAML 支持 "5s" / "2h" / "1m30s" 这类人类可读写法。
type Duration time.Duration

// UnmarshalYAML 实现 yaml.Unmarshaler：把 YAML 节点解析为时长。
// 参数 node：YAML 标量节点，取值为字符串（如 "5s"）或纯数字（视为秒）。
// 返回 error：空串或 "0" 视为零时长；格式非法时返回解析错误。
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		// 兼容纯数字（视为秒）
		var n int64
		if err2 := node.Decode(&n); err2 == nil {
			*d = Duration(time.Duration(n) * time.Second)
			return nil
		}
		return err
	}
	if s == "" || s == "0" {
		*d = 0
		return nil
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("非法时长 %q: %w", s, err)
	}
	*d = Duration(v)
	return nil
}

// MarshalYAML 实现 yaml.Marshaler：把时长序列化为人类可读的字符串形式。
// 返回 (any, error)：零值输出 "0s"，其余输出标准库时长字符串（如 "1m30s"）；不产生错误。
func (d Duration) MarshalYAML() (any, error) {
	if d == 0 {
		return "0s", nil
	}
	return time.Duration(d).String(), nil
}

// Duration 把自定义时长转换为标准库的 time.Duration，便于直接参与时间运算。
// 返回 time.Duration：等价的标准库时长值。
func (d Duration) Duration() time.Duration { return time.Duration(d) }

// Config 是整份配置：对应配置文件的顶层结构，按功能小节划分。
type Config struct {
	Server  Server     `yaml:"server"`  // 数据面/控制面的监听地址与超时
	TLS     TLS        `yaml:"tls"`     // HTTPS 终止与证书配置
	Storage Storage    `yaml:"storage"` // 数据库位置与数据保留/采样策略
	Engine  Engine     `yaml:"engine"`  // WAF 检测引擎参数
	Guard   Guard      `yaml:"guard"`   // 限流与自动封禁策略
	Admin   Admin      `yaml:"admin"`   // 管理后台（控制面）配置
	Demo    Demo       `yaml:"demo"`    // 演示模式配置
	Alert   Alert      `yaml:"alert"`   // 告警通知配置（预留）
	Log     Log        `yaml:"log"`     // 日志配置（预留）
	Sites   []SiteSeed `yaml:"sites"`   // 首次启动时写入数据库的站点种子列表
}

// Server 数据面与控制面的监听地址、超时参数。
type Server struct {
	Listen            string   `yaml:"listen"`              // 数据面 HTTP 监听地址，如 "0.0.0.0:80"
	TLSListen         string   `yaml:"tls_listen"`          // 数据面 HTTPS 监听地址；tls.enabled=true 时必填
	AdminListen       string   `yaml:"admin_listen"`        // 控制面监听地址，默认只绑回环 127.0.0.1:8899
	ReadHeaderTimeout Duration `yaml:"read_header_timeout"` // 读取请求头的超时，防慢速连接占用资源
	ReadTimeout       Duration `yaml:"read_timeout"`        // 读取整个请求（含 body）的超时
	WriteTimeout      Duration `yaml:"write_timeout"`       // 写响应的超时
	IdleTimeout       Duration `yaml:"idle_timeout"`        // keep-alive 空闲连接的最大保持时长
	MaxHeaderBytes    int      `yaml:"max_header_bytes"`    // 请求头最大字节数，防超大请求头攻击
	ShutdownTimeout   Duration `yaml:"shutdown_timeout"`    // 优雅关闭时等待在途请求完成的最长时间
	TrustedProxies    []string `yaml:"trusted_proxies"`     // 可信代理网段（CIDR）：仅这些来源的 X-Forwarded-For 会被采信
}

// CertPair 显式声明的一组证书与私钥文件。
type CertPair struct {
	Cert string `yaml:"cert"` // 证书文件（PEM）路径
	Key  string `yaml:"key"`  // 私钥文件（PEM）路径
}

// TLS HTTPS 终止相关配置：证书来源、SNI 回退与安全选项。
type TLS struct {
	Enabled     bool       `yaml:"enabled"`      // 是否启用数据面 HTTPS 监听
	CertDir     string     `yaml:"cert_dir"`     // 证书目录：扫描其下「域名/fullchain.pem + privkey.pem」子目录自动加载
	DefaultCert string     `yaml:"default_cert"` // 默认证书域名：SNI 未匹配或客户端未携带 SNI 时回退使用
	MinVersion  string     `yaml:"min_version"`  // 最低 TLS 版本：仅 "1.3" 生效，否则一律按 TLS 1.2
	HSTS        bool       `yaml:"hsts"`         // 是否在 HTTPS 响应中下发 HSTS 头
	Certs       []CertPair `yaml:"certs"`        // 显式列出的证书对（与 cert_dir 可同时使用）
}

// Storage 存储相关配置：数据库位置与数据保留/采样策略。
type Storage struct {
	DSN         string  `yaml:"dsn"`          // 数据库连接串：SQLite 文件路径，默认 data/waf.db
	RetainDays  int     `yaml:"retain_days"`  // 事件保留天数（预留：清理实际由计划任务 clean_events 的参数控制）
	ArchiveDir  string  `yaml:"archive_dir"`  // 归档输出目录（预留字段，当前版本未使用）
	ArchiveDays int     `yaml:"archive_days"` // 归档间隔天数（预留字段，当前版本未使用）
	SampleRate  float64 `yaml:"sample_rate"`  // 事件采样率（0~1）：拦截/挑战类事件必记录，其余按该比例采样落库
}

// Engine 检测引擎参数：运行模式、拦截阈值与规则来源。
type Engine struct {
	Mode           string `yaml:"mode"`             // 运行模式：observe 只记录不拦截；protect 执行拦截
	BlockScore     int    `yaml:"block_score"`      // 请求累计风险分达到该值时拦截（可被后台设置覆盖）
	ChallengeScore int    `yaml:"challenge_score"`  // 风险分达到该值时触发人机挑战（可被后台设置覆盖）
	MaxBodyInspect int    `yaml:"max_body_inspect"` // 参与检测的请求 body 最大字节数，超出部分不做检测
	MaxTargets     int    `yaml:"max_targets"`      // 单请求检测目标数上限（预留字段，当前版本未使用）
	RegexTimeoutMS int    `yaml:"regex_timeout_ms"` // 单条正则执行超时毫秒数（预留字段，当前版本未使用）
	RulesFile      string `yaml:"rules_file"`       // 内置规则文件路径，首次启动导入数据库
}

// Guard 限流与封禁参数。
type Guard struct {
	GlobalRPS          int `yaml:"global_rps"`           // 全局每秒请求数上限（预留字段，当前版本未使用）
	IPRPS              int `yaml:"ip_rps"`               // 单 IP 每秒请求数阈值（令牌桶速率）
	IPBurst            int `yaml:"ip_burst"`             // 单 IP 令牌桶突发容量，允许的瞬时峰值
	IPPerMinute        int `yaml:"ip_per_minute"`        // 单 IP 每分钟请求数上限
	ChallengePerMinute int `yaml:"challenge_per_minute"` // 单 IP 每分钟超过该请求数后下发人机校验（0=关闭）
	ChallengeRPS       int `yaml:"challenge_rps"`        // 单 IP 每秒超过该请求数后下发人机校验（0=关闭）
	// ChallengeReferer 跨站来路是否触发人机校验：外站跳转进来（Referer 域名不属于本站，
	// 且不在允许清单里）的页面导航先过一道自动校验，挡掉广告链/群发链接带来的脚本流量。
	ChallengeReferer      *bool    `yaml:"challenge_referer"`
	ChallengeRefererAllow string   `yaml:"challenge_referer_allow"` // 允许的跨站来路（逗号分隔，搜索引擎等）
	BanPerMinute          int      `yaml:"ban_per_minute"`          // 每分钟封禁次数上限（预留字段，当前版本未使用）
	Ban1hPerMinute        int      `yaml:"ban_1h_per_minute"`       // 每分钟 1 小时长封次数上限（预留字段，当前版本未使用）
	BanTTL                Duration `yaml:"ban_ttl"`                 // 默认封禁时长
	BanMaxTTL             Duration `yaml:"ban_max_ttl"`             // 连击升级后允许的最长封禁时长
	AutoBan               bool     `yaml:"auto_ban"`                // 是否启用自动封禁（可被后台设置覆盖）
	WhitelistStatic       bool     `yaml:"whitelist_static"`        // 静态资源是否绕过限流（预留字段，当前版本未使用）

	// 自动封禁策略：window 内命中 threshold 次 → 封 ban_ttl；
	// 解封后再次触发，连击数 +1，第 ban_max_strikes 次直接永久（expires_at = 0）。
	// 两次触发之间超过 ban_strike_reset 没有新命中，连击数清零。
	BanWindow      Duration `yaml:"ban_window"`       // 连击统计窗口时长
	BanThreshold   int      `yaml:"ban_threshold"`    // 窗口内触发封禁的命中次数阈值
	BanMaxStrikes  int      `yaml:"ban_max_strikes"`  // 连击达到该次数后转为永久封禁
	BanStrikeReset Duration `yaml:"ban_strike_reset"` // 连击计数重置间隔：超过该时长无新命中则清零
}

// CORS 管理后台的跨域（CORS）配置。
type CORS struct {
	Enabled          bool     `yaml:"enabled"`           // 是否启用 CORS 响应头处理
	AllowOrigins     []string `yaml:"allow_origins"`     // 允许的来源列表；带凭证时禁止 "*"
	AllowMethods     []string `yaml:"allow_methods"`     // 允许的 HTTP 方法
	AllowHeaders     []string `yaml:"allow_headers"`     // 允许的请求头
	ExposeHeaders    []string `yaml:"expose_headers"`    // 允许浏览器脚本读取的响应头
	AllowCredentials bool     `yaml:"allow_credentials"` // 是否允许携带凭证（Cookie）
	MaxAge           Duration `yaml:"max_age"`           // 预检结果的缓存时长
}

// CSRF 管理后台的 CSRF 防护（双提交 Cookie）配置。
type CSRF struct {
	Enabled    bool   `yaml:"enabled"`     // 是否启用 CSRF 校验
	Header     string `yaml:"header"`      // 请求携带令牌的请求头名称
	CookieName string `yaml:"cookie_name"` // 下发令牌的 Cookie 名称
}

// Admin 管理后台（控制面）配置：访问控制、令牌时效与登录防护。
type Admin struct {
	AllowedIPs        []string `yaml:"allowed_ips"`            // 允许访问后台的 IP 白名单；为空表示不限制
	JWTSecretFile     string   `yaml:"jwt_secret_file"`        // JWT 签名密钥文件路径；文件缺失或过短时自动生成随机密钥写入
	AccessTTL         Duration `yaml:"access_ttl"`             // 访问令牌（access token）有效期
	RefreshTTL        Duration `yaml:"refresh_ttl"`            // 刷新令牌（refresh token）有效期
	LoginMaxFail      int      `yaml:"login_max_fail"`         // 连续登录失败次数上限，超过后触发锁定
	LoginLock         Duration `yaml:"login_lock"`             // 登录锁定时长
	CookieSecure      bool     `yaml:"cookie_secure"`          // 会话 Cookie 是否带 Secure 属性（仅经 HTTPS 传输）
	CookieDomain      string   `yaml:"cookie_domain"`          // 会话 Cookie 的 Domain 属性；为空则不设置
	CORS              CORS     `yaml:"cors"`                   // 后台 API 的跨域配置
	CSRF              CSRF     `yaml:"csrf"`                   // 后台的 CSRF 防护配置
	IKnowWhatIAmDoing bool     `yaml:"i_know_what_i_am_doing"` // 知情开关：管理端口绑定到非回环地址时必须显式置 true 才能启动
}

// Demo 演示模式配置：使用独立数据库，向访客开放受限的体验环境。
type Demo struct {
	Enabled   bool   `yaml:"enabled"`    // 是否启用演示模式
	DBPath    string `yaml:"db_path"`    // 演示专用数据库路径，必须与主库 storage.dsn 不同
	Username  string `yaml:"username"`   // 演示账号用户名，默认 demo
	Password  string `yaml:"password"`   // 演示账号密码
	ReadOnly  bool   `yaml:"read_only"`  // 是否只读；启用演示模式时会被强制置为 true
	MaskIPs   bool   `yaml:"mask_ips"`   // 是否对页面展示的客户端 IP 打码
	ResetCron string `yaml:"reset_cron"` // 重置计划（预留字段：当前版本固定由 app 在每天凌晨 4 点重置）
	Banner    string `yaml:"banner"`     // 演示横幅提示文案，展示在面板顶部
}

// Alert 告警通知配置（预留）。
//
// 当前版本的通知渠道由后台「通知设置」动态读取，本节配置仅加载、未被消费。
type Alert struct {
	Webhook        string   `yaml:"webhook"`         // 告警 webhook 地址（预留字段，当前版本未使用）
	BlockThreshold int      `yaml:"block_threshold"` // 触发告警的拦截次数阈值（预留字段，当前版本未使用）
	Window         Duration `yaml:"window"`          // 告警统计窗口时长（预留字段，当前版本未使用）
}

// Log 日志配置（预留字段，当前版本未接入）。
type Log struct {
	Level      string `yaml:"level"`       // 日志级别：debug/info/warn/error
	File       string `yaml:"file"`        // 日志文件路径
	MaxSizeMB  int    `yaml:"max_size_mb"` // 单个日志文件最大 MB，超过后轮转
	MaxBackups int    `yaml:"max_backups"` // 保留的历史日志文件个数
}

// SiteSeed 首次启动写入的站点。
type SiteSeed struct {
	Name     string   `yaml:"name"`     // 站点显示名称
	Hosts    []string `yaml:"hosts"`    // 站点匹配的域名/主机列表
	Upstream string   `yaml:"upstream"` // 反代上游地址，必须以 http(s):// 开头
	// BehindCDN 站点是否在 CDN 回源之后，留空=直连，cloudflare=橙云。
	// 只影响控制台的风险自检，不改变取 IP 逻辑（那由 server.trusted_proxies 决定）。
	BehindCDN string `yaml:"behind_cdn"`
}

// Load 读取 YAML 配置文件并做默认值填充与校验。
// 参数 path：配置文件路径。
// 返回 (*Config, error)：成功返回填充好默认值的配置指针；文件读取失败、YAML 解析失败或校验不通过返回错误。
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置 %s: %w", path, err)
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("解析配置: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// applyDefaults 为未显式配置的字段填充安全默认值，保证零值配置也能正常启动。
// 仅就地修改 c，无参数、无返回值；必须在 Validate 之前调用。
func (c *Config) applyDefaults() {
	if c.Server.Listen == "" {
		c.Server.Listen = "0.0.0.0:80"
	}
	if c.Server.AdminListen == "" {
		c.Server.AdminListen = "127.0.0.1:8899"
	}
	if c.Server.ReadHeaderTimeout == 0 {
		c.Server.ReadHeaderTimeout = Duration(5 * time.Second)
	}
	if c.Server.IdleTimeout == 0 {
		c.Server.IdleTimeout = Duration(90 * time.Second)
	}
	if c.Server.MaxHeaderBytes == 0 {
		c.Server.MaxHeaderBytes = 32768
	}
	if c.Server.ShutdownTimeout == 0 {
		c.Server.ShutdownTimeout = Duration(15 * time.Second)
	}
	if len(c.Server.TrustedProxies) == 0 {
		// 默认只信任本机代理，避免伪造 X-Forwarded-For 绕过限流与封禁
		c.Server.TrustedProxies = []string{"127.0.0.1/32", "::1/128"}
	}
	if c.Storage.RetainDays == 0 {
		c.Storage.RetainDays = 30
	}
	if c.Storage.DSN == "" {
		c.Storage.DSN = "data/waf.db"
	}
	// 引擎默认以观察模式起步，阈值取常用基线：先观察误报，再切保护模式
	if c.Engine.Mode == "" {
		c.Engine.Mode = "observe"
	}
	if c.Engine.BlockScore == 0 {
		c.Engine.BlockScore = 60
	}
	if c.Engine.ChallengeScore == 0 {
		c.Engine.ChallengeScore = 30
	}
	if c.Engine.MaxBodyInspect == 0 {
		c.Engine.MaxBodyInspect = 256 * 1024
	}
	if c.Engine.MaxTargets == 0 {
		c.Engine.MaxTargets = 64
	}
	if c.Engine.RegexTimeoutMS == 0 {
		c.Engine.RegexTimeoutMS = 50
	}
	if c.Engine.RulesFile == "" {
		c.Engine.RulesFile = "rules/builtin.yaml"
	}
	// 限流与封禁默认值：单 IP 30 RPS / 突发 60，10 分钟窗口内 5 次触发升级封禁
	if c.Guard.IPRPS == 0 {
		c.Guard.IPRPS = 30
	}
	if c.Guard.IPBurst == 0 {
		c.Guard.IPBurst = 60
	}
	if c.Guard.IPPerMinute == 0 {
		c.Guard.IPPerMinute = 600
	}
	if c.Guard.ChallengePerMinute == 0 {
		c.Guard.ChallengePerMinute = 300
	}
	if c.Guard.ChallengeRPS == 0 {
		c.Guard.ChallengeRPS = 10
	}
	if c.Guard.ChallengeRefererAllow == "" {
		// 搜索引擎来路不挑战：落地页被挑战会直接影响收录与投放转化。
		c.Guard.ChallengeRefererAllow = "www.baidu.com,m.baidu.com,www.google.com,www.bing.com,cn.bing.com,www.so.com,www.sogou.com,www.sm.cn"
	}
	if c.Guard.BanPerMinute == 0 {
		c.Guard.BanPerMinute = 600
	}
	if c.Guard.Ban1hPerMinute == 0 {
		c.Guard.Ban1hPerMinute = 1200
	}
	if c.Guard.BanTTL == 0 {
		c.Guard.BanTTL = Duration(5 * time.Minute)
	}
	if c.Guard.BanMaxTTL == 0 {
		c.Guard.BanMaxTTL = Duration(24 * time.Hour)
	}
	if c.Guard.BanWindow == 0 {
		c.Guard.BanWindow = Duration(10 * time.Minute)
	}
	if c.Guard.BanThreshold == 0 {
		c.Guard.BanThreshold = 5
	}
	if c.Guard.BanMaxStrikes == 0 {
		c.Guard.BanMaxStrikes = 3
	}
	if c.Guard.BanStrikeReset == 0 {
		c.Guard.BanStrikeReset = Duration(24 * time.Hour)
	}
	// 管理后台默认值：令牌 2 小时、刷新 7 天，连错 5 次锁 15 分钟
	if c.Admin.AccessTTL == 0 {
		c.Admin.AccessTTL = Duration(2 * time.Hour)
	}
	if c.Admin.RefreshTTL == 0 {
		c.Admin.RefreshTTL = Duration(7 * 24 * time.Hour)
	}
	if c.Admin.LoginMaxFail == 0 {
		c.Admin.LoginMaxFail = 5
	}
	if c.Admin.LoginLock == 0 {
		c.Admin.LoginLock = Duration(15 * time.Minute)
	}
	if c.Admin.JWTSecretFile == "" {
		c.Admin.JWTSecretFile = "data/jwt.key"
	}
	// CSRF 双提交默认的请求头与 Cookie 名
	if c.Admin.CSRF.Header == "" {
		c.Admin.CSRF.Header = "X-CSRF-Token"
	}
	if c.Admin.CSRF.CookieName == "" {
		c.Admin.CSRF.CookieName = "waf_csrf"
	}
	if len(c.Admin.CORS.AllowMethods) == 0 {
		c.Admin.CORS.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	}
	if len(c.Admin.CORS.AllowHeaders) == 0 {
		c.Admin.CORS.AllowHeaders = []string{"Authorization", "Content-Type", "X-CSRF-Token", "X-Request-ID"}
	}
	// 演示账号默认用户名
	if c.Demo.Username == "" {
		c.Demo.Username = "demo"
	}
	// 演示模式强制只读：只要 demo.enabled=true，read_only 一律置为 true，
	// 即使显式写 read_only: false 也会被覆盖，防止演示库被写入脏数据
	if c.Demo.ReadOnly {
	} else if c.Demo.Enabled {
		c.Demo.ReadOnly = true
	}
	if c.Alert.Window == 0 {
		c.Alert.Window = Duration(5 * time.Minute)
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.File == "" {
		c.Log.File = "logs/waf.log"
	}
}

// Validate 启动前校验配置合法性，任何一条不通过都拒绝启动（绝不带病运行）。
// 返回 error：汇总列出所有不通过项；全部通过返回 nil。
func (c *Config) Validate() error {
	var errs []string

	if _, port, err := net.SplitHostPort(c.Server.Listen); err != nil || port == "" {
		errs = append(errs, fmt.Sprintf("server.listen 非法: %q", c.Server.Listen))
	}
	if _, port, err := net.SplitHostPort(c.Server.AdminListen); err != nil || port == "" {
		errs = append(errs, fmt.Sprintf("server.admin_listen 非法: %q", c.Server.AdminListen))
	}
	if c.TLS.Enabled && c.Server.TLSListen == "" {
		errs = append(errs, "tls.enabled=true 但 server.tls_listen 为空")
	}
	if c.TLS.Enabled && len(c.TLS.Certs) == 0 && c.TLS.CertDir == "" {
		errs = append(errs, "tls.enabled=true 但既没有 tls.certs 也没有 tls.cert_dir")
	}

	for _, s := range c.Sites {
		if err := validateUpstream(s.Upstream); err != nil {
			errs = append(errs, fmt.Sprintf("站点 %s 上游非法: %v", s.Name, err))
		}
	}

	if !(c.Engine.BlockScore > c.Engine.ChallengeScore && c.Engine.ChallengeScore > 0) {
		errs = append(errs, "必须满足 block_score > challenge_score > 0")
	}
	if c.Engine.Mode != "observe" && c.Engine.Mode != "protect" {
		errs = append(errs, fmt.Sprintf("engine.mode 只能是 observe/protect，当前 %q", c.Engine.Mode))
	}

	// 管理端口若不在回环地址，必须显式声明知情
	if !IsLoopbackHost(c.Server.AdminListen) && !c.Admin.IKnowWhatIAmDoing {
		errs = append(errs, "admin_listen 不在回环地址，需显式设置 admin.i_know_what_i_am_doing: true")
	}

	// CORS 带凭证时不允许 *
	if c.Admin.CORS.Enabled && c.Admin.CORS.AllowCredentials {
		for _, o := range c.Admin.CORS.AllowOrigins {
			if strings.TrimSpace(o) == "*" {
				errs = append(errs, "cors.allow_credentials=true 时 allow_origins 不能包含 *（浏览器规范禁止）")
			}
		}
	}

	// 演示库不能与主库同路径
	if c.Demo.Enabled {
		main, _ := filepath.Abs(c.Storage.DSN)
		demo, _ := filepath.Abs(c.Demo.DBPath)
		if demo == "" || main == demo {
			errs = append(errs, "demo.enabled=true 时 demo.db_path 必须配置且不能与 storage.dsn 相同")
		}
		if !IsLoopbackHost(c.Server.AdminListen) && !c.Admin.CookieSecure {
			errs = append(errs, "演示模式 + 非回环管理端口时，必须设置 admin.cookie_secure: true")
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("配置校验失败:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// validateUpstream 校验站点上游地址格式，防止把反代流量指到控制面自身（SSRF 自打）。
// 参数 u：上游地址字符串。
// 返回 error：要求非空、以 http(s):// 开头，且禁止指向控制面端口 8899；不满足时返回错误。
func validateUpstream(u string) error {
	if u == "" {
		return fmt.Errorf("为空")
	}
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return fmt.Errorf("必须是 http(s):// 开头")
	}
	host := strings.TrimPrefix(strings.TrimPrefix(u, "http://"), "https://")
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if strings.HasSuffix(host, ":8899") {
		return fmt.Errorf("禁止指向控制面 8899")
	}
	return nil
}

// IsLoopbackHost 判断监听地址的主机部分是否为回环地址（仅本机可访问）。
// 参数 addr：形如 "host:port" 的监听地址；端口解析失败时把整体视为主机部分处理。
// 返回 bool：主机为 127.0.0.1 / ::1 / localhost 或其他回环 IP 时返回 true。
func IsLoopbackHost(addr string) bool {
	h, _, err := net.SplitHostPort(addr)
	if err != nil {
		h = addr
	}
	switch h {
	case "127.0.0.1", "::1", "localhost":
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// BaseDir 返回配置中数据库文件（DSN）所在目录，用于派生数据目录下的其他文件位置（如初始口令文件、备份目录）。
// 返回 string：DSN 的目录部分。
func (c *Config) BaseDir() string {
	return filepath.Dir(c.Storage.DSN)
}

// RefererChallengeEnabled 返回「跨站来路是否触发人机校验」的生效值。
// 用指针类型是为了区分「没配置」和「显式关掉」：缺省开启（挡广告链与群发链接），
// 运维在配置文件或控制台写 false 才关闭。
// 参数：无；返回 bool：启用时为 true。
func (g Guard) RefererChallengeEnabled() bool {
	return g.ChallengeReferer == nil || *g.ChallengeReferer
}
