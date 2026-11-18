# GoWAF 性能基准（docs/benchmarks.md）

> 环境：Windows 11 x64（6 核）/ 回环 / 1.6KB HTML / keep-alive / 无 TLS / gowaf v1.6.3（feat/p1-v1.6.3 @ 语义扩类+OIDC+JA4 之后）。
> 工具：`tools/loadgen`（本仓库自带，`go run ./tools/loadgen` 一键复现）。
> **诚实声明**：本表数字由 loadgen 产生；v1.6.0 对比文档中的历史数字（如 12,836 RPS）由当时的手工口径产生，**工具不同绝对值不可直接互比**，同表内数字自洽可比。
> CRS 预筛：`engine.crs.prescreen=true`（加速模式）；其余新组件（防重放/等候室/日志外发）默认关，不影响本表。

## 一、结果

| 场景 | 并发 | 吞吐 RPS | p50 | p90 | p99 | 说明 |
|---|---|---|---|---|---|---|
| 裸后端（不经 WAF） | C=64 | 45,555 | 1.17ms | 2.34ms | 4.05ms | 吞吐基线 |
| **WAF 全引擎·干净流量** | C=64 | **5,879** | 10.05ms | 17.36ms | 28.06ms | 四引擎全开，预筛旁路 100% |
| WAF 静态资源（/app.js） | C=64 | 6,675 | 8.88ms | 15.77ms | 24.85ms | skip_static 旁路 |
| WAF 干净流量（单请求时延） | C=1 | 1,871 | **0.52ms** | 0.58ms | 2.23ms | 单请求延迟增量 ~0.5ms |
| WAF 攻击流量（8 种轮换 payload） | C=64 | 22,266 | 2.02ms | 5.01ms | 16.62ms | 全部 4xx（含自动封禁快路径，无回源） |

相对开销：干净流量吞吐 = 基线的 12.9%；攻击拦截路径无回源成本，吞吐高于回源路径属预期。

## 二、复现步骤（scripts/bench/README.md）

```bash
# 1. 模拟上游
go run ./tools/loadgen -mode backend -addr 127.0.0.1:18080
# 2. 启动 WAF（独立数据目录与端口）
go run ./cmd/gowaf -c scripts/bench/config.yaml
# 3. 首启后按 scripts/bench/data/initial-password.txt 登录 admin，
#    打开加速模式：PUT /api/settings {"engine.crs.prescreen":"true", ...}
# 4. 跑分（干净在前，攻击最后——攻击会触发自动封禁压测源 IP）
go run ./tools/loadgen -mode load -url http://127.0.0.1:18081/ -c 64 -dur 12s -scenario clean
go run ./tools/loadgen -mode load -url http://127.0.0.1:18081/ -c 64 -dur 12s -scenario attack
```

## 三、本轮跑分顺带修复的两个真问题

1. **CRS 预筛被 `Accept-Encoding: gzip` 击穿**（v1.6.3 发现）：CRS 920280 等规则的字面量
   `gzip` 被提取进预筛词表，而几乎所有客户端默认发送 `Accept-Encoding: gzip`，
   实测预筛命中率≈0（`waf_crs_skipped_total{reason="prescreen"}` 不增长），CRS 全量执行。
   修复：gzip/deflate/identity/compress/zstd/encoding 进入预筛噪声词表；修复后干净流量
   预筛旁路 100%，同口径吞吐 2,560 → 5,879 RPS（2.3 倍）。
2. **CRS 事务双 Content-Type**：Inspect 在有 body 时无条件补
   `Content-Type: application/x-www-form-urlencoded`，与头串里客户端原始值重复，
   CRS 920620（Multiple Content-Type）对每个 POST 告警一次。修复后每个 POST 少一条告警与 5 分噪声分。

## 四、与检测跑分的关系

检测效果（召回/误报）见 `testdata/bench/report.json`（`go run ./tools/bench` 可复现）；
本表只关心吞吐与时延，两者口径独立。
