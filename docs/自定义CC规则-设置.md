# 自定义 CC 规则（rate_limit 动作）设置指南

> 适用版本：v1.5.0+ · 所属模块：精准访问控制 · 控制台入口：**访问策略**

## 一、这是什么

在「精准访问控制」里直接定义 CC 防护规则：**符合条件的请求，按「策略 + 客户端 IP」在统计窗口内计数，超过阈值就处置**（拦截 429 或转人机校验）。

与全局限流的分工：

| | 全局限流（防护配置） | 自定义 CC 规则（本功能） |
|---|---|---|
| 粒度 | 每站点每 IP，全站一刀切 | **按条件圈定范围**：某路径、某 UA、某地域、某来路…… |
| 计数桶 | 站点级 Limiter | 每条策略独立计数（策略 ID + IP） |
| 典型用途 | 兜底防刷 | 保护登录接口、搜索接口、活动页等热点路径 |

对应雷池（SafeLine）的「自定义频率限制」能力。

## 二、工作原理

```
请求 → 精准访问控制(早期阶段,缓存之前) → 命中 rate_limit 策略
  → 计数桶 key = "策略ID|客户端IP"，窗口内 +1
  → 未达阈值 → 放行（继续限流/检测/转发）
  └─达到阈值 → 按 onExceed 处置：
        block     → 429「请求过于频繁（自定义限流规则）」
        challenge → 转人机校验（PoW），通过后 30 分钟内不再挑战
```

关键设计：

1. **门禁在缓存之前执行**：命中缓存的静态资源同样计数，刷缓存接口照样能被限住；
2. **每条策略独立计数**：策略 A 与策略 B 的计数互不挤占；同一 IP 在不同策略下有独立的额度；
3. **计数桶与自动封禁隔离**：CC 计数用独立 HitTracker，48 小时无命中自动逐出，不会污染封禁连击；
4. **观察模式不处置**：引擎 observe 模式下 CC 规则只记录不拦截。

## 三、控制台配置步骤

1. 进入 **访问策略 → 新增策略**；
2. **动作**选择「自定义 CC(rate_limit)」；
3. 配置条件（决定"限谁"）：
   - 保护登录接口：`path` prefix `/api/login`
   - 限制搜索：`path` prefix `/search`
   - 限制某 UA 刷接口：`ua` contains `python-requests`
   - 组合条件：path 前缀 `/api/` AND method eq `POST`
4. CC 配置区块：
   - **统计窗口**（秒）：10 ~ 86400，默认 60；
   - **阈值**（次）：窗口内允许的最大请求数，默认 120；
   - **超限动作**：`block`（429 拦截，默认）或 `challenge`（转人机校验）；
5. 保存即生效。

### 阈值怎么定

先开 observe（引擎观察模式）或把阈值放大观察一天真实流量，再看事件页该策略的命中量：

- 登录接口：单人正常操作 1 分钟 3~5 次，阈值 10~20 足够；
- 搜索接口：正常浏览 1 分钟 10~30 次，阈值 60 起步；
- 活动/秒杀页：按压测峰值 × 2 留余量；
- 不确定就先用 `challenge`：误伤成本只是半秒无感校验，不是 429。

## 四、API 配置示例

保护登录接口（60 秒 20 次，超限转人机校验）：

```bash
curl -X POST http://127.0.0.1:8899/api/policies \
  -H "Authorization: <access token>" -H "Content-Type: application/json" \
  -d '{
    "name": "登录接口 CC",
    "site": "",
    "priority": 30,
    "enabled": 1,
    "action": "rate_limit",
    "conditions": "{\"logic\":\"and\",\"items\":[{\"field\":\"path\",\"op\":\"prefix\",\"value\":\"/api/login\"}]}",
    "config": {"windowSec": 60, "threshold": 20, "onExceed": "challenge"}
  }'
```

限制脚本 UA 批量抓取（5 分钟 300 次，超限 429）：

```bash
curl -X POST http://127.0.0.1:8899/api/policies \
  -H "Authorization: <access token>" -H "Content-Type: application/json" \
  -d '{
    "name": "脚本抓取限速",
    "action": "rate_limit",
    "conditions": "{\"logic\":\"and\",\"items\":[{\"field\":\"ua\",\"op\":\"regex\",\"value\":\"(python-requests|curl|wget|scrapy)\"}]}",
    "config": {"windowSec": 300, "threshold": 300, "onExceed": "block"}
  }'
```

## 五、配置项说明

| 字段 | 说明 | 取值 |
|---|---|---|
| `windowSec` | 统计窗口（秒） | 10 ~ 86400，默认 60 |
| `threshold` | 窗口内最大请求数（按「策略+IP」计） | 1 ~ 1000000，默认 120 |
| `onExceed` | 超限处置 | `block` 429 / `challenge` 人机校验 |

## 六、注意事项

1. **条件不能依赖 body**：CC 是缓存之前的门禁动作，条件树引用 body 会被编译拒绝（请求频率与内容无关，这是有意设计）；
2. **按「策略+IP」计数**：NAT 出口（公司/校园网）共享 IP 时，阈值要按出口总人数放余量，或改用 challenge 动作；
3. **计数在放行时就发生**：正常流量也在消耗额度，阈值要给正常用户留足余量；
4. **与全局限流的先后**：CC 规则在缓存之前执行，先于站点级限流；两道都配置时，先触发的先生效；
5. **challenge 的免检窗口**：超限转挑战后，通过 PoW 的访客 30 分钟内不再被该规则反复挑战；
6. **验证是否生效**：`for i in $(seq 1 30); do curl -s -o /dev/null -w "%{http_code}\n" https://site/api/login; done`，超限后应看到 429（block）或 403 挑战页（challenge）；事件页按规则 ID `policy-<ID>` 检索。

## 七、常见问题

**Q：为什么不用防护配置里的 IP 限流就够？**
全局限流是"每站点每 IP"一刀切，无法表达"只限登录接口""只限脚本 UA"这类范围；而且全局阈值调低会误伤全站正常用户。CC 规则把限流范围精确到条件树，两者互补。

**Q：命中 CC 后 IP 会被自动封禁吗？**
不会。rate_limit 只做频率处置，不写封禁表。需要"超限即封"的用 block 动作配合全局自动封禁（guard.ban_*），或把动作改成 ban 类策略另行叠加。

**Q：窗口和阈值怎么换算 RPS？**
阈值/窗口 ≈ 平均 RPS 上限。60 秒 120 次 ≈ 平均 2 RPS，突发（窗口内前几秒打满）不受限制——这是固定窗口计数的特性，短突发友好、长持续攻击必拦。
