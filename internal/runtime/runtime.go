// Package runtime 是数据面与控制面的共享运行时中枢。
//
// 职责：把数据库里的站点/规则/名单编译成不可变快照，供数据面无锁读取；
// 管理后台修改的设置项（封禁阈值、挑战策略、CRS 开关等）写入数据库后
// 经 Reload 重建快照，秒级热生效，无需重启进程。
// 另提供 CDN 回源场景的真实访客 IP 风险自检（CDNWarnings）与
// 自环请求检测（dropSelfLoopSites），防止错误配置让 WAF 把流量代理回自身。
package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"gowaf/internal/acme"
	"gowaf/internal/bot"
	"gowaf/internal/cache"
	"gowaf/internal/config"
	"gowaf/internal/detect"
	"gowaf/internal/dprotect"
	"gowaf/internal/geo"
	"gowaf/internal/guard"
	"gowaf/internal/intel"
	"gowaf/internal/logship"
	"gowaf/internal/notify"
	"gowaf/internal/policy"
	"gowaf/internal/proxy"
	"gowaf/internal/respfilter"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
	"gowaf/internal/tamper"
)

// 设置键名。settings 表以 key/value 字符串存储：控制面写入，数据面经 Settings
// 快照读取，后台保存后由 Reload 重建快照热生效。各键的初始默认值见 DefaultSettings。
const (
	KeyMode           = "engine.mode"            // 自研规则引擎的工作模式
	KeyBlockScore     = "engine.block_score"     // 全局拦截分数阈值
	KeyChallengeScore = "engine.challenge_score" // 全局挑战（JS/验证码）分数阈值
	KeySampleRate     = "storage.sample_rate"    // 请求日志采样率
	KeyIPRPS          = "guard.ip_rps"           // 全局单 IP 每秒请求数阈值
	KeyIPBurst        = "guard.ip_burst"         // 全局单 IP 令牌桶突发容量
	KeyIPPerMinute    = "guard.ip_per_minute"    // 全局单 IP 每分钟请求数阈值
	// 人机校验触发条件：软阈值超限 / 跨站来路。两者都在硬限流（429）之前生效，
	// 目的是把「可疑但不确定」的流量降级成一次自证，而不是直接拒掉。
	KeyChallengeRPS       = "guard.challenge_rps"        // 单 IP 每秒超过该值下发人机校验（0=关闭）
	KeyChallengePerMinute = "guard.challenge_per_minute" // 单 IP 每分钟超过该值下发人机校验（0=关闭）
	KeyChallengeReferer   = "guard.challenge_referer"    // 跨站来路的页面导航是否下发人机校验
	// KeyChallengeRefererAllow 允许的跨站来路（逗号分隔），命中的 Referer 域名不挑战，
	// 用于放行搜索引擎等正常导流来源。
	KeyChallengeRefererAllow = "guard.challenge_referer_allow"
	// KeyChallEnvCheck 换证时是否校验浏览器环境信号（navigator/WebGL/屏幕/时区），
	// 与 PoW 组成双层人机校验；关闭则退回纯 PoW。
	KeyChallEnvCheck = "guard.challenge.env_check" // 环境信号校验开关，默认开
	// KeyChallMinElapsed 挑战令牌签发到换证请求的最小耗时（毫秒）。
	// 真实浏览器算 PoW 约 0.3 秒 + 页面加载；脚本解 PoW 是微秒级。
	KeyChallMinElapsed = "guard.challenge.min_elapsed_ms"
	KeyAutoBan         = "guard.auto_ban"         // 是否启用自动封禁
	KeyBanThreshold    = "guard.ban_threshold"    // 自动封禁触发分数
	KeyBanWindow       = "guard.ban_window"       // 分数累计窗口（秒）
	KeyBanTTL          = "guard.ban_ttl"          // 封禁时长（秒）
	KeyBanMaxStrikes   = "guard.ban_max_strikes"  // 累计 strike 上限，达到即封禁
	KeyBanStrikeReset  = "guard.ban_strike_reset" // strike 计数重置窗口（秒）

	// CRS 第二引擎（OWASP CRS / Coraza）。
	KeyCRSEnabled  = "engine.crs.enabled"  // 是否启用 CRS 第二引擎
	KeyCRSMode     = "engine.crs.mode"     // CRS 处置模式（如 block）
	KeyCRSParanoia = "engine.crs.paranoia" // CRS 偏执等级（PL1-PL4）
	// KeyCRSMethods CRS 911100 方法白名单（空格分隔）。默认 REST 常规集，
	// 纯表单业务可收紧为 "GET HEAD POST OPTIONS"。
	KeyCRSMethods = "engine.crs.methods" // CRS 允许的 HTTP 方法白名单

	// CRS 性能治理（调度闸门，见 internal/detect/crsgov.go）。
	KeyCRSPrescreen    = "engine.crs.prescreen"      // 可疑度预筛（加速模式，默认关）
	KeyCRSSkipStatic   = "engine.crs.skip_static"    // 无参数静态资源旁路（默认开）
	KeyCRSBodyTextOnly = "engine.crs.body_text_only" // 非 text 请求体跳过 CRS body 解析（默认开）
	KeyCRSExcludePaths = "engine.crs.exclude_paths"  // 路径前缀豁免清单（逗号分隔）
	KeyCRSRemoveRanges = "engine.crs.remove_ranges"  // 编译期整体移除的规则号段（如 "920,921"）

	// 引擎加固与第三检测通道。
	KeyEngineTimeoutMS = "engine.timeout_ms"      // 单引擎检测超时（毫秒），超时按 fail_mode 处理
	KeyEngineFailMode  = "engine.fail_mode"       // 引擎失效策略：open 放行 / closed 拦截
	KeyLibiEnabled     = "engine.libi.enabled"    // 是否启用 libinjection 语义检测通道
	KeyLibiSQLiScore   = "engine.libi.sqli_score" // libinjection SQLi 命中加分
	KeyLibiXSSScore    = "engine.libi.xss_score"  // libinjection XSS 命中加分

	// 语义检测通道（第四引擎：命令注入/RCE/XXE/SSRF/SSTI/表达式/反序列化）。
	KeySemEnabled    = "engine.sem.enabled"     // 总开关，默认开启
	KeySemCmdiScore  = "engine.sem.cmdi_score"  // 命令注入命中加分
	KeySemRCEScore   = "engine.sem.rce_score"   // 远程代码执行命中加分
	KeySemSSRFScore  = "engine.sem.ssrf_score"  // SSRF 命中加分
	KeySemXXEScore   = "engine.sem.xxe_score"   // XXE 命中加分
	KeySemSSTIScore  = "engine.sem.ssti_score"  // 模板注入命中加分
	KeySemExprScore  = "engine.sem.expr_score"  // 表达式注入命中加分
	KeySemDeserScore = "engine.sem.deser_score" // 反序列化命中加分

	// 响应体敏感信息过滤。
	KeyRespFilterEnabled  = "respfilter.enabled"       // 总开关
	KeyRespFilterMode     = "respfilter.mode"          // observe 只记录 / mask 脱敏
	KeyRespFilterMaxBytes = "respfilter.max_bytes"     // 扫描的响应体上限（字节）
	KeyRespFilterExclude  = "respfilter.exclude_paths" // 不扫描的路径前缀（逗号分隔）
	KeyRespFilterCats     = "respfilter.categories"    // 启用的敏感信息类别（逗号分隔）

	// 动态防护（HTML/JS 每次访问动态加密）。
	KeyDProtEnabled   = "dprotect.enabled"       // 总开关
	KeyDProtMaxBytes  = "dprotect.max_bytes"     // 参与加密的页面体积上限（字节）
	KeyDProtExclude   = "dprotect.exclude_paths" // 不加密的路径前缀（逗号分隔）
	KeyDProtChallenge = "dprotect.challenge"     // 未通过人机校验的访客先完成 PoW 再放行

	// ACME 证书自动签发（Let's Encrypt）。
	KeyACMEEmail     = "acme.email"         // ACME 账户联系邮箱（可为空）
	KeyACMEDirectory = "acme.directory_url" // ACME 目录地址；置空或指向 staging 即为测试环境

	// webhook 告警渠道（与邮件渠道共用聚合窗口/冷却/最低分过滤）。
	KeyNotifyWebhookEnabled = "notify.webhook.enabled"         // webhook 渠道总开关
	KeyNotifyFeishuURL      = "notify.webhook.feishu_url"      // 飞书群机器人 webhook 地址
	KeyNotifyFeishuSecret   = "notify.webhook.feishu_secret"   // 飞书签名密钥（可选）
	KeyNotifyDingURL        = "notify.webhook.dingtalk_url"    // 钉钉群机器人 webhook 地址
	KeyNotifyDingSecret     = "notify.webhook.dingtalk_secret" // 钉钉签名密钥（可选）
	KeyNotifyWecomURL       = "notify.webhook.wecom_url"       // 企业微信群机器人 webhook 地址
	KeyNotifyGenericURL     = "notify.webhook.generic_url"     // 通用 JSON POST 地址

	// 检测前多层解码（base64 / 压缩请求体 / unicode 转义的绕过面）。
	KeyDecodeEnabled = "engine.decode.enabled"   // 总开关，默认开启
	KeyDecodeDepth   = "engine.decode.max_depth" // base64 最大解层层深（1~3）
	KeyDecodeBytes   = "engine.decode.max_bytes" // 单变体字节上限

	// 网页防篡改（基线比对）。
	KeyTamperEnabled  = "tamper.enabled"   // 总开关，默认关闭
	KeyTamperMode     = "tamper.mode"      // observe 只记录 / block 替换为基线副本
	KeyTamperPaths    = "tamper.paths"     // 受保护的精确路径列表（逗号分隔）
	KeyTamperMaxBytes = "tamper.max_bytes" // 单个基线副本的字节上限

	// 威胁情报命中后的处置。
	// ban_on_hit=true 时把命中 IP 写进封禁表（source=intel），
	// 好处是封禁管理里能统一看到/解封，坏处是 bans 表会长得快，
	// 所以做成开关，默认开、TTL 默认 1 小时。
	KeyIntelBanOnHit = "intel.ban_on_hit" // 情报命中是否写入封禁表
	KeyIntelBanTTL   = "intel.ban_ttl"    // 情报封禁时长（秒）

	// 告警通知（当前只实现邮件，其余渠道预留）。
	KeyNotifyEnabled  = "notify.enabled"       // 是否启用告警通知
	KeyNotifyHost     = "notify.email.host"    // SMTP 服务器地址
	KeyNotifyPort     = "notify.email.port"    // SMTP 端口（默认 465 SSL）
	KeyNotifyUser     = "notify.email.user"    // SMTP 用户名
	KeyNotifyPass     = "notify.email.pass"    // SMTP 密码/授权码
	KeyNotifyFrom     = "notify.email.from"    // 发件人地址
	KeyNotifyTo       = "notify.email.to"      // 收件人列表（逗号/分号分隔）
	KeyNotifyTLS      = "notify.email.tls"     // TLS 方式（默认 ssl）
	KeyNotifySubject  = "notify.email.subject" // 邮件主题
	KeyNotifyMinScore = "notify.min_score"     // 触发告警的最低分数
	KeyNotifyWindow   = "notify.window"        // 告警聚合窗口（秒）
	KeyNotifyCooldown = "notify.cooldown"      // 同类告警冷却时间（秒）

	// Bot 管理：UA/路径/IP/rDNS 策略 + 蜜罐 + 行为评分。
	KeyBotEnabled        = "bot.enabled"         // Bot 管理总开关
	KeyBotVerifyRDNS     = "bot.verify_rdns"     // 是否对搜索引擎 Bot 做正向-反向 DNS 验证
	KeyBotScoreThreshold = "bot.score_threshold" // 行为评分窗口内的请求次数阈值
	KeyBotAction         = "bot.action"          // 评分超限后的处置动作
	KeyBotWindow         = "bot.window"          // 行为统计窗口（秒）
	KeyBotCacheTTL       = "bot.cache_ttl"       // rDNS 验证结果缓存（秒）

	// 页面缓存与压缩。
	KeyCacheEnabled     = "cache.enabled"            // 页面缓存总开关
	KeyCacheMaxBytes    = "cache.max_bytes"          // 缓存总容量（字节）
	KeyCacheMaxObject   = "cache.max_object_bytes"   // 单对象缓存上限（字节）
	KeyCacheDefaultTTL  = "cache.default_ttl"        // 默认缓存时长（秒）
	KeyCacheCompress    = "cache.compress"           // 是否启用 gzip 压缩
	KeyCacheBrotli      = "cache.brotli"             // 是否启用 brotli 压缩
	KeyCacheWebP        = "cache.webp"               // 是否启用 WebP 转换
	KeyCacheCompressMin = "cache.compress_min_bytes" // 触发压缩的最小字节数

	// 多上游与节点。
	KeyUpstreamHealth   = "upstream.health_interval" // 上游健康检查间隔（秒）
	KeyNodeHeartbeatTTL = "node.heartbeat_ttl"       // 节点心跳超时（秒），超时视为离线
	KeyNodeAgentToken   = "node.agent_token"         // 节点 Agent 上报令牌

	// JA4 TLS 客户端指纹（Bot/扫描器识别的标配）。
	KeyJA4Enabled      = "guard.ja4.enabled"       // 是否采集指纹并写入事件库（默认开）
	KeyJA4Blocklist    = "guard.ja4.blocklist"     // 恶意指纹黑名单（逗号分隔，命中即拦）
	KeyJA4BlockBuiltin = "guard.ja4.block_builtin" // 内置脚本栈指纹命中是否拦截（默认关：脚本栈亦被正常工具使用）

	// 请求防重放。
	KeyReplayEnabled = "guard.replay.enabled" // 总开关（默认关：仅保护显式携带 nonce 的接口）
	KeyReplayWindow  = "guard.replay.window"  // 重放判定窗口（秒）
	KeyReplayHeader  = "guard.replay.header"  // nonce 请求头名（默认 X-WAF-Nonce）

	// 等候室（并发阈值排队放行）。
	KeyWaitEnabled  = "waitroom.enabled"    // 总开关（默认关）
	KeyWaitMax      = "waitroom.max_active" // 同时转发到上游的最大并发数
	KeyWaitTimeout  = "waitroom.timeout_ms" // 排队等候上限（毫秒）
	KeyWaitInterval = "waitroom.interval"   // 等候页自动重试间隔（秒）

	// 日志外发（对接 SIEM：Syslog RFC5424 / Kafka）。
	KeyLogShipEnabled        = "logship.enabled"         // 总开关
	KeyLogShipMinAction      = "logship.min_action"      // 最低外发动作级别：all/log/block
	KeyLogShipSyslogEnabled  = "logship.syslog.enabled"  // Syslog 渠道开关
	KeyLogShipSyslogAddr     = "logship.syslog.addr"     // 目标地址 host:port
	KeyLogShipSyslogProto    = "logship.syslog.proto"    // udp / tcp / tls
	KeyLogShipSyslogFacility = "logship.syslog.facility" // facility（0~23）
	KeyLogShipSyslogHostname = "logship.syslog.hostname" // 帧内 hostname（空=gowaf）
	KeyLogShipKafkaEnabled   = "logship.kafka.enabled"   // Kafka 渠道开关
	KeyLogShipKafkaBrokers   = "logship.kafka.brokers"   // broker 列表（逗号分隔）
	KeyLogShipKafkaTopic     = "logship.kafka.topic"     // 目标 topic
	KeyLogShipQueueSize      = "logship.queue_size"      // 外发队列容量

	// 人机校验挑战降级链：环境校验失败后的处置序列。
	// 取值逗号分隔，子集组合自 "pow,captcha,ban"：pow 纯拒绝（原行为），
	// captcha 降级到图形验证码，ban 答错超阈值封禁。默认 "pow"。
	KeyChallChain           = "guard.challenge.chain"
	KeyCaptchaFailThreshold = "guard.captcha.fail_threshold" // 验证码窗口内答错次数阈值（达阈值且含 ban 时封禁）
	KeyCaptchaFailWindow    = "guard.captcha.fail_window"    // 验证码失败计数窗口（秒）
)

