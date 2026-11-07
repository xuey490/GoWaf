package tasks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gowaf/internal/store"
)

// newTestRunner 构造用于测试的 Runner。
// 参数 t：测试对象；在临时目录中创建 SQLite 库与备份目录，测试结束后自动关闭库并清理目录。
// 返回：已完成内置任务播种的 *Runner；任一步骤失败时直接终止测试。
func newTestRunner(t *testing.T) *Runner {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("打开库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r := New(st, filepath.Join(dir, "backups"))
	if err := r.EnsureSeeded(); err != nil {
		t.Fatalf("种子失败: %v", err)
	}
	return r
}

// TestEnsureSeeded 验证首次播种后任务列表包含且仅包含两个内置任务
// （clean_events 与 backup_rules），且均可按名称检索到。
func TestEnsureSeeded(t *testing.T) {
	r := newTestRunner(t)
	list, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("期望 2 个内置任务，得到 %d", len(list))
	}
	names := map[string]bool{}
	for _, it := range list {
		names[it["name"].(string)] = true
	}
	if !names[TaskCleanEvents] || !names[TaskBackupRules] {
		t.Fatalf("缺少内置任务: %v", names)
	}
}

// TestUpdateValidation 验证 Update 的参数校验：未知任务、enabled 取 2、
// intervalH=0、非数字参数、retainDays=0 均应报错；enabled=0/1 与 retainDays=7 应合法，
// 且合法更新后的值必须真正写进 tasks 表。
func TestUpdateValidation(t *testing.T) {
	r := newTestRunner(t)

	if err := r.Update("unknown", nil, nil, nil); err == nil {
		t.Fatal("未知任务应报错")
	}
	bad := 2
	if err := r.Update(TaskCleanEvents, &bad, nil, nil); err == nil {
		t.Fatal("enabled=2 应报错")
	}
	// enabled=0（停用）是合法值
	zero := 0
	if err := r.Update(TaskCleanEvents, &zero, nil, nil); err != nil {
		t.Fatalf("enabled=0 应合法: %v", err)
	}
	one := 1
	if err := r.Update(TaskCleanEvents, &one, nil, nil); err != nil {
		t.Fatalf("enabled=1 应合法: %v", err)
	}
	h := 0
	if err := r.Update(TaskCleanEvents, nil, &h, nil); err == nil {
		t.Fatal("intervalH=0 应报错")
	}
	if err := r.Update(TaskCleanEvents, nil, nil, map[string]any{"retainDays": "x"}); err == nil {
		t.Fatal("非数字参数应报错")
	}
	if err := r.Update(TaskCleanEvents, nil, nil, map[string]any{"retainDays": float64(0)}); err == nil {
		t.Fatal("retainDays=0 应报错")
	}
	ok := 1
	if err := r.Update(TaskCleanEvents, &ok, nil, map[string]any{"retainDays": float64(7)}); err != nil {
		t.Fatalf("合法更新不应报错: %v", err)
	}
	// 验证真的写进去了
	row, _ := r.get(TaskCleanEvents)
	if row.Enabled != 1 || row.IntervalH != 6 {
		t.Fatalf("更新未生效: %+v", row)
	}
	var params map[string]any
	_ = json.Unmarshal([]byte(row.Params), &params)
	if v, _ := params["retainDays"].(float64); v != 7 {
		t.Fatalf("retainDays 未更新: %v", params)
	}
}

// TestCleanEventsRun 验证 clean_events 任务的清理行为：
// 插入一条 40 天前的事件，按保留 30 天执行后应清理恰好 1 条；再次执行应提示无过期事件。
func TestCleanEventsRun(t *testing.T) {
	r := newTestRunner(t)
	// 灌一条 40 天前的事件
	old := time.Now().AddDate(0, 0, -40).UnixMilli()
	if err := r.st.Gorm().Exec(
		`INSERT INTO events (ts, site, client_ip, method, path, action, status) VALUES (?,?,?,?,?,?,?)`,
		old, "t", "1.2.3.4", "GET", "/", "block", 403,
	).Error; err != nil {
		t.Fatalf("灌数据失败: %v", err)
	}
	// 保留 30 天 → 应清掉 1 条
	if err := r.Update(TaskCleanEvents, nil, nil, map[string]any{"retainDays": float64(30)}); err != nil {
		t.Fatal(err)
	}
	res, err := r.Run(TaskCleanEvents)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res, "1 条") {
		t.Fatalf("期望清理 1 条，得到: %s", res)
	}
	// 再跑一次应为 0
	res, _ = r.Run(TaskCleanEvents)
	if !strings.Contains(res, "无过期") {
		t.Fatalf("第二次应为空: %s", res)
	}
}

// TestBackupRulesRun 验证 backup_rules 任务：生成包含规则的 JSON 快照；
// keep=2 时连续执行 4 次后目录应只保留最新 2 份，且快照内容可解析、规则数大于 0。
func TestBackupRulesRun(t *testing.T) {
	r := newTestRunner(t)
	// 测试库没有业务种子，插一条规则保证快照非空
	if err := r.st.Gorm().Exec(
		`INSERT INTO rules (id, name, category, score, targets, keywords, pattern, action, enabled, builtin, priority, updated_at)
		 VALUES ('t1','测试规则','xss',40,'path','[]','(?i)bad','block',1,0,100,0)`,
	).Error; err != nil {
		t.Fatalf("插规则失败: %v", err)
	}
	if err := r.Update(TaskBackupRules, nil, nil, map[string]any{"keep": float64(2)}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ { // 连续执行 4 次，超过 keep=2 的保留份数
		res, err := r.Run(TaskBackupRules)
		if err != nil {
			t.Fatal(err)
		}
		if i < 3 && !strings.Contains(res, "已备份") {
			t.Fatalf("期望备份成功: %s", res)
		}
	}
	entries, _ := os.ReadDir(r.backupDir)
	if len(entries) != 2 {
		t.Fatalf("期望滚动保留 2 份，实际 %d 份", len(entries))
	}
	// 校验快照内容可解析且含规则
	b, err := os.ReadFile(filepath.Join(r.backupDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	var snap struct {
		Count int             `json:"count"`
		Rules json.RawMessage `json:"rules"`
	}
	if err := json.Unmarshal(b, &snap); err != nil {
		t.Fatalf("快照 JSON 非法: %v", err)
	}
	if snap.Count <= 0 {
		t.Fatalf("期望规则数 > 0，得到 %d", snap.Count)
	}
}

// TestRunDueRespectsInterval 验证调度尊重执行间隔：
// 刚运行过的任务在间隔期内不会被 runDue 重复执行（total_runs 保持不变）。
func TestRunDueRespectsInterval(t *testing.T) {
	r := newTestRunner(t)
	// 刚跑过 → 6 小时内不应再跑
	r.st.Gorm().Exec(`UPDATE tasks SET last_run_ms = ?, last_result = 'x' WHERE name = ?`, store.NowMS(), TaskCleanEvents)
	before := int64(0)
	r.st.Gorm().Raw(`SELECT total_runs FROM tasks WHERE name = ?`, TaskCleanEvents).Scan(&before)
	r.runDue()
	after := int64(0)
	r.st.Gorm().Raw(`SELECT total_runs FROM tasks WHERE name = ?`, TaskCleanEvents).Scan(&after)
	if after != before {
		t.Fatalf("间隔内不应重复执行: before=%d after=%d", before, after)
	}
}
