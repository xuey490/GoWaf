package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"gowaf/internal/store/model"
)

// EventWriter 异步批量写事件：有界 channel + 批量事务 + 满则丢弃。
type EventWriter struct {
	store   *Store           // 所属存储，批量落库时使用其写连接
	ch      chan model.Event // 有界事件队列，容量即创建时的 queueSize
	dropped atomic.Int64     // 累计丢弃条数（队列满或批次落库失败）
	written atomic.Int64     // 累计成功落库条数
	done    chan struct{}    // 后台刷盘协程退出时关闭，供 Close 等待落盘完成
}

// NewEventWriter 创建异步事件写入器，并启动后台刷盘协程。
// 参数 queueSize：队列容量（<=0 时取默认 8192）；batchSize：单批落库条数（<=0 时取 200）；
// flush：定时刷盘间隔（<=0 时取 1s），保证低流量下事件也会及时落库。
// 返回 *EventWriter：写入器实例。
func (s *Store) NewEventWriter(queueSize, batchSize int, flush time.Duration) *EventWriter {
	if queueSize <= 0 {
		queueSize = 8192
	}
	if batchSize <= 0 {
		batchSize = 200
	}
	if flush <= 0 {
		flush = time.Second
	}
	w := &EventWriter{
		store: s,
		ch:    make(chan model.Event, queueSize),
		done:  make(chan struct{}),
	}
	go w.loop(batchSize, flush)
	return w
}

// Enqueue 非阻塞投递一条事件。
// 参数 e：待写入的安全事件。
// 队列满时直接丢弃并累加丢弃计数（fail-open），绝不阻塞请求转发路径。
func (w *EventWriter) Enqueue(e model.Event) {
	select {
	case w.ch <- e:
	default:
		w.dropped.Add(1)
	}
}

// Dropped 返回累计丢弃的事件条数（含落库失败的批次）。
// 返回 int64：丢弃计数（原子读取）。
func (w *EventWriter) Dropped() int64 { return w.dropped.Load() }

// Written 返回累计成功落库的事件条数。
// 返回 int64：写入计数（原子读取）。
func (w *EventWriter) Written() int64 { return w.written.Load() }

// Close 关闭事件队列并阻塞等待后台协程刷完剩余事件。
// 调用后不应再 Enqueue；Close 本身只应调用一次。
func (w *EventWriter) Close() {
	close(w.ch)
	<-w.done
}

// loop 后台刷盘协程：事件攒够 batchSize 或 flush 定时器到期即批量落库；
// 队列被关闭后先清空剩余缓冲再退出，并关闭 done 通知 Close 返回。
// 参数 batchSize：单批条数；flush：定时刷盘间隔。
func (w *EventWriter) loop(batchSize int, flush time.Duration) {
	defer close(w.done)
	ticker := time.NewTicker(flush)
	defer ticker.Stop()

	buf := make([]model.Event, 0, batchSize)
	flushFn := func() {
		if len(buf) == 0 {
			return
		}
		if err := w.store.insertEvents(buf); err != nil {
			w.dropped.Add(int64(len(buf)))
		} else {
			w.written.Add(int64(len(buf)))
		}
		buf = buf[:0]
	}

	for {
		select {
		case e, ok := <-w.ch:
			if !ok {
				flushFn()
				return
			}
			buf = append(buf, e)
			if len(buf) >= batchSize {
				flushFn()
			}
		case <-ticker.C:
			flushFn()
		}
	}
}

