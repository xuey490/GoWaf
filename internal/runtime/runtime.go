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
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"gowaf/internal/config"
	"gowaf/internal/detect"
	"gowaf/internal/geo"
	"gowaf/internal/guard"
	"gowaf/internal/intel"
	"gowaf/internal/notify"
	"gowaf/internal/proxy"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
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
	KeyAutoBan               = "guard.auto_ban"         // 是否启用自动封禁
	KeyBanThreshold          = "guard.ban_threshold"    // 自动封禁触发分数
	KeyBanWindow             = "guard.ban_window"       // 分数累计窗口（秒）
	KeyBanTTL                = "guard.ban_ttl"          // 封禁时长（秒）
	KeyBanMaxStrikes         = "guard.ban_max_strikes"  // 累计 strike 上限，达到即封禁
	KeyBanStrikeReset        = "guard.ban_strike_reset" // strike 计数重置窗口（秒）

	// CRS 第二引擎（OWASP CRS / Coraza）。
	KeyCRSEnabled  = "engine.crs.enabled"  // 是否启用 CRS 第二引擎
	KeyCRSMode     = "engine.crs.mode"     // CRS 处置模式（如 block）
	KeyCRSParanoia = "engine.crs.paranoia" // CRS 偏执等级（PL1-PL4）
	// KeyCRSMethods CRS 911100 方法白名单（空格分隔）。默认 REST 常规集，
	// 纯表单业务可收紧为 "GET HEAD POST OPTIONS"。
	KeyCRSMethods = "engine.crs.methods" // CRS 允许的 HTTP 方法白名单

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
	crs    atomic.Pointer[detect.CRSEngine] // CRS 第二引擎（OWASP CRS/Coraza）；未启用或未构建时为 nil

	// siteLimiters 站点自定义限流器集合（Reload 时重建）。
	// key=SiteID。启用自定义防护的站点用独立实例，计数互不挤占。
	siteLimiters atomic.Pointer[map[uint]*guard.Limiter]

	// challLimiter 是人机校验的软阈值计数器：超过它先下发校验，而不是像硬限流那样直接 429。
	// 独立实例，避免软限流消耗硬限流的令牌（否则「校验通过后继续正常浏览」会被自己的计数卡死）。
	challLimiter atomic.Pointer[guard.Limiter]

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
		Version:   version,
		started:   time.Now(),
	}
	// 预置空快照占位：首次 Reload 前，数据面读路径也不会拿到 nil 指针。
	r.router.Store(&proxy.Router{})
	r.iplist.Store(&guard.IPList{})
	r.cache.Store(&map[string]string{})
	r.siteLimiters.Store(&map[uint]*guard.Limiter{})

	// 挑战通行证的签名密钥取自 JWT 密钥文件内容（不是文件路径字符串）：
	// 拿路径当密钥等于用一个公开常量签名，知道默认路径的人就能伪造通行证。
	secret := challengeSecret(cfg.Admin.JWTSecretFile)
	r.Chall = guard.NewChallenger(secret, 30*time.Minute)
	r.challLimiter.Store(newChallengeLimiter(cfg.Guard.ChallengeRPS, cfg.Guard.ChallengePerMinute))

	// 告警通知器：配置每次发送前现读，改完设置立即生效，不需要重启。
	r.Notify = notify.New(r.NotifyConfig, func(lg model.NotifyLog) {
		if err := st.Gorm().Create(&lg).Error; err != nil {
			log.Printf("[notify] 写通知记录失败: %v", err)
		}
	})
	return r, nil
}

// NotifyConfig 从当前设置快照组装邮件通知配置。
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
	}
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

	router, err := proxy.BuildRouter(sites, guards)
	if err != nil {
		return err
	}

	// 先改内存，最后统一替换指针，保证读方看到的是一致的旧/新组合
	r.router.Store(router)
	r.iplist.Store(guard.BuildIPList(ipRules, now))
	r.Bans.LoadFromDB(bans, now)
	r.cache.Store(&sm)
	r.siteLimiters.Store(&slg)

	r.Engine.SetMode(r.settingFrom(sm, KeyMode, r.Cfg.Engine.Mode))
	r.challLimiter.Store(newChallengeLimiter(
		r.settingFromInt(sm, KeyChallengeRPS, r.Cfg.Guard.ChallengeRPS),
		r.settingFromInt(sm, KeyChallengePerMinute, r.Cfg.Guard.ChallengePerMinute)))
	r.reloadCRS(sm)

	log.Printf("[runtime] 重载完成: 站点 %d, 规则 %d, 例外 %d, IP 名单 %d, 封禁 %d, 模式 %s, CRS %s",
		router.Count(), r.Engine.Info().Rules, len(exList), countIPs(ipRules), len(bans),
		r.Engine.Mode(), r.crsLabel())
	return nil
}

// CRS 返回当前 CRS 第二引擎快照（原子读取）。
// 返回 *detect.CRSEngine：未启用、尚未构建或刚被 InvalidateCRS 置空时为 nil。
func (r *Runtime) CRS() *detect.CRSEngine { return r.crs.Load() }

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
	if !enabled {
		r.crs.Store(nil)
		return
	}
	if cur := r.crs.Load(); cur != nil && cur.Mode() == mode && cur.Paranoia() == pl && cur.Methods() == methods {
		return
	}
	pkgs := r.loadSeclangPackages()
	opts := detect.CRSOptions{Mode: mode, Paranoia: pl, AllowedMethods: methods, Packages: pkgs}
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

		KeyIntelBanOnHit: "true",
		KeyIntelBanTTL:   "3600",

		// 告警默认关闭：没配 SMTP 就发不出去，开着只会一直刷失败日志。
		KeyNotifyEnabled:  "false",
		KeyNotifyPort:     "465",
		KeyNotifyTLS:      "ssl",
		KeyNotifyMinScore: "60",
		KeyNotifyWindow:   "60",
		KeyNotifyCooldown: "3600",
	}
}
