//go:build !windows

package main

import (
	"fmt"
	"os"
)

// 非 Windows 平台：没有 SCM，svc 子命令无意义，明确拒绝而不是静默吞掉。
func maybeRunAsService() bool { return false }

func runSvc(args []string) {
	fmt.Println("svc 子命令仅支持 Windows（Linux 用 systemd：install.sh 已自动安装 gowaf.service）")
	os.Exit(1)
}
