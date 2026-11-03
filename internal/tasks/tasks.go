// Package tasks 实现计划任务中心。
//
// 内置两个任务：
//   - clean_events：按保留天数清理过期攻击事件（接管原 hourly 里的固定清理，
//     保留天数与执行间隔可在面板调整）；
//   - backup_rules：定期把检测规则导出为 JSON 快照，滚动保留 N 份。
//
// 设计要点：
//  1. 调度用"到期检查"而非 cron 表达式：interval_h 粒度足够，避免引依赖；
//  2. 每次运行的结果（条数/错误/耗时）写回 tasks 行，面板直接展示；
//  3. Run 用互斥锁串行化，手动触发与定时触发不会并发跑同一任务。
package tasks

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

// 内置任务名。
const (
	TaskCleanEvents = "clean_events"
	TaskBackupRules = "backup_rules"
)

// defTask 内置任务定义。
type defTask struct {
	intervalH  int
	paramsJSON string
}

var builtinTasks = map[string]defTask{
	TaskCleanEvents: {intervalH: 6, paramsJSON: `{"retainDays":30}`},
	TaskBackupRules: {intervalH: 24, paramsJSON: `{"keep":10}`},
}

// validParams 校验并规范参数，返回错误信息（空串=通过）。
func validParams(name string, params map[string]float64) string {
	switch name {
	case TaskCleanEvents:
		v, ok := params["retainDays"]
		if !ok {
			return "clean_events 需要 retainDays 参数"
		}
		if v < 1 || v > 3650 {
			return "retainDays 必须在 1~3650 之间"
		}
	case TaskBackupRules:
		v, ok := params["keep"]
		if !ok {
			return "backup_rules 需要 keep 参数"
		}
		if v < 1 || v > 100 {
			return "keep 必须在 1~100 之间"
		}
	default:
		return "未知任务: " + name
	}
	return ""
}

// Runner 计划任务运行器。
type Runner struct {
	st        *store.Store
	backupDir string

	mu     sync.Mutex // 串行化任务执行
	stopCh chan struct{}
	once   sync.Once
}

// New 创建运行器。backupDir 为规则快照输出目录。
func New(st *store.Store, backupDir string) *Runner {
	return &Runner{st: st, backupDir: backupDir, stopCh: make(chan struct{})}
}

// Stop 停止调度循环。
func (r *Runner) Stop() { r.once.Do(func() { close(r.stopCh) }) }

// EnsureSeeded 首次启动补齐内置任务行（INSERT OR IGNORE，不覆盖用户改动）。
func (r *Runner) EnsureSeeded() error {
	now := store.NowMS()
	for name, def := range builtinTasks {
		res := r.st.Gorm().Exec(
			`INSERT OR IGNORE INTO tasks (name, enabled, interval_h, params, last_run_ms, last_result, last_duration_ms, total_runs, updated_at)
			 VALUES (?, 1, ?, ?, 0, '', 0, 0, ?)`,
			name, def.intervalH, def.paramsJSON, now)
		if res.Error != nil {
			return fmt.Errorf("种子任务 %s: %w", name, res.Error)
		}
	}
	return nil
}

// Loop 调度主循环。每 30 秒检查一次到期任务。
func (r *Runner) Loop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.runDue()
		}
	}
}

// runDue 执行所有到期的启用任务。
func (r *Runner) runDue() {
	var rows []model.Task
	if err := r.st.Gorm().Where("enabled = 1").Find(&rows).Error; err != nil {
		log.Printf("[tasks] 读取任务失败: %v", err)
		return
	}
	now := store.NowMS()
	for _, t := range rows {
		if _, ok := builtinTasks[t.Name]; !ok {
			continue
		}
		if t.LastRunMs > 0 && now-t.LastRunMs < int64(t.IntervalH)*3600*1000 {
			continue
		}
		if _, err := r.Run(t.Name); err != nil {
			log.Printf("[tasks] 任务 %s 执行失败: %v", t.Name, err)
		}
	}
}

// List 返回任务列表（附下次运行时间估算）。
func (r *Runner) List() ([]ginH, error) {
	var rows []model.Task
	if err := r.st.Gorm().Order("name").Find(&rows).Error; err != nil {
		return nil, err
	}
	now := store.NowMS()
	out := make([]ginH, 0, len(rows))
	for _, t := range rows {
		var params map[string]any
		_ = json.Unmarshal([]byte(t.Params), &params)
		if params == nil {
			params = map[string]any{}
		}
		var nextRunMs int64
		if t.Enabled == 1 {
			intervalMS := int64(t.IntervalH) * 3600 * 1000
			if t.LastRunMs <= 0 {
				nextRunMs = now // 尚未跑过，下个调度周期就执行
			} else {
				nextRunMs = t.LastRunMs + intervalMS
			}
		}
		out = append(out, ginH{
			"name":           t.Name,
			"enabled":        t.Enabled,
			"intervalH":      t.IntervalH,
			"params":         params,
			"lastRunMs":      t.LastRunMs,
			"lastResult":     t.LastResult,
			"lastDurationMs": t.LastDurationMs,
			"totalRuns":      t.TotalRuns,
			"nextRunMs":      nextRunMs,
			"updatedAt":      t.UpdatedAt,
		})
	}
	return out, nil
}

