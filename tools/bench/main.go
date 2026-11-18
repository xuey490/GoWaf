// bench 是 GoWAF 的检测效果跑分器（P1-1 交付物）。
//
// 两种模式：
//
//	go run ./tools/bench -generate          # 生成样本库（testdata/bench，确定性输出）
//	go run ./tools/bench -dir testdata/bench -threshold 30 -report report.json
//
// 跑分口径（《docs/P1-样本库建设方案.md》）：
//   - 样本 = 原始 HTTP 请求文本（一文件一请求），可 http.ReadRequest 回放；
//   - 「检出」= 四引擎合并分数 ≥ 挑战阈值（30），或引擎给出 challenge/block/ban 动作；
//   - 「误报」= 良性样本得分 ≥ 挑战阈值；
//   - 引擎装配与线上一致：自研规则（builtin.yaml）+ CRS PL1 + libi + 语义通道，
//     多层解码变体（engine.decode 默认参数）同线上开启。
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gowaf/internal/detect"
	"gowaf/internal/store/model"
)

func main() {
	gen := flag.Bool("generate", false, "生成样本库到 -dir（确定性输出）")
	dir := flag.String("dir", "testdata/bench", "样本库目录")
	threshold := flag.Int("threshold", 30, "挑战阈值：合并分达到即记为检出")
	reportPath := flag.String("report", "", "输出 report.json 的路径（空则不写文件）")
	detail := flag.Int("detail", 800, "报告里保留的失败样本明细上限")
	probe := flag.String("probe", "", "只跑单个样本文件并打印全部命中（调误报用）")
	flag.Parse()

	if *probe != "" {
		probeFile(*probe)
		return
	}

	if *gen {
		if err := generateCorpus(*dir); err != nil {
			fmt.Fprintln(os.Stderr, "生成失败:", err)
			os.Exit(1)
		}
		return
	}

	rep, err := runBench(*dir, *threshold, *detail)
	if err != nil {
		fmt.Fprintln(os.Stderr, "跑分失败:", err)
		os.Exit(1)
	}
	printReport(rep)
	if *reportPath != "" {
		data, _ := json.MarshalIndent(rep, "", "  ")
		if err := os.WriteFile(*reportPath, data, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "写报告失败:", err)
			os.Exit(1)
		}
		fmt.Printf("报告已写入 %s\n", *reportPath)
	}
}

// benchEngines 跑分用的引擎组：与数据面 edge.go 相同的四引擎组合 + CRS 调度闸门。
type benchEngines struct {
	rules *detect.Engine
	crs   *detect.CRSEngine
	gov   *detect.CRSGov
	libi  *detect.LibiEngine
	sem   *detect.SemanticEngine
}

// newBenchEngines 装配引擎：规则从 builtin.yaml 加载（与线上种子同一份 YAML），
// 其余三个通道用默认分值，CRS 闸门用线上默认参数（prescreen 关、
// skip_static/body_text_only/safe_datauri 开）。任何一步失败都直接报错退出。
func newBenchEngines() (*benchEngines, error) {
	rules := detect.NewEngine(60, 30, "protect")
	rulesPath, err := findRulesYaml()
	if err != nil {
		return nil, err
	}
	rows, err := loadBuiltinRules(rulesPath)
	if err != nil {
		return nil, err
	}
	if err := rules.Reload(rows); err != nil {
		return nil, fmt.Errorf("加载自研规则: %w", err)
	}
	crs, err := detect.NewCRSEngine(detect.CRSOptions{Mode: "block", Paranoia: 1})
	if err != nil {
		return nil, fmt.Errorf("装配 CRS: %w", err)
	}
	return &benchEngines{
		rules: rules,
		crs:   crs,
		gov: detect.NewCRSGov(detect.CRSGovOptions{
			Prescreen:    false,
			SkipStatic:   true,
			BodyTextOnly: true,
			SafeDataURI:  true,
		}),
		libi: detect.NewLibiEngine(true, 0, 0),
		sem:  detect.NewSemanticEngine(true, nil),
	}, nil
}

// benchRule 是 rules/builtin.yaml 的本地镜像（与 store.builtinRule 字段一致）。
// 跑分器独立于 store 包（不建数据库），只做 YAML → model.Rule 的同构转换。
type benchRule struct {
	ID       string   `yaml:"id"`
	Name     string   `yaml:"name"`
	Category string   `yaml:"category"`
	Score    int      `yaml:"score"`
	Targets  []string `yaml:"targets"`
	Keywords []string `yaml:"keywords"`
	Pattern  string   `yaml:"pattern"`
	Action   string   `yaml:"action"`
	Priority int      `yaml:"priority"`
	Remark   string   `yaml:"remark"`
	Group    string   `yaml:"group"`
	Mode     string   `yaml:"mode"`
	Version  string   `yaml:"version"`
}

