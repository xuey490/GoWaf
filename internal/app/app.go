// Package app 负责把所有组件装配起来并管理生命周期。
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gowaf/internal/admin"
	"gowaf/internal/admin/auth"
	"gowaf/internal/admin/handler"
	"gowaf/internal/admin/middleware"
	"gowaf/internal/config"
	"gowaf/internal/edge"
	"gowaf/internal/metrics"
	"gowaf/internal/proxy"
	"gowaf/internal/runtime"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
	"gowaf/internal/tasks"
)

// App 是完整应用实例。
type App struct {
	cfg     *config.Config
	store   *store.Store
	rt      *runtime.Runtime
	certs   *proxy.CertManager
	authMgr *auth.Manager
	login   *middleware.LoginLimiter
	events  *store.EventWriter
	handler *handler.Server
	tasks   *tasks.Runner

	httpSrv  *http.Server
	httpsSrv *http.Server
	adminSrv *http.Server

	mu       sync.Mutex
	closed   bool
	stopOnce sync.Once
	stopCh   chan struct{}

	lastDropped int64
}

// New 构建应用（不监听端口）。
func New(cfg *config.Config, version string) (*App, error) {
	dsn := cfg.Storage.DSN
	if cfg.Demo.Enabled {
		dsn = cfg.Demo.DBPath
		log.Printf("[app] 演示模式：使用独立数据库 %s", dsn)
	}

	st, err := store.Open(dsn)
	if err != nil {
		return nil, err
	}

	if err := seed(cfg, st); err != nil {
		return nil, fmt.Errorf("初始化数据: %w", err)
	}

	var certs *proxy.CertManager
	if cfg.TLS.Enabled {
		certs, err = proxy.NewCertManager(cfg.TLS)
		if err != nil {
			return nil, fmt.Errorf("加载证书: %w", err)
		}
	}

	authMgr, err := auth.NewManager(cfg.Admin.JWTSecretFile, cfg.Admin.AccessTTL.Duration(), cfg.Admin.RefreshTTL.Duration())
	if err != nil {
		return nil, err
	}

	rt, err := runtime.New(cfg, st, certs, version)
	if err != nil {
		return nil, err
	}
	rt.Events = st.NewEventWriter(8192, 200, time.Second)

	// 计划任务中心：内置任务种子 + 调度循环在 Start 里启动
	taskRunner := tasks.New(st, filepath.Join(cfg.BaseDir(), "backups"))
	if err := taskRunner.EnsureSeeded(); err != nil {
		return nil, fmt.Errorf("初始化计划任务: %w", err)
	}

	// 首次重载：站点/规则/名单/设置全部进内存
	if err := rt.Reload(context.Background()); err != nil {
		return nil, fmt.Errorf("初始配置加载失败: %w", err)
	}

	// 混合站点（CF 橙云 + 本地灰云）最容易踩的坑：橙云站点没信任 CF 回源网段。
	// 这种错配不会报错，只会在运行期悄悄把 CF 节点 IP 当成访客 IP，
	// 所以必须在启动日志里喊出来，面板顶部也有一份同样的提示。
	for _, w := range rt.CDNWarnings() {
		log.Printf("[app] 配置风险 %s：%s", w.Code, w.Message)
	}

	login := middleware.NewLoginLimiter(cfg.Admin.LoginMaxFail, cfg.Admin.LoginLock.Duration())

	h := handler.New(handler.Deps{
		Cfg:          cfg,
		Store:        st,
		RT:           rt,
		Auth:         authMgr,
		LoginLimiter: login,
		Version:      version,
		Reload:       rt.Reload,
		Tasks:        taskRunner,
	})

	// 拦截事件 → SSE 广播
	// 统一走 handler.MarshalEvent，保证推送帧与 /api/events 列表结构一致
	rt.OnEvent = func(e model.Event) {
		payload, err := h.MarshalEvent(e)
		if err == nil {
			h.Stream.Publish(payload)
		}
	}

	a := &App{
		cfg:     cfg,
		store:   st,
		rt:      rt,
		certs:   certs,
		authMgr: authMgr,
		login:   login,
		events:  rt.Events,
		handler: h,
		tasks:   taskRunner,
		stopCh:  make(chan struct{}),
	}

	// 数据面
	edgeHandler := edge.New(rt)
	a.httpSrv = &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           edgeHandler,
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout.Duration(),
		ReadTimeout:       cfg.Server.ReadTimeout.Duration(),
		WriteTimeout:      cfg.Server.WriteTimeout.Duration(),
		IdleTimeout:       cfg.Server.IdleTimeout.Duration(),
		MaxHeaderBytes:    cfg.Server.MaxHeaderBytes,
	}
	if cfg.TLS.Enabled {
		a.httpsSrv = &http.Server{
			Addr:              cfg.Server.TLSListen,
			Handler:           edgeHandler,
			TLSConfig:         certs.TLSConfig(),
			ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout.Duration(),
			ReadTimeout:       cfg.Server.ReadTimeout.Duration(),
			WriteTimeout:      cfg.Server.WriteTimeout.Duration(),
			IdleTimeout:       cfg.Server.IdleTimeout.Duration(),
			MaxHeaderBytes:    cfg.Server.MaxHeaderBytes,
		}
	}

	// 控制面
	adminRouter := admin.NewRouter(admin.Options{
		Cfg:     cfg,
		Store:   st,
		RT:      rt,
		Auth:    authMgr,
		Login:   login,
		Handler: h,
		Version: version,
	})
	a.adminSrv = &http.Server{
		Addr:              cfg.Server.AdminListen,
		Handler:           adminRouter,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0, // SSE 需要长连接
		IdleTimeout:       120 * time.Second,
	}

	return a, nil
}

