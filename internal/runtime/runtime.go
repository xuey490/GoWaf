// Package runtime 是数据面与控制面的共享运行时。
//
// 职责：把数据库里的站点/规则/名单编译成不可变快照，供数据面无锁读取；
// 控制面修完数据后调用 Reload 让新配置秒级生效。
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

// 设置键名。
const (
	KeyMode           = "engine.mode"
	KeyBlockScore     = "engine.block_score"
	KeyChallengeScore = "engine.challenge_score"
	KeySampleRate     = "storage.sample_rate"
	KeyIPRPS          = "guard.ip_rps"
	KeyIPBurst        = "guard.ip_burst"
	KeyIPPerMinute    = "guard.ip_per_minute"
	KeyAutoBan        = "guard.auto_ban"
	KeyBanThreshold   = "guard.ban_threshold"
	KeyBanWindow      = "guard.ban_window"
	KeyBanTTL         = "guard.ban_ttl"
	KeyBanMaxStrikes  = "guard.ban_max_strikes"
	KeyBanStrikeReset = "guard.ban_strike_reset"

	// CRS 第二引擎（OWASP CRS / Coraza）。
	KeyCRSEnabled  = "engine.crs.enabled"
	KeyCRSMode     = "engine.crs.mode"
	KeyCRSParanoia = "engine.crs.paranoia"

	// 威胁情报命中后的处置。
	// ban_on_hit=true 时把命中 IP 写进封禁表（source=intel），
	// 好处是封禁管理里能统一看到/解封，坏处是 bans 表会长得快，
	// 所以做成开关，默认开、TTL 默认 1 小时。
	KeyIntelBanOnHit = "intel.ban_on_hit"
	KeyIntelBanTTL   = "intel.ban_ttl"

	// 告警通知（当前只实现邮件，其余渠道预留）。
	KeyNotifyEnabled = "notify.enabled"
	KeyNotifyHost    = "notify.email.host"
	KeyNotifyPort    = "notify.email.port"
	KeyNotifyUser    = "notify.email.user"
	KeyNotifyPass    = "notify.email.pass"
	KeyNotifyFrom    = "notify.email.from"
	KeyNotifyTo      = "notify.email.to"
	KeyNotifyTLS     = "notify.email.tls"
	KeyNotifySubject = "notify.email.subject"
	KeyNotifyMinScore  = "notify.min_score"
	KeyNotifyWindow    = "notify.window"
	KeyNotifyCooldown  = "notify.cooldown"
)

// Runtime 聚合所有共享组件。
type Runtime struct {
	Cfg       *config.Config
	Store     *store.Store
	Engine    *detect.Engine
	Bans      *guard.BanList
	Limiter   *guard.Limiter // 全局限流器：未开站点自定义防护的站点共用
	Hits      *guard.HitTracker
	Chall     *guard.Challenger
	Upstreams *proxy.Upstreams
	Certs     *proxy.CertManager
	Events    *store.EventWriter
	Version   string

	// Geo 离线 IP 归属地，Intel 威胁情报 IP 库，Notify 告警通知。
	Geo    *geo.DB
	Intel  *intel.Manager
	Notify *notify.Notifier
	crs    atomic.Pointer[detect.CRSEngine]

	// siteLimiters 站点自定义限流器集合（Reload 时重建）。
	// key=SiteID。启用自定义防护的站点用独立实例，计数互不挤占。
	siteLimiters atomic.Pointer[map[uint]*guard.Limiter]

	router  atomic.Pointer[proxy.Router]
	iplist  guard.Holder
	started time.Time

	cache atomic.Pointer[map[string]string]

	// OnEvent 由 app 注入，用于把拦截事件推给 SSE 订阅者。
	OnEvent func(model.Event)
}

// New 构建运行时（不启动后台协程）。
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
	r.router.Store(&proxy.Router{})
	r.iplist.Store(&guard.IPList{})
	r.cache.Store(&map[string]string{})
	r.siteLimiters.Store(&map[uint]*guard.Limiter{})

	secret := []byte(cfg.Admin.JWTSecretFile)
	r.Chall = guard.NewChallenger(secret, 30*time.Minute)

	// 告警通知器：配置每次发送前现读，改完设置立即生效，不需要重启。
	r.Notify = notify.New(r.NotifyConfig, func(lg model.NotifyLog) {
		if err := st.Gorm().Create(&lg).Error; err != nil {
			log.Printf("[notify] 写通知记录失败: %v", err)
		}
	})
	return r, nil
}

// NotifyConfig 从设置快照组装通知配置。
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

// splitList 拆分逗号/分号/空白分隔的列表（收件人用）。
func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.FieldsFunc(s, func(c rune) bool {
		return c == ',' || c == ';' || c == ' ' || c == '\n' || c == '\r' || c == '，'
	})
}

// StartedAt 返回启动时间。
func (r *Runtime) StartedAt() time.Time { return r.started }

// Router 返回当前路由快照。
func (r *Runtime) Router() *proxy.Router { return r.router.Load() }

// IPList 返回当前名单快照。
func (r *Runtime) IPList() *guard.IPList { return r.iplist.Load() }

// Settings 返回设置快照。
func (r *Runtime) Settings() map[string]string {
	if m := r.cache.Load(); m != nil {
		return *m
	}
	return map[string]string{}
}

// Setting 读取单个设置，带兜底默认值。
func (r *Runtime) Setting(key, fallback string) string {
	if v, ok := r.Settings()[key]; ok && v != "" {
		return v
	}
	return fallback
}

// SettingInt 读取整型设置。
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

// SettingBool 读取布尔设置。
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