// Runtime 聚合数据面与控制面共享的全部组件，是整个 WAF 的运行时中枢。
// 快照类字段（router/iplist/cache/siteLimiters/crs）均通过 atomic.Pointer 或
// Holder 整体替换：Reload 写入新快照，数据面读路径无锁，也不会读到半新半旧的组合。
type Runtime struct {
	Cfg       *config.Config     // 静态配置：来自配置文件，修改后需重启进程
	Store     *store.Store       // SQLite 存储层：站点/规则/设置等持久化数据的读写入口
	Engine    *detect.Engine     // 自研规则检测引擎：打分、拦截与挑战判定
	Bans      *guard.BanList     // 内存封禁列表：自动封禁、情报封禁与手动封禁共用
	Limiter   *guard.Limiter     // 全局限流器：未开站点自定义防护的站点共用
	Hits      *guard.HitTracker  // IP 命中评分统计器：自动封禁与告警的分数来源
	Chall     *guard.Challenger  // 人机挑战签发/校验器（令牌有效期 30 分钟）
	Upstreams *proxy.Upstreams   // 反代上游池：按站点持有后端转发配置
	Certs     *proxy.CertManager // TLS 证书管理器：按站点加载与热更证书
	Events    *store.EventWriter // 拦截事件异步写入器：由 app 层在启动时装配
	Version   string             // 程序版本号：启动日志与后台展示用

	// Geo 离线 IP 归属地，Intel 威胁情报 IP 库，Notify 告警通知。
	Geo    *geo.DB
	Intel  *intel.Manager
	Notify *notify.Notifier
	// Policy 精准访问控制快照，Bot Bot 管理引擎，Cache 页面缓存/压缩引擎。
	Policy atomic.Pointer[policy.Set]
	Bot    *bot.Manager
	Cache  *cache.Manager
	crs    atomic.Pointer[detect.CRSEngine] // CRS 第二引擎（OWASP CRS/Coraza）；未启用或未构建时为 nil
	crsGov atomic.Pointer[detect.CRSGov]    // CRS 调度闸门：预筛/静态旁路/路径豁免/号段移除

	// libi 第三检测通道（libinjection 语义识别 SQLi/XSS）；
	// sem 第四检测通道（命令注入/RCE/XXE/SSRF/SSTI/表达式/反序列化）；
	// respFilter/dprot/tamper/decode 响应侧与检测增强设置，Reload 时整体替换。
	libi       atomic.Pointer[detect.LibiEngine]
	sem        atomic.Pointer[detect.SemanticEngine]
	respFilter atomic.Pointer[respfilter.Settings]
	dprot      atomic.Pointer[dprotect.Settings]
	dprotMgr   *dprotect.Manager // 动态防护页面加密器（密钥在进程启动时从 JWT 密钥文件派生）
	tamper     atomic.Pointer[tamper.Settings]
	decode     atomic.Pointer[detect.DecodeConfig]
	// Tamper 防篡改基线仓库（管理员手动刷新基线，进程内有效）。
	Tamper *tamper.Manager
	// PolicyHits 自定义 CC 规则（rate_limit 动作）的窗口计数器，与自动封禁
	// 计数器隔离：窗口上限不同（CC 可到 1 天），共用会被清理策略误逐。
	PolicyHits *guard.HitTracker
	// policyAuthSecret 策略认证通行证的 HMAC 签名密钥（从 JWT 密钥文件内容派生，
	// 与挑战/动态防护密钥同源不同值，一路泄漏不影响另一路）。
	policyAuthSecret []byte

	// ACME 证书自动签发管理器（Let's Encrypt HTTP-01/DNS-01），进程启动时装配。
	ACME *acme.Manager

	// siteLimiters 站点自定义限流器集合（Reload 时重建）。
	// key=SiteID。启用自定义防护的站点用独立实例，计数互不挤占。
	siteLimiters atomic.Pointer[map[uint]*guard.Limiter]

	// challLimiter 是人机校验的软阈值计数器：超过它先下发校验，而不是像硬限流那样直接 429。
	// 独立实例，避免软限流消耗硬限流的令牌（否则「校验通过后继续正常浏览」会被自己的计数卡死）。
	challLimiter atomic.Pointer[guard.Limiter]

	// WaitRoom 等候室（并发阈值排队）：关闭或未 Reload 时为 nil。
	WaitRoom atomic.Pointer[guard.WaitRoom]
	// Shipper 日志外发器（Syslog/Kafka）：关闭时为 nil。Reload 时按配置整体重建。
	Shipper atomic.Pointer[logship.Shipper]
	// Replay 防重放守卫（nonce 去重窗口）。
	Replay *guard.ReplayGuard
	// captchaFails 验证码答错计数器（窗口内达阈值且降级链含 ban 时封禁）。
	captchaFails *guard.HitTracker

	router  atomic.Pointer[proxy.Router] // 路由快照：Reload 时整体替换，读路径无锁
	iplist  guard.Holder                 // IP 名单快照容器（内部 atomic.Pointer，读无锁）
	started time.Time                    // 进程启动时间：运行时长统计用

	cache atomic.Pointer[map[string]string] // 设置键值快照：Reload 时整体替换

	OnEvent func(model.Event) // 事件回调，由 app 注入：把拦截事件推给 SSE 订阅者
}

