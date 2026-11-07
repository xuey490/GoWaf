package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gowaf/internal/app"
	"gowaf/internal/config"
)

// runForeground 阻塞等待退出时机，随后对已启动的应用执行优雅关闭。
// 直到「系统信号」或「服务停止请求」到来才开始退出流程。
// 参数 cfg：已加载的全局配置，用于读取优雅退出超时时长 ShutdownTimeout。
// 参数 a：已启动的应用实例，退出时调用其 Shutdown 完成优雅关闭。
// 参数 stopCh：Windows 服务模式的停止请求通道——SCM 的停止请求由 svc_windows.go
// 的 Execute goroutine 通过它传入；前台模式传 nil，仅监听 SIGINT/SIGTERM。
// 返回：错误值；当前实现始终返回 nil，退出过程中的错误仅记录日志。
func runForeground(cfg *config.Config, a *app.App, stopCh chan struct{}) error {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM) // 同时监听 Ctrl-C（SIGINT）与 SIGTERM

	select {
	case s := <-sig:
		log.Printf("[main] 收到信号 %s，开始优雅退出", s)
	case <-stopCh: // stopCh 为 nil 时该分支永远阻塞，前台模式等价于只等系统信号
		log.Printf("[main] 收到服务停止请求，开始优雅退出")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout.Duration())
	defer cancel()
	if err := a.Shutdown(ctx); err != nil {
		log.Printf("[main] 退出过程中有错误: %v", err)
	}
	time.Sleep(100 * time.Millisecond) // 略作等待，确保退出日志在进程结束前刷出
	log.Printf("[main] 已退出")
	return nil
}
