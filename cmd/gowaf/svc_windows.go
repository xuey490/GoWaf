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

const (
	svcName        = "gowaf"
	svcDisplayName = "GoWAF"
	svcDescription = "GoWAF（反向代理 + 规则检测）。安装命令：gowaf svc install [配置文件路径]"
)

// maybeRunAsService：SCM 启动本进程时返回 true（内部已完整运行并退出，调用方直接 return）。
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

func defaultSvcConfigPath() string {
	return filepath.Join(os.Getenv("ProgramData"), "gowaf", "configs", "config.yaml")
}

type gowafService struct {
	cfgPath string
}

func (s *gowafService) Execute(args []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const exitOK = 0
	const exitFail = 1

	status <- svc.Status{State: svc.StartPending}

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

	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	// SCM 停止/关机请求 → close(stopCh)，runForeground 里走优雅退出
	stopCh := make(chan struct{})
	reqDone := make(chan struct{})
	go func() {
		defer close(reqDone)
		for c := range r {
			if c.Cmd == svc.Stop || c.Cmd == svc.Shutdown {
				close(stopCh)
				return
			}
		}
	}()

	runErr := runForeground(cfg, a, stopCh)
	<-reqDone
	_ = runErr

	status <- svc.Status{State: svc.StopPending}
	time.Sleep(200 * time.Millisecond)
	return false, exitOK
}

// runSvc 处理 gowaf svc <install|start|stop|uninstall> 子命令（管理员 PowerShell/cmd 下执行）。
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
			time.Sleep(300 * time.Millisecond)
		}
		s, err := m.CreateService(svcName, exe, mgr.Config{
			DisplayName: svcDisplayName,
			Description: svcDescription,
			StartType:   mgr.StartAutomatic,
		}, "-c", cfgPath)
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
		_, _ = s.Control(svc.Stop)
		time.Sleep(800 * time.Millisecond)
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
