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

// runForeground 前台运行：起服务后阻塞，直到「系统信号」或「服务停止请求」到来，
// 然后优雅退出。stopCh 非 nil 时表示 Windows 服务模式——SCM 的停止请求由
// svc_windows.go 的 Execute goroutine 通过它传入。
func runForeground(cfg *config.Config, a *app.App, stopCh chan struct{}) error {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	select {
	case s := <-sig:
		log.Printf("[main] 收到信号 %s，开始优雅退出", s)
	case <-stopCh:
		log.Printf("[main] 收到服务停止请求，开始优雅退出")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout.Duration())
	defer cancel()
	if err := a.Shutdown(ctx); err != nil {
		log.Printf("[main] 退出过程中有错误: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	log.Printf("[main] 已退出")
	return nil
}