// Reload 从数据库重建全部快照。失败时保留旧快照（fail-safe）。
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

	var bans []model.Ban
	if err := r.Store.Gorm().WithContext(ctx).Where("released_at = 0").Find(&bans).Error; err != nil {
		return err
	}

	var exceptions []model.RuleException
	if err := r.Store.Gorm().WithContext(ctx).Where("enabled = 1").Find(&exceptions).Error; err != nil {
		return err
	}
	exList := make([]detect.Exception, 0, len(exceptions))
	for _, e := range exceptions {
		exList = append(exList, detect.Exception{
			Site: e.Site, RuleID: e.RuleID, Category: e.Category,
			PathPrefix: e.PathPrefix, Param: e.Param,
		})
	}
	r.Engine.SetExceptions(exList)

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
	r.reloadCRS(sm)

	log.Printf("[runtime] 重载完成: 站点 %d, 规则 %d, 例外 %d, IP 名单 %d, 封禁 %d, 模式 %s, CRS %s",
		router.Count(), r.Engine.Info().Rules, len(exList), countIPs(ipRules), len(bans),
		r.Engine.Mode(), r.crsLabel())
	return nil
}

// CRS 返回当前 CRS 引擎（未启用时为 nil）。
func (r *Runtime) CRS() *detect.CRSEngine { return r.crs.Load() }

// InvalidateCRS 强制下次 Reload 重建 CRS 引擎（SecLang 规则包增删改时调用，
// 否则 reloadCRS 会因 mode/paranoia 未变而跳过重建）。
func (r *Runtime) InvalidateCRS() { r.crs.Store(nil) }

func (r *Runtime) crsLabel() string {
	c := r.crs.Load()
	if c == nil {
		return "off"
	}
	return fmt.Sprintf("%s/PL%d", c.Mode(), c.Paranoia())
}

// reloadCRS 按设置重建 CRS 引擎。加载约 70ms，只在设置或规则包变了才重建。
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
	if !enabled {
		r.crs.Store(nil)
		return
	}
	if cur := r.crs.Load(); cur != nil && cur.Mode() == mode && cur.Paranoia() == pl {
		return
	}
	pkgs := r.loadSeclangPackages()
	eng, err := detect.NewCRSEngine(detect.CRSOptions{Mode: mode, Paranoia: pl, Packages: pkgs})
	if err != nil && len(pkgs) > 0 {
		// 规则包组合编译失败（如包间 rule id 冲突）：退回纯 CRS，绝不能让
		// 一个坏包把 CRS 引擎整体打瘫。
		log.Printf("[runtime] CRS+规则包编译失败，退回纯 CRS: %v", err)
		eng, err = detect.NewCRSEngine(detect.CRSOptions{Mode: mode, Paranoia: pl})
	}
	if err != nil {
		log.Printf("[runtime] CRS 加载失败，本次仍用自研规则: %v", err)
		return
	}
	r.crs.Store(eng)
}

// loadSeclangPackages 读取启用的 SecLang 规则包。逐包隔离：单包校验失败
// 只跳过该包并记录 last_error，不影响其他包与 CRS 本体。
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

// ReloadIntel 拉取情报源并重建快照。网络失败不影响主流程。
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

func (r *Runtime) settingFrom(m map[string]string, key, fallback string) string {
	if v, ok := m[key]; ok && v != "" {
		return v
	}
	return fallback
}

// settingFromInt 读取整型设置，非法值回落默认。
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

// orDefault 站点覆盖值：0 表示未设置，跟随全局。
func orDefault(site, global int) int {
	if site > 0 {
		return site
	}
	return global
}

// LimiterFor 返回站点应使用的限流器：启用自定义防护的站点用独立实例
// （计数不占全局），其余共用全局限流器。
func (r *Runtime) LimiterFor(siteID uint) *guard.Limiter {
	if m := r.siteLimiters.Load(); m != nil {
		if l, ok := (*m)[siteID]; ok {
			return l
		}
	}
	return r.Limiter
}

// SiteLimiters 返回全部站点限流器（含全局），供后台清理协程遍历。
func (r *Runtime) SiteLimiters() []*guard.Limiter {
	out := []*guard.Limiter{r.Limiter}
	if m := r.siteLimiters.Load(); m != nil {
		for _, l := range *m {
			out = append(out, l)
		}
	}
	return out
}

func countIPs(rows []model.IPRule) int { return len(rows) }

// DefaultSettings 返回首次启动写入的默认设置。
func DefaultSettings(cfg *config.Config) map[string]string {
	return map[string]string{
		KeyMode:           cfg.Engine.Mode,
		KeyBlockScore:     strconv.Itoa(cfg.Engine.BlockScore),
		KeyChallengeScore: strconv.Itoa(cfg.Engine.ChallengeScore),
		KeySampleRate:     strconv.FormatFloat(cfg.Storage.SampleRate, 'f', 4, 64),
		KeyIPRPS:          strconv.Itoa(cfg.Guard.IPRPS),
		KeyIPBurst:        strconv.Itoa(cfg.Guard.IPBurst),
		KeyIPPerMinute:    strconv.Itoa(cfg.Guard.IPPerMinute),
		KeyAutoBan:        strconv.FormatBool(cfg.Guard.AutoBan),
		KeyBanThreshold:   strconv.Itoa(cfg.Guard.BanThreshold),
		KeyBanWindow:      strconv.Itoa(int(cfg.Guard.BanWindow.Duration() / time.Second)),
		KeyBanTTL:         strconv.Itoa(int(cfg.Guard.BanTTL.Duration() / time.Second)),
		KeyBanMaxStrikes:  strconv.Itoa(cfg.Guard.BanMaxStrikes),
		KeyBanStrikeReset: strconv.Itoa(int(cfg.Guard.BanStrikeReset.Duration() / time.Second)),
		KeyCRSEnabled:     "true",
		KeyCRSMode:        "block",
		KeyCRSParanoia:    "1",

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
