package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/response"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

// eventDTO 是事件返回结构，字段与前端表格列一一对应。
type eventDTO struct {
	ID         uint     `json:"id"`         // 事件主键
	TS         int64    `json:"ts"`         // 请求时间（毫秒时间戳）
	Site       string   `json:"site"`       // 命中站点名
	ClientIP   string   `json:"clientIp"`   // 客户端 IP（演示模式开启打码时为脱敏值）
	Country    string   `json:"country"`    // IP 属地（国家/地区）
	Method     string   `json:"method"`     // HTTP 方法
	Host       string   `json:"host"`       // 请求 Host
	Path       string   `json:"path"`       // 请求路径
	Query      string   `json:"query"`      // 查询字符串
	Status     int      `json:"status"`     // 响应状态码
	Action     string   `json:"action"`     // 处置动作（如 block/challenge/log/pass）
	Score      int      `json:"score"`      // 风险评分
	Categories []string `json:"categories"` // 命中的规则分类
	RuleIDs    []string `json:"ruleIds"`    // 命中的规则 ID
	UA         string   `json:"ua"`         // User-Agent
	Referer    string   `json:"referer"`    // Referer 请求头
	ReqID      string   `json:"reqId"`      // 请求 ID，用于去重与追踪
	LatencyUS  int64    `json:"latencyUs"`  // 请求耗时（微秒）
	Upstream   string   `json:"upstream"`   // 命中的上游（回源目标）
}

// toDTO 把事件模型转换为返回 DTO，并处理演示模式的 IP 打码。
// 参数 e：事件模型记录（Categories/RuleIDs 在库中以 JSON 字符串存储）。
// 返回：eventDTO；Categories/RuleIDs 解析失败或为空时返回空切片，保证 JSON 输出为 [] 而非 null。
func (s *Server) toDTO(e model.Event) eventDTO {
	// Categories/RuleIDs 落库为 JSON 字符串，解析失败按空处理
	var cats, rules []string
	_ = json.Unmarshal([]byte(e.Categories), &cats)
	_ = json.Unmarshal([]byte(e.RuleIDs), &rules)
	if cats == nil {
		cats = []string{}
	}
	if rules == nil {
		rules = []string{}
	}
	ip := e.ClientIP
	// 演示模式下按配置对客户端 IP 打码
	if s.Cfg.Demo.Enabled && s.Cfg.Demo.MaskIPs {
		ip = maskIP(ip)
	}
	return eventDTO{
		ID: e.ID, TS: e.TS, Site: e.Site, ClientIP: ip, Country: e.Country, Method: e.Method, Host: e.Host,
		Path: e.Path, Query: e.Query, Status: e.Status, Action: e.Action, Score: e.Score,
		Categories: cats, RuleIDs: rules, UA: e.UA, Referer: e.Referer, ReqID: e.ReqID,
		LatencyUS: e.LatencyUS, Upstream: e.Upstream,
	}
}

// maskIP 对客户端 IP 打码：IPv4 保留前三段，最后一段用 *** 代替。
// 参数 ip：原始 IP 文本。
// 返回：打码后的文本；IPv6 地址（含 ":"）或分段数不为 4 的非法输入统一返回 "***"。
func maskIP(ip string) string {
	if strings.Contains(ip, ":") {
		return "***"
	}
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return "***"
	}
	return parts[0] + "." + parts[1] + "." + parts[2] + ".***"
}

// eventQuery 从请求参数组装事件查询条件。
// 参数 c：Gin 上下文，携带 site、action、category、ip、keyword 过滤参数，
// range（15m/1h/6h/24h/7d/30d）或 from/to（毫秒时间戳）时间范围，以及 current/size 分页参数。
// 返回：store.EventQuery 查询条件。
func (s *Server) eventQuery(c *gin.Context) store.EventQuery {
	from, to, _ := parseRange(c)
	current, size := pageParams(c)
	return store.EventQuery{
		Site:     c.Query("site"),
		Action:   c.Query("action"),
		Category: c.Query("category"),
		IP:       c.Query("ip"),
		Keyword:  c.Query("keyword"),
		From:     from,
		To:       to,
		Current:  current,
		Size:     size,
	}
}

// EventList 分页查询安全事件。
// 参数 c：Gin 上下文，携带时间范围、站点、动作、攻击类型、IP、关键词等过滤参数与分页参数。
// 返回：无（200 返回分页 JSON：list、current、size、total；数据库失败返回 500）。
// 路由：GET /api/events（仅需登录）。
func (s *Server) EventList(c *gin.Context) {
	q := s.eventQuery(c)
	rows, total, err := s.Store.QueryEvents(q)
	if err != nil {
		s.failDB(c, err)
		return
	}
	list := make([]eventDTO, 0, len(rows))
	for _, r := range rows {
		list = append(list, s.toDTO(r))
	}
	response.OK(c, response.NewPage(list, q.Current, q.Size, total))
}

