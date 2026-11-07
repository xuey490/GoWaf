// Package app 负责应用的整体装配与生命周期管理。
//
// 启动时按依赖顺序构建存储、检测引擎、数据面代理、管理后台等组件并监听端口；
// 运行期维护事件落盘、威胁情报同步、规则命中刷库等后台循环；
// 退出时统一执行优雅关闭（停止接新请求 → 等事件落盘 → 关闭数据库）。
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

// App 是完整应用实例：持有全部核心组件与三个 HTTP 服务，统一负责启动与优雅关闭。
type App struct {
	cfg     *config.Config           // 全局配置：启动时加载，运行期只读
	store   *store.Store             // SQLite 存储层：站点/规则/事件等持久化数据
	rt      *runtime.Runtime         // 运行时：检测引擎与内存态站点/规则/名单
	certs   *proxy.CertManager       // 证书管理器：未启用 TLS 时为 nil
	authMgr *auth.Manager            // 管理后台的 JWT 认证管理器
	login   *middleware.LoginLimiter // 登录失败限流器：防止后台口令被爆破
	events  *store.EventWriter       // 拦截事件异步落盘器：带缓冲 channel，后台批量写库
	handler *handler.Server          // 管理后台业务处理器：含 SSE 实时推送
	tasks   *tasks.Runner            // 计划任务中心：备份、事件清理等定时任务

	httpSrv  *http.Server // 数据面 HTTP 服务（明文）
	httpsSrv *http.Server // 数据面 HTTPS 服务：未启用 TLS 时为 nil
	adminSrv *http.Server // 控制面（管理后台）服务

	mu       sync.Mutex    // 保护 closed 标志的互斥锁
	closed   bool          // 应用是否已（开始）关闭
	stopOnce sync.Once     // 保证 stopCh 只被关闭一次，使 Shutdown 可重复调用
	stopCh   chan struct{} // 停机信号：关闭后所有后台循环依次退出

	lastDropped int64 // 上次采样时的事件丢弃累计值，用于换算增量喂给监控指标
}

// New 构建应用实例：按依赖顺序装配各组件，但不监听任何端口（监听由 Start 负责）。
// 参数 cfg：已加载并校验通过的配置；version：程序版本号，用于展示与日志。
// 返回 (*App, error)：全部组件装配成功返回应用实例；任一组件初始化失败返回错误。
func New(cfg *config.Config, version string) (*App, error) {
	// 默认使用主库；演示模式切换到独立演示库，保证演示数据与生产数据隔离
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
	// 拦截事件异步落盘：缓冲 8192 条，每满 200 条或每隔 1 秒批量写入
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

// seed 首次启动时初始化数据库种子数据：默认设置、内置规则、管理员账号与站点列表。
// 参数 cfg：当前配置；st：已打开的存储实例。
// 返回 error：种子写入失败返回错误；内置规则导入失败仅记录日志，不视为致命错误。
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

	// 创建初始管理员；仅数据库中尚无 admin 时才返回新生成的随机口令
	pwd, err := st.SeedAdmin("admin")
	if err != nil {
		return err
	}
	// 口令非空表示本次新建：把随机口令写入文件供首次登录查看，避免只在日志里闪现
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

// Start 启动所有监听与后台任务：数据面 HTTP/HTTPS、控制面，以及各类后台维护循环。
// 返回 error：任一端口绑定失败或服务启动初期出错返回错误；全部就绪返回 nil。
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

	// 缓冲 3：与 HTTP/HTTPS/控制面三个服务一一对应，保证出错 goroutine 发送不阻塞
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

	// 启动初期若收到错误立即返回；等 300ms 无报错即视为启动成功，
	// 之后的运行期错误由各服务 goroutine 写入 errCh（此处不再等待，否则 Start 会一直阻塞）。
	select {
	case err := <-errCh:
		return err
	case <-time.After(300 * time.Millisecond):
		return nil
	}
}

// intelLoop 定时同步威胁情报：启动后先等 5 秒再首次拉取（避免与端口监听抢资源），之后每 3 小时刷新一次。
// 无参数；stopCh 关闭时退出，无返回值。
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
// 无参数；stopCh 关闭时退出，无返回值。
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

// streamLoop 每秒向 SSE 订阅者推送一次实时汇总统计（请求量、拦截量等面板数据）。
// 无参数；stopCh 关闭时退出，无返回值。
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

// maintenance 周期维护循环：30 秒一轮的清理与指标采集、6 小时一次的证书热加载、每小时一次的低频维护。
// 无参数；stopCh 关闭时退出，无返回值。
func (a *App) maintenance() {
	cleanTicker := time.NewTicker(30 * time.Second) // 内存态清理：限流器、命中计数、封禁列表等
	defer cleanTicker.Stop()

	certTicker := time.NewTicker(6 * time.Hour) // 证书热加载：续期脚本更新证书目录后无需重启
	defer certTicker.Stop()

	hourTicker := time.NewTicker(time.Hour) // 每小时低频任务：见 hourly
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

// hourly 每小时执行一次的低频维护：演示模式在每天 04:00 重置演示库，并吊销已过期的刷新令牌。
// 无参数；无返回值，内部错误仅记录日志。
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

// Shutdown 优雅退出：按「停止接新请求 → 等事件落盘 → 关闭数据库」的顺序收尾。
// 参数 ctx：控制各 HTTP 服务等待在途请求完成的超时，超时后未完成的连接被中断。
// 返回 error：各阶段产生的错误合并返回；全部顺利返回 nil。重复调用安全（stopOnce 保证收尾逻辑只执行一次）。
func (a *App) Shutdown(ctx context.Context) error {
	// 先广播停机信号：maintenance/streamLoop/intelLoop/ruleHitsLoop 等后台循环随之退出
	a.stopOnce.Do(func() { close(a.stopCh) })
	a.tasks.Stop()
	// 通知器先停：它持有独立的发送协程，关库前必须收干净
	if a.rt.Notify != nil {
		a.rt.Notify.Stop()
	}

	// 依次关闭三个服务（先数据面后控制面）：Shutdown 停止接收新连接并等待在途请求处理完
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