// New 构建运行时并装配各共享组件；不启动后台协程（定期 Reload、清理、告警等由 app 层负责）。
// 参数 cfg：静态配置；参数 st：SQLite 存储层；参数 certs：TLS 证书管理器；参数 version：程序版本号。
// 返回 (*Runtime, error)：就绪的运行时实例；组件构造失败时返回错误。
func New(cfg *config.Config, st *store.Store, certs *proxy.CertManager, version string) (*Runtime, error) {
	r := &Runtime{
		Cfg:       cfg,
		Store:     st,
		Engine:    detect.NewEngine(cfg.Engine.BlockScore, cfg.Engine.ChallengeScore, cfg.Engine.Mode),
		Bans:      guard.NewBanList(),
		Limiter:   guard.NewLimiter(cfg.Guard.IPRPS, cfg.Guard.IPBurst, cfg.Guard.IPPerMinute),
		Hits:      guard.NewHitTracker(),
		Upstreams: proxy.NewUpstreams(),
		Certs:     certs,
		Geo:       geo.Default(),
		Intel:     intel.New(),
		Bot:       bot.NewManager(),
		Cache:     cache.NewManager(),
		Version:   version,
		started:   time.Now(),
	}
	// 预置空快照占位：首次 Reload 前，数据面读路径也不会拿到 nil 指针。
	r.router.Store(&proxy.Router{})
	r.iplist.Store(&guard.IPList{})
	r.cache.Store(&map[string]string{})
	r.siteLimiters.Store(&map[uint]*guard.Limiter{})
	r.Policy.Store(&policy.Set{})
	r.libi.Store(detect.NewLibiEngine(false, 0, 0))
	r.respFilter.Store(&respfilter.Settings{})
	r.dprot.Store(&dprotect.Settings{})
	r.tamper.Store(&tamper.Settings{})
	r.decode.Store(&detect.DecodeConfig{})

	// 挑战通行证的签名密钥取自 JWT 密钥文件内容（不是文件路径字符串）：
	// 拿路径当密钥等于用一个公开常量签名，知道默认路径的人就能伪造通行证。
	secret := challengeSecret(cfg.Admin.JWTSecretFile)
	r.Chall = guard.NewChallenger(secret, 30*time.Minute)
	r.challLimiter.Store(newChallengeLimiter(cfg.Guard.ChallengeRPS, cfg.Guard.ChallengePerMinute))

	// 动态防护的根密钥：与挑战密钥同源（JWT 密钥文件内容）但派生一次哈希，
	// 两个子系统不复用同一数值——即使一路泄漏也不影响另一路。
	dprotSecret := sha256.Sum256(append([]byte("gowaf-dprotect|"), secret...))
	dprotMgr, err := dprotect.New(dprotSecret[:])
	if err != nil {
		return nil, err
	}
	r.dprotMgr = dprotMgr
	// 策略认证通行证密钥：同样从 JWT 密钥文件内容派生，前缀区分用途。
	paSecret := sha256.Sum256(append([]byte("gowaf-policy-auth|"), secret...))
	r.policyAuthSecret = paSecret[:]
	// 防篡改基线仓库与自定义 CC 计数器。
	r.Tamper = tamper.New()
	r.PolicyHits = guard.NewHitTracker()
	// 防重放守卫（窗口热生效走 Reload；这里先建默认实例防 nil）
	r.Replay = guard.NewReplayGuard(300)
	// 验证码答错计数器（降级链 ban 级使用）。
	r.captchaFails = guard.NewHitTracker()

	// ACME 管理器：状态（账户密钥、订单）落盘 data/acme/，签出的证书写入
	// tls.cert_dir 后回调 CertManager.Reload 热生效；未启用 TLS 时回调为空操作。
	reloadCerts := func() error {
		if certs == nil {
			return nil
		}
		return certs.Reload()
	}
	r.ACME = acme.New(acme.Options{
		StateDir:    filepath.Join(cfg.BaseDir(), "acme"),
		CertDir:     cfg.TLS.CertDir,
		LoadConf:    r.ACMEConfig,
		ReloadCerts: reloadCerts,
	})

	// 告警通知器：配置每次发送前现读，改完设置立即生效，不需要重启。
	r.Notify = notify.New(r.NotifyConfig, func(lg model.NotifyLog) {
		if err := st.Gorm().Create(&lg).Error; err != nil {
			log.Printf("[notify] 写通知记录失败: %v", err)
		}
	})
	return r, nil
}

// NotifyConfig 从当前设置快照组装告警通知配置（邮件 + webhook 渠道）。
// 返回 notify.Config：每次调用现读现算，后台改完设置下一次发送即生效；
// 端口非法（<=0）时回落 465，TLS 方式缺省 ssl。
func (r *Runtime) NotifyConfig() notify.Config {
	port := r.SettingInt(KeyNotifyPort, 465)
	if port <= 0 {
		port = 465
	}
	return notify.Config{
		Enabled:  r.SettingBool(KeyNotifyEnabled, false),
		Host:     strings.TrimSpace(r.Setting(KeyNotifyHost, "")),
		Port:     port,
		User:     strings.TrimSpace(r.Setting(KeyNotifyUser, "")),
		Pass:     r.Setting(KeyNotifyPass, ""),
		From:     strings.TrimSpace(r.Setting(KeyNotifyFrom, "")),
		To:       splitList(r.Setting(KeyNotifyTo, "")),
		TLS:      strings.TrimSpace(r.Setting(KeyNotifyTLS, "ssl")),
		Subject:  strings.TrimSpace(r.Setting(KeyNotifySubject, "")),
		MinScore: r.SettingInt(KeyNotifyMinScore, 0),
		Window:   time.Duration(r.SettingInt(KeyNotifyWindow, 60)) * time.Second,
		Cooldown: time.Duration(r.SettingInt(KeyNotifyCooldown, 3600)) * time.Second,
		Webhook: notify.WebhookConfig{
			Enabled:      r.SettingBool(KeyNotifyWebhookEnabled, false),
			FeishuURL:    strings.TrimSpace(r.Setting(KeyNotifyFeishuURL, "")),
			FeishuSecret: r.Setting(KeyNotifyFeishuSecret, ""),
			DingURL:      strings.TrimSpace(r.Setting(KeyNotifyDingURL, "")),
			DingSecret:   r.Setting(KeyNotifyDingSecret, ""),
			WecomURL:     strings.TrimSpace(r.Setting(KeyNotifyWecomURL, "")),
			GenericURL:   strings.TrimSpace(r.Setting(KeyNotifyGenericURL, "")),
		},
	}
}

// ACMEConfig 从当前设置快照组装 ACME 客户端配置。
// 返回 acme.Conf：目录地址缺省 Let's Encrypt 生产环境；acme.directory_url 指向
// staging 地址即切换测试环境（测试环境无速率限制，调通流程后切回生产）。
func (r *Runtime) ACMEConfig() acme.Conf {
	return acme.Conf{
		Email:     strings.TrimSpace(r.Setting(KeyACMEEmail, "")),
		Directory: strings.TrimSpace(r.Setting(KeyACMEDirectory, acme.LetsEncryptDirectory)),
	}
}

