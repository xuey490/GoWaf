//go:build !windows

package main

import (
	"fmt"
	"os"
)

// maybeRunAsService 报告是否应进入 Windows 服务模式；非 Windows 平台没有
// 服务控制管理器（SCM），svc 子命令无意义，明确拒绝而不是静默吞掉。
// 参数：无。
// 返回：恒为 false，调用方继续按前台模式启动。
func maybeRunAsService() bool { return false }

// runSvc 处理 `gowaf svc ...` 子命令；非 Windows 平台不支持，提示改用 systemd 后退出。
// 参数 args：svc 之后的命令行参数（本实现不使用）。
// 返回：无（打印提示后以状态码 1 退出）。
func runSvc(args []string) {
	fmt.Println("svc 子命令仅支持 Windows（Linux 用 systemd：install.sh 已自动安装 gowaf.service）")
	os.Exit(1)
}
