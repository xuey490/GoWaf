// Package handler 实现控制面全部业务接口。
package handler

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/auth"
	"gowaf/internal/admin/middleware"
	"gowaf/internal/admin/response"
	"gowaf/internal/config"
	"gowaf/internal/runtime"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
	"gowaf/internal/tasks"
)

// StreamHub 负责 SSE 广播。
type StreamHub struct {
	subscribers map[chan []byte]struct{}
	register    chan chan []byte
	unregister  chan chan []byte
	broadcast   chan []byte
}

// NewStreamHub 创建广播中心。
func NewStreamHub() *StreamHub {
	h := &StreamHub{
		subscribers: make(map[chan []byte]struct{}),
		register:    make(chan chan []byte),
		unregister:  make(chan chan []byte),
		broadcast:   make(chan []byte, 64),
	}
	go h.loop()
	return h
}

func (h *StreamHub) loop() {
	for {
		select {
		case ch := <-h.register:
			h.subscribers[ch] = struct{}{}
		case ch := <-h.unregister:
			if _, ok := h.subscribers[ch]; ok {
				delete(h.subscribers, ch)
				close(ch)
			}
		case msg := <-h.broadcast:
			for ch := range h.subscribers {
				select {
				case ch <- msg:
				default:
					// 订阅者太慢，丢弃这一帧，不阻塞其它订阅者
				}
			}
		}
	}
}

// Publish 广播一条消息。
func (h *StreamHub) Publish(msg []byte) {
	select {
	case h.broadcast <- msg:
	default:
	}
}

// Subscribe 注册订阅者。
func (h *StreamHub) Subscribe() chan []byte {
	ch := make(chan []byte, 16)
	h.register <- ch
	return ch
}

// Unsubscribe 注销订阅者。
func (h *StreamHub) Unsubscribe(ch chan []byte) {
	h.unregister <- ch
}

// Deps 是 handler 的全部依赖。
type Deps struct {
	Cfg   *config.Config
	Store *store.Store
	RT    *runtime.Runtime
	Auth  *auth.Manager
	// LoginLimiter 不能叫 Login：会和 handler 方法 Login 冲突（方法提升会覆盖字段）
	LoginLimiter *middleware.LoginLimiter
	Stream       *StreamHub
	Version      string
	// Reload 由 app 注入，避免 handler 反向依赖 runtime 的装配细节
	Reload func(ctx context.Context) error
	// Tasks 计划任务运行器，由 app 注入
	Tasks *tasks.Runner
}

// Server 承载所有 handler。
type Server struct {
	Deps
}

// New 创建 handler 集合。
func New(d Deps) *Server {
	if d.Stream == nil {
		d.Stream = NewStreamHub()
	}
	return &Server{Deps: d}
}

// ===== 通用工具 =====

// pageParams 解析分页，对齐前端 current/size 约定。
func pageParams(c *gin.Context) (current, size int) {
	current, _ = strconv.Atoi(c.DefaultQuery("current", "1"))
	size, _ = strconv.Atoi(c.DefaultQuery("size", "20"))
	if current <= 0 {
		current = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	return
}

// parseRange 支持 range=1h/24h/7d 与显式 from/to（毫秒）。
func parseRange(c *gin.Context) (from, to int64, label string) {
	to = time.Now().UnixMilli()
	if f := c.Query("from"); f != "" {
		if v, err := strconv.ParseInt(f, 10, 64); err == nil {
			from = v
		}
	}
	if t := c.Query("to"); t != "" {
		if v, err := strconv.ParseInt(t, 10, 64); err == nil {
			to = v
		}
	}
	if from > 0 {
		return from, to, "custom"
	}
	label = c.DefaultQuery("range", "1h")
	var d time.Duration
	switch label {
	case "15m":
		d = 15 * time.Minute
	case "1h":
		d = time.Hour
	case "6h":
		d = 6 * time.Hour
	case "24h":
		d = 24 * time.Hour
	case "7d":
		d = 7 * 24 * time.Hour
	case "30d":
		d = 30 * 24 * time.Hour
	default:
		d = time.Hour
		label = "1h"
	}
	return to - int64(d/time.Millisecond), to, label
}

// bucketSeconds 依时间范围选择聚合粒度，避免返回上万个点。
func bucketSeconds(from, to int64) int {
	span := (to - from) / 1000
	switch {
	case span <= 900:
		return 15
	case span <= 3600:
		return 60
	case span <= 6*3600:
		return 300
	case span <= 24*3600:
		return 900
	case span <= 7*24*3600:
		// 7 天用 1 小时桶，约 168 个点
		return 3600
	default:
		// 30 天用 6 小时桶，约 120 个点
		return 21600
	}
}

// splitCSV 解析逗号分隔字段。
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// failDB 统一处理数据库错误。
func (s *Server) failDB(c *gin.Context, err error) {
	response.Internal(c, "数据库操作失败: "+err.Error())
}

// eventStreamMsg 是 SSE 推送的事件帧。
//
// 通过嵌入 eventDTO 复用列表接口的字段定义，避免推送结构随 DTO 演进而漂移；
// 额外带 type 字段，供前端区分事件帧与 stat 统计帧。
type eventStreamMsg struct {
	eventDTO
	Type string `json:"type"`
}

// MarshalEvent 把落库事件编码为 SSE 事件帧。
//
// 注意：事件由数据面异步批量落库，推送时主键可能尚未分配（ID 为 0），
// 前端应以 reqId 作为去重与列表 key。
func (s *Server) MarshalEvent(e model.Event) ([]byte, error) {
	return json.Marshal(eventStreamMsg{eventDTO: s.toDTO(e), Type: "event"})
}

// reloadRuntime 让新配置立即生效。
func (s *Server) reloadRuntime(c *gin.Context) error {
	if s.Reload == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	return s.Reload(ctx)
}

// touchSettings 版本号 +1，用于热更新检测。
func (s *Server) touchSettings(key, value, operator string) error {
	now := store.NowMS()
	res := s.Store.Gorm().Exec(
		`UPDATE settings SET value = ?, version = version + 1, updated_at = ? WHERE key = ?`,
		value, now, key)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return s.Store.Gorm().Exec(
			`INSERT INTO settings(key, value, version, updated_at) VALUES(?,?,1,?)`,
			key, value, now).Error
	}
	return nil
}