// Libi 返回当前 libinjection 第三检测通道快照（原子读取）。
// 返回 *detect.LibiEngine：未初始化时返回禁用状态的引擎，调用方无需判空。
func (r *Runtime) Libi() *detect.LibiEngine {
	if e := r.libi.Load(); e != nil {
		return e
	}
	return detect.NewLibiEngine(false, 0, 0)
}

// Sem 返回当前语义检测通道（第四引擎）快照（原子读取）。
// 返回 *detect.SemanticEngine：未初始化时返回禁用状态的引擎，调用方无需判空。
func (r *Runtime) Sem() *detect.SemanticEngine {
	if e := r.sem.Load(); e != nil {
		return e
	}
	return detect.NewSemanticEngine(false, nil)
}

// RespFilter 返回当前响应敏感信息过滤设置（原子读取）。
// 返回 *respfilter.Settings：未初始化时返回零值（即关闭），调用方无需判空。
func (r *Runtime) RespFilter() *respfilter.Settings {
	if s := r.respFilter.Load(); s != nil {
		return s
	}
	return &respfilter.Settings{}
}

// DProtect 返回当前动态防护设置（原子读取）。
// 返回 *dprotect.Settings：未初始化时返回零值（即关闭），调用方无需判空。
func (r *Runtime) DProtect() *dprotect.Settings {
	if s := r.dprot.Load(); s != nil {
		return s
	}
	return &dprotect.Settings{}
}

// DProtectMgr 返回动态防护页面加密器。
// 返回 *dprotect.Manager：进程启动时装配，恒非 nil。
func (r *Runtime) DProtectMgr() *dprotect.Manager { return r.dprotMgr }

// TamperSettings 返回当前防篡改设置（原子读取）。
// 返回 *tamper.Settings：未初始化时返回零值（即关闭），调用方无需判空。
func (r *Runtime) TamperSettings() *tamper.Settings {
	if s := r.tamper.Load(); s != nil {
		return s
	}
	return &tamper.Settings{}
}

// DecodeCfg 返回当前多层解码配置（原子读取）。
// 返回 *detect.DecodeConfig：未初始化时返回零值（即关闭）。
func (r *Runtime) DecodeCfg() *detect.DecodeConfig {
	if c := r.decode.Load(); c != nil {
		return c
	}
	return &detect.DecodeConfig{}
}

// PolicyAuthSecret 返回策略认证通行证的 HMAC 签名密钥。
// 返回 []byte：进程启动时从 JWT 密钥文件内容派生，恒非空。
func (r *Runtime) PolicyAuthSecret() []byte { return r.policyAuthSecret }

// TamperRefresh 重新生成防篡改基线：对每个启用站点 × 每个受保护路径回源抓取
// 响应并记录哈希与副本。
// 参数 ctx：抓取请求共用的上下文（当前未做单请求级取消，整体走 20 秒超时）。
// 返回 []tamper.FetchResult：逐「站点×路径」的抓取结果；防篡改未启用或未配置
// 受保护路径时返回错误说明。
//
// 只抓 200 且体积 ≤ max_bytes 的响应；上游不可达/非 200 的路径保留旧基线
// （宁可用旧基线也不清空保护）。
func (r *Runtime) TamperRefresh() ([]tamper.FetchResult, error) {
	s := r.TamperSettings()
	if !s.Enabled {
		return nil, fmt.Errorf("防篡改未启用，请先在设置中开启 tamper.enabled")
	}
	if len(s.Paths) == 0 {
		return nil, fmt.Errorf("未配置受保护路径（tamper.paths）")
	}
	router := r.Router()
	sites := router.Sites()
	if len(sites) == 0 {
		return nil, fmt.Errorf("没有启用的站点")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	results := make([]tamper.FetchResult, 0, len(sites)*len(s.Paths))
	for _, route := range sites {
		if route.Upstream == nil {
			continue
		}
		// 用站点第一个非泛域名做 Host 头（回源时上游按 Host 路由虚拟站点）
		host := ""
		for _, h := range route.Hosts {
			if !strings.HasPrefix(h, "*") {
				host = h
				break
			}
		}
		for _, path := range s.Paths {
			res := tamper.FetchResult{Site: route.SiteName, Path: path}
			target := route.Upstream.String()
			target = strings.TrimSuffix(target, "/") + path
			req, err := http.NewRequest(http.MethodGet, target, nil)
			if err != nil {
				res.Error = err.Error()
				results = append(results, res)
				continue
			}
			if host != "" {
				req.Host = host
			}
			resp, err := client.Do(req)
			if err != nil {
				res.Error = "回源失败: " + err.Error()
				results = append(results, res)
				continue
			}
			body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(s.MaxBytes)+1))
			_ = resp.Body.Close()
			ct := resp.Header.Get("Content-Type")
			status := resp.StatusCode
			switch {
			case status != http.StatusOK:
				res.Error = fmt.Sprintf("上游返回 %d，保留旧基线", status)
			case len(body) == 0:
				res.Error = "响应体为空"
			case len(body) > s.MaxBytes:
				res.Error = fmt.Sprintf("响应 %d 字节超过 max_bytes %d，保留旧基线", len(body), s.MaxBytes)
			default:
				if err := r.Tamper.Store(route.SiteName, path, body, ct); err != nil {
					res.Error = err.Error()
				} else {
					sum := sha256.Sum256(body)
					res.OK, res.SHA256, res.Size = true, hex.EncodeToString(sum[:]), len(body)
				}
			}
			results = append(results, res)
		}
	}
	log.Printf("[tamper] 基线刷新完成: %d 项", len(results))
	return results, nil
}

// splitList 拆分逗号/分号/空白（含全角逗号）分隔的列表字符串，用于收件人等多值输入。
// 参数 s：原始字符串。
// 返回 []string：拆分出的非空片段；输入为空白时返回 nil。
func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.FieldsFunc(s, func(c rune) bool {
		return c == ',' || c == ';' || c == ' ' || c == '\n' || c == '\r' || c == '，'
	})
}

// StartedAt 返回运行时的构建（启动）时间，用于后台展示运行时长。
// 返回 time.Time：New 执行时刻。
func (r *Runtime) StartedAt() time.Time { return r.started }

// Router 返回当前路由快照（原子读取）。
// 返回 *proxy.Router：站点匹配与转发路由；Reload 时整体替换，读路径无锁。
func (r *Runtime) Router() *proxy.Router { return r.router.Load() }

// IPList 返回当前 IP 名单快照（原子读取）。
// 返回 *guard.IPList：黑白名单查询入口；Reload 时整体替换，读路径无锁。
func (r *Runtime) IPList() *guard.IPList { return r.iplist.Load() }

// Settings 返回设置键值快照。
// 返回 map[string]string：全部设置项。该 map 是 Reload 整体替换的内部快照，调用方只读勿改；
// 尚未初始化时返回空 map。
func (r *Runtime) Settings() map[string]string {
	if m := r.cache.Load(); m != nil {
		return *m
	}
	return map[string]string{}
}

// Setting 读取字符串型设置项。
// 参数 key：设置键名；参数 fallback：缺省值。
// 返回 string：当前值；键不存在或值为空串时返回 fallback。
func (r *Runtime) Setting(key, fallback string) string {
	if v, ok := r.Settings()[key]; ok && v != "" {
		return v
	}
	return fallback
}