// EventDetail 查询单条安全事件详情。
// 参数 c：Gin 上下文，路径参数 id 为事件 ID。
// 返回：无（200 返回事件 DTO；id 非法返回 400，事件不存在返回 404，数据库失败返回 500）。
// 路由：GET /api/events/:id（仅需登录）。
func (s *Server) EventDetail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "id 非法")
		return
	}
	e, err := s.Store.EventByID(uint(id))
	if err != nil {
		s.failDB(c, err)
		return
	}
	if e == nil {
		response.NotFound(c, "事件不存在")
		return
	}
	response.OK(c, s.toDTO(*e))
}

// EventExport 按与列表一致的过滤条件把安全事件导出为 CSV，最多 5 万行。
// 参数 c：Gin 上下文，过滤参数与 EventList 相同（忽略分页参数，从第一页起按每页 200 条滚动拉取）。
// 返回：无（直接向响应流写出带 UTF-8 BOM 的 CSV，浏览器以附件形式下载；查询中途失败则提前结束输出）。
// 路由：GET /api/events/export（仅需登录）。
func (s *Server) EventExport(c *gin.Context) {
	q := s.eventQuery(c)
	q.Current = 1
	q.Size = 200

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition",
		fmt.Sprintf("attachment; filename=waf-events-%s.csv", time.Now().Format("20060102-150405")))
	// BOM 让 Excel 正确识别 UTF-8
	_, _ = c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"时间", "站点", "客户端IP", "IP属地", "方法", "Host", "路径", "查询", "状态码",
		"动作", "评分", "类型", "命中规则", "UA", "请求ID", "耗时(us)"})

	// 每页 200 条分批查询、边查边写，避免一次性加载超过 5 万行
	exported := 0
	for exported < 50000 {
		rows, total, err := s.Store.QueryEvents(q)
		if err != nil || len(rows) == 0 {
			break
		}
		for _, r := range rows {
			d := s.toDTO(r)
			_ = w.Write([]string{
				time.UnixMilli(d.TS).Format("2006-01-02 15:04:05"),
				d.Site, d.ClientIP, d.Country, d.Method, d.Host, d.Path, d.Query,
				strconv.Itoa(d.Status), d.Action, strconv.Itoa(d.Score),
				strings.Join(d.Categories, "|"), strings.Join(d.RuleIDs, "|"),
				d.UA, d.ReqID, strconv.FormatInt(d.LatencyUS, 10),
			})
			exported++
		}
		if int64(exported) >= total {
			break // 已写完全部匹配行，提前结束
		}
		q.Current++
	}
	w.Flush()
}

// EventStream 通过 SSE 长连接推送实时拦截事件与统计消息。
// 参数 c：Gin 上下文，无业务参数，仅用于鉴权与承载长连接。
// 返回：不返回，持续写出 SSE 帧：ready 首帧、message 数据帧（事件/统计 JSON）、": ping" 注释心跳帧；客户端断开或广播通道关闭时结束。
// 路由：GET /api/stream/events（仅需登录，不套 CSRF 与审计中间件）。
func (s *Server) EventStream(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.Flush()

	ch := s.Stream.Subscribe()
	defer s.Stream.Unsubscribe(ch)

	// 首帧：告诉前端连接成功
	_, _ = c.Writer.Write([]byte("event: ready\ndata: {\"ok\":true}\n\n"))
	c.Writer.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 心跳，防止中间代理断开长连接
			_, _ = c.Writer.Write([]byte(": ping\n\n"))
			c.Writer.Flush()
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_, _ = c.Writer.Write([]byte("event: message\ndata: "))
			_, _ = c.Writer.Write(msg)
			_, _ = c.Writer.Write([]byte("\n\n"))
			c.Writer.Flush()
		}
	}
}

// StreamStats 把最近 10 秒的汇总统计广播给所有 SSE 订阅者。
// 参数：无，由 app 启动的定时器周期调用，不经过 HTTP 路由。
// 返回：无；查询或 JSON 编码失败时静默跳过本轮广播。
func (s *Server) StreamStats() {
	now := time.Now().UnixMilli()
	// 统计全站（站点为空）最近 10 秒的汇总指标
	ov, err := s.Store.Overview("", now-10000, now)
	if err != nil {
		return
	}
	payload, err := json.Marshal(gin.H{
		"type":       "stat",
		"ts":         now,
		"qps":        ov.QPS,
		"blocked":    ov.Blocked,
		"challenged": ov.Challenged,
		"mode":       s.RT.Engine.Mode(),
	})
	if err != nil {
		return
	}
	s.Stream.Publish(payload)
}
