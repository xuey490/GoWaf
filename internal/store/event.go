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
	store   *Store
	ch      chan model.Event
	dropped atomic.Int64
	written atomic.Int64
	done    chan struct{}
}

// NewEventWriter 创建写入器。
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

// Enqueue 非阻塞投递；队列满直接丢弃并计数（fail-open，不影响转发）。
func (w *EventWriter) Enqueue(e model.Event) {
	select {
	case w.ch <- e:
	default:
		w.dropped.Add(1)
	}
}

// Dropped 返回丢弃条数。
func (w *EventWriter) Dropped() int64 { return w.dropped.Load() }

// Written 返回已写条数。
func (w *EventWriter) Written() int64 { return w.written.Load() }

// Close 关闭队列并等待落盘。
func (w *EventWriter) Close() {
	close(w.ch)
	<-w.done
}

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

const eventInsertSQL = `INSERT INTO events
(ts, site, client_ip, method, host, path, query, status, action, score,
 categories, rule_ids, ua, referer, req_id, latency_us, upstream, country, bytes)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func defaultJSON(s string) string {
	if s == "" {
		return "[]"
	}
	return s
}

// EventQuery 事件查询条件。
type EventQuery struct {
	Site     string
	Action   string
	Category string
	IP       string
	Keyword  string
	From     int64
	To       int64
	Current  int
	Size     int
}

// QueryEvents 分页查询事件。
func (s *Store) QueryEvents(q EventQuery) ([]model.Event, int64, error) {
	where, args := buildEventWhere(q)
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
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// EventByID 查单条。
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

// Overview 汇总指标。
type Overview struct {
	Total      int64 `json:"total"`
	Passed     int64 `json:"passed"`
	Blocked    int64 `json:"blocked"`
	Challenged int64 `json:"challenged"`
	Status5xx  int64 `json:"status5xx"`
	QPS        int64 `json:"qps"`
}

// Overview 按时间范围汇总。
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

// Series 时间序列点。
type Series struct {
	TS      int64 `json:"ts"`
	Total   int64 `json:"total"`
	Blocked int64 `json:"blocked"`
	Chal    int64 `json:"challenged"`
}

// Timeseries 按桶聚合。
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

// TopItem TopN 结果。
type TopItem struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

// Top 支持 ip / path / category 三个维度。
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

// CategoryStat 按类别统计（解析 categories JSON）。
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

func sortTop(items []TopItem) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].Count > items[j-1].Count; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

// Audit 写一条审计日志。
func (s *Store) Audit(a model.AuditLog) {
	if a.TS == 0 {
		a.TS = NowMS()
	}
	if a.Result == "" {
		a.Result = "ok"
	}
	_ = s.gorm.Create(&a).Error
}

// PurgeBefore 删除过期事件（分批，避免长事务）。
func (s *Store) PurgeBefore(ts int64) (int64, error) {
	var total int64
	for {
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

// ArchiveEvents 把过期事件导出为 JSONL 文本（调用方负责压缩落盘）。
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