// SettingInt 读取整数型设置项。
// 参数 key：设置键名；参数 fallback：缺省值。
// 返回 int：当前值；键不存在或解析失败时返回 fallback。
func (r *Runtime) SettingInt(key string, fallback int) int {
	v := r.Setting(key, "")
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// SettingBool 读取布尔型设置项。
// 参数 key：设置键名；参数 fallback：缺省值。
// 返回 bool：当前值；键不存在或解析失败时返回 fallback。
func (r *Runtime) SettingBool(key string, fallback bool) bool {
	v := r.Setting(key, "")
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

// Reload 从数据库重建全部内存快照：启用站点（含自环过滤）、检测规则与例外、
// IP 名单、未解封封禁、设置表、站点自定义防护与限流器、CRS 第二引擎。
// 参数 ctx：本次重建中所有数据库查询共用的上下文。
// 返回 error：任一查询或编译失败即中止并返回错误；旧快照原样保留（fail-safe），数据面不受影响。
func (r *Runtime) Reload(ctx context.Context) error {
	now := store.NowMS()

	var sites []model.Site
	if err := r.Store.Gorm().WithContext(ctx).Where("enabled = 1").Find(&sites).Error; err != nil {
		return err
	}
	// 自环防护：上游指向 WAF 自身监听端口的站点一律拒绝加载（会引发无限代理自环）
	sites = dropSelfLoopSites(sites, selfListenPorts(
		r.Cfg.Server.Listen, r.Cfg.Server.TLSListen, r.Cfg.Server.AdminListen,
		r.Cfg.Server.ListenV6, r.Cfg.Server.TLSListenV6,
	))

	var rules []model.Rule
	if err := r.Store.Gorm().WithContext(ctx).Where("enabled = 1").Find(&rules).Error; err != nil {
		return err
	}
	if err := r.Engine.Reload(rules); err != nil {
		return err
	}

	var ipRules []model.IPRule
	if err := r.Store.Gorm().WithContext(ctx).Find(&ipRules).Error; err != nil {
		return err
	}

	// 只载入未解封（released_at = 0）的封禁记录，已解封的不再参与拦截。
	var bans []model.Ban
	if err := r.Store.Gorm().WithContext(ctx).Where("released_at = 0").Find(&bans).Error; err != nil {
		return err
	}

	var exceptions []model.RuleException
	if err := r.Store.Gorm().WithContext(ctx).Where("enabled = 1").Find(&exceptions).Error; err != nil {
		return err
	}
	// 规则例外：把库中启用的例外编译成引擎可用的豁免规则（按站点/规则/路径/参数组合跳过检测）。
	exList := make([]detect.Exception, 0, len(exceptions))
	for _, e := range exceptions {
		exList = append(exList, detect.Exception{
			Site: e.Site, RuleID: e.RuleID, Category: e.Category,
			PathPrefix: e.PathPrefix, Param: e.Param,
		})
	}
	r.Engine.SetExceptions(exList)

	// 读出设置表；先在临时变量 sm 中算好全部派生值，最后统一发布为新快照。
	var settings []model.Setting
	if err := r.Store.Gorm().WithContext(ctx).Find(&settings).Error; err != nil {
		return err
	}
	sm := make(map[string]string, len(settings))
	for _, s := range settings {
		sm[s.Key] = s.Value
	}

	// 精准访问控制：条件树 JSON 在 Reload 时解析并编译成不可变快照。
	// auth / rate_limit 动作的 config 列随定义一起传入，编译期强校验。
	var apRows []model.AccessPolicy
	if err := r.Store.Gorm().WithContext(ctx).Where("enabled = 1").Find(&apRows).Error; err != nil {
		return err
	}
	apDefs := make([]policy.Definition, 0, len(apRows))
	for _, row := range apRows {
		g, err := policy.DecodeGroup(row.Conditions)
		if err != nil {
			return fmt.Errorf("策略 %s 条件解析失败: %w", row.Name, err)
		}
		apDefs = append(apDefs, policy.Definition{
			ID: row.ID, Name: row.Name, Site: row.Site, Priority: row.Priority,
			Enabled: row.Enabled == 1, Action: policy.Action(row.Action),
			Conditions: g, Config: row.Config,
		})
	}
	pol, err := policy.Compile(apDefs)
	if err != nil {
		return err
	}

	// Bot 策略与蜜罐路径。
	var botRows []model.BotPolicy
	if err := r.Store.Gorm().WithContext(ctx).Where("enabled = 1").Order("priority ASC, id ASC").Find(&botRows).Error; err != nil {
		return err
	}
	botDefs := make([]bot.PolicyDef, 0, len(botRows))
	for _, row := range botRows {
		botDefs = append(botDefs, bot.PolicyDef{
			ID: row.ID, Name: row.Name, Site: row.Site, Category: row.Category,
			Action: bot.Action(row.Action), MatchType: row.MatchType, Pattern: row.Pattern,
			Priority: row.Priority, Enabled: row.Enabled == 1,
		})
	}
	var hpRows []model.BotHoneypot
	if err := r.Store.Gorm().WithContext(ctx).Where("enabled = 1").Find(&hpRows).Error; err != nil {
		return err
	}
	hpDefs := make([]bot.HoneypotDef, 0, len(hpRows))
	for _, row := range hpRows {
		hpDefs = append(hpDefs, bot.HoneypotDef{
			ID: row.ID, Site: row.Site, Path: row.Path, Enabled: row.Enabled == 1,
		})
	}
	botSettings := bot.Settings{
		Enabled:        r.settingFromBool(sm, KeyBotEnabled, false),
		VerifyRDNS:     r.settingFromBool(sm, KeyBotVerifyRDNS, true),
		ScoreThreshold: r.settingFromInt(sm, KeyBotScoreThreshold, 60),
		Action:         bot.Action(r.settingFrom(sm, KeyBotAction, string(bot.ActionChallenge))),
		Window:         time.Duration(r.settingFromInt(sm, KeyBotWindow, 60)) * time.Second,
		CacheTTL:       time.Duration(r.settingFromInt(sm, KeyBotCacheTTL, 600)) * time.Second,
	}
	if err := r.Bot.Reload(botDefs, hpDefs, botSettings); err != nil {
		return err
	}

	// 页面缓存规则与压缩设置。
	var cacheRows []model.CacheRule
	if err := r.Store.Gorm().WithContext(ctx).Where("enabled = 1").Order("priority ASC, id ASC").Find(&cacheRows).Error; err != nil {
		return err
	}
	cacheDefs := make([]cache.RuleDef, 0, len(cacheRows))
	for _, row := range cacheRows {
		cacheDefs = append(cacheDefs, cache.RuleDef{
			ID: row.ID, Name: row.Name, Site: row.Site, Kind: row.Kind, Pattern: row.Pattern,
			TTL: time.Duration(row.TTLSeconds) * time.Second, Priority: row.Priority,
			Enabled: row.Enabled == 1,
		})
	}
	cacheSettings := cache.DecodeSettings(func(k string) string { return sm[k] }, cache.DefaultSettings())
	if err := r.Cache.Reload(cacheDefs, cacheSettings); err != nil {
		return err
	}

	// 响应侧能力：敏感信息过滤与动态防护的设置编译。
	// 两者都是纯读快照，编译只做默认值兜底与列表拆分，不会失败。
	rf := &respfilter.Settings{
		Enabled:      r.settingFromBool(sm, KeyRespFilterEnabled, false),
		Mode:         r.settingFrom(sm, KeyRespFilterMode, "observe"),
		MaxBytes:     r.settingFromInt(sm, KeyRespFilterMaxBytes, respfilter.DefaultMaxBytes),
		ExcludePaths: splitList(r.settingFrom(sm, KeyRespFilterExclude, "")),
		Categories:   splitList(r.settingFrom(sm, KeyRespFilterCats, strings.Join(respfilter.DefaultCategories(), ","))),
	}
	if rf.Mode != "mask" {
		rf.Mode = "observe"
	}
	if rf.MaxBytes <= 0 {
		rf.MaxBytes = respfilter.DefaultMaxBytes
	}
	r.respFilter.Store(rf)

	dp := &dprotect.Settings{
		Enabled:      r.settingFromBool(sm, KeyDProtEnabled, false),
		MaxBytes:     r.settingFromInt(sm, KeyDProtMaxBytes, dprotect.DefaultMaxBytes),
		ExcludePaths: splitList(r.settingFrom(sm, KeyDProtExclude, "")),
		Challenge:    r.settingFromBool(sm, KeyDProtChallenge, false),
	}
	if dp.MaxBytes <= 0 {
		dp.MaxBytes = dprotect.DefaultMaxBytes
	}
	r.dprot.Store(dp)

	// libinjection 第三检测通道：开关与分值热生效。
	r.libi.Store(detect.NewLibiEngine(
		r.settingFromBool(sm, KeyLibiEnabled, true),
		r.settingFromInt(sm, KeyLibiSQLiScore, detect.DefaultLibiSQLiScore),
		r.settingFromInt(sm, KeyLibiXSSScore, detect.DefaultLibiXSSScore),
	))

	// 语义检测通道（第四引擎）：开关与各类分值热生效。
	r.sem.Store(detect.NewSemanticEngine(
		r.settingFromBool(sm, KeySemEnabled, true),
		map[string]int{
			detect.CategoryCmdi:  r.settingFromInt(sm, KeySemCmdiScore, detect.DefaultSemCmdiScore),
			detect.CategoryRCE:   r.settingFromInt(sm, KeySemRCEScore, detect.DefaultSemRCEScore),
			detect.CategorySSRF:  r.settingFromInt(sm, KeySemSSRFScore, detect.DefaultSemSSRFScore),
			detect.CategoryXXE:   r.settingFromInt(sm, KeySemXXEScore, detect.DefaultSemXXEScore),
			detect.CategorySSTI:  r.settingFromInt(sm, KeySemSSTIScore, detect.DefaultSemSSTIScore),
			detect.CategoryExpr:  r.settingFromInt(sm, KeySemExprScore, detect.DefaultSemExprScore),
			detect.CategoryDeser: r.settingFromInt(sm, KeySemDeserScore, detect.DefaultSemDeserScore),
		},
	))

	// 检测前多层解码：开关与上限热生效。
	dc := &detect.DecodeConfig{
		Enabled:  r.settingFromBool(sm, KeyDecodeEnabled, true),
		MaxDepth: r.settingFromInt(sm, KeyDecodeDepth, detect.DefaultDecodeDepth),
		MaxBytes: r.settingFromInt(sm, KeyDecodeBytes, detect.DefaultDecodeBytes),
	}
	r.decode.Store(dc)

	// 网页防篡改：开关/模式/受保护路径/副本上限。
	tp := &tamper.Settings{
		Enabled:  r.settingFromBool(sm, KeyTamperEnabled, false),
		Mode:     r.settingFrom(sm, KeyTamperMode, tamper.ModeObserve),
		Paths:    splitList(r.settingFrom(sm, KeyTamperPaths, "")),
		MaxBytes: r.settingFromInt(sm, KeyTamperMaxBytes, tamper.DefaultMaxBytes),
	}
	if tp.Mode != tamper.ModeBlock {
		tp.Mode = tamper.ModeObserve
	}
	if tp.MaxBytes <= 0 {
		tp.MaxBytes = tamper.DefaultMaxBytes
	}
	if len(tp.Paths) > tamper.MaxPaths {
		tp.Paths = tp.Paths[:tamper.MaxPaths]
	}
	r.tamper.Store(tp)

	// 等候室：开关与并发阈值热生效（实例重建即清空在途排队，
	// 持有槽位的旧请求结束时 Release 旧实例，无跨实例影响）。
	if r.settingFromBool(sm, KeyWaitEnabled, false) {
		maxActive := r.settingFromInt(sm, KeyWaitMax, 100)
		if maxActive <= 0 {
			maxActive = 100
		}
		r.WaitRoom.Store(guard.NewWaitRoom(maxActive))
	} else {
		r.WaitRoom.Store(nil)
	}

	// 防重放：窗口变化重建守卫（旧 seen 表随之作废，安全侧选择）。
	if r.settingFromBool(sm, KeyReplayEnabled, false) {
		r.Replay = guard.NewReplayGuard(r.settingFromInt(sm, KeyReplayWindow, 300))
	}

	// 日志外发（Syslog/Kafka）：配置整体重建 Shipper。旧实例交给 Close 耗尽；
	// Reload 频率有限（设置保存时触发），泄漏面可忽略。
	if old := r.Shipper.Swap(nil); old != nil {
		go old.Close()
	}
	if cfgShip := r.compileShipConfig(sm); cfgShip.Enabled {
		queueSize := r.settingFromInt(sm, KeyLogShipQueueSize, 0)
		r.Shipper.Store(logship.New(cfgShip, queueSize))
	}

	// 站点多上游（双链路）：按站点分组，BuildRouter 把主上游与额外目标合并。
	var upRows []model.SiteUpstream
	if err := r.Store.Gorm().WithContext(ctx).Where("enabled = 1").
		Order("priority ASC, weight DESC, id ASC").Find(&upRows).Error; err != nil {
		return err
	}
	extraUpstreams := make(map[uint][]model.SiteUpstream, len(sites))
	for _, row := range upRows {
		extraUpstreams[row.SiteID] = append(extraUpstreams[row.SiteID], row)
	}

	// 设置页阈值应用到引擎（此前只落库不生效）
	gBlock := r.settingFromInt(sm, KeyBlockScore, r.Cfg.Engine.BlockScore)
	gChallenge := r.settingFromInt(sm, KeyChallengeScore, r.Cfg.Engine.ChallengeScore)
	r.Engine.SetScores(gBlock, gChallenge)

	// 站点自定义防护：解析 protect_cfg 并与全局合并成生效值，
	// 同时为启用的站点建独立限流器（计数与全局隔离，互不挤占）。
	gRPS := r.settingFromInt(sm, KeyIPRPS, r.Cfg.Guard.IPRPS)
	gBurst := r.settingFromInt(sm, KeyIPBurst, r.Cfg.Guard.IPBurst)
	gPerMin := r.settingFromInt(sm, KeyIPPerMinute, r.Cfg.Guard.IPPerMinute)
	guards := make(map[uint]*proxy.SiteGuard)
	slg := make(map[uint]*guard.Limiter)
	for i := range sites {
		p, err := model.ParseSiteProtect(sites[i].ProtectCfg)
		if err != nil {
			log.Printf("[runtime] 站点 %s 防护配置非法，已忽略: %v", sites[i].Name, err)
			continue
		}
		if p == nil || p.Enabled != 1 {
			continue
		}
		sg := &proxy.SiteGuard{
			Mode:           p.Mode,
			BlockScore:     orDefault(p.BlockScore, gBlock),
			ChallengeScore: orDefault(p.ChallengeScore, gChallenge),
			IPRPS:          orDefault(p.IPRPS, gRPS),
			IPBurst:        orDefault(p.IPBurst, gBurst),
			IPPerMinute:    orDefault(p.IPPerMinute, gPerMin),
		}
		// 只覆盖了一个阈值时，与全局值合并后可能出现 challenge >= block，
		// 此时 challenge 永远不触发，修正成合法关系
		if sg.BlockScore <= sg.ChallengeScore {
			sg.ChallengeScore = sg.BlockScore - 1
			if sg.ChallengeScore <= 0 {
				sg.ChallengeScore = sg.BlockScore / 2
			}
		}
		guards[sites[i].ID] = sg
		slg[sites[i].ID] = guard.NewLimiter(sg.IPRPS, sg.IPBurst, sg.IPPerMinute)
	}

	router, err := proxy.BuildRouterWithUpstreams(sites, guards, extraUpstreams)
	if err != nil {
		return err
	}

	// 先改内存，最后统一替换指针，保证读方看到的是一致的旧/新组合
	r.router.Store(router)
	r.iplist.Store(guard.BuildIPList(ipRules, now))
	r.Bans.LoadFromDB(bans, now)
	r.cache.Store(&sm)
	r.siteLimiters.Store(&slg)
	r.Policy.Store(pol)

	r.Engine.SetMode(r.settingFrom(sm, KeyMode, r.Cfg.Engine.Mode))
	r.challLimiter.Store(newChallengeLimiter(
		r.settingFromInt(sm, KeyChallengeRPS, r.Cfg.Guard.ChallengeRPS),
		r.settingFromInt(sm, KeyChallengePerMinute, r.Cfg.Guard.ChallengePerMinute)))
	r.reloadCRS(sm)

	// CRS 调度闸门：每次 Reload 重建（AC 构建毫秒级，代价可忽略）。
	r.crsGov.Store(detect.NewCRSGov(detect.CRSGovOptions{
		Prescreen:    r.settingFromBool(sm, KeyCRSPrescreen, false),
		SkipStatic:   r.settingFromBool(sm, KeyCRSSkipStatic, true),
		BodyTextOnly: r.settingFromBool(sm, KeyCRSBodyTextOnly, true),
		ExcludePaths: splitList(r.settingFrom(sm, KeyCRSExcludePaths, "")),
	}))

	log.Printf("[runtime] 重载完成: 站点 %d, 规则 %d, 例外 %d, IP 名单 %d, 封禁 %d, 模式 %s, CRS %s, 策略 %d, Bot %d/%d, 缓存规则 %d, 额外上游 %d",
		router.Count(), r.Engine.Info().Rules, len(exList), countIPs(ipRules), len(bans),
		r.Engine.Mode(), r.crsLabel(), pol.Count(), len(botDefs), len(hpDefs), len(cacheDefs), len(upRows))
	return nil
}

// PolicySet 返回当前精准访问控制快照（原子读取）。
func (r *Runtime) PolicySet() *policy.Set { return r.Policy.Load() }

// compileShipConfig 从待生效设置表编译日志外发配置。
// 参数 sm：待生效的设置键值表。
// 返回 logship.Config：总开关关闭时各渠道字段无效。
func (r *Runtime) compileShipConfig(sm map[string]string) logship.Config {
	cfg := logship.Config{
		Enabled:   r.settingFromBool(sm, KeyLogShipEnabled, false),
		MinAction: r.settingFrom(sm, KeyLogShipMinAction, "block"),
	}
	cfg.Syslog = logship.SyslogConfig{
		Enabled:  cfg.Enabled && r.settingFromBool(sm, KeyLogShipSyslogEnabled, false),
		Addr:     strings.TrimSpace(r.settingFrom(sm, KeyLogShipSyslogAddr, "")),
		Proto:    strings.ToLower(r.settingFrom(sm, KeyLogShipSyslogProto, "udp")),
		Facility: r.settingFromInt(sm, KeyLogShipSyslogFacility, 4),
		Hostname: strings.TrimSpace(r.settingFrom(sm, KeyLogShipSyslogHostname, "")),
	}
	if cfg.Syslog.Proto != "udp" && cfg.Syslog.Proto != "tcp" && cfg.Syslog.Proto != "tls" {
		cfg.Syslog.Proto = "udp"
	}
	if cfg.Syslog.Facility < 0 || cfg.Syslog.Facility > 23 {
		cfg.Syslog.Facility = 4
	}
	cfg.Kafka = logship.KafkaConfig{
		Enabled: cfg.Enabled && r.settingFromBool(sm, KeyLogShipKafkaEnabled, false),
		Topic:   strings.TrimSpace(r.settingFrom(sm, KeyLogShipKafkaTopic, "")),
		Brokers: splitList(r.settingFrom(sm, KeyLogShipKafkaBrokers, "")),
	}
	// 至少一个渠道就绪才启用（配置不完整时静默不外发，避免每次 Reload 报错刷屏）
	if cfg.Syslog.Enabled && cfg.Syslog.Addr == "" {
		cfg.Syslog.Enabled = false
	}
	if cfg.Kafka.Enabled && (cfg.Kafka.Topic == "" || len(cfg.Kafka.Brokers) == 0) {
		cfg.Kafka.Enabled = false
	}
	return cfg
}

// Ship 事件外发入口：未启用时为空操作，热路径只做一次非阻塞入队。
// 参数 e：待外发事件。
func (r *Runtime) Ship(e model.Event) {
	if s := r.Shipper.Load(); s != nil {
		s.Send(e)
	}
}

// CaptchaFail 统计一次验证码答错，返回窗口内次数与是否达到封禁阈值。
// 参数 ip：客户端 IP。返回 (int, bool)：窗口累计次数与是否达阈值。
func (r *Runtime) CaptchaFail(ip string) (int, bool) {
	window := int64(r.SettingInt(KeyCaptchaFailWindow, 600)) * 1000
	threshold := r.SettingInt(KeyCaptchaFailThreshold, 5)
	return r.captchaFails.Add(ip, store.NowMS(), window, threshold)
}

// CaptchaCleanup 清理验证码答错计数（后台维护循环调用）。
// 参数 now：当前毫秒时间戳。返回 int：清理条数。
func (r *Runtime) CaptchaCleanup(now int64) int {
	return r.captchaFails.Cleanup(now, 3600*1000)
}

// UpstreamHealthInterval 返回上游健康检查间隔。
func (r *Runtime) UpstreamHealthInterval() time.Duration {
	sec := r.SettingInt(KeyUpstreamHealth, 10)
	if sec <= 0 {
		sec = 10
	}
	return time.Duration(sec) * time.Second
}

// NodeHeartbeatTTL 返回节点心跳超时时间。
func (r *Runtime) NodeHeartbeatTTL() time.Duration {
	sec := r.SettingInt(KeyNodeHeartbeatTTL, 300)
	if sec <= 0 {
		sec = 300
	}
	return time.Duration(sec) * time.Second
}

// NodeAgentToken 返回节点 Agent 令牌；为空表示未启用节点接入。
func (r *Runtime) NodeAgentToken() string {
	return strings.TrimSpace(r.Setting(KeyNodeAgentToken, ""))
}

// CRS 返回当前 CRS 第二引擎快照（原子读取）。
// 返回 *detect.CRSEngine：未启用、尚未构建或刚被 InvalidateCRS 置空时为 nil。
func (r *Runtime) CRS() *detect.CRSEngine { return r.crs.Load() }

// CRSGov 返回当前 CRS 调度闸门快照（原子读取）。
// 返回 *detect.CRSGov：进程尚未 Reload 时为 nil（视为「不旁路」）。
func (r *Runtime) CRSGov() *detect.CRSGov { return r.crsGov.Load() }

// InvalidateCRS 置空 CRS 引擎，强制下次 Reload 重建（无参数、无返回值）。
// SecLang 规则包增删改后必须调用：reloadCRS 只比对 mode/paranoia/methods，
// 包内容变化不会触发重建，不置空会导致新规则包一直不生效。
func (r *Runtime) InvalidateCRS() { r.crs.Store(nil) }

// crsLabel 返回 CRS 当前状态的可读标签，用于 Reload 完成日志。
// 返回 string："off"（未启用）或 "模式/偏执级"（如 "block/PL1"）。
func (r *Runtime) crsLabel() string {
	c := r.crs.Load()
	if c == nil {
		return "off"
	}
	return fmt.Sprintf("%s/PL%d", c.Mode(), c.Paranoia())
}

// reloadCRS 按本次待生效的设置重建 CRS 第二引擎。
// 构建耗时约 70ms，因此只在开关/模式/偏执级/方法白名单发生变化时才重建，否则复用现有引擎。
// 参数 sm：本次 Reload 读出的设置键值表（尚未发布）。
// 无返回值；构建失败只记录日志并保持现状，自研规则引擎不受影响（CRS 是叠加的第二引擎）。
func (r *Runtime) reloadCRS(sm map[string]string) {
	enabled := true
	if v, ok := sm[KeyCRSEnabled]; ok && v != "" {
		enabled = v == "true" || v == "1"
	}
	mode := r.settingFrom(sm, KeyCRSMode, "block")
	pl := 1
	if v, ok := sm[KeyCRSParanoia]; ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			pl = n
		}
	}
	methods := r.settingFrom(sm, KeyCRSMethods, detect.DefaultCRSMethods)
	removeRanges := validateRangeList(r.settingFrom(sm, KeyCRSRemoveRanges, ""))
	if !enabled {
		r.crs.Store(nil)
		return
	}
	if cur := r.crs.Load(); cur != nil && cur.Mode() == mode && cur.Paranoia() == pl &&
		cur.Methods() == methods && cur.RemoveRanges() == strings.Join(removeRanges, ",") {
		return
	}
	pkgs := r.loadSeclangPackages()
	opts := detect.CRSOptions{Mode: mode, Paranoia: pl, AllowedMethods: methods, Packages: pkgs, RemoveRanges: removeRanges}
	eng, err := detect.NewCRSEngine(opts)
	if err != nil && len(pkgs) > 0 {
		// 规则包组合编译失败（如包间 rule id 冲突）：退回纯 CRS，绝不能让
		// 一个坏包把 CRS 引擎整体打瘫。
		log.Printf("[runtime] CRS+规则包编译失败，退回纯 CRS: %v", err)
		opts.Packages = nil
		eng, err = detect.NewCRSEngine(opts)
	}
	if err != nil {
		log.Printf("[runtime] CRS 加载失败，本次仍用自研规则: %v", err)
		return
	}
	r.crs.Store(eng)
}

