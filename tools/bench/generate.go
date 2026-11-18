package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// smuggSeeds HTTP 走私种子：每条是追加头行列表（| 分隔），不走 payload 变形。
// semCheckSmugg 判两类结构：CL 重复、TE 与 CL 并存。
var smuggSeeds = [][]string{
	{"Transfer-Encoding: chunked", "Content-Length: 38"},
	{"Content-Length: 5", "Content-Length: 11"},
	{"Transfer-Encoding: chunked", "Content-Length: 0"},
	{"Transfer-Encoding : chunked", "Content-Length: 12"},
	{"Content-Length: 5", "Content-Length : 15"},
	{"Transfer-Encoding: xchunked", "Content-Length: 20"},
	{"Transfer-Encoding: chunked", "Content-Length: 5", "Content-Length: 33"},
}

// queryPlacements 攻击样本在 query 放置时轮换的参数名。
var queryPlacements = []string{"q", "s", "file", "url", "id", "name", "redirect", "data", "search", "page", "cmd", "input", "path", "token"}

// redirectParams 开放重定向类种子专用参数名（与语义通道判定式保持一致）。
var redirectParams = []string{"next", "redirect", "return", "returnurl", "url", "goto", "target", "dest", "destination", "continue", "callback", "link", "forward", "rurl", "return_to"}

// generateCorpus 生成完整样本库：attacks/（种子 × 变形 × 放置）+ benign/（生成器族）
// + manifest.jsonl + README.md。输出确定性：同版本代码重跑逐字节一致。
func generateCorpus(dir string) error {
	start := totalSeeds()
	fmt.Printf("生成样本库到 %s：种子 %d 条\n", dir, start)

	// 清空重建，保证目录与 manifest 完全一致
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "attacks"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "benign"), 0o755); err != nil {
		return err
	}

	manifest, err := os.Create(filepath.Join(dir, "manifest.jsonl"))
	if err != nil {
		return err
	}
	defer manifest.Close()
	w := func(file, label, cat string) {
		fmt.Fprintf(manifest, `{"file":%q,"label":%q,"category":%q,"source":"self-curated","license":"MIT"}`+"\n", file, label, cat)
	}

	count := 0
	for cat, list := range seeds {
		catDir := filepath.Join(dir, "attacks", cat)
		if err := os.MkdirAll(catDir, 0o755); err != nil {
			return err
		}
		for si, payload := range list {
			for mi, m := range mutations {
				for pi := 0; pi < 2; pi++ { // 0=query 1=form-body
					mutated := m.fn(payload, si+mi)
					// 开放重定向种子用 redirect 类参数名：真实攻击就在
					// next/redirect/url 里携带跨域 URL，普通 q/s 参数名
					// 装绝对 URL 是另一种（更宽的）误报面，不在本库范围。
					names := queryPlacements
					if cat == "openredirect" {
						names = redirectParams
					}
					var raw string
					if pi == 0 {
						// query 放置一律做最小编码：空格/#/&/+= 会截断请求行
						// 或参数结构（case/comment/entity 等变体保留原始空格，
						// 不编码会丢 payload）；Normalize 多层解码等价还原。
						raw = buildRaw("GET", benchHost,
							"/a?"+names[si%len(names)]+"="+minimalQueryEncode(mutated), "", nil, "")
					} else {
						raw = buildRaw("POST", benchHost, "/submit", "", nil, names[si%len(names)]+"="+mutated)
					}
					name := fmt.Sprintf("%s-%03d-%s-%d.txt", cat, si, m.name, pi)
					full := filepath.Join(catDir, name)
					if err := os.WriteFile(full, []byte(raw), 0o644); err != nil {
						return err
					}
					w("attacks/"+cat+"/"+name, "attack", cat)
					count++
				}
			}
		}
		fmt.Printf("  attacks/%-14s %4d 种子 → %5d 样本\n", cat, len(list), len(list)*len(mutations)*2)
	}

	// 走私种子：纯头组合，identity 一种形态
	smuggDir := filepath.Join(dir, "attacks", "smugg")
	if err := os.MkdirAll(smuggDir, 0o755); err != nil {
		return err
	}
	for i, hs := range smuggSeeds {
		raw := buildRaw("POST", benchHost, "/upload", "", hs, "")
		name := fmt.Sprintf("smugg-%03d-identity-0.txt", i)
		if err := os.WriteFile(filepath.Join(smuggDir, name), []byte(raw), 0o644); err != nil {
			return err
		}
		w("attacks/smugg/"+name, "attack", "smugg")
		count++
	}

	// 良性样本：逐族独立 RNG
	for _, fam := range benignFamilies {
		for i := 0; i < fam.count; i++ {
			r := newFamilyRng(fam.name, i)
			raw := fam.fn(r, i)
			name := fmt.Sprintf("benign-%s-%05d.txt", fam.name, i)
			if err := os.WriteFile(filepath.Join(dir, "benign", name), []byte(raw), 0o644); err != nil {
				return err
			}
			w("benign/"+name, "benign", "benign")
			count++
		}
	}
	fmt.Printf("  benign %5d\n", totalBenign())

	// README：来源与许可纪律说明（对应方案 3.2/3.6）
	readme := `# testdata/bench 样本库

GoWAF 检测跑分样本库（P1-1 交付物），由 ` + "`go run ./tools/bench -generate`" + ` 确定性生成：

- attacks/：自建攻击种子（` + fmt.Sprint(totalSeeds()) + ` 条，MIT）× 12 种变形（编码/语法/结构，
  见 tools/bench/mutate.go）× 2 种参数放置（query / form-body），smugg 类为
  头组合种子（TE/CL 冲突、重复 CL）。
- benign/：良性样本 ` + fmt.Sprint(totalBenign()) + ` 条，覆盖框架 URL、REST/GraphQL、
  富编码（base64/JWT/data-URI）、运维文本（SQL/shell/YAML/日志）、中文搜索、
  Emoji/Markdown 等语义引擎高危误报面。
- manifest.jsonl：逐条 {file,label,category,source,license}；本库全部
  source=self-curated、license=MIT（随仓库分发，无第三方再分发限制）。
- 跑分：` + "`go run ./tools/bench -dir testdata/bench -threshold 30 -report report.json`" + `。
- 「检出」= 四引擎合并分 ≥ 30 或引擎给出 challenge/block/ban；报告口径见
  docs/P1-样本库建设方案.md。
`
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644); err != nil {
		return err
	}
	fmt.Printf("完成：共 %d 个样本文件（攻击 %d + 良性 %d）\n", count, count-totalBenign(), totalBenign())
	return nil
}

// totalSeeds 统计种子总量。
func totalSeeds() int {
	n := 0
	for _, v := range seeds {
		n += len(v)
	}
	return n + len(smuggSeeds)
}

// totalBenign 统计良性样本总量。
func totalBenign() int {
	n := 0
	for _, fam := range benignFamilies {
		n += fam.count
	}
	return n
}

// 确认 strings 引用（buildRaw 相关辅助在同包）。
var _ = strings.TrimSpace