// Update 修改任务配置。enabled/intervalH/params 均为可选。
func (r *Runner) Update(name string, enabled *int, intervalH *int, params map[string]any) error {
	if _, ok := builtinTasks[name]; !ok {
		return fmt.Errorf("未知任务: %s", name)
	}
	row, err := r.get(name)
	if err != nil {
		return err
	}
	if enabled != nil {
		if *enabled != 0 && *enabled != 1 {
			return fmt.Errorf("enabled 只能是 0 或 1")
		}
		row.Enabled = *enabled
	}
	if intervalH != nil {
		if *intervalH < 1 || *intervalH > 720 {
			return fmt.Errorf("intervalH 必须在 1~720 小时之间")
		}
		row.IntervalH = *intervalH
	}
	if params != nil {
		nums := map[string]float64{}
		for k, v := range params {
			f, ok := v.(float64)
			if !ok {
				return fmt.Errorf("参数 %s 必须是数字", k)
			}
			nums[k] = f
		}
		if msg := validParams(name, nums); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		b, _ := json.Marshal(params)
		row.Params = string(b)
	}
	row.UpdatedAt = store.NowMS()
	return r.st.Gorm().Model(&model.Task{}).
		Where("name = ?", name).
		Updates(map[string]any{
			"enabled": row.Enabled, "interval_h": row.IntervalH,
			"params": row.Params, "updated_at": row.UpdatedAt,
		}).Error
}

// Run 立即执行任务（手动触发与定时触发共用，锁保证不并发）。
func (r *Runner) Run(name string) (result string, err error) {
	if _, ok := builtinTasks[name]; !ok {
		return "", fmt.Errorf("未知任务: %s", name)
	}
	row, err := r.get(name)
	if err != nil {
		return "", err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	start := time.Now()
	switch name {
	case TaskCleanEvents:
		result, err = r.runCleanEvents(row.Params)
	case TaskBackupRules:
		result, err = r.runBackupRules(row.Params)
	}

	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		result = "失败: " + err.Error()
	}
	// 执行结果落库，失败也记录（面板可见）
	_ = r.st.Gorm().Model(&model.Task{}).Where("name = ?", name).
		Updates(map[string]any{
			"last_run_ms":      store.NowMS(),
			"last_result":      truncate(result, 240),
			"last_duration_ms": elapsed,
			"total_runs":       row.TotalRuns + 1,
		}).Error
	return result, err
}

func (r *Runner) get(name string) (*model.Task, error) {
	var row model.Task
	if err := r.st.Gorm().Where("name = ?", name).First(&row).Error; err != nil {
		return nil, fmt.Errorf("任务 %s 不存在: %w", name, err)
	}
	return &row, nil
}

// runCleanEvents 清理超过保留期的攻击事件。
func (r *Runner) runCleanEvents(paramsJSON string) (string, error) {
	var p struct {
		RetainDays int `json:"retainDays"`
	}
	if err := json.Unmarshal([]byte(paramsJSON), &p); err != nil || p.RetainDays <= 0 {
		p.RetainDays = 30
	}
	cutoff := time.Now().AddDate(0, 0, -p.RetainDays).UnixMilli()
	n, err := r.st.PurgeBefore(cutoff)
	if err != nil {
		return "", err
	}
	if n == 0 {
		return fmt.Sprintf("无过期事件（保留 %d 天）", p.RetainDays), nil
	}
	return fmt.Sprintf("已清理过期事件 %d 条（保留 %d 天）", n, p.RetainDays), nil
}

// runBackupRules 导出全部检测规则为 JSON 快照，滚动保留 keep 份。
func (r *Runner) runBackupRules(paramsJSON string) (string, error) {
	var p struct {
		Keep int `json:"keep"`
	}
	if err := json.Unmarshal([]byte(paramsJSON), &p); err != nil || p.Keep <= 0 {
		p.Keep = 10
	}

	var rules []map[string]any
	if err := r.st.Gorm().Table("rules").Find(&rules).Error; err != nil {
		return "", err
	}
	payload, err := json.MarshalIndent(map[string]any{
		"exportedAt": store.NowMS(),
		"count":      len(rules),
		"rules":      rules,
	}, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(r.backupDir, 0o750); err != nil {
		return "", err
	}
	// 文件名带毫秒：手动连续触发时避免同秒覆盖
	file := filepath.Join(r.backupDir, "rules-"+time.Now().Format("20060102-150405.000")+".json")
	if err := os.WriteFile(file, payload, 0o600); err != nil {
		return "", err
	}

	removed := pruneBackups(r.backupDir, "rules-", p.Keep)
	return fmt.Sprintf("已备份 %d 条规则 → %s（清理旧快照 %d 份）", len(rules), filepath.Base(file), removed), nil
}

// pruneBackups 按文件名（即时间）降序保留最新 keep 份。
func pruneBackups(dir, prefix string, keep int) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) && strings.HasSuffix(e.Name(), ".json") {
			files = append(files, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files))) // 文件名含时间戳，字典序即时间序
	removed := 0
	for _, f := range files[min(keep, len(files)):] {
		if os.Remove(filepath.Join(dir, f)) == nil {
			removed++
		}
	}
	return removed
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// ginH 避免处理器包依赖，这里用类型别名。
type ginH = map[string]any