// findRulesYaml 在常见工作目录下定位 rules/builtin.yaml（跑分器可能在
// 仓库根或 tools/bench 目录里启动）。
func findRulesYaml() (string, error) {
	for _, p := range []string{"rules/builtin.yaml", "../rules/builtin.yaml", "../../rules/builtin.yaml"} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("找不到 rules/builtin.yaml（请在仓库内运行）")
}

// loadBuiltinRules 读 builtin.yaml 并转为模型规则行（缺省值兜底与 store.ImportBuiltinRules 一致）。
func loadBuiltinRules(path string) ([]model.Rule, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var list []benchRule
	if err := yamlUnmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("解析内置规则 %s: %w", path, err)
	}
	rows := make([]model.Rule, 0, len(list))
	for _, r := range list {
		targets := strings.Join(r.Targets, ",")
		if targets == "" {
			targets = "path,query,body"
		}
		kws, _ := json.Marshal(r.Keywords)
		action := r.Action
		if action == "" {
			action = "block"
		}
		group := r.Group
		if group == "" {
			group = r.Category
		}
		version := r.Version
		if version == "" {
			version = "2"
		}
		rows = append(rows, model.Rule{
			ID: r.ID, Name: r.Name, Category: r.Category, Score: r.Score,
			Targets: targets, Keywords: string(kws), Pattern: r.Pattern,
			Action: action, Enabled: 1, Builtin: 1,
			Priority: r.Priority, Remark: r.Remark,
			Source: "builtin", Vendor: "gowaf", Version: version, Group: group,
		})
	}
	return rows, nil
}

// catStat 逐类统计。
type catStat struct {
	Category string `json:"category"`
	Total    int    `json:"total"`
	Detected int    `json:"detected"`
	Rate     float64 `json:"rate"`
}

// benchReport 跑分报告（report.json 的结构，提交进仓库供复现对照）。
type benchReport struct {
	GeneratedAt string         `json:"generatedAt"`
	Threshold   int            `json:"threshold"`
	Attacks     int            `json:"attacks"`
	Detected    int            `json:"detected"`
	Recall      float64        `json:"recall"`
	Benign      int            `json:"benign"`
	FalsePos    int            `json:"falsePositives"`
	FPRate      float64        `json:"fpRate"`
	ElapsedMS   int64          `json:"elapsedMs"`
	ByCategory  []catStat      `json:"byCategory"`
	Failures    []benchFailure `json:"failures"`
	// FPExamples 误报样本明细（独立于攻击失败清单，cap 400）；FPRuleCounts
	// 是全部误报的规则命中聚合——调优误报从这里入手。
	FPExamples   []benchFailure  `json:"fpExamples,omitempty"`
	FPRuleCounts map[string]int  `json:"fpRuleCounts,omitempty"`
	FPCatCounts  map[string]int  `json:"fpFamilyCounts,omitempty"`
}

// benchFailure 一条未检出的攻击样本（回归清单：修判定式就从这里下手）。
type benchFailure struct {
	File     string   `json:"file"`
	Category string   `json:"category"`
	Score    int      `json:"score"`
	Action   string   `json:"action"`
	RuleIDs  []string `json:"ruleIds,omitempty"`
}

