// Package runtime 是数据面与控制面的共享运行时。
//
// 职责：把数据库里的站点/规则/名单编译成不可变快照，供数据面无锁读取；
// 控制面修完数据后调用 Reload 让新配置秒级生效。
package runtime

import (
	"context"
	"log"
	"strconv"
	"sync/atomic"
	"time"

	"gowaf/internal/config"
	"gowaf/internal/detect"
	"gowaf/internal/guard"
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
		Version:   version,
		started:   time.Now(),
	}
	r.router.Store(&proxy.Router{})
	r.iplist.Store(&guard.IPList{})
	r.cache.Store(&map[string]string{})
	r.siteLimiters.Store(&map[uint]*guard.Limiter{})

	secret := []byte(cfg.Admin.JWTSecretFile)
	r.Chall = guard.NewChallenger(secret, 30*time.Minute)
	return r, nil
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

	log.Printf("[runtime] 重载完成: 站点 %d, 规则 %d, IP 名单 %d, 封禁 %d, 模式 %s",
		router.Count(), r.Engine.Info().Rules, countIPs(ipRules), len(bans), r.Engine.Mode())
	return nil
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
	}
}
