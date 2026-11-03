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

// Duration 让 YAML 支持 "5s" / "2h" / "1m30s" 写法。
type Duration time.Duration

// UnmarshalYAML 实现 yaml.Unmarshaler。
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

// MarshalYAML 输出人类可读的时长。
func (d Duration) MarshalYAML() (any, error) {
	if d == 0 {
		return "0s", nil
	}
	return time.Duration(d).String(), nil
}

// Duration 转标准库时长。
func (d Duration) Duration() time.Duration { return time.Duration(d) }

// Config 是整份配置。
type Config struct {
	Server  Server     `yaml:"server"`
	TLS     TLS        `yaml:"tls"`
	Storage Storage    `yaml:"storage"`
	Engine  Engine     `yaml:"engine"`
	Guard   Guard      `yaml:"guard"`
	Admin   Admin      `yaml:"admin"`
	Demo    Demo       `yaml:"demo"`
	Alert   Alert      `yaml:"alert"`
	Log     Log        `yaml:"log"`
	Sites   []SiteSeed `yaml:"sites"`
}

// Server 数据面监听与超时。
type Server struct {
	Listen            string   `yaml:"listen"`
	TLSListen         string   `yaml:"tls_listen"`
	AdminListen       string   `yaml:"admin_listen"`
	ReadHeaderTimeout Duration `yaml:"read_header_timeout"`
	ReadTimeout       Duration `yaml:"read_timeout"`
	WriteTimeout      Duration `yaml:"write_timeout"`
	IdleTimeout       Duration `yaml:"idle_timeout"`
	MaxHeaderBytes    int      `yaml:"max_header_bytes"`
	ShutdownTimeout   Duration `yaml:"shutdown_timeout"`
	TrustedProxies    []string `yaml:"trusted_proxies"`
}

// CertPair 显式证书配置。
type CertPair struct {
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
}

// TLS 终止相关。
type TLS struct {
	Enabled     bool       `yaml:"enabled"`
	CertDir     string     `yaml:"cert_dir"`
	DefaultCert string     `yaml:"default_cert"`
	MinVersion  string     `yaml:"min_version"`
	HSTS        bool       `yaml:"hsts"`
	Certs       []CertPair `yaml:"certs"`
}

// Storage 存储相关。
type Storage struct {
	DSN         string  `yaml:"dsn"`
	RetainDays  int     `yaml:"retain_days"`
	ArchiveDir  string  `yaml:"archive_dir"`
	ArchiveDays int     `yaml:"archive_days"`
	SampleRate  float64 `yaml:"sample_rate"`
}

// Engine 检测引擎参数。
type Engine struct {
	Mode           string `yaml:"mode"` // observe | protect
	BlockScore     int    `yaml:"block_score"`
	ChallengeScore int    `yaml:"challenge_score"`
	MaxBodyInspect int    `yaml:"max_body_inspect"`
	MaxTargets     int    `yaml:"max_targets"`
	RegexTimeoutMS int    `yaml:"regex_timeout_ms"`
	RulesFile      string `yaml:"rules_file"`
}

// Guard 限流与封禁参数。
type Guard struct {
	GlobalRPS          int      `yaml:"global_rps"`
	IPRPS              int      `yaml:"ip_rps"`
	IPBurst            int      `yaml:"ip_burst"`
	IPPerMinute        int      `yaml:"ip_per_minute"`
	ChallengePerMinute int      `yaml:"challenge_per_minute"`
	BanPerMinute       int      `yaml:"ban_per_minute"`
	Ban1hPerMinute     int      `yaml:"ban_1h_per_minute"`
	BanTTL             Duration `yaml:"ban_ttl"`
	BanMaxTTL          Duration `yaml:"ban_max_ttl"`
	AutoBan            bool     `yaml:"auto_ban"`
	WhitelistStatic    bool     `yaml:"whitelist_static"`

	// 自动封禁策略：window 内命中 threshold 次 → 封 ban_ttl；
	// 解封后再次触发，连击数 +1，第 ban_max_strikes 次直接永久（expires_at = 0）。
	// 两次触发之间超过 ban_strike_reset 没有新命中，连击数清零。
	BanWindow      Duration `yaml:"ban_window"`
	BanThreshold   int      `yaml:"ban_threshold"`
	BanMaxStrikes  int      `yaml:"ban_max_strikes"`
	BanStrikeReset Duration `yaml:"ban_strike_reset"`
}

