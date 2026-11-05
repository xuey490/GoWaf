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

// StreamHub 负责 SSE 广播：维护订阅者通道集合，把事件帧分发给所有订阅方。
//
// 并发模型：全部状态经由四个通道在唯一的 loop 协程内串行读写，自身不加锁。
type StreamHub struct {
	subscribers map[chan []byte]struct{} // 订阅者集合，key 为每个客户端的数据通道
	register    chan chan []byte         // 注册入口：新订阅者的通道由此加入
	unregister  chan chan []byte         // 注销入口：断开的客户端由此移除并关闭通道
	broadcast   chan []byte              // 广播入口：待分发消息（带 64 帧缓冲）
}

// NewStreamHub 创建广播中心，并启动后台分发协程。
// 返回：就绪的 StreamHub 指针。
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

// loop 是广播中心的常驻协程：串行处理注册、注销与广播三类事件。
// 广播时对每个订阅者做非阻塞投递，缓冲已满则丢弃该帧，避免慢客户端拖垮其它订阅者。
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

// Publish 广播一条消息给所有订阅者。
// 参数 msg：待广播的消息帧（通常是编码后的 JSON）。
// 返回：无；广播缓冲已满时直接丢弃，不阻塞调用方。
func (h *StreamHub) Publish(msg []byte) {
	select {
	case h.broadcast <- msg:
	default:
	}
}

// Subscribe 注册一个新订阅者。
// 返回：带 16 帧缓冲的数据通道，SSE 处理器从中读取并推送给客户端；断开后须调用 Unsubscribe。
func (h *StreamHub) Subscribe() chan []byte {
	ch := make(chan []byte, 16)
	h.register <- ch
	return ch
}

// Unsubscribe 注销订阅者并关闭其数据通道。
// 参数 ch：此前由 Subscribe 返回的通道；重复注销无副作用。
func (h *StreamHub) Unsubscribe(ch chan []byte) {
	h.unregister <- ch
}

// Deps 是 handler 的全部依赖，由应用装配层一次性注入。
type Deps struct {
	Cfg   *config.Config   // 全局静态配置（启动时加载）
	Store *store.Store     // 持久层封装，内含 GORM 数据库句柄
	RT    *runtime.Runtime // 运行时：设置表、检测引擎、情报与地理库等
	Auth  *auth.Manager    // 会话与用户管理器
	// LoginLimiter 不能叫 Login：会和 handler 方法 Login 冲突（方法提升会覆盖字段）
	LoginLimiter *middleware.LoginLimiter // 登录失败限流器
	Stream       *StreamHub               // SSE 事件广播中心，为 nil 时由 New 补建
	Version      string                   // 程序版本号，供系统信息接口展示
	// Reload 由 app 注入，避免 handler 反向依赖 runtime 的装配细节
	Reload func(ctx context.Context) error // 触发配置全量热加载的回调
	// Tasks 计划任务运行器，由 app 注入
	Tasks *tasks.Runner // 计划任务调度运行器
}

// Server 承载所有 HTTP 处理器，内嵌 Deps 获得全部依赖。
type Server struct {
	Deps
}

// New 创建 handler 集合。
// 参数 d：依赖集合，其中 Stream 为 nil 时自动补建广播中心。
// 返回：*Server 实例。
func New(d Deps) *Server {
	if d.Stream == nil {
		d.Stream = NewStreamHub()
	}
	return &Server{Deps: d}
}

// ===== 通用工具 =====

// pageParams 解析分页参数，对齐前端 current/size 约定。
// 参数 c：Gin 上下文，读取查询参数 current（页码）与 size（每页条数）。
// 返回：current 页码（最小 1）；size 每页条数（默认 20，上限 200）。
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

// parseRange 解析统计时间范围，支持 range=15m/1h/6h/24h/7d/30d 与显式 from/to（毫秒时间戳）。
// 参数 c：Gin 上下文，读取查询参数 from、to、range。
// 返回：from 起始毫秒时间戳；to 结束毫秒时间戳（缺省为当前时刻）；
// label 规范化后的范围标识（显式传入 from 时为 custom，非法 range 回落为 1h）。
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

// bucketSeconds 依时间范围选择聚合粒度，避免返回上万个数据点。
// 参数 from、to：毫秒时间戳区间。
// 返回：聚合桶宽度（秒），跨度越大粒度越粗，取值 15 秒至 6 小时。
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

// splitCSV 解析逗号分隔字段，逐项去首尾空白并丢弃空项。
// 参数 s：逗号分隔的原始字符串。
// 返回：非空项切片；输入为空白时返回 nil。
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

// failDB 统一处理数据库错误，向前端返回 500 与通用错误文案。
// 参数 c：Gin 上下文；err：底层错误，其文本会拼接进响应便于排障。
// 返回：无。
func (s *Server) failDB(c *gin.Context, err error) {
	response.Internal(c, "数据库操作失败: "+err.Error())
}

// eventStreamMsg 是 SSE 推送的事件帧。
//
// 通过嵌入 eventDTO 复用列表接口的字段定义，避免推送结构随 DTO 演进而漂移；
// 额外带 type 字段，供前端区分事件帧与 stat 统计帧。
type eventStreamMsg struct {
	eventDTO        // 嵌入列表接口的事件字段，保证推送结构与列表一致
	Type     string `json:"type"` // 帧类型，固定为 "event"
}

// MarshalEvent 把落库事件编码为 SSE 事件帧。
// 参数 e：事件模型记录。
// 返回：JSON 编码后的字节切片与可能的编码错误。
//
// 注意：事件由数据面异步批量落库，推送时主键可能尚未分配（ID 为 0），
// 前端应以 reqId 作为去重与列表 key。
func (s *Server) MarshalEvent(e model.Event) ([]byte, error) {
	return json.Marshal(eventStreamMsg{eventDTO: s.toDTO(e), Type: "event"})
}

// reloadRuntime 调用注入的 Reload 回调，让落库的新配置立即生效（限 5 秒超时）。
// 参数 c：Gin 上下文，超时基于其请求上下文派生。
// 返回：重载失败时的错误；Reload 未注入时直接返回 nil。
func (s *Server) reloadRuntime(c *gin.Context) error {
	if s.Reload == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	return s.Reload(ctx)
}

// touchSettings 写入单条设置并自增版本号，供热更新检测比对版本。
// 参数 key：设置键名；value：新值；operator：操作者名称（当前 SQL 未落库，保留作审计扩展）。
// 返回：错误；键不存在时自动插入（版本号从 1 开始），即 upsert 语义。
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