// validateRangeList 清洗 CRS 号段移除配置（转发给 detect 层校验）。
func validateRangeList(v string) []string { return detect.ValidateRemoveRanges(v) }

// loadSeclangPackages 读取启用的 SecLang 规则包文件并逐包校验，供 CRS 引擎编译。
// 逐包隔离：单包读文件失败或编译失败只跳过该包，并把错误写入 last_error 字段，
// 不影响其他包与 CRS 本体。
// 返回 []detect.CRSPackage：可用规则包列表；无启用包或查询失败时返回 nil。
func (r *Runtime) loadSeclangPackages() []detect.CRSPackage {
	var rows []model.SeclangPackage
	if err := r.Store.Gorm().Where("enabled = 1").Order("id ASC").Find(&rows).Error; err != nil {
		return nil
	}
	if len(rows) == 0 {
		return nil
	}
	dir := filepath.Join(r.Cfg.BaseDir(), "seclang")
	out := make([]detect.CRSPackage, 0, len(rows))
	for _, row := range rows {
		path := filepath.Join(dir, row.Slug+".conf")
		data, err := os.ReadFile(path)
		if err != nil {
			r.Store.Gorm().Model(&model.SeclangPackage{}).Where("id = ?", row.ID).
				Update("last_error", "读取包文件失败: "+err.Error())
			log.Printf("[seclang] 包 %s 文件缺失，跳过: %v", row.Slug, err)
			continue
		}
		content := string(data)
		if _, verr := detect.ValidateSecLang(content); verr != nil {
			r.Store.Gorm().Model(&model.SeclangPackage{}).Where("id = ?", row.ID).
				Update("last_error", verr.Error())
			log.Printf("[seclang] 包 %s 编译失败，跳过: %v", row.Slug, verr)
			continue
		}
		out = append(out, detect.CRSPackage{ID: row.Slug, Content: content})
	}
	return out
}