// CORS 跨域配置。
type CORS struct {
	Enabled          bool     `yaml:"enabled"`
	AllowOrigins     []string `yaml:"allow_origins"`
	AllowMethods     []string `yaml:"allow_methods"`
	AllowHeaders     []string `yaml:"allow_headers"`
	ExposeHeaders    []string `yaml:"expose_headers"`
	AllowCredentials bool     `yaml:"allow_credentials"`
	MaxAge           Duration `yaml:"max_age"`
}

// CSRF 双提交配置。
type CSRF struct {
	Enabled    bool   `yaml:"enabled"`
	Header     string `yaml:"header"`
	CookieName string `yaml:"cookie_name"`
}

// Admin 控制面配置。
type Admin struct {
	AllowedIPs        []string `yaml:"allowed_ips"`
	JWTSecretFile     string   `yaml:"jwt_secret_file"`
	AccessTTL         Duration `yaml:"access_ttl"`
	RefreshTTL        Duration `yaml:"refresh_ttl"`
	LoginMaxFail      int      `yaml:"login_max_fail"`
	LoginLock         Duration `yaml:"login_lock"`
	CookieSecure      bool     `yaml:"cookie_secure"`
	CookieDomain      string   `yaml:"cookie_domain"`
	CORS              CORS     `yaml:"cors"`
	CSRF              CSRF     `yaml:"csrf"`
	IKnowWhatIAmDoing bool     `yaml:"i_know_what_i_am_doing"`
}

// Demo 演示模式配置。
type Demo struct {
	Enabled   bool   `yaml:"enabled"`
	DBPath    string `yaml:"db_path"`
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
	ReadOnly  bool   `yaml:"read_only"`
	MaskIPs   bool   `yaml:"mask_ips"`
	ResetCron string `yaml:"reset_cron"`
	Banner    string `yaml:"banner"`
}

// Alert 告警配置。
type Alert struct {
	Webhook        string   `yaml:"webhook"`
	BlockThreshold int      `yaml:"block_threshold"`
	Window         Duration `yaml:"window"`
}

// Log 日志配置。
type Log struct {
	Level      string `yaml:"level"`
	File       string `yaml:"file"`
	MaxSizeMB  int    `yaml:"max_size_mb"`
	MaxBackups int    `yaml:"max_backups"`
}

// SiteSeed 首次启动写入的站点。
type SiteSeed struct {
	Name     string   `yaml:"name"`
	Hosts    []string `yaml:"hosts"`
	Upstream string   `yaml:"upstream"`
	// BehindCDN 站点是否在 CDN 回源之后，留空=直连，cloudflare=橙云。
	// 只影响控制台的风险自检，不改变取 IP 逻辑（那由 server.trusted_proxies 决定）。
	BehindCDN string `yaml:"behind_cdn"`
}

// Load 读取并校验配置。
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
		c.Server.TrustedProxies = []string{"127.0.0.1/32", "::1/128"}
	}
	if c.Storage.RetainDays == 0 {
		c.Storage.RetainDays = 30
	}
	if c.Storage.DSN == "" {
		c.Storage.DSN = "data/waf.db"
	}
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
	if c.Demo.Username == "" {
		c.Demo.Username = "demo"
	}
	if c.Demo.ReadOnly {
		// 演示模式默认只读，配置里显式写 read_only: true 才生效
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

// Validate 启动前校验。任何一条不通过都拒绝启动。
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

// validateUpstream 校验上游地址，防 SSRF 打自己。
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

// IsLoopbackHost 判断监听地址是否只在回环。
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

// BaseDir 返回配置中 DSN 所在目录，用于创建 data 目录。
func (c *Config) BaseDir() string {
	return filepath.Dir(c.Storage.DSN)
}
