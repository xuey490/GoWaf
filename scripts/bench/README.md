# scripts/bench — 基准压测环境

GoWAF 数据面吞吐/时延基准的可复现环境（结果见 `docs/benchmarks.md`）。

## 组成

- `config.yaml`：专用 WAF 配置——数据面 127.0.0.1:18081、控制面 127.0.0.1:18899、
  上游指向模拟后端 127.0.0.1:18080、`sample_rate: 0`（压测不落事件库）、
  guard 阈值放到压测量级（真实部署勿用）。
- 数据目录 `data/`（首启自动生成，含 initial-password.txt；`.gitignore` 已排除）。

## 步骤

```bash
# 1. 模拟上游（1.6KB HTML）
go run ./tools/loadgen -mode backend -addr 127.0.0.1:18080

# 2. 启动 WAF（另开终端）
go run ./cmd/gowaf -c scripts/bench/config.yaml

# 3. 首启后查看 scripts/bench/data/initial-password.txt 得到 admin 口令，
#    登录 http://127.0.0.1:18899 并打开加速模式（与 v1.6.0 跑分口径一致）：
curl -X POST http://127.0.0.1:18899/api/auth/login -H "Content-Type: application/json" \
  -d '{"userName":"admin","password":"<口令>"}'
# 拿 token 后：
curl -X PUT http://127.0.0.1:18899/api/settings -H "Authorization: <token>" \
  -H "Content-Type: application/json" \
  -d '{"guard.challenge_rps":"1000000","guard.challenge_per_minute":"10000000",
       "guard.ip_rps":"1000000","guard.ip_burst":"2000000",
       "guard.ip_per_minute":"10000000",
       "engine.crs.prescreen":"true","engine.decode.enabled":"true"}'

# 4. 压测（干净在前，攻击最后——攻击 payload 会触发自动封禁压测源 IP）
go run ./tools/loadgen -mode load -url http://127.0.0.1:18081/ -c 64 -dur 12s -scenario clean
go run ./tools/loadgen -mode load -url http://127.0.0.1:18081/ -c 64 -dur 12s -scenario static
go run ./tools/loadgen -mode load -url http://127.0.0.1:18081/ -c 64 -dur 12s -scenario attack
```

## 注意

- guard 阈值只为消除限速干扰，生产配置不要照抄。
- 压测前确认预筛生效：`curl http://127.0.0.1:18899/metrics | grep waf_crs_skipped_total`
  （干净流量下 `reason="prescreen"` 应与请求数同步增长）。