// eventInsertSQL 事件批量插入的预编译 SQL，占位符顺序与 events 表列一一对应。
const eventInsertSQL = `INSERT INTO events
(ts, site, client_ip, method, host, path, query, status, action, score,
 categories, rule_ids, ua, referer, req_id, latency_us, upstream, country, bytes)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

// insertEvents 在单个事务中批量写入一批事件。
// 参数 batch：待写入的事件，逐行绑定到预编译语句执行。
// 返回 error：任一行失败即回滚整批并返回错误。
func (s *Store) insertEvents(batch []model.Event) error {
	tx, err := s.write.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(eventInsertSQL)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	for i := range batch {
		e := &batch[i]
		// 逐行绑定参数：path/query/ua/referer 按列长上限截断，categories/rule_ids 空值补 "[]"
		if _, err := stmt.Exec(e.TS, e.Site, e.ClientIP, e.Method, e.Host, truncate(e.Path, 1024),
			truncate(e.Query, 2048), e.Status, e.Action, e.Score,
			defaultJSON(e.Categories), defaultJSON(e.RuleIDs), truncate(e.UA, 512),
			truncate(e.Referer, 512), e.ReqID, e.LatencyUS, e.Upstream, e.Country, e.Bytes); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return err
		}
	}
	_ = stmt.Close()
	return tx.Commit()
}

// truncate 按字节截断字符串到最多 n 字节，防止超长输入拖慢写入。
// 参数 s：原始字符串；n：字节上限。
// 返回 string：未超限时原样返回；超限时返回前 n 字节（按字节截断，可能切断多字节字符）。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// defaultJSON 把空 JSON 字符串归一化为 "[]"，保证 JSON 列内容始终合法。
// 参数 s：原始字符串。
// 返回 string：非空则原样返回，否则返回 "[]"。
func defaultJSON(s string) string {
	if s == "" {
		return "[]"
	}
	return s
}

// EventQuery 事件查询条件，零值字段不参与过滤。
type EventQuery struct {
	Site     string // 站点名，精确匹配
	Action   string // 处置动作（pass/block/challenge），精确匹配
	Category string // 攻击类别，对 categories JSON 文本做 LIKE 模糊匹配
	IP       string // 客户端 IP，精确匹配
	Keyword  string // 模糊关键字，匹配 path / ua / query 任意一处
	From     int64  // 起始时间（毫秒时间戳，>0 时生效）
	To       int64  // 结束时间（毫秒时间戳，>0 时生效）
	Current  int    // 页码，从 1 开始（<=0 视为 1）
	Size     int    // 每页条数（<=0 取 20，最大 200）
	// RuleHitOnly 只保留真正命中规则的事件（rule_ids 非空）。
	// 封禁详情用：封禁生效后的每次请求都会记一条「无规则、0 分」的拦截事件，
	// 这类重复噪声时间最新，会把真正触发封禁的命中记录挤出首屏。
	RuleHitOnly bool
}

// QueryEvents 按条件分页查询安全事件。
// 参数 q：过滤与分页条件（见 EventQuery）。
// 返回 ([]model.Event, int64, error)：当前页事件列表（按时间倒序）、符合条件的总条数；查询失败时返回错误。
func (s *Store) QueryEvents(q EventQuery) ([]model.Event, int64, error) {
	where, args := buildEventWhere(q)
	// 先查总数供分页展示；总数为 0 时直接返回空页，跳过明细查询
	var total int64
	if err := s.read.QueryRow("SELECT COUNT(1) FROM events"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []model.Event{}, 0, nil
	}
	if q.Current <= 0 {
		q.Current = 1
	}
	if q.Size <= 0 {
		q.Size = 20
	}
	if q.Size > 200 {
		q.Size = 200
	}
	offset := (q.Current - 1) * q.Size

	sqlStr := `SELECT id, ts, site, client_ip, method, host, path, query, status, action, score,
 categories, rule_ids, ua, referer, req_id, latency_us, upstream, country, bytes
 FROM events` + where + ` ORDER BY ts DESC LIMIT ? OFFSET ?`
	rows, err := s.read.Query(sqlStr, append(args, q.Size, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]model.Event, 0, q.Size)
	for rows.Next() {
		var e model.Event
		if err := rows.Scan(&e.ID, &e.TS, &e.Site, &e.ClientIP, &e.Method, &e.Host, &e.Path, &e.Query,
			&e.Status, &e.Action, &e.Score, &e.Categories, &e.RuleIDs, &e.UA, &e.Referer,
			&e.ReqID, &e.LatencyUS, &e.Upstream, &e.Country, &e.Bytes); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// buildEventWhere 把查询条件拼装成 WHERE 子句与对应的绑定参数。
// 参数 q：过滤条件，零值字段不参与过滤；各条件之间为 AND 关系。
// 返回 (string, []any)：形如 " WHERE a = ? AND b LIKE ?" 的子句（无条件时为空串）与按序参数列表。
func buildEventWhere(q EventQuery) (string, []any) {
	var conds []string
	var args []any
	if q.Site != "" {
		conds = append(conds, "site = ?")
		args = append(args, q.Site)
	}
	if q.Action != "" {
		conds = append(conds, "action = ?")
		args = append(args, q.Action)
	}
	if q.Category != "" {
		conds = append(conds, "categories LIKE ?")
		args = append(args, "%\""+q.Category+"\"%")
	}
	if q.IP != "" {
		conds = append(conds, "client_ip = ?")
		args = append(args, q.IP)
	}
	if q.Keyword != "" {
		conds = append(conds, "(path LIKE ? OR ua LIKE ? OR query LIKE ?)")
		kw := "%" + q.Keyword + "%"
		args = append(args, kw, kw, kw)
	}
	if q.From > 0 {
		conds = append(conds, "ts >= ?")
		args = append(args, q.From)
	}
	if q.To > 0 {
		conds = append(conds, "ts <= ?")
		args = append(args, q.To)
	}
	if q.RuleHitOnly {
		conds = append(conds, "rule_ids IS NOT NULL AND rule_ids NOT IN ('', '[]')")
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// EventByID 按主键查询单条事件详情。
// 参数 id：事件主键。
// 返回 (*model.Event, error)：事件；不存在时返回 (nil, nil)；查询失败时返回错误。
func (s *Store) EventByID(id uint) (*model.Event, error) {
	row := s.read.QueryRow(`SELECT id, ts, site, client_ip, method, host, path, query, status, action, score,
 categories, rule_ids, ua, referer, req_id, latency_us, upstream, country, bytes FROM events WHERE id = ?`, id)
	var e model.Event
	if err := row.Scan(&e.ID, &e.TS, &e.Site, &e.ClientIP, &e.Method, &e.Host, &e.Path, &e.Query,
		&e.Status, &e.Action, &e.Score, &e.Categories, &e.RuleIDs, &e.UA, &e.Referer,
		&e.ReqID, &e.LatencyUS, &e.Upstream, &e.Country, &e.Bytes); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &e, nil
}

// Overview 仪表盘汇总指标。
type Overview struct {
	Total      int64 `json:"total"`      // 事件总数
	Passed     int64 `json:"passed"`     // 放行数（action=pass）
	Blocked    int64 `json:"blocked"`    // 拦截数（action=block）
	Challenged int64 `json:"challenged"` // 质询数（action=challenge）
	Status5xx  int64 `json:"status5xx"`  // 响应状态码 >=500 的事件数
	QPS        int64 `json:"qps"`        // 平均每秒请求数，由总数除以时间跨度折算
}

// Overview 按时间范围（可选限定站点）汇总事件指标。
// 参数 site：站点名，空串表示全部站点；from/to：毫秒时间戳闭区间。
// 返回 (Overview, error)：汇总结果（含折算后的 QPS）；查询失败时返回错误。
func (s *Store) Overview(site string, from, to int64) (Overview, error) {
	var o Overview
	where := " WHERE ts >= ? AND ts <= ?"
	args := []any{from, to}
	if site != "" {
		where += " AND site = ?"
		args = append(args, site)
	}
	q := `SELECT COUNT(1),
 COALESCE(SUM(CASE WHEN action = 'pass' THEN 1 ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN action = 'block' THEN 1 ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN action = 'challenge' THEN 1 ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN status >= 500 THEN 1 ELSE 0 END),0)
 FROM events` + where
	if err := s.read.QueryRow(q, args...).Scan(&o.Total, &o.Passed, &o.Blocked, &o.Challenged, &o.Status5xx); err != nil {
		return o, err
	}
	span := float64(to-from) / 1000.0
	if span > 0 {
		o.QPS = int64(float64(o.Total) / span)
	}
	return o, nil
}

// Series 时间序列上的一个桶。
type Series struct {
	TS      int64 `json:"ts"`         // 桶起始时间（毫秒时间戳）
	Total   int64 `json:"total"`      // 桶内事件总数
	Blocked int64 `json:"blocked"`    // 桶内拦截数
	Chal    int64 `json:"challenged"` // 桶内质询数
}

// Timeseries 按固定时间桶聚合事件数，供趋势图使用。
// 参数 site：站点名，空串表示全部；from/to：毫秒时间戳闭区间；bucketSec：桶宽秒数（<=0 时取 60）。
// 返回 ([]Series, error)：按桶起始时间升序排列的序列；查询失败时返回错误。
func (s *Store) Timeseries(site string, from, to int64, bucketSec int) ([]Series, error) {
	if bucketSec <= 0 {
		bucketSec = 60
	}
	where := " WHERE ts >= ? AND ts <= ?"
	args := []any{from, to}
	if site != "" {
		where += " AND site = ?"
		args = append(args, site)
	}
	// 整除分桶：ts 除以桶宽毫秒数再乘回，得到桶起点的毫秒时间戳
	q := fmt.Sprintf(`SELECT (ts / %d) * %d AS bucket, COUNT(1),
 COALESCE(SUM(CASE WHEN action='block' THEN 1 ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN action='challenge' THEN 1 ELSE 0 END),0)
 FROM events%s GROUP BY bucket ORDER BY bucket ASC`, bucketSec*1000, bucketSec*1000, where)
	rows, err := s.read.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Series, 0, 64)
	for rows.Next() {
		var p Series
		if err := rows.Scan(&p.TS, &p.Total, &p.Blocked, &p.Chal); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// TopItem TopN 统计中的一项。
type TopItem struct {
	Key   string `json:"key"`   // 维度取值（IP、路径或 rule_ids 字符串）
	Count int64  `json:"count"` // 出现次数
}

// Top 按指定维度统计 TopN。
// 参数 field：维度，"ip"（客户端 IP，默认）/ "path"（被拦截的路径）/ "category"（被拦截事件的 rule_ids 串）；
// site：站点名，空串表示全部；from/to：毫秒时间戳闭区间；limit：返回条数（<=0 或 >100 时取 10）。
// 返回 ([]TopItem, error)：按次数降序的结果；查询失败时返回错误。
func (s *Store) Top(field, site string, from, to int64, limit int) ([]TopItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	where := " WHERE ts >= ? AND ts <= ?"
	args := []any{from, to}
	if site != "" {
		where += " AND site = ?"
		args = append(args, site)
	}

	var q string
	switch field {
	case "path":
		q = "SELECT path AS k, COUNT(1) AS c FROM events" + where + " AND action='block' GROUP BY path ORDER BY c DESC LIMIT ?"
	case "category":
		// SQLite 无 JSON 数组展开，用规则命中表更准；这里退化为按 rule_ids 的字符串分组
		q = "SELECT rule_ids AS k, COUNT(1) AS c FROM events" + where + " AND action='block' GROUP BY rule_ids ORDER BY c DESC LIMIT ?"
	default:
		q = "SELECT client_ip AS k, COUNT(1) AS c FROM events" + where + " AND client_ip <> '' GROUP BY client_ip ORDER BY c DESC LIMIT ?"
	}
	rows, err := s.read.Query(q, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TopItem, 0, limit)
	for rows.Next() {
		var it TopItem
		if err := rows.Scan(&it.Key, &it.Count); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// CategoryStat 统计被拦截事件命中的攻击类别分布。
// 实现：取出符合条件的 categories JSON 数组逐行解析，在内存中按类别计数，
// 单条事件内的重复类别只计一次。
// 参数 site：站点名，空串表示全部；from/to：毫秒时间戳闭区间。
// 返回 ([]TopItem, error)：按次数降序的类别统计；查询失败时返回错误。
func (s *Store) CategoryStat(site string, from, to int64) ([]TopItem, error) {
	where := " WHERE ts >= ? AND ts <= ? AND action='block'"
	args := []any{from, to}
	if site != "" {
		where += " AND site = ?"
		args = append(args, site)
	}
	rows, err := s.read.Query("SELECT categories FROM events"+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int64{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var arr []string
		if json.Unmarshal([]byte(raw), &arr) != nil {
			continue
		}
		seen := map[string]struct{}{}
		for _, c := range arr {
			if _, ok := seen[c]; ok {
				continue
			}
			seen[c] = struct{}{}
			counts[c]++
		}
	}
	out := make([]TopItem, 0, len(counts))
	for k, v := range counts {
		out = append(out, TopItem{Key: k, Count: v})
	}
	sortTop(out)
	return out, rows.Err()
}

// sortTop 按次数降序对统计项原地排序（插入排序，适用于小数据量）。
// 参数 items：待排序切片，会被就地修改。
func sortTop(items []TopItem) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].Count > items[j-1].Count; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

// Audit 写一条审计日志，失败时静默忽略，不影响业务主流程。
// 参数 a：审计记录；TS 为零值时自动补当前毫秒时间戳，Result 为空时补 "ok"。
func (s *Store) Audit(a model.AuditLog) {
	if a.TS == 0 {
		a.TS = NowMS()
	}
	if a.Result == "" {
		a.Result = "ok"
	}
	_ = s.gorm.Create(&a).Error
}

// PurgeBefore 删除指定时间之前的过期事件。
// 参数 ts：毫秒时间戳，删除所有 ts 早于该值的事件。
// 返回 (int64, error)：累计删除条数；中途出错时返回已删除数量与错误。
func (s *Store) PurgeBefore(ts int64) (int64, error) {
	var total int64
	for {
		// 每轮 DELETE 子查询最多圈定 5000 行，循环到删空为止，避免单条长事务长时间锁写
		res, err := s.write.Exec(`DELETE FROM events WHERE id IN (SELECT id FROM events WHERE ts < ? LIMIT 5000)`, ts)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
		if n == 0 {
			break
		}
	}
	return total, nil
}

// ArchiveEvents 按时间升序导出指定时间之前的过期事件，供调用方归档（如压缩为 JSONL）。
// 参数 ctx：查询取消信号；ts：毫秒时间戳，导出 ts 早于该值的事件；limit：最多导出条数（<=0 时取 5000）。
// 返回 ([]model.Event, error)：导出的事件列表；查询失败时返回错误。
func (s *Store) ArchiveEvents(ctx context.Context, ts int64, limit int) ([]model.Event, error) {
	if limit <= 0 {
		limit = 5000
	}
	rows, err := s.read.QueryContext(ctx, `SELECT id, ts, site, client_ip, method, host, path, query, status,
 action, score, categories, rule_ids, ua, referer, req_id, latency_us, upstream, country, bytes
 FROM events WHERE ts < ? ORDER BY ts ASC LIMIT ?`, ts, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Event
	for rows.Next() {
		var e model.Event
		if err := rows.Scan(&e.ID, &e.TS, &e.Site, &e.ClientIP, &e.Method, &e.Host, &e.Path, &e.Query,
			&e.Status, &e.Action, &e.Score, &e.Categories, &e.RuleIDs, &e.UA, &e.Referer,
			&e.ReqID, &e.LatencyUS, &e.Upstream, &e.Country, &e.Bytes); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
