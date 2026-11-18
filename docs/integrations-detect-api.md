# 检测引擎独立调用（网关集成）

v1.6.0 起 GoWAF 的检测能力可以脱离流量代理独立部署：配置
`server.detect_listen` 后，任意网关（Nginx / Kong / Traefik /
Ingress-NGINX / OpenResty）都能把请求交给 GoWAF 判定，再决定放行或拦截。
这是与雷池 T1K 协议对标的接入形态——WAF 不再只能「整体接管 80/443」。

## 启用

```yaml
server:
  detect_listen: "127.0.0.1:8020"   # 绑回环或内网；0.0.0.0 时务必配置令牌
  detect_token: "change-me"          # 可选；为空时不认证
```

重启后日志出现 `[app] 独立检测引擎监听 127.0.0.1:8020`。

## 端点

| 端点 | 方法 | 用途 |
|---|---|---|
| `/detect` | POST | 完整 JSON 协议，返回结构化判定（lua-resty / SDK 调用） |
| `/check` | GET/POST | 网关鉴权式：2xx 放行、403 拦截、401 需人机校验 |
| `/healthz` | GET | 存活探测 |

认证：`Authorization: Bearer <token>` 或 `X-WAF-Token: <token>`。

### POST /detect

```json
{
  "method": "GET",
  "host": "example.com",
  "path": "/api/user",
  "query": "id=1 union select password from users",
  "body": "…",              // 或 body_b64: base64 串（二进制）
  "headers": {"User-Agent": "…"},
  "site": "example"          // 可选，供例外规则匹配
}
```

响应：

```json
{
  "action": "block",          // pass / log / challenge / block / ban
  "score": 120,
  "hits": [{"ruleId": "crs-942100", "score": 60, "…": "…"}],
  "rule_ids": ["crs-942100", "libi-sqli"],
  "categories": ["sqli"],
  "elapsed_us": 813,
  "versions": {"rules": "…", "crs": "crs-4.30.0", "mode": "protect"}
}
```

### /check（Traefik forwardAuth / nginx auth_request）

网关注入 `X-Forwarded-Method` / `X-Forwarded-Uri` / `X-Forwarded-Host`，
其余头原样参与检测。响应头 `X-WAF-Action` / `X-WAF-Score` 携带结论。

## 集成示例

- Nginx/OpenResty：[nginx/waf_detect.lua](nginx/waf_detect.lua) +
  [nginx.conf.example](nginx/nginx.conf.example)
- Traefik：[traefik/forwardauth.yml](traefik/forwardauth.yml)
- Kong：用 request-termination + pre-function 组合调 `/detect`，或接
  [Kong WAF 插件模式](https://docs.konghq.com/hub/)（照 lua 示例改写）

## 与内置代理模式的差异

| 行为 | 内置数据面 | /detect /check |
|---|---|---|
| 引擎编排 | rules + CRS（闸门调度）+ libi + 语义 + payload | rules + CRS（全量）+ libi + 语义 |
| 调度闸门 | engine.crs.* 生效 | 不生效（旁路由网关缓存实现） |
| 多层解码 | engine.decode.* 生效 | 生效（同配置） |
| 动作判定 | 站点级 guard 覆盖 | 全局阈值与模式 |
| 处置执行 | 内置（403/挑战/封禁） | 返回结论，网关执行 |