// seed 首次启动初始化数据。
func seed(cfg *config.Config, st *store.Store) error {
	if err := st.SeedSettings(runtime.DefaultSettings(cfg)); err != nil {
		return err
	}
	if err := st.ImportBuiltinRules(cfg.Engine.RulesFile); err != nil {
		log.Printf("[app] 导入内置规则失败（可忽略）: %v", err)
	}

	if cfg.Demo.Enabled {
		if err := st.SeedDemoUser(cfg.Demo.Username, cfg.Demo.Password); err != nil {
			return err
		}
		return st.SeedDemoData()
	}

	pwd, err := st.SeedAdmin("admin")
	if err != nil {
		return err
	}
	if pwd != "" {
		path := filepath.Join(cfg.BaseDir(), "initial-password.txt")
		_ = os.MkdirAll(filepath.Dir(path), 0o750)
		if werr := os.WriteFile(path, []byte(pwd+"\n"), 0o600); werr == nil {
			log.Printf("[app] 已创建初始管理员 admin，口令写入 %s（登录后请删除该文件）", path)
		} else {
			log.Printf("[app] 初始管理员 admin 口令: %s（写入文件失败: %v）", pwd, werr)
		}
	}

	return st.SeedSites(cfg.Sites)
}

// Start 启动所有监听与后台任务。
func (a *App) Start() error {
	// 先同步绑定所有端口，再打日志、再提供服务。
	// 否则端口被占用时日志已经打出"监听 xxx"，实际却 bind 失败，排查时会被误导。
	httpLn, err := net.Listen("tcp", a.cfg.Server.Listen)
	if err != nil {
		return fmt.Errorf("数据面 HTTP: %w（端口被占用？可用 netstat -ano 查占用进程）", err)
	}

	var httpsLn net.Listener
	if a.httpsSrv != nil && a.certs != nil {
		httpsLn, err = net.Listen("tcp", a.cfg.Server.TLSListen)
		if err != nil {
			_ = httpLn.Close()
			return fmt.Errorf("数据面 HTTPS: %w", err)
		}
	}

	adminLn, err := net.Listen("tcp", a.cfg.Server.AdminListen)
	if err != nil {
		_ = httpLn.Close()
		if httpsLn != nil {
			_ = httpsLn.Close()
		}
		return fmt.Errorf("控制面: %w", err)
	}

	errCh := make(chan error, 3)

	go func() {
		log.Printf("[app] 数据面 HTTP 监听 %s", httpLn.Addr())
		if err := a.httpSrv.Serve(httpLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("数据面 HTTP: %w", err)
		}
	}()

	if httpsLn != nil {
		go func() {
			log.Printf("[app] 数据面 HTTPS 监听 %s（已加载 %d 张证书）",
				httpsLn.Addr(), a.certs.Count())
			a.httpsSrv.TLSConfig = a.certs.TLSConfig()
			if err := a.httpsSrv.ServeTLS(httpsLn, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("数据面 HTTPS: %w", err)
			}
		}()
	}

	go func() {
		log.Printf("[app] 控制面监听 %s", adminLn.Addr())
		if err := a.adminSrv.Serve(adminLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("控制面: %w", err)
		}
	}()

	go a.maintenance()
	go a.streamLoop()
	go a.tasks.Loop()
	go a.intelLoop()
	go a.ruleHitsLoop()
	if a.rt.Notify != nil {
		a.rt.Notify.Start()
	}

	select {
	case err := <-errCh:
		return err
	case <-time.After(300 * time.Millisecond):
		return nil
	}
}

