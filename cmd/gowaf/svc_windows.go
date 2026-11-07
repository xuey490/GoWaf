//go:build windows

// Windows 服务模式：gowaf svc install/start/stop/uninstall。
//
// 设计约束：
//   - 不依赖 NSSM/WinSW 等第三方，保住「单二进制」卖点（x/sys/windows/svc 直接实现 SCM 协议）
//   - 服务名固定 gowaf；安装路径取当前 exe 绝对路径（CreateService 自动加引号）
//   - 被 SCM 拉起时（IsWindowsService）走 Execute，不解析 --help，只从参数提取 -c
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"gowaf/internal/app"
	"gowaf/internal/config"
)

// Windows 服务注册信息常量：服务名、显示名与描述。
const (
	svcName        = "gowaf"                                              // 服务内部名称（net start/stop、SCM 标识用的名字）
	svcDisplayName = "GoWAF"                                              // 服务管理器中显示的名称
	svcDescription = "GoWAF（反向代理 + 规则检测）。安装命令：gowaf svc install [配置文件路径]" // 服务描述，同时向用户提示安装命令
)

// maybeRunAsService 判断本进程是否被 Windows 服务控制管理器（SCM）拉起；
// 是则在内部完整运行服务直至退出，调用方拿到 true 后直接 return。
// 参数：无（配置路径从 os.Args 中的 -c 提取，缺省用 ProgramData 下的默认路径）。
// 返回：true 表示已按服务模式运行完毕，main 不再继续；false 表示普通前台启动。
func maybeRunAsService() bool {
	isSvc, _ := svc.IsWindowsService()
	if !isSvc {
		return false
	}

	// 从原始参数提取配置路径（SCM 只会带我们安装时给的参数）
	cfgPath := defaultSvcConfigPath()
	for i := 1; i < len(os.Args)-1; i++ {
		if os.Args[i] == "-c" {
			cfgPath = os.Args[i+1]
		}
	}
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	if err := svc.Run(svcName, &gowafService{cfgPath: cfgPath}); err != nil {
		log.Fatalf("[svc] 服务运行失败: %v", err)
	}
	return true
}

// defaultSvcConfigPath 返回服务模式下的默认配置文件路径。
// 参数：无。
// 返回：%ProgramData%\gowaf\configs\config.yaml 对应的绝对路径字符串。
func defaultSvcConfigPath() string {
	return filepath.Join(os.Getenv("ProgramData"), "gowaf", "configs", "config.yaml")
}

// gowafService 实现 svc.Handler 接口，是 Windows 服务模式下被 SCM 调度的服务主体。
type gowafService struct {
	cfgPath string // 服务启动时使用的配置文件路径（进入服务模式前已从参数解析）
}

// Execute 是 svc.Handler 接口回调：SCM 拉起服务后由 svc.Run 调用，承载完整的服务生命周期
// （加载配置 → 启动应用 → 上报 Running → 等待停止请求 → 优雅退出 → 上报 Stopped）。
// 参数 args：SCM 传入的服务启动参数（本实现未使用，配置路径已在构造时固化）。
// 参数 r：SCM 控制请求通道，停止/关机等指令由此到达。
// 参数 status：向 SCM 回报服务状态的通道（StartPending/Running/StopPending 等）。
// 返回：bool 表示进程是否因严重错误需立即退出（本实现始终为 false）；
// uint32 为服务退出码，0 表示成功，1 表示失败。
func (s *gowafService) Execute(args []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const exitOK = 0   // 服务正常退出的退出码
	const exitFail = 1 // 初始化/启动失败时的退出码

	status <- svc.Status{State: svc.StartPending} // 告知 SCM 正在启动，尚未就绪

	cfg, err := config.Load(s.cfgPath)
	if err != nil {
		log.Printf("[svc] 配置错误: %v", err)
		return false, exitFail
	}
	a, err := app.New(cfg, version)
	if err != nil {
		log.Printf("[svc] 初始化失败: %v", err)
		return false, exitFail
	}
	if err := a.Start(); err != nil {
		log.Printf("[svc] 启动失败: %v", err)
		return false, exitFail
	}
	log.Printf("[svc] 服务已启动（配置 %s）", s.cfgPath)

	// 上报 Running，并声明可接受 SCM 的「停止」与「关机」控制请求
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	// SCM 停止/关机请求 → close(stopCh)，runForeground 里走优雅退出
	stopCh := make(chan struct{})
	reqDone := make(chan struct{}) // 控制请求 goroutine 退出完成的通知
	go func() {
		defer close(reqDone)
		for c := range r {
			if c.Cmd == svc.Stop || c.Cmd == svc.Shutdown {
				close(stopCh)
				return
			}
		}
	}()

	runErr := runForeground(cfg, a, stopCh) // 阻塞至收到停止请求并完成优雅关闭
	<-reqDone                               // 等待控制请求 goroutine 退出，避免泄漏
	_ = runErr

	status <- svc.Status{State: svc.StopPending} // 告知 SCM 正在停止
	time.Sleep(200 * time.Millisecond)           // 给 SCM 一点时间感知 StopPending 再返回
	return false, exitOK
}