// runBench 遍历样本库逐条跑四引擎，产出报告。
func runBench(dir string, threshold, detailLimit int) (*benchReport, error) {
	eng, err := newBenchEngines()
	if err != nil {
		return nil, err
	}
	decodeCfg := detect.DecodeConfig{Enabled: true, MaxDepth: detect.DefaultDecodeDepth, MaxBytes: detect.DefaultDecodeBytes}

	attacks, err := listSamples(filepath.Join(dir, "attacks"))
	if err != nil {
		return nil, err
	}
	benign, err := listSamples(filepath.Join(dir, "benign"))
	if err != nil {
		return nil, err
	}

	rep := &benchReport{Threshold: threshold}
	byCat := map[string]*catStat{}
	start := time.Now()

	for _, f := range attacks {
		req, err := parseRawFile(f)
		if err != nil {
			continue // 坏样本跳过并计数
		}
		in := buildBenchInput(req)
		res := runAll(eng, &in, decodeCfg)
		total := res.score
		detected := total >= threshold || res.strongAction
		cat := categoryOf(f, "attacks")
		st := byCat[cat]
		if st == nil {
			st = &catStat{Category: cat}
			byCat[cat] = st
		}
		st.Total++
		if detected {
			st.Detected++
			rep.Detected++
		} else if len(rep.Failures) < detailLimit {
			rep.Failures = append(rep.Failures, benchFailure{
				File:     relPath(dir, f),
				Category: cat,
				Score:    total,
				Action:   res.action,
				RuleIDs:  res.ruleIDs,
			})
		}
		rep.Attacks++
	}
	for _, f := range benign {
		req, err := parseRawFile(f)
		if err != nil {
			continue
		}
		in := buildBenchInput(req)
		res := runAll(eng, &in, decodeCfg)
		rep.Benign++
		if res.score >= threshold || res.strongAction {
			rep.FalsePos++
			if rep.FPRuleCounts == nil {
				rep.FPRuleCounts = map[string]int{}
				rep.FPCatCounts = map[string]int{}
			}
			for _, rid := range res.ruleIDs {
				rep.FPRuleCounts[rid]++
			}
			fam := benignFamily(f)
			rep.FPCatCounts[fam]++
			if len(rep.FPExamples) < 400 {
				rep.FPExamples = append(rep.FPExamples, benchFailure{
					File:     relPath(dir, f),
					Category: "benign:" + fam,
					Score:    res.score,
					Action:   res.action,
					RuleIDs:  res.ruleIDs,
				})
			}
		}
	}
	rep.ElapsedMS = time.Since(start).Milliseconds()

	if rep.Attacks > 0 {
		rep.Recall = float64(rep.Detected) / float64(rep.Attacks)
	}
	if rep.Benign > 0 {
		rep.FPRate = float64(rep.FalsePos) / float64(rep.Benign)
	}
	for _, st := range byCat {
		if st.Total > 0 {
			st.Rate = float64(st.Detected) / float64(st.Total)
		}
		rep.ByCategory = append(rep.ByCategory, *st)
	}
	sort.Slice(rep.ByCategory, func(i, j int) bool { return rep.ByCategory[i].Category < rep.ByCategory[j].Category })
	rep.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	return rep, nil
}

// benchRes 四引擎合并结果。
type benchRes struct {
	score        int
	action       string
	strongAction bool     // 引擎给出 challenge/block/ban（score 之外的第二检出判据）
	ruleIDs      []string // 全部命中规则 ID（失败归因用）
}

// runAll 按线上顺序跑引擎并合并分数（与 edge.go 完全同构：自研规则 →
// CRS 调度闸门判定后跑 CRS → libi → 语义；ban 命中短路后续引擎；
// CRS 不喂解码变体）。
func runAll(eng *benchEngines, in *detect.Input, dc detect.DecodeConfig) benchRes {
	qv, bv := detect.DecodeVariants(in.Query, in.Body, dc)
	in.DecodedQuery, in.DecodedBody = qv, bv

	var out benchRes
	strong := func(a string) bool { return a == "challenge" || a == "block" || a == "ban" }

	absorb := func(r detect.Result) {
		out.score += r.Score
		out.ruleIDs = append(out.ruleIDs, r.RuleIDs...)
		if strong(r.Action) || r.Ban {
			out.strongAction = true
		}
		if r.Action != "" && r.Action != "pass" {
			out.action = r.Action
		}
	}
	rres := eng.rules.Inspect(in)
	absorb(rres)
	if rres.Action != "ban" {
		// CRS 调度闸门：与 edge.go 相同的旁路判定（excluded/static/safe_datauri/prescreen）
		if ok, _ := eng.gov.ShouldInspect("", in.Method, in.Path, in.Query, in.Body, in.Headers); ok {
			absorb(eng.crs.Inspect(in))
		}
		absorb(eng.libi.Inspect(in))
		absorb(eng.sem.Inspect(in))
	}
	return out
}

// buildBenchInput 与 edge.buildInput 同构：头串拼接、Cookie 单列。
func buildBenchInput(r *parsedReq) detect.Input {
	headers := ""
	for _, h := range r.others {
		headers += h.k + ": " + h.v + " "
	}
	if len(headers) > 4096 {
		headers = headers[:4096]
	}
	return detect.Input{
		Method:  r.Method,
		Host:    r.Host,
		Path:    r.Path,
		Query:   r.RawQuery,
		Body:    r.Body,
		UA:      r.UA,
		Referer: r.Referer,
		Headers: headers,
		Cookies: r.Cookies,
	}
}

// benignFamily 从 benign/benign-<family>-<idx>.txt 文件名取生成器族名。
func benignFamily(path string) string {
	name := filepath.Base(path)
	name = strings.TrimSuffix(name, ".txt")
	name = strings.TrimPrefix(name, "benign-")
	if i := strings.LastIndexByte(name, '-'); i > 0 {
		return name[:i]
	}
	return name
}

// listSamples 递归收集目录下的 .txt 样本文件，按路径排序保证复现顺序稳定。
func listSamples(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".txt") {
			out = append(out, path)
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("样本目录不存在: %s", root)
	}
	sort.Strings(out)
	return out, nil
}

