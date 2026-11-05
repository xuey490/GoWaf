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

// 内置任务名（tasks 表主键，同时作为任务分发依据）。
const (
	TaskCleanEvents = "clean_events" // 清理过期攻击事件
	TaskBackupRules = "backup_rules" // 备份检测规则快照
)

// defTask 内置任务定义。
type defTask struct {
	intervalH  int    // 默认执行间隔（小时）
	paramsJSON string // 默认参数（JSON 字符串）
}

// builtinTasks 内置任务及其默认配置：首次启动由 EnsureSeeded 播种进 tasks 表，
// 之后以表中行为准（用户可调整间隔与参数）。
var builtinTasks = map[string]defTask{
	TaskCleanEvents: {intervalH: 6, paramsJSON: `{"retainDays":30}`}, // 默认每 6 小时清理，保留 30 天
	TaskBackupRules: {intervalH: 24, paramsJSON: `{"keep":10}`},      // 默认每 24 小时备份，保留 10 份
}

// validParams 校验任务参数的合法性与取值范围。
// 参数 name：任务名；参数 params：待校验的参数键值对（JSON 反序列化后的数值）。
// 返回：空串表示校验通过；否则返回可直接展示给用户的错误信息（含未知任务）。
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

// Runner 计划任务运行器：调度、执行内置任务，并把每次运行结果写回 tasks 表。
type Runner struct {
	st        *store.Store // 存储句柄
	backupDir string       // 规则快照输出目录

	mu     sync.Mutex    // 串行化任务执行：手动触发与定时触发互斥
	stopCh chan struct{} // 停止信号：关闭后 Loop 退出
	once   sync.Once     // 保证 stopCh 只被关闭一次（Stop 可安全重复调用）
}

// New 创建运行器。
// 参数 st：存储句柄；参数 backupDir：规则快照输出目录。
// 返回：初始化完成的 *Runner。
func New(st *store.Store, backupDir string) *Runner {
	return &Runner{st: st, backupDir: backupDir, stopCh: make(chan struct{})}
}

// Stop 停止调度循环。
// 参数：无。
// 返回：无。通过关闭 stopCh 通知 Loop 退出；sync.Once 保证重复调用安全。
func (r *Runner) Stop() { r.once.Do(func() { close(r.stopCh) }) }

// EnsureSeeded 首次启动补齐内置任务行。
// 参数：无。使用 INSERT OR IGNORE 播种，已存在的任务行（含用户改动）不会被覆盖。
// 返回：播种失败时返回带任务名的包装错误；全部成功返回 nil。
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

