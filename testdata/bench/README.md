# testdata/bench 样本库

GoWAF 检测跑分样本库（P1-1 交付物），由 `go run ./tools/bench -generate` 确定性生成：

- attacks/：自建攻击种子（431 条，MIT）× 12 种变形（编码/语法/结构，
  见 tools/bench/mutate.go）× 2 种参数放置（query / form-body），smugg 类为
  头组合种子（TE/CL 冲突、重复 CL）。
- benign/：良性样本 9970 条，覆盖框架 URL、REST/GraphQL、
  富编码（base64/JWT/data-URI）、运维文本（SQL/shell/YAML/日志）、中文搜索、
  Emoji/Markdown 等语义引擎高危误报面。
- manifest.jsonl：逐条 {file,label,category,source,license}；本库全部
  source=self-curated、license=MIT（随仓库分发，无第三方再分发限制）。
- 跑分：`go run ./tools/bench -dir testdata/bench -threshold 30 -report report.json`。
- 「检出」= 四引擎合并分 ≥ 30 或引擎给出 challenge/block/ban；报告口径见
  docs/P1-样本库建设方案.md。