// runSvc 处理 `gowaf svc <install|start|stop|uninstall>` 子命令，需在管理员 PowerShell/cmd 下执行。
// 参数 args：svc 之后的参数，args[0] 为子命令；install 时 args[1] 可选，指定配置文件路径。
// 返回：无（用法错误或操作失败时打印提示并以状态码 1 退出）。
func runSvc(args []string) {
	usage := func() {
		fmt.Println("用法：gowaf svc <install|start|stop|uninstall> [配置文件路径]")
		fmt.Println("  install    安装为 Windows 服务（开机自启，默认配置 ProgramData/gowaf/configs/config.yaml）")
		fmt.Println("  start      启动服务")
		fmt.Println("  stop       停止服务")
		fmt.Println("  uninstall  停止并卸载服务（不动数据目录）")
	}
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	m, err := mgr.Connect()
	if err != nil {
		fmt.Printf("连接服务管理器失败（需要管理员权限）: %v\n", err)
		os.Exit(1)
	}
	defer m.Disconnect()

	switch args[0] {
	case "install":
		cfgPath := defaultSvcConfigPath()
		if len(args) > 1 {
			cfgPath = args[1]
		}
		exe, err := os.Executable()
		if err != nil {
			fmt.Printf("取 exe 路径失败: %v\n", err)
			os.Exit(1)
		}
		// 覆盖安装：旧服务先删（保留数据目录）
		if s, err := m.OpenService(svcName); err == nil {
			_ = s.Delete()
			_ = s.Close()
			time.Sleep(300 * time.Millisecond) // 等待 SCM 释放旧服务句柄，避免重名冲突
		}
		s, err := m.CreateService(svcName, exe, mgr.Config{
			DisplayName: svcDisplayName,
			Description: svcDescription,
			StartType:   mgr.StartAutomatic, // 开机自启
		}, "-c", cfgPath) // 启动参数固定携带 -c，SCM 拉起时服务据此定位配置
		if err != nil {
			fmt.Printf("创建服务失败: %v\n", err)
			os.Exit(1)
		}
		defer s.Close()
		if err := s.Start(); err != nil {
			fmt.Printf("服务已安装，但启动失败（可稍后 net start %s）: %v\n", svcName, err)
		} else {
			fmt.Printf("服务已安装并启动：gowaf（配置 %s）\n", cfgPath)
		}

	case "start":
		s, err := m.OpenService(svcName)
		if err != nil {
			fmt.Printf("服务未安装: %v\n", err)
			os.Exit(1)
		}
		defer s.Close()
		if err := s.Start(); err != nil {
			fmt.Printf("启动失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("gowaf 已启动")

	case "stop":
		s, err := m.OpenService(svcName)
		if err != nil {
			fmt.Printf("服务未安装: %v\n", err)
			os.Exit(1)
		}
		defer s.Close()
		if _, err := s.Control(svc.Stop); err != nil {
			fmt.Printf("停止失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("gowaf 已停止")

	case "uninstall", "remove":
		s, err := m.OpenService(svcName)
		if err != nil {
			fmt.Printf("服务未安装: %v\n", err)
			os.Exit(1)
		}
		_, _ = s.Control(svc.Stop)         // 先尝试停止运行中的服务（未运行则忽略错误）
		time.Sleep(800 * time.Millisecond) // 等待服务完全停止后再删除
		if err := s.Delete(); err != nil {
			fmt.Printf("卸载失败: %v\n", err)
			os.Exit(1)
		}
		_ = s.Close()
		fmt.Println("gowaf 服务已卸载（数据目录未删除）")

	default:
		usage()
		os.Exit(1)
	}
}