// ReloadIntel 拉取全部启用的威胁情报源，重建内存区间快照，并把同步结果回写数据库。
// 参数 ctx：网络请求与数据库操作共用的上下文。
// 返回 []intel.SyncResult：每个源的同步结果；读取源列表失败时返回 nil。
// 网络失败不影响主流程：失败原因记录在返回值与源的 last_error 字段中。
func (r *Runtime) ReloadIntel(ctx context.Context) []intel.SyncResult {
	var rows []model.IntelSource
	if err := r.Store.Gorm().WithContext(ctx).Where("enabled = 1").Find(&rows).Error; err != nil {
		log.Printf("[intel] 读取情报源失败: %v", err)
		return nil
	}
	// 注意：即使没有启用的源也必须走一次 Sync，否则删掉/停用最后一个源之后，
	// 内存里的区间快照不会被清空，已删除的情报会一直拦下去。
	res := r.Intel.Sync(ctx, rows)
	now := store.NowMS()
	for _, item := range res {
		updates := map[string]any{"last_sync_at": now, "last_count": item.Count}
		if item.Error != "" {
			updates["last_error"] = item.Error
		} else {
			updates["last_error"] = ""
		}
		r.Store.Gorm().Model(&model.IntelSource{}).Where("id = ?", item.ID).Updates(updates)
	}
	log.Printf("[intel] 同步完成: %d 个源，全局区间 %d", len(res), r.Intel.GlobalCount())
	return res
}

// settingFrom 从待生效设置表中读取字符串项。与 Setting 的区别：Reload 构建新快照
// 期间新值尚未发布，需直接读临时表 sm，而不是旧快照。
// 参数 m：待生效的设置键值表；参数 key：设置键名；参数 fallback：缺省值。
// 返回 string：当前值；键不存在或为空串时返回 fallback。
func (r *Runtime) settingFrom(m map[string]string, key, fallback string) string {
	if v, ok := m[key]; ok && v != "" {
		return v
	}
	return fallback
}