// Loop 调度主循环：每 30 秒轮询一次，执行所有到期任务，直到 Stop 被调用。
// 参数：无。
// 返回：无（stopCh 关闭后返回）。
func (r *Runner) Loop() {
	ticker := time.NewTicker(30 * time.Second) // 轮询周期：30 秒检查一次到期任务
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

// runDue 执行所有已启用且到期的内置任务。
// 参数：无。逐行检查 enabled=1 的任务：不在内置白名单的跳过；
// 未到执行间隔（now - last_run_ms < interval_h 小时）的跳过。
// 返回：无。单个任务失败只记日志，不中断后续任务。
func (r *Runner) runDue() {
	var rows []model.Task
	if err := r.st.Gorm().Where("enabled = 1").Find(&rows).Error; err != nil {
		log.Printf("[tasks] 读取任务失败: %v", err)
		return
	}
	now := store.NowMS()
	for _, t := range rows {
		if _, ok := builtinTasks[t.Name]; !ok {
			continue // 白名单之外的任务不自动执行
		}
		if t.LastRunMs > 0 && now-t.LastRunMs < int64(t.IntervalH)*3600*1000 {
			continue // 未到执行间隔
		}
		if _, err := r.Run(t.Name); err != nil {
			log.Printf("[tasks] 任务 %s 执行失败: %v", t.Name, err)
		}
	}
}

// List 返回任务列表（按任务名排序，附下次运行时间估算）。
// 参数：无。
// 返回：任务信息切片（params 已反序列化为对象，启用中的任务带 nextRunMs 估算）与错误；
// 查询失败时返回 (nil, err)。
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

// Update 修改任务配置，三个可变字段均为可选（传 nil 表示不修改该项）。
// 参数 name：任务名；参数 enabled：启停（0/1）；参数 intervalH：执行间隔（1~720 小时）；
// 参数 params：任务参数（各值必须是数字，并按任务分别校验范围）。
// 返回：任务不存在、校验失败或落库失败时返回错误；成功返回 nil。
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
		// JSON 反序列化后数字一律是 float64，这里统一转成数值再走校验
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

// Run 立即执行指定任务，手动触发与定时触发共用；互斥锁保证同一时刻只执行一个任务。
// 参数 name：任务名。
// 返回：result 为结果描述（失败时带错误信息），err 非 nil 表示执行失败；
// 无论成败都会把耗时、结果、累计次数写回 tasks 行（面板可见）。
func (r *Runner) Run(name string) (result string, err error) {
	if _, ok := builtinTasks[name]; !ok {
		return "", fmt.Errorf("未知任务: %s", name)
	}
	row, err := r.get(name)
	if err != nil {
		return "", err
	}

	r.mu.Lock() // 串行化：避免手动触发与定时触发并发执行同一任务
	defer r.mu.Unlock()

	start := time.Now()
	switch name {
	case TaskCleanEvents:
		result, err = r.runCleanEvents(row.Params)
	case TaskBackupRules:
		result, err = r.runBackupRules(row.Params)
	}

	elapsed := time.Since(start).Milliseconds() // 本次执行耗时（毫秒）
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

// get 按名称查询单个任务行。
// 参数 name：任务名。
// 返回：任务行指针；不存在或查询失败时返回 (nil, 包装错误)。
func (r *Runner) get(name string) (*model.Task, error) {
	var row model.Task
	if err := r.st.Gorm().Where("name = ?", name).First(&row).Error; err != nil {
		return nil, fmt.Errorf("任务 %s 不存在: %w", name, err)
	}
	return &row, nil
}

// runCleanEvents 清理超过保留期的攻击事件。
// 参数 paramsJSON：任务参数 JSON（字段 retainDays）；解析失败或非正数时回退默认 30 天。
// 返回：结果描述字符串（清理条数）；清理执行失败时返回错误。
func (r *Runner) runCleanEvents(paramsJSON string) (string, error) {
	var p struct {
		RetainDays int `json:"retainDays"`
	}
	if err := json.Unmarshal([]byte(paramsJSON), &p); err != nil || p.RetainDays <= 0 {
		p.RetainDays = 30 // 参数非法时回退默认保留 30 天
	}
	cutoff := time.Now().AddDate(0, 0, -p.RetainDays).UnixMilli() // 保留期截止时间（Unix 毫秒）
	n, err := r.st.PurgeBefore(cutoff)
	if err != nil {
		return "", err
	}
	if n == 0 {
		return fmt.Sprintf("无过期事件（保留 %d 天）", p.RetainDays), nil
	}
	return fmt.Sprintf("已清理过期事件 %d 条（保留 %d 天）", n, p.RetainDays), nil
}

// runBackupRules 导出全部检测规则为 JSON 快照，并滚动保留最新 keep 份。
// 参数 paramsJSON：任务参数 JSON（字段 keep）；解析失败或非正数时回退默认 10。
// 返回：结果描述字符串（规则数、文件名、清理份数）；查询规则、建目录或写文件失败时返回错误。
func (r *Runner) runBackupRules(paramsJSON string) (string, error) {
	var p struct {
		Keep int `json:"keep"`
	}
	if err := json.Unmarshal([]byte(paramsJSON), &p); err != nil || p.Keep <= 0 {
		p.Keep = 10 // 参数非法时回退默认保留 10 份
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

// pruneBackups 按文件名（含时间戳，字典序即时间序）降序排序，保留最新 keep 份，删除其余。
// 参数 dir：快照目录；参数 prefix：文件名前缀（如 "rules-"）；参数 keep：保留份数。
// 返回：实际删除的文件数；目录不存在或读取失败时返回 0。
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

// min 返回两个整数中的较小值。
// 参数 a、b：待比较的两个整数。
// 返回：较小者。
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// truncate 把字符串截断到最多 n 个字节。
// 参数 s：原始字符串；参数 n：最大保留字节数。
// 返回：不超过 n 字节的前缀（按字节截断，可能切断多字节字符，仅用于结果摘要展示）。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// ginH 任务信息响应对象的类型别名，避免 tasks 包直接依赖 gin 框架。
type ginH = map[string]any