// intelLoop 定时同步威胁情报。启动后先等 5 秒，避免和监听抢时间。
func (a *App) intelLoop() {
	select {
	case <-a.stopCh:
		return
	case <-time.After(5 * time.Second):
	}
	a.rt.ReloadIntel(context.Background())

	ticker := time.NewTicker(3 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.rt.ReloadIntel(context.Background())
		}
	}
}

// ruleHitsLoop 定期把内存里的规则命中计数刷进库。
//
// 命中统计走在热路径上，不能每个请求写库；这里每 60 秒批量落一次，
// 面板上的「命中次数 / 最后命中时间」最多滞后一分钟，足够做规则取舍。
func (a *App) ruleHitsLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			stats := a.rt.Engine.DrainHits()
			if len(stats) == 0 {
				continue
			}
			for _, st := range stats {
				a.rt.Store.Gorm().Exec(
					`UPDATE rules SET hits = hits + ?, last_hit_at = ? WHERE id = ?`,
					st.Count, st.LastMS, st.RuleID)
			}
		}
	}
}

// streamLoop 每秒推一次汇总给 SSE 订阅者。
func (a *App) streamLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.handler.StreamStats()
		}
	}
}

// maintenance 周期任务：清理、指标、归档、证书热加载。
func (a *App) maintenance() {
	cleanTicker := time.NewTicker(30 * time.Second)
	defer cleanTicker.Stop()

	certTicker := time.NewTicker(6 * time.Hour)
	defer certTicker.Stop()

	hourTicker := time.NewTicker(time.Hour)
	defer hourTicker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-cleanTicker.C:
			now := store.NowMS()
			for _, l := range a.rt.SiteLimiters() {
				l.Cleanup(now)
			}
			// 命中计数至少要留满两个统计窗口，否则窗口还没走完就被清掉，
			// 「N 分钟内 M 次」会永远凑不齐（窗口最长可被改成 24 小时）。
			hitIdle := int64(10 * 60 * 1000)
			if w := a.rt.SettingInt(runtime.KeyBanWindow, int(a.rt.Cfg.Guard.BanWindow.Duration()/time.Second)); w > 0 {
				if d := int64(w) * 1000 * 2; d > hitIdle {
					hitIdle = d
				}
			}
			a.rt.Hits.Cleanup(now, hitIdle)
			a.rt.Bans.Cleanup(now)
			a.login.Cleanup()
			metrics.BannedGauge.Set(float64(a.rt.Bans.Size()))

			// Dropped 是累计值，这里换算成增量再喂给 Counter
			if dropped := a.events.Dropped(); dropped > a.lastDropped {
				metrics.EventDropped.Add(float64(dropped - a.lastDropped))
				a.lastDropped = dropped
			}

		case <-certTicker.C:
			if a.certs != nil {
				if err := a.certs.Reload(); err != nil {
					log.Printf("[app] 证书热加载失败（保留旧证书）: %v", err)
				}
			}

		case <-hourTicker.C:
			a.hourly()
		}
	}
}

// hourly 归档与清理，凌晨 4 点重置演示库。
func (a *App) hourly() {
	now := time.Now()

	// 演示模式：每天 04:00 重置演示数据
	if a.cfg.Demo.Enabled && now.Hour() == 4 && now.Minute() < 5 {
		if err := a.store.ResetDemo(); err != nil {
			log.Printf("[app] 重置演示数据失败: %v", err)
		} else {
			log.Printf("[app] 演示数据已重置")
		}
	}

	// 事件保留已交由计划任务 clean_events 按面板配置执行（internal/tasks），
	// 这里不再固定清理——保留天数与执行间隔都可在面板调整。

	// 吊销过期刷新令牌
	a.store.Gorm().Exec(`DELETE FROM sys_refresh_tokens WHERE expires_at < ?`, store.NowMS())
}

// Shutdown 优雅退出：停止接新请求 → 等事件落盘 → 关库。
func (a *App) Shutdown(ctx context.Context) error {
	a.stopOnce.Do(func() { close(a.stopCh) })
	a.tasks.Stop()
	// 通知器先停：它持有独立的发送协程，关库前必须收干净
	if a.rt.Notify != nil {
		a.rt.Notify.Stop()
	}

	var errs []error
	if a.httpsSrv != nil {
		if err := a.httpsSrv.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if err := a.httpSrv.Shutdown(ctx); err != nil {
		errs = append(errs, err)
	}
	if err := a.adminSrv.Shutdown(ctx); err != nil {
		errs = append(errs, err)
	}
	// 先把 channel 里剩余事件写完，再关库
	if a.events != nil {
		a.events.Close()
	}
	if err := a.store.Close(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