// settingFromInt 从待生效设置表读取整数项。
// 参数 m：待生效的设置键值表；参数 key：设置键名；参数 fallback：缺省值。
// 返回 int：当前值；键不存在、解析失败或为负数时返回 fallback。
func (r *Runtime) settingFromInt(m map[string]string, key string, fallback int) int {
	v := r.settingFrom(m, key, "")
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

// settingFromBool 从待生效设置表读取布尔项。
// 参数 m：待生效设置表；key：设置键；fallback：缺省值。
func (r *Runtime) settingFromBool(m map[string]string, key string, fallback bool) bool {
	v := strings.TrimSpace(r.settingFrom(m, key, ""))
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

// orDefault 合并站点级覆盖值与全局默认值。
// 参数 site：站点覆盖值（0 表示未设置）；参数 global：全局默认值。
// 返回 int：site > 0 时返回 site，否则返回 global。
func orDefault(site, global int) int {
	if site > 0 {
		return site
	}
	return global
}

// LimiterFor 返回指定站点应使用的限流器。
// 参数 siteID：站点 ID。
// 返回 *guard.Limiter：启用自定义防护的站点返回其独立限流器（计数不占全局），其余站点共用全局限流器。
func (r *Runtime) LimiterFor(siteID uint) *guard.Limiter {
	if m := r.siteLimiters.Load(); m != nil {
		if l, ok := (*m)[siteID]; ok {
			return l
		}
	}
	return r.Limiter
}

// SiteLimiters 返回全局限流器加全部站点独立限流器的列表。
// 返回 []*guard.Limiter：供后台清理协程遍历，定期清理过期计数，防止内部 map 无限增长。
func (r *Runtime) SiteLimiters() []*guard.Limiter {
	out := []*guard.Limiter{r.Limiter}
	if cl := r.challLimiter.Load(); cl != nil {
		out = append(out, cl)
	}
	if m := r.siteLimiters.Load(); m != nil {
		for _, l := range *m {
			out = append(out, l)
		}
	}
	return out
}

// countIPs 统计本次载入的 IP 规则条数，仅用于 Reload 完成日志。
// 参数 rows：IP 规则列表；返回 int：条数。
func countIPs(rows []model.IPRule) int { return len(rows) }

// DefaultSettings 生成首次启动时写入 settings 表的默认设置（来源为静态配置文件）。
// 参数 cfg：静态配置。
// 返回 map[string]string：设置键名到字符串值的映射；时间类键按秒存储，与后台编辑界面单位一致。
func DefaultSettings(cfg *config.Config) map[string]string {
	return map[string]string{
		KeyMode:           cfg.Engine.Mode,
		KeyBlockScore:     strconv.Itoa(cfg.Engine.BlockScore),
		KeyChallengeScore: strconv.Itoa(cfg.Engine.ChallengeScore),
		KeySampleRate:     strconv.FormatFloat(cfg.Storage.SampleRate, 'f', 4, 64),
		KeyIPRPS:          strconv.Itoa(cfg.Guard.IPRPS),
		KeyIPBurst:        strconv.Itoa(cfg.Guard.IPBurst),
		KeyIPPerMinute:    strconv.Itoa(cfg.Guard.IPPerMinute),

		KeyChallengeRPS:          strconv.Itoa(cfg.Guard.ChallengeRPS),
		KeyChallengePerMinute:    strconv.Itoa(cfg.Guard.ChallengePerMinute),
		KeyChallengeReferer:      strconv.FormatBool(cfg.Guard.RefererChallengeEnabled()),
		KeyChallengeRefererAllow: cfg.Guard.ChallengeRefererAllow,
		KeyChallEnvCheck:         "true",
		KeyChallMinElapsed:       "500",
		KeyAutoBan:               strconv.FormatBool(cfg.Guard.AutoBan),
		KeyBanThreshold:          strconv.Itoa(cfg.Guard.BanThreshold),
		KeyBanWindow:             strconv.Itoa(int(cfg.Guard.BanWindow.Duration() / time.Second)),
		KeyBanTTL:                strconv.Itoa(int(cfg.Guard.BanTTL.Duration() / time.Second)),
		KeyBanMaxStrikes:         strconv.Itoa(cfg.Guard.BanMaxStrikes),
		KeyBanStrikeReset:        strconv.Itoa(int(cfg.Guard.BanStrikeReset.Duration() / time.Second)),
		KeyCRSEnabled:            "true",
		KeyCRSMode:               "block",
		KeyCRSParanoia:           "1",
		KeyCRSMethods:            detect.DefaultCRSMethods,
		KeyCRSPrescreen:          "false",
		KeyCRSSkipStatic:         "true",
		KeyCRSBodyTextOnly:       "true",
		KeyCRSExcludePaths:       "",
		KeyCRSRemoveRanges:       "",

		KeyIntelBanOnHit: "true",
		KeyIntelBanTTL:   "3600",

		// 告警默认关闭：没配 SMTP 就发不出去，开着只会一直刷失败日志。
		KeyNotifyEnabled:  "false",
		KeyNotifyPort:     "465",
		KeyNotifyTLS:      "ssl",
		KeyNotifyMinScore: "60",
		KeyNotifyWindow:   "60",
		KeyNotifyCooldown: "3600",

		// Bot 管理默认关闭：先观察再启用，避免误伤正常访客。
		KeyBotEnabled:        "false",
		KeyBotVerifyRDNS:     "true",
		KeyBotScoreThreshold: "60",
		KeyBotAction:         string(bot.ActionChallenge),
		KeyBotWindow:         "60",
		KeyBotCacheTTL:       "600",

		// 页面缓存默认关闭：缓存策略与业务强相关，交给管理员按站点开启。
		KeyCacheEnabled:     "false",
		KeyCacheMaxBytes:    strconv.Itoa(cache.DefaultMaxBytes),
		KeyCacheMaxObject:   strconv.Itoa(cache.DefaultMaxObjectBytes),
		KeyCacheDefaultTTL:  strconv.Itoa(cache.DefaultTTL),
		KeyCacheCompress:    "true",
		KeyCacheBrotli:      "true",
		KeyCacheWebP:        "false",
		KeyCacheCompressMin: strconv.Itoa(cache.DefaultCompressMin),

		KeyUpstreamHealth:   "10",
		KeyNodeHeartbeatTTL: "300",

		// 引擎加固与语义检测通道：默认开 libi（分数压在挑战阈值以下），
		// 超时 2 秒 + fail-open——引擎出问题时保业务优先。
		KeyEngineTimeoutMS: "2000",
		KeyEngineFailMode:  "open",
		KeyLibiEnabled:     "true",
		KeyLibiSQLiScore:   strconv.Itoa(detect.DefaultLibiSQLiScore),
		KeyLibiXSSScore:    strconv.Itoa(detect.DefaultLibiXSSScore),

		// 语义检测通道默认开启：单类命中分数压在拦截阈值之下，叠加才拦截。
		KeySemEnabled:    "true",
		KeySemCmdiScore:  strconv.Itoa(detect.DefaultSemCmdiScore),
		KeySemRCEScore:   strconv.Itoa(detect.DefaultSemRCEScore),
		KeySemSSRFScore:  strconv.Itoa(detect.DefaultSemSSRFScore),
		KeySemXXEScore:   strconv.Itoa(detect.DefaultSemXXEScore),
		KeySemSSTIScore:  strconv.Itoa(detect.DefaultSemSSTIScore),
		KeySemExprScore:  strconv.Itoa(detect.DefaultSemExprScore),
		KeySemDeserScore: strconv.Itoa(detect.DefaultSemDeserScore),

		// 检测前多层解码：默认开启（URL 多层解码本就由 Normalize 覆盖，
		// 这里补 base64/压缩体/unicode 转义三类绕过面，成本有界）。
		KeyDecodeEnabled: "true",
		KeyDecodeDepth:   strconv.Itoa(detect.DefaultDecodeDepth),
		KeyDecodeBytes:   strconv.Itoa(detect.DefaultDecodeBytes),

		// 防篡改默认关闭：基线由管理员显式生成，开启后也先跑 observe。
		KeyTamperEnabled:  "false",
		KeyTamperMode:     tamper.ModeObserve,
		KeyTamperMaxBytes: strconv.Itoa(tamper.DefaultMaxBytes),

		// 响应敏感信息过滤与动态防护默认关闭：两者都会改动响应内容，
		// 先观察（respfilter.observe / 手动开启）确认无误伤再放量。
		KeyRespFilterEnabled:  "false",
		KeyRespFilterMode:     "observe",
		KeyRespFilterMaxBytes: strconv.Itoa(respfilter.DefaultMaxBytes),
		KeyRespFilterCats:     strings.Join(respfilter.DefaultCategories(), ","),
		KeyDProtEnabled:       "false",
		KeyDProtMaxBytes:      strconv.Itoa(dprotect.DefaultMaxBytes),
		KeyDProtChallenge:     "false",

		// webhook 渠道默认关闭：没填地址发不出去，开着只会一直刷失败日志。
		KeyNotifyWebhookEnabled: "false",

		// ACME 默认 Let's Encrypt 生产环境目录；测试签发请临时指向 staging 地址。
		KeyACMEDirectory: acme.LetsEncryptDirectory,

		// JA4 指纹：采集默认开（事件库记录，成本低价值高）；
		// 内置脚本栈指纹默认只记录不拦截（脚本栈亦被正常工具使用，避免误伤）。
		KeyJA4Enabled:      "true",
		KeyJA4Blocklist:    "",
		KeyJA4BlockBuiltin: "false",

		// 请求防重放：默认关。只对显式携带 nonce 头的请求生效，
		// 由客户端（如 API 网关）决定哪些接口参与防重放。
		KeyReplayEnabled: "false",
		KeyReplayWindow:  "300",
		KeyReplayHeader:  "X-WAF-Nonce",

		// 等候室：默认关。大促/放票类站点按需开启。
		KeyWaitEnabled:  "false",
		KeyWaitMax:      "100",
		KeyWaitTimeout:  "5000",
		KeyWaitInterval: "3",

		// 日志外发：默认关，渠道地址就绪后由管理员开启。
		KeyLogShipEnabled:        "false",
		KeyLogShipMinAction:      "block",
		KeyLogShipSyslogEnabled:  "false",
		KeyLogShipSyslogProto:    "udp",
		KeyLogShipSyslogFacility: "4",
		KeyLogShipKafkaEnabled:   "false",
		KeyLogShipQueueSize:      "4096",

		// 挑战降级链：默认 pow（环境校验失败即拒绝，与 v1.6.0 行为一致）。
		KeyChallChain:           "pow",
		KeyCaptchaFailThreshold: "5",
		KeyCaptchaFailWindow:    "600",
	}
}