// categoryOf 从 attacks/<cat>/xxx.txt 路径取类别段。
func categoryOf(path, root string) string {
	p := filepath.ToSlash(path)
	i := strings.Index(p, root+"/")
	if i < 0 {
		return "unknown"
	}
	rest := p[i+len(root)+1:]
	if j := strings.IndexByte(rest, '/'); j >= 0 {
		return rest[:j]
	}
	return "unknown"
}

// relPath 把样本绝对路径转成相对 -dir 的展示路径。
func relPath(dir, path string) string {
	if a, err := filepath.Abs(dir); err == nil {
		if b, err2 := filepath.Abs(path); err2 == nil {
			if rel, err3 := filepath.Rel(a, b); err3 == nil {
				return filepath.ToSlash(rel)
			}
		}
	}
	return filepath.ToSlash(path)
}

// countPair 计数聚合的展示辅助。
type countPair struct {
	k string
	n int
}

// topCounts 取计数 map 的前 n 项（降序，同分按 key 稳定排序）。
func topCounts(m map[string]int, n int) []countPair {
	var l []countPair
	for k, v := range m {
		l = append(l, countPair{k, v})
	}
	sort.Slice(l, func(i, j int) bool {
		if l[i].n != l[j].n {
			return l[i].n > l[j].n
		}
		return l[i].k < l[j].k
	})
	if len(l) > n {
		l = l[:n]
	}
	return l
}

// parseRawFile 读文件并解析为宽松请求结构。
func parseRawFile(path string) (*parsedReq, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r, err := parseRaw(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return r, nil
}

// probeFile 单样本调试：打印解析结果与四引擎全部命中明细。
func probeFile(path string) {
	eng, err := newBenchEngines()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	req, err := parseRawFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	in := buildBenchInput(req)
	qv, bv := detect.DecodeVariants(in.Query, in.Body, detect.DecodeConfig{Enabled: true, MaxDepth: detect.DefaultDecodeDepth, MaxBytes: detect.DefaultDecodeBytes})
	in.DecodedQuery, in.DecodedBody = qv, bv
	fmt.Printf("method=%s path=%s\nquery=%s\nbody=%.200s\nheaders=%s\nua=%s\n",
		in.Method, in.Path, in.Query, in.Body, in.Headers, in.UA)
	for name, fn := range map[string]func(*detect.Input) detect.Result{
		"rules": eng.rules.Inspect,
		"crs":   eng.crs.Inspect,
		"libi":  eng.libi.Inspect,
		"sem":   eng.sem.Inspect,
	} {
		r := fn(&in)
		if r.Score == 0 && len(r.Hits) == 0 {
			continue
		}
		fmt.Printf("-- %s: score=%d action=%s\n", name, r.Score, r.Action)
		for _, h := range r.Hits {
			fmt.Printf("   [%s] %s target=%s evidence=%.160s\n", h.RuleID, h.Name, h.Target, h.Evidence)
		}
	}
}

// printReport 控制台摘要输出。
func printReport(rep *benchReport) {
	fmt.Printf("== GoWAF 检测跑分 ==\n攻击样本 %d，检出 %d，召回 %.2f%%\n良性样本 %d，误报 %d，误报率 %.3f%%\n耗时 %dms\n",
		rep.Attacks, rep.Detected, rep.Recall*100, rep.Benign, rep.FalsePos, rep.FPRate*100, rep.ElapsedMS)
	for _, st := range rep.ByCategory {
		fmt.Printf("  %-12s %5d/%-5d %.1f%%\n", st.Category, st.Detected, st.Total, st.Rate*100)
	}
	if len(rep.FPRuleCounts) > 0 {
		topN := topCounts(rep.FPRuleCounts, 10)
		fmt.Println("误报规则聚合（top 10）:")
		for _, x := range topN {
			fmt.Printf("  %5d  %s\n", x.n, x.k)
		}
		topF := topCounts(rep.FPCatCounts, 12)
		fmt.Println("误报按良性族:")
		for _, x := range topF {
			fmt.Printf("  %5d  %s\n", x.n, x.k)
		}
	}
	if len(rep.Failures) > 0 {
		fmt.Printf("失败样本前 %d 条（明细见 report.json）:\n", min(10, len(rep.Failures)))
		for i, f := range rep.Failures {
			if i >= 10 {
				break
			}
			fmt.Printf("  [%s] %s score=%d action=%s\n", f.Category, f.File, f.Score, f.Action)
		}
	}
	// 验收口径硬判定，供 CI 直接读取退出码使用
	if rep.Recall < 0.70 {
		fmt.Println("⚠️ 召回未达 70% 验收线")
	}
	if rep.FPRate > 0.001 {
		fmt.Println("⚠️ 误报率未达 0.1% 验收线")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// bufio 引用占位：保持 import 集合与 parseRaw 的读者实现对齐。
var _ = bufio.NewReader
