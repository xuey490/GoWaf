// Command gowaf 是 Go-WAF 的唯一入口。
//
// 启动顺序：读配置 → 校验 → 建库迁移 → 装配 → 起三组监听 → 等信号优雅退出。
//
// Windows：SCM 拉起时自动进入服务模式（svc_windows.go）；
// `gowaf svc install|start|stop|uninstall` 管理服务注册。
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"

	"gowaf/internal/app"
	"gowaf/internal/config"
)

// 以下三个变量由构建脚本通过 -ldflags -X 注入，源码内的值仅作默认。
var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

func main() {
	// 被 Windows SCM 拉起：进服务模式，不解析 flag
	if maybeRunAsService() {
		return
	}
	// `gowaf svc ...`：Windows 服务注册管理
	if len(os.Args) > 1 && os.Args[1] == "svc" {
		runSvc(os.Args[2:])
		return
	}

	cfgPath := flag.String("c", "configs/config.yaml", "配置文件路径")
	showVer := flag.Bool("v", false, "打印版本并退出")
	checkOnly := flag.Bool("t", false, "只校验配置，不启动")
	flag.Parse()

	if *showVer {
		fmt.Printf("gowaf %s (commit %s, built %s, %s/%s)\n",
			version, commit, buildTime, runtime.GOOS, runtime.GOARCH)
		return
	}

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("[main] 启动 gowaf %s (commit %s)", version, commit)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("[main] 配置错误: %v", err)
	}
	if *checkOnly {
		fmt.Println("配置校验通过")
		return
	}

	a, err := app.New(cfg, version)
	if err != nil {
		log.Fatalf("[main] 初始化失败: %v", err)
	}

	if err := a.Start(); err != nil {
		log.Fatalf("[main] 启动失败: %v", err)
	}

	_ = runForeground(cfg, a, nil)
}
