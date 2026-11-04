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
	ID         uint     `json:"id"`
	TS         int64    `json:"ts"`
	Site       string   `json:"site"`
	ClientIP   string   `json:"clientIp"`
	Country    string   `json:"country"`
	Method     string   `json:"method"`
	Host       string   `json:"host"`
	Path       string   `json:"path"`
	Query      string   `json:"query"`
	Status     int      `json:"status"`
	Action     string   `json:"action"`
	Score      int      `json:"score"`
	Categories []string `json:"categories"`
	RuleIDs    []string `json:"ruleIds"`
	UA         string   `json:"ua"`
	Referer    string   `json:"referer"`
	ReqID      string   `json:"reqId"`
	LatencyUS  int64    `json:"latencyUs"`
	Upstream   string   `json:"upstream"`
}

// toDTO 转换并处理演示模式的 IP 打码。
func (s *Server) toDTO(e model.Event) eventDTO {
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

// maskIP 保留前三段，最后一段用 * 代替。
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

// eventQuery 组装查询条件。
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

// EventList 分页查询事件。
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

// EventDetail 单条详情。
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

// EventExport 导出 CSV（上限 5 万行）。
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
			break
		}
		q.Current++
	}
	w.Flush()
}

// EventStream SSE：推送实时统计与拦截事件。
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

// StreamStats 定时把汇总推给所有订阅者（由 app 启动的 ticker 调用）。
func (s *Server) StreamStats() {
	now := time.Now().UnixMilli()
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
