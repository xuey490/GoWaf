# Go-WAF 开发方案

**全平台、全环境兼容的反向代理型 WAF 与 Vue3 可视化控制台**（宝塔 / 1Panel / 小皮 / 原生 Nginx·Apache / IIS）

| 项目 | 内容 |
| --- | --- |
| 文档版本 | **v1.1**（2026-11-02 修订） |
| 编写日期 | 2026-11-02 |
| 目标读者 | 后端 / 运维 / 前端开发者本人（单人全栈实施） |
| 技术栈 | 数据面：Go 1.22+ + `net/http`；控制面：**Gin + GORM + SQLite**；前端：**Vue 3 + Vite + TypeScript + naive-ui**（复用 `Frontend/` 现有后台框架） |
| 部署形态 | 单二进制 + systemd，数据面 80/443，控制面 127.0.0.1:8899 |
| 实施周期 | 约 6 周（单人，含灰度） |
| 鉴权体系 | JWT（access + refresh）+ bcrypt + CSRF + CORS + IP 白名单 + 审计，内置**演示模式** |
| 前端基准 | `Frontend/`（Fssadmin，naive-ui 版，Vue 3.5 / Vite 8 / Pinia 4 / ECharts 6） |

**v1.1 修订要点**（相对 v1.0）

1. 控制面技术栈明确为 **Gin + GORM + SQLite**，并锁定纯 Go 驱动（`glebarez/sqlite`），保证 `CGO_ENABLED=0` 交叉编译；
2. 前端不再是"从零搭"，改为在 `Frontend/` 现有框架上增量扩展（5.12 / 9.1 / 9.2）；
3. 接口契约全部对齐前端既有实现：`code=200` 成功、`current/size` 分页、`records` 列表、`Authorization` 裸 token（7.1）；
4. 新增 **CSRF / CORS / 演示模式**三组中间件与装配顺序（8.10），新增演示模式的库隔离方案（5.13）；
5. 新增 GORM 使用边界（6.1）、版本化迁移流程（6.5）、`go:embed` 跨包限制的处理（5.12）。

---

## 目录

- [0. 方案速览](#0-方案速览)
- [1. 项目背景与目标](#1-项目背景与目标)
- [2. 威胁模型与防护边界](#2-威胁模型与防护边界)
- [3. 总体架构](#3-总体架构)
- [4. 功能清单](#4-功能清单)
- [5. 核心模块设计](#5-核心模块设计)
- [6. 数据模型](#6-数据模型)
- [7. 管理 API 设计](#7-管理-api-设计)
- [8. 关键代码实现](#8-关键代码实现)
- [9. 前端控制台设计](#9-前端控制台设计)
- [10. 性能与容量](#10-性能与容量)
- [11. WAF 自身安全](#11-waf-自身安全)
- [12. 可观测性](#12-可观测性)
- [13. 部署与上线（宝塔实操）](#13-部署与上线宝塔实操)
- [14. 测试方案](#14-测试方案)
- [15. 迭代计划](#15-迭代计划)
- [16. 风险与对策](#16-风险与对策)
- [17. 附录](#17-附录)
- [结语](#结语)

---

## 0. 方案速览

一句话概括：**用 Go 写一个七层反向代理，串在 Apache 前面接管 80/443，对每个请求做「归一化 → 规则匹配 → 评分 → 决策」，恶意流量就地拦截，正常流量转发给 Apache，同时把过程和结果写进 SQLite，通过 8899 端口上的 Vue3 控制台下发配置、查看态势。**

三个必须提前想清楚的问题，决定了整个方案的形态：

1. **WAF 必须终止 TLS**。如果 HTTPS 仍在 Apache 上终止，Go 只能看到密文，除四层限速外什么都检测不了。因此方案里 WAF 接管 443，Apache 降级为只监听 `127.0.0.1:8080` 的明文后端。
2. **WAF 不能成为单点故障**。进程崩溃、上游超时、规则写错，都必须有兜底：`recover`、健康检查、观察模式、秒级回滚脚本。
3. **误报比漏报更致命**。一次错误的拦截可能打掉支付回调或后台登录。所以默认先跑 3～7 天 **observe（只记录不拦截）**，用真实流量校准阈值，再切 protect。

关键指标目标：

| 指标 | 目标值 |
| --- | --- |
| 纯转发吞吐（4 核） | ≥ 30,000 QPS |
| 开启全量检测吞吐 | ≥ 8,000 QPS |
| 附加延迟 P99（小请求） | ≤ 3 ms |
| 进程常驻内存 | ≤ 400 MB |
| 单条规则判定耗时 | ≤ 50 µs |
| 故障恢复 | 停止 WAF、Apache 改回 80，≤ 60 秒 |

**技术栈分两块，不要混用：**

| 层 | 用什么 | 为什么 |
| --- | --- | --- |
| 数据面（80/443） | 标准库 `net/http` + `httputil.ReverseProxy` + 自研检测器 | 每请求都过，要极简、零反射、零 ORM，延迟才压得住 |
| 控制面（8899） | **Gin + GORM + SQLite** | 接口 CRUD 密集、开发效率优先，性能不敏感（QPS 个位数） |
| 可视化前端 | **Vue 3 + naive-ui**（`Frontend/` 现有框架扩展） | 后台骨架、权限、i18n、主题都已具备，只加 WAF 业务模块 |

> 关键取舍：**GORM 只用于配置类表**（用户、站点、规则、IP 名单、审计）。`events` / `stats_1m` 这种每秒几百上千条写入的表，仍然走原生 SQL + 批量事务，ORM 的反射和钩子开销在这里没有必要。详见 6.1。

---

## 1. 项目背景与目标

### 1.1 现状

现有若干站点部署在同一台（或几台）宝塔面板服务器上，形态统一为 **前后端分离**：

- Apache 监听 `0.0.0.0:80`（以及 443），按 `ServerName` 做虚拟主机；
- Apache 通过 `ProxyPass` / `mod_proxy` 把请求转发到本机不同端口：`8080`（站点 A 后端）、`8081`（站点 B 后端）、`8082`（站点 C 后端）；
- 前端静态资源由 Apache 直接托管，接口请求走后端；
- 面板、防火墙、SSL 证书都由宝塔管理。

问题在于：**80 端口直接暴露在公网，Apache 只负责转发，不做任何应用层检测**。于是日志里长期充斥着：

```
/.env                    /.git/config             /wp-admin/setup.php
/phpmyadmin/index.php    /config.php.bak          /vendor/phpunit/...
/?id=1' AND SLEEP(5)--   /index.php?s=/Index/\think\app/invokefunction
GET /shell.php           (UA: sqlmap/1.7.2#stable)
```

这些扫描绝大多数打不穿业务，但带来三个实际危害：日志被污染、带宽和 CPU 被消耗、一旦某个后台接口有未授权访问或文件上传缺陷，就会被批量工具直接拿下。

### 1.2 痛点与对策

| 现状问题 | 具体影响 | 本方案对策 |
| --- | --- | --- |
| 扫描器 7×24 扫路径 | 日志噪声、CPU 空转、暴露指纹 | 敏感路径库 + 扫描器 UA 指纹 + 频率突增检测 |
| 无 CC 防护 | 单 IP 高频打接口，后端 502 | 三级限流 + 自动封禁 + 人机校验 |
| 注入/XSS 无检测 | 依赖业务代码自觉，无兜底 | 归一化 + 多模式规则引擎 + 评分制 |
| 攻击无法追溯 | 只知被扫，不知是否成功 | 全链路事件日志（请求、命中规则、处置、响应码） |
| 封 IP 要改 Apache 配置再 reload | 响应慢、易写错 | 控制台一键封禁，内存生效，秒级 |
| 多站点端口分散 | 配置散落在各 vhost | 站点表统一管理 Host → Upstream 映射 |
| 宝塔改配置会被覆盖 | 手工改动易丢失 | WAF 独立配置，不依赖 Apache 配置 |

### 1.3 设计目标

**功能性**

- 单进程内同时提供数据面（80/443）与控制面（8899），也可拆成两个进程。
- Host 维度的多站点路由，支持 HTTP/HTTPS、WebSocket、SSE、大文件上传。
- 五类检测：IP/区域、频率、协议合规、请求内容（注入/XSS/穿越/命令）、客户端指纹（UA/扫描器）。
- 四种处置：放行、记录、人机校验、拦截（403/429/444）。
- 规则、站点、阈值全部支持热更新，不重启、不掉连接。
- 控制台提供态势大屏、实时日志、IP 名单、规则管理、站点管理、审计。

**非功能性**

| 维度 | 要求 |
| --- | --- |
| 性能 | 附加延迟 P99 ≤ 3ms；检测可开关、可按路径降级 |
| 可用性 | 数据面 panic 不影响控制面；上游故障返回 502 页面而非挂死 |
| 可运维 | 单二进制、systemd 托管、日志轮转、配置校验失败自动回滚 |
| 可观测 | Prometheus 指标 + 结构化日志 + 健康检查 |
| 安全 | 控制面默认只监听回环地址，JWT + IP 白名单 + 审计日志 |
| 合规 | 攻击日志留存 ≥ 180 天（网络安全法要求网络日志不少于六个月），敏感字段脱敏 |

### 1.4 明确的非目标

写清楚不做什么，比写做什么更重要：

- **不做四层 DDoS 清洗**。SYN Flood、UDP 反射这类攻击应该在云厂商清洗中心或机房防火墙解决，Go 进程扛不住，也不该扛。
- **不做网页防篡改、不做病毒扫描、不做数据防泄漏（DLP）**。
- **不做证书自动签发**。证书继续由宝塔申请和续期，WAF 只负责读取和热加载。
- **不替代业务侧的权限校验**。WAF 是兜底，不是越权漏洞的解决方案。
- **不做分布式集群**。单机多站点场景，单实例足够；真要横向扩展，见 16.4 的演进路径。

### 1.5 自研还是用开源

这一点必须先说清楚，避免重复造轮子。

| 方案 | 优势 | 劣势 | 适合场景 |
| --- | --- | --- | --- |
| **雷池 SafeLine** | 开箱即用、语义分析引擎强、自带控制台 | 资源占用较高（约 1～2 GB）、二次开发受限、架构较重 | 只想尽快上线防护 |
| **Coraza + Caddy/Nginx** | OWASP CRS 规则成熟、Go 生态、可嵌入 | 控制台需自己做、CRS 误报需要调、文档偏英文 | 想要成熟规则 + 轻量定制 |
| **BunkerWeb** | 功能全、有 UI、容器化 | 依赖 Docker/Nginx，和宝塔 Apache 共存麻烦 | 新部署、可容器化 |
| **mod_security** | Apache 原生、CRS 生态 | 性能损耗明显、规则调优门槛高、日志难用 | 不想加新进程 |
| **本方案（自研）** | 完全贴合现有端口/站点结构、性能可控、控制台自主、可深度定制 | 需要自己维护规则、需要自己踩误报的坑 | 有开发能力、要长期演进 |

**建议**：如果目标只是"明天就不被扫穿"，直接上雷池，半天搞定。如果目标是"我要一个能改、能看、能演进的组件"，按本方案做，但**规则库不要从零写**——把 OWASP CRS 的核心规则子集转成自己的 YAML 格式导入，能省掉 80% 的规则设计工作。

---

## 2. 威胁模型与防护边界

### 2.1 攻击面

一个典型的前后端分离站点，暴露面包括：

```
公网
 ├── 静态资源        /static/* /assets/* /upload/*        ← 主要风险：webshell、目录遍历
 ├── 前端路由        / /login /dashboard                  ← 主要风险：无（SPA 壳）
 ├── 业务 API        /api/*                              ← 主要风险：注入、越权、CC、撞库
 ├── 认证接口        /api/auth/login /api/auth/sms       ← 主要风险：爆破、短信轰炸
 ├── 文件接口        /api/upload /api/export             ← 主要风险：恶意文件、超大 body
 ├── 回调接口        /api/pay/notify /api/wx/callback    ← 主要风险：伪造、重放（WAF 不宜拦）
 └── 运维/框架残留   /.env /.git/actuator /phpinfo       ← 主要风险：信息泄露、直接入侵
```

### 2.2 检测能力矩阵

| 类别 | 检测目标 | 检测方法 | 规则示例 | 默认动作 | 误报风险 |
| --- | --- | --- | --- | --- | --- |
| 扫描器 | UA、请求头顺序、路径枚举 | 指纹库 + 路径库 + 频率 | `UA ~ sqlmap|nikto|nmap` | 拦截 | 低 |
| 敏感文件 | 路径 | 精确/前缀匹配 | `/.env`、`/.git/config`、`/*.bak` | 拦截 | 极低 |
| SQL 注入 | query、body、cookie | 关键词预筛 + 正则 + 评分 | `union\s+select`、`sleep(` | 拦截 | 中 |
| XSS | query、body | 标签/事件/伪协议正则 | `<script`、`onerror=`、`javascript:` | 拦截/挑战 | 中 |
| 路径穿越 | path、query | `../` 及其编码变体 | `%2e%2e%2f` | 拦截 | 低 |
| 命令注入 | query、body | 管道符 + 命令名组合 | `;wget `、`|curl `、`$(whoami)` | 拦截 | 中 |
| 文件包含 | query | 协议前缀 + 路径 | `php://input`、`file://` | 拦截 | 低 |
| 反序列化 | body | 魔术字节 + 类名特征 | `O:8:"stdClass"`、`rO0AB` | 拦截 | 低 |
| Webshell | 上传文件名/内容 | 扩展名 + 一句话特征 | `eval($_POST`、`.php.jpg` | 拦截 | 低 |
| CC / 爆破 | 频率、路径集中度 | 令牌桶 + 滑动窗口 | 单 IP 600 次/分 | 挑战→封禁 | 中 |
| 恶意 Bot | UA、Header 完整性 | 规则 + 行为 | UA 为空、缺 Accept | 记录/挑战 | 中 |
| 协议异常 | 方法、Host、长度 | 白名单校验 | `PUT /.env`、超长 URL | 拦截 | 低 |
| 响应泄露 | 响应体 | 正则（可选） | 身份证、手机号、密钥 | 仅记录 | 高 |

> **原则**：只对"高置信度、低误报"的类别做**直接拦截**（敏感文件、扫描器、路径穿越、反序列化）；对"有业务歧义"的类别（SQLi、XSS）采用**评分 + 阈值**，并允许按路径配置为只记录。

### 2.3 处置动作

| 动作 | HTTP 表现 | 使用场景 |
| --- | --- | --- |
| `pass` | 正常转发 | 白名单、健康检查、回调 |
| `log` | 正常转发，记事件 | 观察模式、低分命中 |
| `challenge` | 返回 200 + JS 挑战页 | 疑似 CC、可疑 UA |
| `block` | 403 + 拦截页（含 Request ID） | 明确攻击特征 |
| `drop` | 直接关闭连接（ hijack close） | 高频恶意扫描，省带宽 |
| `ban` | 后续请求直接 403 | 自动封禁命中 |

### 2.4 误报是最大的敌人

必须建立三条机制：

1. **观察模式**：`engine.mode = observe` 时所有 `block` 降级为 `log`，只积累数据。
2. **白名单优先级最高**：IP 白名单 > 路径白名单 > 站点白名单 > 规则。后台地址、支付回调、健康检查必须能配白名单。
3. **可回滚**：每次配置变更生成版本号，控制台一键回滚到上一版本；规则文件写坏时进程加载失败应保留旧快照而不是退出。

---

## 3. 总体架构

### 3.1 架构图

```
                         Internet
                            │
                            ▼
              ┌─────────────────────────────┐
              │   Go-WAF 数据面 :80 / :443  │
              │  ┌───────────────────────┐  │
              │  │ 1. TLS 终止 (SNI)     │  │
              │  │ 2. 站点路由 (Host)    │  │
              │  │ 3. IP 名单 / 限流     │  │
              │  │ 4. 协议校验           │  │
              │  │ 5. Body 采样(限长)    │  │
              │  │ 6. 归一化 + 规则匹配  │  │
              │  │ 7. 评分 → 决策        │  │
              │  │ 8. 反向代理           │  │
              │  └───────────┬───────────┘  │
              └──────────────┼──────────────┘
                             │ 异步事件 / 统计
        ┌────────────────────┼─────────────────────┐
        ▼                    ▼                     ▼
  ┌───────────┐      ┌─────────────┐      ┌──────────────┐
  │  Apache   │      │   SQLite    │      │ Go-WAF 控制面 │
  │ 127.0.0.1 │      │  WAL 模式   │◄────►│ 127.0.0.1:8899│
  │   :8080   │      │  events     │      │  Gin + JWT    │
  └─────┬─────┘      │  rules      │      │  Vue3 dist    │
        │            │  stats_1m   │      └──────┬────────┘
        ├─► :8081 站点A 后端         └─────────────┘        │
        ├─► :8082 站点B 后端                                ▼
        └─► :8083 站点C 后端                         浏览器（你）
```

### 3.2 双服务模型

| | 数据面（Data Plane） | 控制面（Control Plane） |
| --- | --- | --- |
| 监听 | `0.0.0.0:80`、`0.0.0.0:443` | `127.0.0.1:8899`（默认） |
| 职责 | 转发、检测、拦截、记录 | 静态资源、REST API、鉴权、配置下发 |
| 延迟要求 | 极敏感（毫秒级） | 不敏感（百毫秒级） |
| 框架 | 标准库 `net/http` + `httputil` | Gin |
| 故障影响 | 全站不可用 | 仅无法管理，防护继续生效 |

**为什么用标准库而不是 Gin 做数据面**：Gin 的 `Context` 池、路由树和中间件链在纯代理场景是额外开销；`httputil.ReverseProxy` 已经处理好 Upgrade、Flush、Trailer 等细节，套在 Gin 里反而容易踩坑（比如 `c.Request.Body` 被中间件读过）。控制面用 Gin 则非常合适。

**为什么放同一个进程**：单二进制部署简单，配置共享无需 IPC。代价是控制面 panic 可能带崩数据面，因此：

- 控制面所有 handler 外层包 `recover`；
- 数据面每个请求 `defer recover`，panic 时返回 502 而不是让进程退出；
- 控制面设置独立的 `http.Server`，超时、连接数与数据面隔离。

### 3.3 请求生命周期

```
① accept ──► ② TLS 握手 / SNI 选证 ──► ③ 解析 Host 匹配站点
                                              │
                                              ▼
④ 生成 Request-ID、提取真实客户端 IP（可信代理链）
                                              │
                                              ▼
⑤ IP 名单：静态黑名单 → 临时封禁 → 白名单（命中直接放行）
                                              │
                                              ▼
⑥ 限流：全站 QPS → 单 IP 令牌桶 → 路径级滑动窗口
                                              │
                                              ▼
⑦ 协议校验：方法白名单、Host 合法性、URL 长度、Header 数量
                                              │
                                              ▼
⑧ 读 Body（最多 N KB，超出部分流式透传，不落地）
                                              │
                                              ▼
⑨ 归一化（多次解码、去注释、NFKC、折叠空白）
                                              │
                                              ▼
⑩ 规则匹配（AC 预筛 → 正则 → 命中集合）
                                              │
                                              ▼
⑪ 评分 → 阈值 → 动作（pass/log/challenge/block/ban）
                                              │
                              ┌───────────────┴───────────────┐
                              ▼                               ▼
                        拦截：写事件、返回错误页        放行：ReverseProxy
                                                              │
                                                              ▼
                                                    ⑫ 响应回写 + 异步落库 + 指标累加
```

### 3.4 端口规划

| 端口 | 绑定地址 | 用途 | 是否对公网 |
| --- | --- | --- | --- |
| 80 | `0.0.0.0` | WAF HTTP 入口（兼 ACME 验证、跳转 443） | 是 |
| 443 | `0.0.0.0` | WAF HTTPS 入口（TLS 终止） | 是 |
| 8899 | `127.0.0.1` | 控制台 + API | **否**（SSH 隧道或 Nginx 反代 + Basic Auth） |
| 8080 | `127.0.0.1` | Apache 新监听端口 | 否 |
| 8081/8082/8083 | `127.0.0.1` | 各站点后端应用 | 否 |

> 很多教程会让 8899 直接监听 `0.0.0.0`，这是很危险的：一旦控制台有未授权漏洞，攻击者可以直接关掉防护、封掉你的 IP。**默认必须是 127.0.0.1**，需要远程访问时用 `ssh -L 8899:127.0.0.1:8899`。

### 3.5 接入方式（已选定）：Apache 整体后移，WAF 接管全部入口流量

**决策：采用「WAF 前置 + Apache 后移」模式。** WAF 接管 80/443，成为唯一的公网入口；Apache 不再监听公网，改为监听 `127.0.0.1:8080`，原有的 vhost、`ProxyPass`、静态资源、PHP-FPM 配置**全部保留**，只是换了监听地址和端口。

这样选的理由：

- **改造面最小**：Apache 的站点配置、伪静态、目录权限、SSL 证书申请流程都不用推翻，只改 `Listen` 和 vhost 的绑定地址。
- **职责清晰**：WAF 只管"看流量、拦坏人"，Apache 继续管"托管静态资源、跑 PHP、按域名分发到 8080/8081/8082 后端"。
- **可回滚**：出问题时把 Apache 的 `Listen` 改回 80、停掉 WAF 即可，业务代码零改动。
- **HTTPS 可检测**：TLS 在 WAF 终止，注入、扫描、CC 全部可见；如果让 Apache 继续终止 TLS，WAF 只能看到密文，等于白做。

#### 3.5.1 调用链

```
客户端
  │  HTTP :80  /  HTTPS :443（TLS 在此终止）
  ▼
┌──────────────────────────────────────────────┐
│ Go-WAF                                        │
│  证书: /www/server/panel/vhost/cert/<域名>/   │
│  站点: Host → 127.0.0.1:8080（统一入口）      │
└───────────────────┬──────────────────────────┘
                    │ HTTP/1.1 明文，带 X-Forwarded-*
                    ▼
┌──────────────────────────────────────────────┐
│ Apache  127.0.0.1:8080                        │
│  按 ServerName 匹配的原有 vhost               │
│  ProxyPass /api → 127.0.0.1:8081（站点A 后端）│
│  ProxyPass /api → 127.0.0.1:8082（站点B 后端）│
│  DocumentRoot  → 前端 dist 静态资源           │
└───────────────────┬──────────────────────────┘
                    ▼
          各站点应用（8081 / 8082 / 8083）
```

> 注意：WAF **不直接对接** 8081/8082，而是统一转给 Apache。这样 Apache 的 vhost 分流逻辑、伪静态、静态资源缓存策略全部继续生效，WAF 的站点表只需要维护"域名 → Apache"这一层映射。

#### 3.5.2 端口与监听地址变更

| 组件 | 改造前 | 改造后 | 说明 |
| --- | --- | --- | --- |
| WAF HTTP | — | `0.0.0.0:80` | 公网入口，兼 ACME HTTP-01 验证 |
| WAF HTTPS | — | `0.0.0.0:443` | 公网入口，SNI 多证书 |
| Apache HTTP | `0.0.0.0:80` | `127.0.0.1:8080` | 只允许本机访问 |
| Apache HTTPS | `0.0.0.0:443` | **关闭** | 证书交给 WAF，避免端口冲突 |
| 站点后端 | `127.0.0.1:8081` 等 | 不变 | 保持现状 |
| 控制台 | — | `127.0.0.1:8899` | 不对公网暴露 |

Apache 侧需要改三处（宝塔环境下的具体路径见第 13 章）：

```apache
# 1) 主配置：监听地址后移
# /www/server/apache/conf/httpd.conf
Listen 127.0.0.1:8080
# 原来的 Listen 80 / Listen 443 全部注释掉

# 2) vhost：绑定地址跟着改，SSL 关闭
# /www/server/panel/vhost/apache/<域名>.conf
<VirtualHost 127.0.0.1:8080>
    ServerName www.example.com
    DocumentRoot /www/wwwroot/example/dist
    ProxyPass        /api  http://127.0.0.1:8081/api
    ProxyPassReverse /api  http://127.0.0.1:8081/api
    # SSLEngine on 及证书相关配置整段注释
</VirtualHost>

# 3) 真实 IP 与协议还原（关键）
SetEnvIf X-Forwarded-Proto "^https$" HTTPS=on
RemoteIPHeader X-Forwarded-For
RemoteIPTrustedProxy 127.0.0.1
LogFormat "%a %l %u %t \"%r\" %>s %b \"%{Referer}i\" \"%{User-Agent}i\" %D" combined
```

#### 3.5.3 必须同步处理的四个副作用

| 副作用 | 表现 | 处理方式 |
| --- | --- | --- |
| 后端取不到真实 IP | `$_SERVER['REMOTE_ADDR']` 全是 `127.0.0.1` | Apache 启用 `mod_remoteip`；PHP 侧读 `X-Forwarded-For` 第一段（需校验可信来源） |
| HTTPS 判断失效 | 后端生成 `http://` 链接、Cookie 丢 `Secure` | `SetEnvIf X-Forwarded-Proto "^https$" HTTPS=on`；框架层信任代理头 |
| 访问日志全是本机 | Apache access.log 无法溯源 | `LogFormat` 用 `%a` + `mod_remoteip`，或直接 `%{X-Forwarded-For}i` |
| 证书续期验证失败 | 宝塔 SSL 续期走 80 端口 HTTP-01 | WAF 对 `/.well-known/acme-challenge/*` **无条件放行并转发到 Apache**，不检测、不限流 |

#### 3.5.4 证书管理

- 证书继续由宝塔申请、续期，落在 `/www/server/panel/vhost/cert/<domain>/fullchain.pem` 与 `privkey.pem`；
- WAF 启动时扫描该目录，按域名加载证书到内存 map；
- 每 60 秒比对文件 `mtime`，变化则重新加载（宝塔续期后自动生效，无需重启）；
- 支持通配符证书；SNI 为空或未知域名时使用 `default_cert` 兜底；
- 若某域名没有证书，443 上的请求直接返回 421/404，**不要降级到 80 明文**。

#### 3.5.5 其他细节

- **HTTP/2**：Go 在 TLS 下默认协商 h2，对客户端无感；WAF 到 Apache 走 HTTP/1.1，行为正常。
- **WebSocket**：Apache 的 `mod_proxy_wstunnel` 继续生效，WAF 侧 `ReverseProxy` 自动处理 `Upgrade`，不需要额外配置。
- **大文件上传**：WAF 只检测前 256 KB，其余流式透传；Apache 的 `LimitRequestBody` 继续作为最终限制。
- **静态资源**：仍然由 Apache 直接返回（WAF 只做转发），避免把 WAF 变成静态文件服务器。
- **80 跳 443**：在 WAF 层统一做 301，不依赖 Apache 的 `RewriteRule`，减少一次回源。

### 3.6 为什么不用 mod_security

不是说 mod_security 不好，而是在这个具体场景里不合适：

1. **性能**：mod_security + CRS 全量规则下，Apache 的吞吐通常下降 30%～50%，而 Apache 本身已经在做 PHP/静态资源，压力不小。
2. **调优成本**：CRS 的误报需要逐条 `SecRuleRemoveById`，改完还要 reload，排查链路长。
3. **日志不可用**：审计日志是文本格式，做 TopN、趋势图、实时流都要额外解析。
4. **与宝塔冲突**：宝塔会重写 vhost 配置，手工加的自定义规则容易在面板操作后丢失。

Go-WAF 把检测从 Apache 进程里拿出来，Apache 只做它擅长的事（静态资源 + PHP-FPM），职责更清晰。

---

## 4. 功能清单

### 4.1 数据面（WAF 引擎）

| 编号 | 功能 | 说明 |
| --- | --- | --- |
| D-01 | 多站点 Host 路由 | `Host` 精确匹配 + 通配符 `*.example.com`，未匹配走默认站点或 404 |
| D-02 | 反向代理 | 支持 keep-alive 连接池、超时控制、Header 重写、WebSocket、SSE、分块传输 |
| D-03 | TLS 终止与 SNI | 多证书热加载，TLS 1.2+，HSTS 注入 |
| D-04 | 真实 IP 还原 | 可信代理链解析 `X-Forwarded-For`，防伪造 |
| D-05 | IP 黑名单 | 单 IP / CIDR / 网段，支持过期时间与备注 |
| D-06 | IP 白名单 | 优先级最高，命中直接放行且不计数 |
| D-07 | 临时封禁 | 自动封禁、手动封禁、递增惩罚、到期自动解封 |
| D-08 | 全站限流 | 保护后端的总 QPS 上限（令牌桶） |
| D-09 | 单 IP 限流 | 令牌桶 + 滑动窗口双算法，支持突发 |
| D-10 | 路径级限流 | 登录、短信、导出等敏感接口单独阈值 |
| D-11 | 人机校验 | JS Challenge + 签名 Cookie，通过后 N 分钟内免检 |
| D-12 | 规则引擎 | 关键词（AC 自动机）预筛 + RE2 正则 + 评分 |
| D-13 | 归一化 | 多次 URL 解码、HTML 实体、Unicode NFKC、去注释、全角转半角 |
| D-14 | 内置规则库 | SQLi / XSS / 穿越 / 命令注入 / 文件包含 / 反序列化 / Webshell |
| D-15 | 扫描器识别 | UA 指纹 + 敏感路径 + 请求头完整性 + 行为特征 |
| D-16 | 上传检测 | 扩展名黑白名单、双扩展名、Content-Type 与魔数比对、一句话特征 |
| D-17 | 协议校验 | 方法白名单、URL 长度、Header 大小、畸形请求 |
| D-18 | 响应检测 | 可选，敏感信息（身份证/手机号/密钥）命中仅记录 |
| D-19 | 事件日志 | 异步批量落库，含命中规则、评分、处置、耗时 |
| D-20 | 统计聚合 | 分钟级 QPS、拦截数、TopN IP/路径/类型 |
| D-21 | 健康检查 | `/__waf/health` 本地回环专用，供 systemd/负载均衡探活 |
| D-22 | 观察模式 | 全局或按站点，block 降级为 log |

### 4.2 控制面（Vue3 控制台）

| 编号 | 功能 | 说明 |
| --- | --- | --- |
| C-01 | 登录鉴权 | 用户名密码 + JWT（Authorization 头为主，HttpOnly Cookie 为辅）+ 登录失败限流 + 演示只读模式 |
| C-02 | 态势大屏 | QPS/拦截趋势、攻击类型占比、Top IP、Top 路径、实时事件流 |
| C-03 | 实时日志 | SSE 推送，支持站点/动作/类型/IP/路径/时间过滤，详情抽屉 |
| C-04 | 事件检索 | 分页查询、组合条件、导出 CSV |
| C-05 | IP 管理 | 黑白名单 CRUD、CIDR、批量导入、到期时间、一键封禁/解封 |
| C-06 | 规则管理 | 内置规则开关与调分、自定义规则 CRUD、启用/停用、优先级 |
| C-07 | 规则测试器 | 输入样例请求，实时显示命中规则与评分（上线前必备） |
| C-08 | 站点管理 | 域名、上游地址、超时、Body 检测开关、白名单路径 |
| C-09 | 限流配置 | 全局/IP/路径三级阈值，挑战或拦截策略 |
| C-10 | 系统设置 | 观察模式开关、日志保留、告警 Webhook、控制面 IP 白名单 |
| C-11 | 审计日志 | 记录"谁在何时改了什么，改前改后" |
| C-12 | 版本回滚 | 配置快照，一键回滚上一版本 |

### 4.3 优先级

| 优先级 | 范围 | 说明 |
| --- | --- | --- |
| **P0（MVP，2 周）** | D-01～D-06、D-11、D-13、D-19、C-01、C-03、C-05 | 能转发、能封 IP、能看日志 |
| **P1（核心，+2 周）** | D-07～D-10、D-12、D-14、D-15、D-20、C-02、C-04、C-06、C-08、C-09 | 完整 WAF 能力 + 大屏 |
| **P2（增强，+2 周）** | D-16～D-18、D-21、D-22、C-07、C-10～C-12 | 上传检测、观察模式、测试器、审计 |
| **P3（演进）** | GeoIP、JA3/JA4 指纹、规则自动学习、多实例共享封禁 | 视需求再做 |

---

## 5. 核心模块设计

### 5.1 站点路由

**匹配顺序**（从上到下，命中即止）：

1. `Host` 完全匹配（忽略大小写、去掉端口与末尾点）
2. 通配符匹配：`*.example.com`（只允许左侧一级或多级通配）
3. 默认站点（`default_site` 配置，未配置则返回 404）

**数据结构**：启动时把配置编译成两张表，避免每次请求遍历切片。

```go
type Router struct {
    exact   map[string]*Site   // "www.example.com" -> site
    wildcard []wildcardSite    // 按域名长度倒序，保证最具体优先
    def     *Site
}

func (r *Router) Match(host string) *Site {
    host = normalizeHost(host)          // 小写、去端口、去尾点
    if s, ok := r.exact[host]; ok {
        return s
    }
    for _, w := range r.wildcard {      // 后缀匹配
        if strings.HasSuffix(host, w.suffix) {
            return w.site
        }
    }
    return r.def
}
```

**坑点**：

- `Host` 头可能带端口（`example.com:8080`）或非法字符，必须校验：只允许 `[a-z0-9.-]`，长度 ≤ 253。
- HTTP/1.0 请求可能没有 `Host`，直接 400。
- 未匹配到站点的请求不要转发到默认后端，否则会被扫描器当成"任意 Host 都能打到业务"，也可能造成 Host 头攻击。

### 5.2 反向代理

基于 `httputil.ReverseProxy`，需要定制四处：

**① 请求重写（Rewrite）**

```go
Rewrite: func(pr *httputil.ProxyRequest) {
    pr.SetURL(site.Upstream)          // scheme + host + path 前缀
    pr.Out.Host = pr.In.Host          // 保留原始 Host，后端按域名分流时要用

    // 关键：先删掉客户端伪造的转发头，再写入可信值
    pr.Out.Header.Del("X-Forwarded-For")
    pr.Out.Header.Del("X-Real-IP")
    pr.Out.Header.Del("X-Forwarded-Proto")
    pr.Out.Header.Del("X-Forwarded-Host")

    pr.SetXForwarded()                // 写入真实 IP / Host / Proto
    pr.Out.Header.Set("X-Request-Id", reqID)
}
```

**② 连接池（Transport）**

```go
tr := &http.Transport{
    Proxy:                 nil,                 // 不走环境变量代理
    DialContext:           (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
    MaxIdleConns:          2048,
    MaxIdleConnsPerHost:   256,                 // 默认只有 2，必须调大
    MaxConnsPerHost:       0,                   // 不限制，靠上游自己保护
    IdleConnTimeout:       90 * time.Second,
    TLSHandshakeTimeout:   5 * time.Second,
    ExpectContinueTimeout: 1 * time.Second,
    ResponseHeaderTimeout: 30 * time.Second,    // 防后端挂死
    DisableCompression:    true,                // 透传 Accept-Encoding，交给后端压缩
    ForceAttemptHTTP2:     false,               // 上游是明文 HTTP/1.1
}
```

**③ 超时与流式**

- `FlushInterval: 100 * time.Millisecond`：让 SSE 和分块响应及时下发；纯大文件下载可设为 `-1`（立即 flush）。
- 不要给整个请求设 `WriteTimeout`（会掐断大文件），用 `ResponseHeaderTimeout` + `IdleTimeout` 控制。
- WebSocket 由 `ReverseProxy` 自动处理 `Upgrade`，但要求 `Transport` 支持 hijack（默认支持），且不要在外层包 gzip 中间件。

**④ 错误处理**

```go
ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
    if errors.Is(err, context.Canceled) {
        return // 客户端主动断开，不记 5xx
    }
    log.Warn("upstream error", "site", site.Name, "err", err, "req_id", reqID)
    w.Header().Set("X-WAF-Request-Id", reqID)
    http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
}
```

**请求体处理**（重点）：既要检测 body，又不能把 1 GB 上传读进内存。

```go
const maxInspect = 256 << 10 // 256KB

func bufferBody(r *http.Request, limit int64) ([]byte, error) {
    if r.Body == nil || r.ContentLength == 0 {
        return nil, nil
    }
    if r.ContentLength > 0 && r.ContentLength <= limit {
        buf := make([]byte, r.ContentLength)
        if _, err := io.ReadFull(r.Body, buf); err != nil {
            return nil, err
        }
        r.Body = io.NopCloser(bytes.NewReader(buf)) // 重置，供代理读取
        return buf, nil
    }

    // 超大 body：只读前 limit 字节用于检测，其余用 MultiReader 拼回去流式转发
    head := make([]byte, limit)
    n, err := io.ReadFull(r.Body, head)
    if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
        return nil, err
    }
    r.Body = struct {
        io.Reader
        io.Closer
    }{io.MultiReader(bytes.NewReader(head[:n]), r.Body), r.Body}
    r.ContentLength = -1          // 长度未知，Go 会改用 chunked
    r.TransferEncoding = nil
    return head[:n], nil
}
```

**坑点**：

- 读 body 之后必须重置 `r.Body`，否则代理转发出去的是空 body。
- `Content-Length` 与实际长度不一致会导致后端挂起，必须同步修正。
- `multipart/form-data` 的文件部分不要读进内存检测内容，只取文件名和 `Content-Type`，内容看前 512 字节魔数即可。

### 5.3 规则引擎：五段式流水线

```
① 提取目标  →  ② 归一化  →  ③ AC 预筛  →  ④ 正则确认  →  ⑤ 评分决策
```

**① 提取目标（Target）**

一个请求最多提取 6 类目标，每类可能有多个值：

| 目标 | 来源 | 说明 |
| --- | --- | --- |
| `path` | `URL.Path` + `RawPath` | 保留原始编码形式，同时提供解码形式 |
| `query` | `URL.Query()` 的每个 value，以及原始 `RawQuery` | 键名也参与检测（`?union=select`） |
| `body` | 表单值、JSON 字符串值、XML 文本、纯文本 | JSON 递归遍历，限制深度 8、节点数 2000 |
| `header` | 除 `Cookie`、`Authorization` 外的头 | `User-Agent`、`Referer`、`X-Forwarded-*` 重点 |
| `cookie` | 每个 Cookie 的 name + value | |
| `ua` | 单独抽出 | 用于扫描器指纹，避免与通用规则混在一起 |

**② 归一化**

攻击者绕过规则的第一手段就是编码。归一化必须做在匹配之前，且**保留原文**用于日志。

```go
var (
    sqlComment = regexp.MustCompile(`/\*.*?\*/`)
    spaceRe    = regexp.MustCompile(`[\s\x00]+`)
)

func Normalize(s string) string {
    // 1. 最多三次 URL 解码（应对 %252e%252e%252f）
    for i := 0; i < 3; i++ {
        d, err := url.QueryUnescape(s)
        if err != nil || d == s {
            break
        }
        s = d
    }
    // 2. HTML 实体（&#x65;&#108;）
    s = html.UnescapeString(s)
    // 3. Unicode 兼容归一（全角转半角、ﬁ -> fi）
    s = norm.NFKC.String(s)
    // 4. 去 SQL 注释（union/**/select）
    s = sqlComment.ReplaceAllString(s, "")
    // 5. 折叠空白与控制字符
    s = spaceRe.ReplaceAllString(s, " ")
    // 6. 转小写（RE2 的 (?i) 也可以，但统一小写更省）
    return strings.ToLower(strings.TrimSpace(s))
}
```

注意：

- `url.QueryUnescape` 会把 `+` 转成空格，对检测无影响（攻击 payload 里 `+` 通常是分隔）。
- 不要做"删除所有特殊字符"这种激进的归一化，会把正常内容变成攻击特征，制造误报。
- 中文站点要考虑 GBK 编码的 `%xx`，`QueryUnescape` 得到的是字节串，正则里用字节匹配即可，不要强转 UTF-8。

**③ AC 预筛（性能关键）**

如果每条规则都对每个目标跑正则，20 条规则 × 10 个目标 × 每请求 = 200 次正则，QPS 上千就顶不住。做法是：**把所有规则的字面关键词建成 Aho-Corasick 自动机，一次扫描找出候选规则，只对候选跑正则。**

```
规则库：
  sqli-001  keywords: ["union select", "union/**/select"]  regex: `union\s+select`
  sqli-002  keywords: ["sleep(", "benchmark("]             regex: `(sleep|benchmark)\s*\(`
  xss-001   keywords: ["<script", "onerror="]              regex: `<script[^>]*>`
  ...

请求 "id=1 union select 1,2":
  AC 扫描 → 命中 "union select" → 候选规则 [sqli-001]
  → 只跑 sqli-001 的正则 → 命中 → 评分 50
```

实测可以把正则调用次数降低 90% 以上。

**④ 正则确认**

- Go 的 `regexp` 是 RE2，线性时间、无回溯，天然免疫 ReDoS，可以放心用。
- 但**不要写 `.*` 嵌套**，虽然不会回溯爆炸，但会拖慢匹配。
- 每个规则的正则**在加载时编译一次**，绝不能在请求里 `regexp.MustCompile`。
- 正则统一加 `(?i)` 或依赖归一化后的小写文本，二选一，不要混用。

**⑤ 评分决策**

```go
type Hit struct {
    RuleID   string
    Category string
    Score    int
    Target   string // 命中的是 query 还是 body
    Evidence string // 截断后的证据，≤ 256 字节
}

func Decide(hits []Hit, cfg Threshold) (Action, int) {
    total := 0
    for _, h := range hits {
        total += h.Score
    }
    switch {
    case total >= cfg.BlockScore:      // 默认 60
        return ActionBlock, total
    case total >= cfg.ChallengeScore:  // 默认 30
        return ActionChallenge, total
    case total > 0:
        return ActionLog, total
    default:
        return ActionPass, 0
    }
}
```

评分建议（可按站点覆盖）：

| 规则类型 | 分值 | 说明 |
| --- | --- | --- |
| 反序列化 / RCE / Webshell 上传 | 80 | 单条即拦 |
| SQL 注入（联合查询、时间盲注） | 50 | 两条不同目标命中即拦 |
| 命令注入 / 文件包含 | 50 | |
| 路径穿越 | 40 | |
| XSS（script 标签、事件属性） | 35 | |
| 敏感文件探测（`/.env`） | 40 | |
| 扫描器 UA | 25 | 单独命中只记录或挑战 |
| 可疑 Header（缺 Accept、UA 过短） | 10 | 辅助分 |
| 高频访问 | 20 | 与内容分叠加 |

**去重**：同一规则在同一请求的多个目标命中，只计一次分，避免"一个 payload 出现在 query 和 referer 里被算两次"。

### 5.4 内置检测器设计

#### 5.4.1 SQL 注入

分三组规则，从强到弱：

```yaml
# 强特征：直接拦
- id: sqli-union
  pattern: 'union\s+(all\s+)?select'
  score: 50
- id: sqli-time
  pattern: '(sleep|benchmark|pg_sleep)\s*\(\s*\d'
  score: 60
- id: sqli-meta
  pattern: '(information_schema|sys\.tables|load_file\s*\(|into\s+outfile)'
  score: 55
# 中特征：需组合
- id: sqli-tautology
  pattern: "('|\")\\s*(or|and)\\s+('|\")?\\d+('|\")?\\s*=\\s*('|\")?\\d+"
  score: 35
- id: sqli-comment
  pattern: "('|\")\\s*(--|#|;)"
  score: 20
```

误报控制：`order by`、`group by` 这类词不要单独成规则（正常业务会用）；`'` 单引号不要单独成规则（英文作文、代码片段会带）。**必须组合多个特征才给高分**。

#### 5.4.2 XSS

```yaml
- id: xss-script
  pattern: '<\s*script[^>]*>'
  score: 40
- id: xss-event
  pattern: '<[^>]+\son(error|load|click|mouseover|focus)\s*='
  score: 40
- id: xss-protocol
  pattern: '(javascript|vbscript|data:text/html)\s*:'
  score: 35
- id: xss-danger
  pattern: '(document\.cookie|window\.location|eval\s*\(|atob\s*\()'
  score: 25
```

注意：如果站点有富文本编辑器、Markdown 预览、代码高亮，`<script>` 可能是**正常内容**（比如教程站、博客评论带代码块）。这类路径必须配白名单或降级为 `log`。

#### 5.4.3 路径穿越 / 文件包含

```yaml
- id: trav-1
  pattern: '(\.\./){2,}|(\.\.\\){2,}'
  score: 45
- id: trav-enc
  pattern: '(%2e%2e%2f|%252e%252e%252f|\.\.%2f|%c0%ae%c0%ae)'
  score: 50
- id: lfi-php
  pattern: '(php|file|phar|zip|data)://'
  score: 50
- id: rfi
  pattern: '^(https?|ftp)://[^/]+\.(txt|jpg|gif)\?'
  score: 40
```

归一化后 `%2e%2e%2f` 会变成 `../`，所以第一组规则就能覆盖大部分；保留编码规则是为了应对"只解码一次"的漏网场景。

#### 5.4.4 命令注入

```yaml
- id: cmdi-1
  pattern: '[;|`]\s*(cat|ls|whoami|id|uname|curl|wget|nc|bash|sh|python|perl)\b'
  score: 55
- id: cmdi-2
  pattern: '\$\((cat|ls|whoami|id|uname|curl|wget)\b'
  score: 55
- id: cmdi-3
  pattern: '(;|\|)&?\s*(ping|telnet|nc)\s+-'
  score: 45
```

误报点：搜索框里输入 `a|b`、代码分享站点。这类路径建议只记录。

#### 5.4.5 扫描器与恶意 Bot

**UA 指纹**（维护一个约 200 条的黑名单，支持子串匹配）：

```
sqlmap, nikto, nmap, masscan, zgrab, zmap, dirbuster, gobuster, feroxbuster,
wpscan, joomscan, droopescan, whatweb, wafw00f, xray, awvs, appscan, nestat,
acunetix, netsparker, burpsuite, hydra, medusa, patator, ffuf, dirsearch,
nuclei, fscan, goby, vulmap, pocsuite, ysoserial, metasploit, cobaltstrike
```

**敏感路径库**（前缀或正则，约 300 条）：

```
/.env  /.env.bak  /.git/config  /.svn/  /.DS_Store  /web.config  /config.php.bak
/phpinfo.php  /info.php  /test.php  /shell.php  /admin.php  /wp-login.php
/wp-admin/  /phpmyadmin/  /pma/  /adminer.php  /actuator/env  /actuator/heapdump
/druid/index.html  /swagger-ui.html  /v2/api-docs  /console/  /jmx-console/
/manager/html  /solr/  /elasticsearch/  /_cat/indices  /vendor/phpunit/
/server-status  /backup.zip  /www.zip  /db.sql  /dump.sql
```

**行为特征**：

- 单 IP 在 60 秒内请求 ≥ 30 个不同的 404 路径 → 扫描；
- 单 IP 请求路径的"熵"很高（随机字符串）→ 目录爆破；
- UA 为空、UA 长度 < 10、UA 与 `Accept` 不匹配（声称 Chrome 却没有 `Accept-Language`）→ 可疑；
- 请求方法为 `PUT`/`DELETE`/`TRACE` 且路径是静态资源 → 可疑。

处置：单独命中 UA 只记 25 分；叠加敏感路径（40 分）→ 65 分 → 拦截。**不要只凭 UA 就拦**，UA 可以随便伪造，也会误伤一些老旧的采集器。

#### 5.4.6 上传检测

只对 `multipart/form-data` 且路径命中上传白名单的请求生效：

1. **扩展名**：取最后一个 `.` 后的部分，同时检查双扩展名（`shell.php.jpg`）和空字节（`shell.php%00.jpg`）。
2. **黑名单**：`php, php3, php5, phtml, jsp, jspx, asp, aspx, exe, sh, py, pl, cgi`。
3. **Content-Type 与魔数比对**：声明 `image/jpeg` 但前两字节不是 `FF D8` → 可疑。
4. **一句话特征**：文件前 4KB 内出现 `eval($_POST`、`assert($_REQUEST`、`base64_decode(` + `system(` 等组合。
5. **图片马**：`GIF89a` 开头但中间含 `<?php` → 高分拦截。

注意：图片站、附件站会有大量正常上传，建议上传检测只做"记录 + 可疑时阻断"，并把规则阈值调高。

### 5.5 CC 防护与限流

三层结构，从粗到细：

```
第一层：全站令牌桶     保护后端总容量（如 5000 QPS）
第二层：单 IP 令牌桶   防单点高频（如 30 r/s，burst 60）
第三层：路径滑动窗口   防集中打某个接口（如 /api/auth/login 10 次/分）
```

**令牌桶**用 `golang.org/x/time/rate`，每个 IP 一个 limiter，用分片 map + LRU 清理：

```go
type shard struct {
    mu sync.Mutex
    m  map[string]*entry
}
type entry struct {
    lim  *rate.Limiter
    last time.Time
}

type IPLimiter struct {
    shards [16]shard
    r      rate.Limit
    burst  int
}

func (l *IPLimiter) Allow(ip string) bool {
    s := &l.shards[fnv32(ip)%16]
    s.mu.Lock()
    e, ok := s.m[ip]
    if !ok {
        e = &entry{lim: rate.NewLimiter(l.r, l.burst)}
        s.m[ip] = e
    }
    e.last = time.Now()
    ok = e.lim.Allow()
    s.mu.Unlock()
    return ok
}
```

清理协程每 60 秒扫一遍，删除 10 分钟未活动的 entry，防止 map 无限增长（这是内存泄漏最常见的原因）。

**滑动窗口**用于"每分钟 N 次"这种语义，用当前窗口 + 上一窗口加权：

```go
type slidingWindow struct {
    curStart time.Time
    cur, prev int64
}

func (w *slidingWindow) Add(now time.Time, win time.Duration) int64 {
    if now.Sub(w.curStart) >= win {
        if now.Sub(w.curStart) < 2*win {
            w.prev = w.cur
        } else {
            w.prev = 0
        }
        w.cur = 0
        w.curStart = now
    }
    w.cur++
    // 加权：上一窗口剩余权重
    elapsed := now.Sub(w.curStart).Seconds()
    weight := 1 - elapsed/win.Seconds()
    if weight < 0 {
        weight = 0
    }
    return w.cur + int64(float64(w.prev)*weight)
}
```

**分级处置**（阈值可按站点配置）：

| 条件（60 秒内） | 动作 |
| --- | --- |
| > 300 次 | 返回 JS Challenge |
| > 600 次 | 封禁 5 分钟 |
| > 1200 次 | 封禁 1 小时 |
| 解封后再次触发 | 惩罚时长 ×2（5m → 15m → 1h → 6h → 24h） |
| 24 小时内触发 5 次 | 封禁 7 天，并推送告警 |

**不计入限流的请求**：

- 白名单 IP
- 静态资源（`/static/*`、`.js`、`.css`、`.png` 等），或按权重 0.1 计
- 健康检查
- 已通过 Challenge 的 Cookie（有效期内）

### 5.6 人机校验（JS Challenge）

用于 CC 场景：直接封 IP 容易误伤 NAT 后的正常用户（公司出口、校园网），返回一个挑战页更温和。

流程：

1. WAF 返回 200，页面内嵌一段 JS，计算 `SHA256(challenge + nonce)` 使结果前 4 位为 0（约 16 次尝试，用户无感）；
2. JS 把结果 POST 到 `/__waf/challenge`；
3. WAF 校验通过后，下发签名 Cookie：`waf_pass=<ts>.<hmac(ts|ip|ua)>`，有效期 30 分钟；
4. 后续请求带此 Cookie 且签名有效 → 跳过 CC 检查（但仍做内容检测）。

注意：

- 签名必须绑定 IP 和 UA，防止 Cookie 被复制；
- 不要对 API 请求返回 HTML 挑战页（前端 axios 会解析失败），应返回 `429` + `Retry-After`，由前端提示；
- 爬虫/SEO 场景要放行已知搜索引擎（Googlebot/Baiduspider），并做 **rDNS 验证**，不能只看 UA。

### 5.7 IP 名单与自动封禁

**匹配结构**：单 IP 用 map，CIDR 用前缀树（或简单的有序切片 + 二分），规模在几千条时切片足够。

```go
type IPMatcher struct {
    exact map[uint32]struct{}
    nets  []net.IPNet
}

func (m *IPMatcher) Contains(ip net.IP) bool {
    if v4 := ip.To4(); v4 != nil {
        if _, ok := m.exact[binary.BigEndian.Uint32(v4)]; ok {
            return true
        }
    }
    for i := range m.nets {
        if m.nets[i].Contains(ip) {
            return true
        }
    }
    return false
}
```

**封禁来源**：

| 来源 | 存储 | 生效方式 |
| --- | --- | --- |
| 手动黑名单 | `ip_rules` 表 | 内存快照，秒级 |
| 手动白名单 | `ip_rules` 表 | 优先级最高 |
| 自动封禁 | `bans` 表 + 内存 | 定时器到期自动删除 |
| 规则触发 | 内存计数 | 达到阈值写入 `bans` |

**防止把自己封了**：

- 控制台登录 IP 自动加入白名单（可关闭）；
- 白名单 IP 永不参与限流和自动封禁；
- 一键" panic 按钮"：清空所有临时封禁（控制台醒目位置）。

### 5.8 日志管道

**核心原则：日志绝不能阻塞转发。** 采用"有界 channel + 批量写 + 丢弃计数"。

```
请求 ──► eventCh (cap 8192) ──► writer goroutine ──► 批量 INSERT ──► SQLite
              │
              └─ 满了？丢弃 + dropped++（绝不阻塞）
```

**记录策略**（全量记录会撑爆磁盘）：

| 请求类型 | 是否入库 |
| --- | --- |
| `block` / `challenge` / `ban` | 100% 记录 |
| `log`（低分命中） | 100% 记录 |
| 正常请求（5xx） | 100% 记录 |
| 正常请求（2xx/3xx） | 按采样率记录（默认 1%，可按站点调） |
| 静态资源 | 不记录（只计入统计） |

**事件字段**：

```go
type Event struct {
    ID         int64     `json:"id"`
    Ts         time.Time `json:"ts"`
    Site       string    `json:"site"`
    ClientIP   string    `json:"client_ip"`
    Method     string    `json:"method"`
    Host       string    `json:"host"`
    Path       string    `json:"path"`
    Query      string    `json:"query"`
    Status     int       `json:"status"`
    Action     string    `json:"action"`
    Score      int       `json:"score"`
    Categories []string  `json:"categories"`
    RuleIDs    []string  `json:"rule_ids"`
    UA         string    `json:"ua"`
    Referer    string    `json:"referer"`
    ReqID      string    `json:"req_id"`
    LatencyUS  int64     `json:"latency_us"`
    Upstream   string    `json:"upstream"`
    Country    string    `json:"country,omitempty"`
    Bytes      int64     `json:"bytes"`
}
```

**脱敏**：`password`、`token`、`secret`、`authorization`、`cookie` 的值在入库前替换为 `***`；query 中的 `phone`、`id_card` 做掩码。

**批量写入**：

```go
func (s *Store) loop(ctx context.Context) {
    batch := make([]*Event, 0, 256)
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()

    flush := func() {
        if len(batch) == 0 {
            return
        }
        if err := s.insertBatch(batch); err != nil {
            log.Error("insert events", "err", err, "n", len(batch))
        }
        batch = batch[:0]
    }

    for {
        select {
        case ev := <-s.ch:
            batch = append(batch, ev)
            if len(batch) >= 256 {
                flush()
            }
        case <-ticker.C:
            flush()
        case <-ctx.Done():
            flush()
            return
        }
    }
}
```

### 5.9 统计聚合

控制台的大屏不能每次都 `SELECT COUNT(*) FROM events WHERE ts > ...`，几百万行会卡死。做法是**内存聚合 + 分钟级落库**。

```go
type MinuteStat struct {
    Minute   int64            `json:"minute"`
    Site     string           `json:"site"`
    Total    int64            `json:"total"`
    Blocked  int64            `json:"blocked"`
    Challenged int64          `json:"challenged"`
    Passed   int64            `json:"passed"`
    Status5xx int64           `json:"status_5xx"`
    Categories map[string]int64 `json:"categories"`
    TopIPs   map[string]int64 `json:"top_ips"`
    TopPaths map[string]int64 `json:"top_paths"`
}
```

- 每个站点每分钟一个结构，用 `sync.Mutex` 保护（写入频率高但临界区极小）；
- 内存中保留最近 6 小时（360 个点），足够大屏展示；
- 每 60 秒把上一分钟的数据 upsert 到 `stats_1m` 表；
- TopN 的 map 每 10 分钟清空一次，防止长尾 key 撑爆内存（或限制最多 1000 个 key，超出丢弃）。

### 5.10 配置中心与热更新

**配置分三层**：

| 层 | 内容 | 变更频率 | 存储 |
| --- | --- | --- | --- |
| 静态配置 | 监听地址、证书目录、数据库路径 | 极低 | YAML 文件，重启生效 |
| 业务配置 | 站点、规则、阈值、白名单 | 中 | SQLite，热更新 |
| 运行时状态 | 封禁列表、计数、统计 | 高 | 内存 + 定期持久化 |

**热更新实现**：不可变快照 + 原子指针。

```go
type Snapshot struct {
    Version  int64
    Sites    map[string]*Site
    Rules    *RuleSet
    IPBlack  *IPMatcher
    IPWhite  *IPMatcher
    Settings Settings
}

type Manager struct {
    cur atomic.Pointer[Snapshot]
    mu  sync.Mutex // 只保护"构建新快照"的过程
}

func (m *Manager) Load() *Snapshot { return m.cur.Load() }

func (m *Manager) Reload(ctx context.Context) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    next, err := buildSnapshot(ctx) // 从 DB 读、编译规则、构建 matcher
    if err != nil {
        return err // 构建失败保留旧快照，绝不能置空
    }
    next.Version = m.cur.Load().Version + 1
    m.cur.Store(next)
    return nil
}
```

请求处理开始时 `snap := mgr.Load()` 取一次，整个请求用同一个快照，保证一致性；即使中途配置更新，也不会出现"规则匹配到一半换了规则集"的问题。旧快照由 GC 回收。

**触发方式**：

- 控制面修改配置后直接调 `Reload()`（同进程，同步生效）；
- 数据库外部变更（比如直接改 SQLite）→ 每 30 秒检查 `settings.version`，变化则 reload；
- 规则文件变更 → `fsnotify` 监听，失败重试 3 次并告警。

### 5.11 控制面技术栈与鉴权

#### 5.11.1 技术栈

| 关注点 | 选型 | 说明 |
| --- | --- | --- |
| Web 框架 | **Gin** | 路由分组、参数绑定、中间件链成熟 |
| ORM | **GORM v2** | 只用在配置类表，见 6.1 的边界说明 |
| SQLite 驱动 | **`github.com/glebarez/sqlite`** | 纯 Go，GORM 接口兼容 |
| JWT | `golang-jwt/jwt/v5`（HS256） | access + refresh 双令牌 |
| 密码 | `golang.org/x/crypto/bcrypt`（cost=12） | |
| 参数校验 | Gin binding tag（`binding:"required,min=3"`） | 不额外引 validator |
| API 文档 | 手写 Markdown（第 7 章）+ 可选 swag | 单人项目不必上 Swagger UI |

**驱动必须选纯 Go 版**：GORM 官方的 `gorm.io/driver/sqlite` 底层是 `mattn/go-sqlite3`，**需要 CGO**。一旦引入，`CGO_ENABLED=0` 交叉编译直接失效，产物还会绑定目标机 glibc 版本。`glebarez/sqlite` 基于 `modernc.org/sqlite`，同一套 GORM 接口，代价是写入略慢（本项目配置写入是低频操作，无影响）。

```go
import (
    "github.com/glebarez/sqlite"
    "gorm.io/gorm"
)

db, err := gorm.Open(sqlite.Open("file:/www/server/gowaf/data/waf.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"), &gorm.Config{
    Logger: logger.Default.LogMode(logger.Silent), // 不要打 SQL 到 stdout，会拖慢并泄漏参数
    NowFunc: func() time.Time { return time.Now().UTC() },
})
```

#### 5.11.2 双令牌与双通道

| 令牌 | 有效期 | 载体 | 存储 | 吊销 |
| --- | --- | --- | --- | --- |
| access token | 2h | `Authorization: <token>` | 前端 Pinia + localStorage | 靠短有效期自然失效 |
| refresh token | 7d | 请求体 / HttpOnly Cookie | `sys_refresh_tokens` 表 | 落库，可立即吊销 |

**两种凭证通道，职责分开**：

| 通道 | 载体 | 用于 | CSRF |
| --- | --- | --- | --- |
| 主 | `Authorization: <access_token>`（**裸 token，不带 `Bearer ` 前缀**） | 控制台全部 XHR 请求 | 不需要 |
| 备 | `waf_token` HttpOnly Cookie | SSE、文件直链下载、浏览器直接打开 | 强制校验 `X-CSRF-Token` |

**为什么带 Authorization 头就不需要 CSRF**：浏览器不会自动为跨站请求附加自定义头，攻击者也无法伪造——这类请求天然免疫 CSRF。只有"浏览器会自动带上 Cookie"的请求才需要双提交校验。所以 `CSRF` 中间件只对**凭 Cookie 认证**的写请求生效。

> 这里推翻本文档早期版本"JWT 一律放 HttpOnly Cookie"的结论。原因是前端基线（`Frontend/`，Fssadmin）把 token 存在 Pinia + localStorage，登录后所有请求走 `Authorization` 头。强行改成 Cookie 要动前端请求层、401 逻辑、多标签页同步三处。取舍：保留 `Authorization` 头为主通道，**用 CSP + 严格的输出转义补偿 XSS 风险**（见 11 章），同时保留 Cookie 通道给 SSE/下载用。

**登录防爆破**：按 `IP + 用户名` 双维度计数，5 次失败锁 15 分钟，锁定期直接 `429`（不返回"密码错误"，避免用户名枚举）。

**中间件顺序**（顺序不可随意调）：

```
Recover → RequestID → AccessLog → CORS → IPAllowlist
        → LoginRateLimit(仅 /api/auth/login)
        → Auth(JWT) → CSRF → DemoGuard → Audit
        → Handler
```

- `CORS` 必须早于 `Auth`，否则预检请求 `OPTIONS` 会被鉴权拦成 401，浏览器报跨域错误却看不到真实原因；
- `DemoGuard` 必须在 `Audit` 之前，演示模式下的越权尝试也要留痕；
- `IPAllowlist` 在鉴权之前，非白名单 IP 连 token 都不解析。


### 5.12 前端控制台

**不新建前端工程**，直接在仓库里已有的 `Frontend/`（Fssadmin，naive-ui 版）上扩展业务模块。基线盘点：

| 能力 | 现状 | 对 WAF 模块的意义 |
| --- | --- | --- |
| 框架 | Vue 3.5 + `<script setup>` + TS | 直接写业务 |
| 构建 | Vite 8 + pnpm | 已有 `dev`/`build` |
| UI | **naive-ui v2**（禁止 Element Plus / Ant Design Vue） | 表格、表单、抽屉、弹窗齐全 |
| 图标 | `@vicons/antd` + Iconify 离线集 | 不额外引图标包 |
| 状态 | Pinia 4 + `persist` 插件 | 用户态、SSE 连接、主题 |
| 路由 | vue-router 5，`src/router/modules/*.ts` 分模块 | 新增 `waf.ts` 一个文件即接入 |
| 图表 | **ECharts 6**（已装） | 大屏、趋势、环形图 |
| HTTP | `src/utils/http`（axios 封装） | 响应结构 `{code,msg,data}`，401 自动登出 |
| 国际化 | vue-i18n 11，`zh.json` / `en.json` | 新增文案两份都要补 |
| 样式 | Tailwind 4 + SCSS，主题变量 `--app-*` | 不写死色值 |
| 权限模式 | `VITE_ACCESS_MODE = frontend`（前端定义路由） | 后端只返回 `roles` / `buttons`，不下发菜单 |

**扩展原则**（避免把框架改烂）：

1. **只加不改**：WAF 相关代码集中在 `src/views/waf/`、`src/api/waf/`、`src/router/modules/waf.ts`，不改 `src/router/core/*`、`src/utils/http/*`、`src/store/modules/user.ts` 等框架内核。
2. **沿用既有约定**：组件命名 `ArtXxx`，目录 `src/components/core/<分类>/art-xxx/index.vue`；提示统一用 `@/utils/ui/feedback`，不要直接 `import { message } from 'naive-ui'`。
3. **接口层按 `{url, params}` 风格**：与现有 `src/api/auth.ts` 保持一致，不自造一套 fetch 封装。
4. **前端不负责构建**：`dist/` 由维护者本地 `pnpm build` 产出，后端 `go:embed` 只消费产物（见 9.6）。

**已有接口契约必须原样对齐**（`src/api/auth.ts` 已经写死，改后端比改前端便宜）：

| 用途 | 前端已调用的路径 | 请求 | 期望响应 `data` |
| --- | --- | --- | --- |
| 登录 | `POST /api/auth/login` | `{userName, password}` | `{token, refreshToken}` |
| 当前用户 | `GET /api/user/info` | — | `{userId, userName, email, avatar, roles[], buttons[]}` |
| 文件上传 | `POST /api/upload/image` | `FormData` | `{url}` |
| 用户下拉 | `GET /api/user/list` | `{current,size,userName}` | `{records[], total}` |


**打包进二进制**：

```go
//go:embed all:dist
var webFS embed.FS

func mountAdmin(r *gin.Engine) {
    sub, _ := fs.Sub(webFS, "dist")
    r.NoRoute(func(c *gin.Context) {
        if strings.HasPrefix(c.Request.URL.Path, "/api/") {
            c.JSON(404, gin.H{"code": 404, "msg": "not found"})
            return
        }
        // SPA fallback：非静态资源一律返回 index.html
        c.FileFromFS("index.html", http.FS(sub))
    })
    r.StaticFS("/assets", http.FS(sub))
}
```

**注意 `go:embed` 的路径限制**：embed 只能引用**当前包目录及其子目录**，不允许 `..`。而前端产物在仓库根的 `Frontend/dist`，不在 `internal/web/` 下。所以构建时先同步一份：

```makefile
web:
	cd Frontend && pnpm install --ignore-workspace && pnpm build
	rm -rf internal/web/dist && cp -r Frontend/dist internal/web/dist
```

`internal/web/dist` 加进 `.gitignore`（产物不入库），CI 顺序固定为 `pnpm build → 拷贝 → go build`。

### 5.13 演示模式（Demo Mode）

**目标**：把面板临时挂到公网给别人看效果时，**任何人都只能看、不能改**，并且看不到真实的站点域名与攻击 IP。

**配置**：

```yaml
demo:
  enabled: true
  db_path: "/www/server/gowaf/data/demo.db"    # 与生产库物理隔离
  username: "demo"
  password: "demo123"                          # 仅首次建库时写入
  read_only: true                              # 写操作一律 403
  mask_ips: true                               # 列表里真实 IP 打码
  reset_cron: "0 4 * * *"                      # 每天 04:00 重置演示库
  banner: "演示环境：数据为模拟数据，修改操作已禁用"
```

**三条硬隔离**（缺一条就等于没做）：

| 隔离项 | 做法 | 校验点 |
| --- | --- | --- |
| 数据库 | 强制使用 `demo.db`，与生产 `storage.dsn` 物理隔离 | 启动时比对绝对路径，相同则**拒绝启动** |
| 写操作 | `DemoGuard` 中间件拦截所有非 `GET/HEAD/OPTIONS` 请求 | 白名单之外一律 `403 {"code":40302}` |
| 数据 | 启动时灌 `seed/demo.sql`：站点、规则、2000 条事件 | IP 全部取自 `198.51.100.0/24`（RFC 5737 文档专用段），Host 用 `demo.example.com`，不含任何真实地址 |

**白名单**（演示模式下仍放行，因为它们不改数据）：

- `POST /api/auth/login`、`POST /api/auth/logout`、`POST /api/auth/refresh`
- `POST /api/rules/test` —— 纯计算，不落库

**前端配合**：

- 登录页展示演示账号并支持一键填充；
- 顶栏挂「演示模式」徽章（`n-tag type="warning"`），文案取自 `demo.banner`；
- 所有写操作按钮 `disabled` + `n-tooltip` 提示，**不要等点了才报错**；
- 该状态由 `GET /api/system/info` 返回的 `{demo:true}` 驱动，缓存在 `settingStore`。

**为什么值得做**：实现成本约半天，换来的是"敢把链接发出去"。否则演示要么暴露真实拓扑，要么得临时改防火墙。


---

## 6. 数据模型

### 6.1 存储选型

| 候选 | 优势 | 问题 | 结论 |
| --- | --- | --- | --- |
| SQLite | 零依赖、单文件、WAL 并发读、备份就是拷文件 | 单写者，高频写入需批量 | **选它** |
| MySQL | 运维熟悉、并发强 | 多一个服务要维护，宝塔里还要建库授权 | 不值得 |
| PostgreSQL | 功能强 | 同上，且更重 | 不值得 |
| 纯文件 JSONL | 最简单 | 查询、分页、聚合都要自己写 | 只用于冷归档 |

单机 WAF 的写入量：按 5000 QPS、1% 采样 + 全部拦截记录，约 50～200 条/秒。SQLite 在 WAL + 批量事务下可以轻松做到 1 万条/秒以上，完全够用。

关键 PRAGMA：

```sql
PRAGMA journal_mode = WAL;      -- 读写不互斥
PRAGMA synchronous = NORMAL;    -- 兼顾安全与性能（断电可能丢最后 1 秒日志，可接受）
PRAGMA busy_timeout = 5000;     -- 避免 "database is locked" 直接报错
PRAGMA temp_store = MEMORY;
PRAGMA cache_size = -32000;     -- 32MB 页缓存
```

另外：**只开一个写连接**（`db.SetMaxOpenConns(1)` 对写库），读连接单独开一个只读池，避免写事务把读请求堵住。

**GORM 的使用边界**（决定性能上限，必须分清楚）：

| 表 | 访问方式 | 理由 |
| --- | --- | --- |
| `sys_users` / `sys_refresh_tokens` / `sites` / `rules` / `ip_rules` / `audit_logs` / `settings` | **GORM**（`db.Create` / `Updates` / `Find`） | 低频、字段多、要事务与钩子 |
| `events` / `stats_1m` / `bans` 热路径 | **原生 SQL**（`db.Exec`、批量 `INSERT`） | 每秒数百行，ORM 的反射与钩子开销没有意义 |

两个连接池并存：GORM 实例承担写（`SetMaxOpenConns(1)`），只读统计与事件查询走独立的 `*sql.DB`（DSN 加 `?mode=ro`）。

**迁移策略**：`AutoMigrate` 只在**首次建库**时用；之后所有结构变更走 `migrations/NNN_xxx.sql`，由 `schema_meta` 记录已执行版本。原因：SQLite 改列是"建新表 + 拷数据 + 换名"，`AutoMigrate` 对索引重建、默认值、`NOT NULL` 加列处理得并不可靠，线上跑一次就可能丢数据。

### 6.2 表结构（DDL）

```sql
-- 元信息与版本
CREATE TABLE IF NOT EXISTS schema_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- 管理用户（对应 GORM model.User，前端 /api/user/info 直接映射）
CREATE TABLE IF NOT EXISTS sys_users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    user_name     TEXT    NOT NULL UNIQUE,
    password_hash TEXT    NOT NULL,           -- bcrypt cost=12
    nick_name     TEXT,
    email         TEXT,
    avatar        TEXT,
    role          TEXT    NOT NULL DEFAULT 'admin',  -- admin / viewer / demo
    buttons       TEXT,                        -- JSON 数组，前端按钮级权限
    status        INTEGER NOT NULL DEFAULT 1,  -- 1 启用 / 0 禁用
    last_login_at INTEGER,
    last_login_ip TEXT,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL,
    deleted_at    INTEGER                      -- 软删除（GORM gorm.DeletedAt）
);
CREATE INDEX IF NOT EXISTS idx_sys_users_name ON sys_users(user_name);

-- 刷新令牌（可吊销，登录/刷新都在这里留痕）
CREATE TABLE IF NOT EXISTS sys_refresh_tokens (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL,
    jti        TEXT    NOT NULL UNIQUE,
    expires_at INTEGER NOT NULL,
    revoked_at INTEGER,
    client_ip  TEXT,
    user_agent TEXT,
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_refresh_user ON sys_refresh_tokens(user_id, expires_at);

-- 站点
CREATE TABLE IF NOT EXISTS sites (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL,
    hosts           TEXT NOT NULL,          -- JSON: ["www.a.com","a.com"]
    upstream        TEXT NOT NULL,          -- http://127.0.0.1:8080
    enabled         INTEGER NOT NULL DEFAULT 1,
    detect_body     INTEGER NOT NULL DEFAULT 1,
    max_body_bytes  INTEGER NOT NULL DEFAULT 262144,
    whitelist_paths TEXT,                   -- JSON: ["/api/pay/notify","/static/*"]
    rate_limit      TEXT,                   -- JSON: {"rps":30,"burst":60,"per_minute":600}
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
);

-- 规则（内置 + 自定义）
CREATE TABLE IF NOT EXISTS rules (
    id         TEXT PRIMARY KEY,            -- sqli-union / custom-1712...
    name       TEXT NOT NULL,
    category   TEXT NOT NULL,               -- sqli/xss/traversal/cmdi/scanner/upload/custom
    score      INTEGER NOT NULL DEFAULT 30,
    targets    TEXT NOT NULL,               -- "path,query,body,header,cookie,ua"
    keywords   TEXT,                        -- JSON 数组，AC 预筛用，可为空
    pattern    TEXT,                        -- RE2 正则，可为空（纯关键词规则）
    action     TEXT NOT NULL DEFAULT 'block', -- block/challenge/log
    enabled    INTEGER NOT NULL DEFAULT 1,
    builtin    INTEGER NOT NULL DEFAULT 1,
    priority   INTEGER NOT NULL DEFAULT 100,
    remark     TEXT,
    updated_at INTEGER NOT NULL
);

-- IP 黑白名单
CREATE TABLE IF NOT EXISTS ip_rules (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    ip         TEXT NOT NULL,               -- 单 IP 或 CIDR
    kind       TEXT NOT NULL,               -- black / white
    scope      TEXT NOT NULL DEFAULT 'global', -- global 或站点名
    reason     TEXT,
    expires_at INTEGER NOT NULL DEFAULT 0,  -- 0 = 永久
    created_by TEXT,
    created_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_ip_rules ON ip_rules(ip, kind, scope);

-- 封禁（自动 + 手动）
CREATE TABLE IF NOT EXISTS bans (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    ip         TEXT NOT NULL,
    reason     TEXT,
    source     TEXT NOT NULL,               -- auto / manual / rule
    hit_count  INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    released_at INTEGER
);
CREATE INDEX IF NOT EXISTS idx_bans_ip ON bans(ip, expires_at);
CREATE INDEX IF NOT EXISTS idx_bans_active ON bans(expires_at) WHERE released_at IS NULL;

-- 事件日志（热数据）
CREATE TABLE IF NOT EXISTS events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    ts         INTEGER NOT NULL,            -- 毫秒时间戳
    site       TEXT NOT NULL,
    client_ip  TEXT NOT NULL,
    method     TEXT NOT NULL,
    host       TEXT NOT NULL,
    path       TEXT NOT NULL,
    query      TEXT,
    status     INTEGER NOT NULL,
    action     TEXT NOT NULL,
    score      INTEGER NOT NULL DEFAULT 0,
    categories TEXT,                        -- JSON 数组
    rule_ids   TEXT,                        -- JSON 数组
    ua         TEXT,
    referer    TEXT,
    req_id     TEXT NOT NULL,
    latency_us INTEGER,
    upstream   TEXT,
    country    TEXT,
    bytes      INTEGER
);
CREATE INDEX IF NOT EXISTS idx_events_ts       ON events(ts DESC);
CREATE INDEX IF NOT EXISTS idx_events_ip_ts    ON events(client_ip, ts DESC);
CREATE INDEX IF NOT EXISTS idx_events_action   ON events(action, ts DESC);
CREATE INDEX IF NOT EXISTS idx_events_site_ts  ON events(site, ts DESC);

-- 分钟级统计
CREATE TABLE IF NOT EXISTS stats_1m (
    minute     INTEGER NOT NULL,
    site       TEXT NOT NULL,
    total      INTEGER NOT NULL DEFAULT 0,
    passed     INTEGER NOT NULL DEFAULT 0,
    blocked    INTEGER NOT NULL DEFAULT 0,
    challenged INTEGER NOT NULL DEFAULT 0,
    status_5xx INTEGER NOT NULL DEFAULT 0,
    categories TEXT,                        -- JSON: {"sqli":12,"xss":3}
    top_ips    TEXT,                        -- JSON: {"1.2.3.4":10}
    top_paths  TEXT,                        -- JSON: {"/api/login":8}
    PRIMARY KEY (minute, site)
);

-- 审计日志
CREATE TABLE IF NOT EXISTS audit_logs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    ts         INTEGER NOT NULL,
    operator   TEXT NOT NULL,
    client_ip  TEXT,
    action     TEXT NOT NULL,               -- create/update/delete/reload/rollback/login
    object     TEXT NOT NULL,               -- ip_rule/rule/site/settings...
    object_id  TEXT,
    before     TEXT,
    after      TEXT,
    result     TEXT NOT NULL DEFAULT 'ok'
);
CREATE INDEX IF NOT EXISTS idx_audit_ts ON audit_logs(ts DESC);

-- 配置版本（用于回滚）
CREATE TABLE IF NOT EXISTS config_versions (
    version    INTEGER PRIMARY KEY AUTOINCREMENT,
    snapshot   TEXT NOT NULL,               -- 全量 JSON
    operator   TEXT,
    remark     TEXT,
    created_at INTEGER NOT NULL
);
```

### 6.3 查询模式与索引

| 查询 | SQL 形态 | 依赖索引 |
| --- | --- | --- |
| 最近事件 | `ORDER BY ts DESC LIMIT 50` | `idx_events_ts` |
| 某 IP 的攻击史 | `WHERE client_ip=? ORDER BY ts DESC` | `idx_events_ip_ts` |
| 按动作筛选 | `WHERE action=? AND ts>?` | `idx_events_action` |
| 站点维度 | `WHERE site=? AND ts>?` | `idx_events_site_ts` |
| 大屏趋势 | 查 `stats_1m`，不查 `events` | 主键 |
| TopN | 查 `stats_1m` 的 JSON 字段，或内存快照 | — |

**分页必须用游标**，不要 `LIMIT 100000, 20`：

```sql
-- 推荐：基于 id 的游标分页
SELECT * FROM events
WHERE id < ? AND ts >= ?
ORDER BY id DESC
LIMIT 50;
```

**时间范围查询**要带上下界，避免全表扫：

```sql
WHERE ts BETWEEN ? AND ?
```

### 6.4 保留、归档与容量

| 数据 | 热存 | 冷存 | 说明 |
| --- | --- | --- | --- |
| `events` | 30 天 | 180 天（gzip JSONL） | 网络安全法要求网络日志留存不少于 6 个月 |
| `stats_1m` | 180 天 | — | 数据量小，直接留 |
| `audit_logs` | 365 天 | — | 变更追溯 |
| `bans` | 永久 | — | 历史封禁记录有价值 |

每日凌晨 03:00 执行归档任务：

```
1. 查出 30 天前的 events，按天分组
2. 导出为 archive/2026-10/events-2026-10-01.jsonl.gz
3. 校验行数与 gzip 完整性
4. DELETE FROM events WHERE ts < ?（分批，每批 5000 行，避免长事务）
5. 每月执行一次 VACUUM（低峰期）
```

**容量估算**：单条事件平均 600 字节（含索引），10 万条/天 ≈ 60 MB/天 ≈ 1.8 GB/月。30 天热数据约 2 GB，加冷归档 6 个月约 10 GB。建议数据盘 ≥ 50 GB。

### 6.5 初始化与迁移

启动顺序（写死在 `store.Open` 里，不要靠调用方自觉）：

```
1. 打开 SQLite（WAL + busy_timeout）
2. 读 schema_meta.version
3. version == 0  → 执行 migrations/001_init.sql（建全表），写入 version=1
4. version < N  → 依次执行缺失的 migrations/NNN_xxx.sql（每个文件一个事务）
5. version 已最新 → 跳过
6. 若 sys_users 为空 → 创建初始管理员
```

- 迁移文件 `//go:embed migrations/*.sql` 打进二进制，**不读外部目录**（否则升级时忘了拷文件就炸）；
- 每个迁移文件必须幂等（`CREATE TABLE IF NOT EXISTS`），失败整事务回滚并拒绝启动；
- 初始管理员：用户名 `admin`，随机 16 位密码打印到 stdout 并写入 `data/initial-password.txt`（权限 600），`must_change_password=1`，登录后强制改；
- `demo.enabled = true` 时**跳过上面全部步骤**，改为打开 `demo.db`、导入 `seed/demo.sql`、创建 `demo` 账号（`role=demo`）；
- `settings` 表首次写入默认值（`engine.mode=observe`，各项阈值），之后以库为准，`config.yaml` 只作为兜底。


---

## 7. 管理 API 设计

### 7.1 约定

> **本节以 `Frontend/` 现有前端为准，不是反过来。** 前端的 `src/types/common/response.ts`、`src/types/api/api.d.ts`、`src/utils/http/index.ts` 已经把契约写死了，后端照此实现，前端零改动。

**统一响应**（`code === 200` 才算成功，前端 `ApiStatus.success = 200`）：

```json
{ "code": 200, "msg": "ok", "data": { } }
```

**错误码**：

| code | HTTP | 含义 | 前端表现 |
| --- | --- | --- | --- |
| 200 | 200 | 成功 | — |
| 400 | 400 | 参数校验失败 | toast 提示 `msg` |
| 401 | 401 | 未登录 / token 过期 | **自动登出并跳登录页**（带防抖） |
| 403 | 403 | 无权限 / IP 不在白名单 | toast 提示 |
| 40301 | 403 | 演示模式禁止写操作 | toast「演示环境已禁用」 |
| 404 | 404 | 资源不存在 | toast |
| 429 | 429 | 请求过于频繁（含登录锁定） | toast，显示剩余时间 |
| 500 | 500 | 内部错误 | toast |

> 注意：HTTP 状态码与业务 `code` **保持一致**。前端的响应拦截器先判 `code`，网络层拦截器再判 `response.status === 401`；两者不一致会出现"业务码 401 但 HTTP 200"的单边登出 bug。

**认证头**：`Authorization: <access_token>`，裸 token，**不加 `Bearer ` 前缀**（`src/utils/http/index.ts` 里是 `headers.set('Authorization', accessToken)`）。后端解析时兼容两种写法：先剥掉可选的 `Bearer `，再验签。

**分页**（对齐 `Api.Common.PaginationParams`）：请求 `current`（从 1 开始）、`size`（默认 20，最大 200）；响应：

```json
{ "code": 200, "msg": "ok", "data": { "records": [], "current": 1, "size": 20, "total": 0 } }
```

**时间**：数据库存 Unix 毫秒（`int64`），JSON 也输出毫秒数，前端用 dayjs 转换。不要输出 RFC3339 字符串——前端表格列格式化是按数字写的，混用会导致时间显示成 `Invalid Date`。

**幂等**：`PUT` 全量更新，`PATCH` 局部更新，`POST` 创建，`DELETE` 删除。

**路径**：统一 `/api/` 前缀，不带版本号。版本真要升级时走 `/api/v2/` 并行一段时间，不要给现有路径加 `/v1`（前端 `src/api/auth.ts` 里写的是 `/api/auth/login`，除非同时改 `VITE_API_URL`）。

### 7.2 接口清单

#### 认证与用户

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/auth/login` | 入参 `{userName, password}`，返回 `{token, refreshToken}`；**同时下发 HttpOnly Cookie** |
| POST | `/api/auth/logout` | 吊销 refresh token、清 Cookie |
| POST | `/api/auth/refresh` | 轮换 refresh token，返回新 `{token, refreshToken}` |
| GET | `/api/user/info` | 当前用户 `{userId, userName, email, avatar, roles[], buttons[]}` |
| PUT | `/api/user/password` | 修改自己的密码（演示模式禁用） |
| GET | `/api/user/list` | 用户列表（分页 `current/size`，供下拉选择器复用） |
| GET | `/api/auth/csrf` | 取 CSRF Token（仅 Cookie 通道需要） |

> 前端登录后先拿 token，再由路由守卫调 `/api/user/info` 补齐 `roles` 与 `buttons`。`VITE_ACCESS_MODE=frontend` 表示菜单由前端定义，后端**只需**返回角色与按钮权限，不要下发菜单树。

#### 仪表盘

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/dashboard/overview?range=1h` | 总请求、拦截、挑战、QPS、5xx |
| GET | `/api/dashboard/timeseries?range=24h&site=` | 时间序列（QPS/拦截/挑战） |
| GET | `/api/dashboard/top?field=ip&range=1h&limit=10` | TopN（ip/path/category/country） |
| GET | `/api/dashboard/realtime` | SSE，每秒推送一次汇总 |

#### 事件

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/events` | 分页查询，支持 site/action/category/ip/path/from/to |
| GET | `/api/events/:id` | 详情（含原始请求头、命中证据） |
| GET | `/api/events/export` | 导出 CSV（上限 5 万行） |
| GET | `/api/stream/events` | SSE 实时事件流 |

#### IP 与封禁

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/ips?kind=black&keyword=` | 名单列表 |
| POST | `/api/ips` | 新增（支持 CIDR、批量数组） |
| PUT | `/api/ips/:id` | 修改 |
| DELETE | `/api/ips/:id` | 删除 |
| POST | `/api/ips/import` | 批量导入（文本，每行一个） |
| GET | `/api/bans?active=1` | 当前封禁列表 |
| POST | `/api/bans` | 手动封禁（ip、时长、原因） |
| DELETE | `/api/bans/:id` | 解封 |
| DELETE | `/api/bans` | 清空所有临时封禁（panic 按钮） |

#### 规则

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/rules?category=&enabled=` | 规则列表 |
| POST | `/api/rules` | 新增自定义规则 |
| PUT | `/api/rules/:id` | 修改 |
| DELETE | `/api/rules/:id` | 删除（内置规则不可删，只能停用） |
| PATCH | `/api/rules/:id/toggle` | 启用/停用 |
| POST | `/api/rules/test` | 规则测试器：传入样例请求，返回命中与评分 |
| POST | `/api/rules/reload` | 重新编译规则（校验失败返回错误详情） |

#### 站点

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/sites` | 站点列表 |
| POST | `/api/sites` | 新增 |
| PUT | `/api/sites/:id` | 修改 |
| DELETE | `/api/sites/:id` | 删除 |
| POST | `/api/sites/:id/test` | 连通性测试（请求 `/` 看响应码与耗时） |

#### 设置与审计

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/settings` | 全局设置 |
| PUT | `/api/settings` | 修改（观察模式、阈值、采样率、告警） |
| POST | `/api/settings/reload` | 热加载 |
| GET | `/api/settings/versions` | 配置历史 |
| POST | `/api/settings/rollback` | 回滚到指定版本 |
| GET | `/api/audit` | 审计日志 |
| GET | `/api/system/info` | 版本、启动时间、Goroutine、内存 |
| GET | `/metrics` | Prometheus 指标（仅回环） |

### 7.3 接口示例

**登录**（注意入参是 `userName` 驼峰，与前端 `Api.Auth.LoginParams` 一致）

```http
POST /api/auth/login
Content-Type: application/json

{ "userName": "admin", "password": "********" }
```

```http
HTTP/1.1 200 OK
Set-Cookie: waf_token=eyJhbGci...; Path=/; HttpOnly; SameSite=Strict; Max-Age=7200
X-CSRF-Token: 3f9c...            # 前端存内存，写请求时回填到请求头
Content-Type: application/json

{ "code": 200, "msg": "ok", "data": { "token": "eyJhbGci...", "refreshToken": "9c1f..." } }
```

**当前用户**（前端路由守卫登录后立刻调用）

```http
GET /api/user/info
Authorization: eyJhbGci...
```

```json
{
  "code": 200,
  "msg": "ok",
  "data": {
    "userId": 1,
    "userName": "admin",
    "nickName": "管理员",
    "email": "admin@example.com",
    "avatar": "",
    "roles": ["admin"],
    "buttons": ["rule:create", "rule:delete", "ip:ban", "settings:write"]
  }
}
```

**查询事件**

```http
GET /api/events?action=block&range=24h&current=1&size=20
```

```json
{
  "code": 200,
  "msg": "ok",
  "data": {
    "current": 1,
    "size": 20,
    "total": 1284,
    "records": [
      {
        "id": 98211,
        "ts": 1793612345678,
        "site": "官网",
        "client_ip": "45.12.34.56",
        "method": "GET",
        "path": "/.env",
        "status": 403,
        "action": "block",
        "score": 65,
        "categories": ["scanner", "sensitive_file"],
        "rule_ids": ["scan-ua-001", "path-env"],
        "ua": "sqlmap/1.7.2#stable",
        "req_id": "01JBX8K2Q3"
      }
    ]
  }
}
```

**规则测试**

```http
POST /api/rules/test
{
  "method": "GET",
  "path": "/api/user",
  "query": "id=1%20union%20select%201,2",
  "headers": { "User-Agent": "Mozilla/5.0" },
  "body": ""
}
```

```json
{
  "code": 200,
  "msg": "ok",
  "data": {
    "action": "block",
    "score": 50,
    "hits": [
      { "rule_id": "sqli-union", "category": "sqli", "score": 50, "target": "query", "evidence": "1 union select 1,2" }
    ],
    "normalized": "1 union select 1,2",
    "elapsed_us": 38
  }
}
```

### 7.4 SSE 事件格式

```
event: stat
data: {"ts":1793612345000,"qps":132,"blocked":7,"challenged":2}

event: attack
data: {"id":98212,"ip":"45.12.34.56","path":"/.env","action":"block","score":65,"category":"scanner"}

event: ban
data: {"ip":"45.12.34.56","until":1793615945000,"reason":"cc"}
```

前端用 `EventSource`，断线自动重连（浏览器原生行为），服务端每 15 秒发一次 `: ping` 注释帧保活。

---

## 8. 关键代码实现

### 8.1 目录结构

```
gowaf/
├── cmd/gowaf/main.go              # 入口：加载配置、GORM 建库、启动三组协程
├── internal/
│   ├── config/                    # YAML 加载、校验、默认值
│   ├── proxy/                     # 站点路由、ReverseProxy、证书管理（纯 net/http）
│   ├── detect/                    # 规则引擎
│   │   ├── engine.go
│   │   ├── normalize.go
│   │   ├── ac.go                  # Aho-Corasick
│   │   ├── target.go              # 请求目标提取
│   │   └── builtin.go             # 内置规则加载
│   ├── guard/                     # IP 名单、限流、封禁、Challenge
│   ├── store/                     # 数据访问层
│   │   ├── db.go                  # gorm.Open + 两个连接池
│   │   ├── migrate.go             # schema_meta 版本化迁移
│   │   ├── events.go              # 原生 SQL（热路径）
│   │   └── stats.go
│   ├── admin/                     # 控制面（Gin）
│   │   ├── router.go              # 路由注册 + 中间件装配
│   │   ├── model/                 # GORM 模型：user/site/rule/ip_rule/audit
│   │   ├── dto/                   # 请求响应结构 + binding tag
│   │   ├── handler/               # auth/user/dashboard/event/ip/rule/site/setting
│   │   ├── service/               # 业务逻辑（事务在这里）
│   │   ├── middleware/
│   │   │   ├── recover.go
│   │   │   ├── requestid.go
│   │   │   ├── accesslog.go
│   │   │   ├── cors.go
│   │   │   ├── ipallow.go
│   │   │   ├── loginlimit.go
│   │   │   ├── auth.go            # JWT 解析
│   │   │   ├── csrf.go
│   │   │   ├── demo.go            # 演示模式只读拦截
│   │   │   └── audit.go
│   │   ├── jwt/                   # 签发/验签/刷新
│   │   └── response/              # 统一 {code,msg,data} 封装
│   ├── web/                       # //go:embed all:dist（构建时从 Frontend/dist 同步）
│   └── metrics/                   # Prometheus
├── rules/builtin.yaml             # 内置规则（可被 DB 覆盖）
├── seed/demo.sql                  # 演示模式种子数据
├── Frontend/                      # 现有 Vue3 + naive-ui 后台（独立 pnpm 工程）
│   ├── src/
│   └── dist/                      # 构建产物，构建时拷到 internal/web/dist
├── deploy/
│   ├── gowaf.service
│   ├── config.example.yaml
│   └── switch.sh                  # 一键切换/回滚脚本
├── docs/
├── go.mod
└── Makefile
```

**分层纪律**（守不住就会被自己坑）：

| 层 | 允许依赖 | 禁止 |
| --- | --- | --- |
| `handler` | `service`、`dto` | 直接碰 `*gorm.DB`、写 SQL |
| `service` | `model`、`store`、`detect`、`guard` | 读 `gin.Context` |
| `store` | `*gorm.DB` / `*sql.DB` | 业务判断 |
| `proxy` / `detect` / `guard` | 标准库 + `config` | **任何** admin/store 依赖（数据面不能反向依赖控制面） |

最后一条最重要：数据面（80/443）一旦 import 了 GORM，单请求延迟和二进制体积都会失控，而且会形成循环依赖。

### 8.2 配置示例

```yaml
# /www/server/gowaf/config.yaml
server:
  listen: "0.0.0.0:80"
  tls_listen: "0.0.0.0:443"
  admin_listen: "127.0.0.1:8899"
  read_header_timeout: 5s
  idle_timeout: 90s
  max_header_bytes: 32768
  shutdown_timeout: 15s
  trusted_proxies: ["127.0.0.1/32"]   # 若前面还有 CDN，加上 CDN 回源段

tls:
  enabled: true
  cert_dir: "/www/server/panel/vhost/cert"
  default_cert: "www.example.com"
  min_version: "1.2"
  hsts: true

storage:
  dsn: "/www/server/gowaf/data/waf.db"
  retain_days: 30
  archive_dir: "/www/server/gowaf/data/archive"
  archive_days: 180
  sample_rate: 0.01                   # 正常请求采样率

# 首次启动的站点种子：导入 SQLite 后即以控制台/数据库为准
sites:
  - name: "官网"
    hosts: ["www.example.com", "example.com"]
    upstream: "http://127.0.0.1:8080"
  - name: "商城"
    hosts: ["shop.example.com"]
    upstream: "http://127.0.0.1:8080"

engine:
  mode: "observe"                     # observe | protect
  block_score: 60
  challenge_score: 30
  max_body_inspect: 262144            # 256KB
  max_targets: 64
  regex_timeout_ms: 50                # 单条规则软超时（仅统计用）

guard:
  global_rps: 0                       # 0 = 不限
  ip_rps: 30
  ip_burst: 60
  ip_per_minute: 600
  challenge_per_minute: 300
  ban_per_minute: 600
  ban_1h_per_minute: 1200
  ban_ttl: 1h                 # 单次自动封禁时长
  ban_max_ttl: 24h            # 时长上限
  ban_window: 10m             # 统计窗口
  ban_threshold: 5            # 窗口内命中多少次触发
  ban_max_strikes: 3          # 连续第几次触发升级为永久封禁（0 = 永不）
  ban_strike_reset: 24h       # 这么久无新命中则连击数清零
  auto_ban: true
  whitelist_static: true

admin:
  allowed_ips: ["127.0.0.1/32"]       # 空数组 = 不限制（仍建议前面挡防火墙）
  jwt_secret_file: "/www/server/gowaf/data/jwt.key"
  access_ttl: 2h                      # access token
  refresh_ttl: 168h                   # refresh token（7 天）
  login_max_fail: 5
  login_lock: 15m
  cookie_secure: false                # 生产上 HTTPS 后必须改 true
  cookie_domain: ""                   # 空 = 当前域
  cors:
    enabled: true
    allow_origins:                    # 开发期写 Vite 地址；生产同源时留空数组
      - "http://127.0.0.1:3006"
      - "http://localhost:3006"
    allow_methods: ["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"]
    allow_headers: ["Authorization", "Content-Type", "X-CSRF-Token", "X-Request-ID"]
    expose_headers: ["X-Request-ID", "X-CSRF-Token"]
    allow_credentials: true           # 要带 Cookie 就不能用 "*"
    max_age: 12h
  csrf:
    enabled: true
    header: "X-CSRF-Token"
    cookie_name: "waf_csrf"
    # 只对"凭 Cookie 认证"的写请求生效，带 Authorization 头的请求自动豁免

demo:
  enabled: false                      # 演示部署时改 true
  db_path: "/www/server/gowaf/data/demo.db"
  username: "demo"
  password: "demo123"                 # 仅首次建库写入，之后以库为准
  read_only: true
  mask_ips: true
  reset_cron: "0 4 * * *"
  banner: "演示环境：数据为模拟数据，修改操作已禁用"

alert:
  webhook: ""                         # 企业微信/钉钉机器人
  block_threshold: 500                # 5 分钟内拦截超过该值告警
  window: 5m

log:
  level: "info"
  file: "/www/server/gowaf/logs/waf.log"
  max_size_mb: 100
  max_backups: 7
```

配置校验规则（启动时执行，失败拒绝启动）：

- 端口不重复、不为 0；
- 上游地址必须是 `http(s)://host:port`，**禁止指向 127.0.0.1:8899**（防 SSRF 打自己）；
- `block_score > challenge_score > 0`；
- 证书目录存在且至少有一个证书（TLS 开启时）；
- `admin_listen` 若不在回环地址，必须显式配置 `i_know_what_i_am_doing: true`，否则拒绝启动；
- `cors.allow_credentials = true` 时，`allow_origins` **不允许包含 `*`**（浏览器规范禁止，配了也不生效，只会静默失败）；
- `demo.enabled = true` 时，`demo.db_path` 的绝对路径必须 ≠ `storage.dsn`，相同则拒绝启动（防止演示模式写穿生产库）；
- `demo.enabled = true` 且 `admin.listen` 不在回环地址时，强制要求 `cookie_secure = true`，否则演示口令会在明文 HTTP 上传。

### 8.3 入口：三组协程

```go
func main() {
    cfgPath := flag.String("c", "/www/server/gowaf/config.yaml", "config path")
    showVer := flag.Bool("v", false, "print version")
    flag.Parse()
    if *showVer {
        fmt.Println(version)
        return
    }

    cfg, err := config.Load(*cfgPath)
    if err != nil {
        log.Fatalf("load config: %v", err)
    }
    app, err := core.New(cfg)
    if err != nil {
        log.Fatalf("init: %v", err)
    }

    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    g, gctx := errgroup.WithContext(ctx)
    g.Go(func() error { return app.RunDataPlane(gctx) })
    g.Go(func() error { return app.RunControlPlane(gctx) })
    g.Go(func() error { return app.RunMaintenance(gctx) }) // 归档、清理、统计落库
    g.Go(func() error { return app.RunMetrics(gctx) })

    if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
        log.Error("exit", "err", err)
    }
    app.Shutdown() // 等待在途请求，flush 日志
}
```

`RunDataPlane` 关键点：

```go
func (a *App) RunDataPlane(ctx context.Context) error {
    srv := &http.Server{
        Addr:              a.cfg.Server.Listen,
        Handler:           a.dataHandler,
        ReadHeaderTimeout: a.cfg.Server.ReadHeaderTimeout,
        IdleTimeout:       a.cfg.Server.IdleTimeout,
        MaxHeaderBytes:    a.cfg.Server.MaxHeaderBytes,
        ErrorLog:          log.StdLogger(), // 抑制默认的 net/http 日志格式
    }

    go func() {
        <-ctx.Done()
        shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
        defer cancel()
        _ = srv.Shutdown(shutdownCtx)
    }()

    var err error
    if a.cfg.TLS.Enabled {
        tlsSrv := &http.Server{
            Addr:    a.cfg.Server.TLSListen,
            Handler: a.dataHandler,
            TLSConfig: &tls.Config{
                GetCertificate: a.certs.GetCertificate,
                MinVersion:     tls.VersionTLS12,
                NextProtos:     []string{"h2", "http/1.1"},
            },
            ReadHeaderTimeout: a.cfg.Server.ReadHeaderTimeout,
            IdleTimeout:       a.cfg.Server.IdleTimeout,
        }
        err = tlsSrv.ListenAndServeTLS("", "")
    } else {
        err = srv.ListenAndServe()
    }
    if errors.Is(err, http.ErrServerClosed) {
        return nil
    }
    return err
}
```

### 8.4 数据面主流程

```go
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    start := time.Now()
    reqID := id.New()
    w.Header().Set("X-Request-Id", reqID)

    // 任何 panic 都不能带崩进程
    defer func() {
        if rec := recover(); rec != nil {
            a.metrics.Panic.Inc()
            log.Error("panic recovered", "req_id", reqID, "err", rec, "stack", string(debug.Stack()))
            http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
        }
    }()

    snap := a.cfg.Load() // 整个请求使用同一份快照

    host := normalizeHost(r.Host)
    if host == "" {
        http.Error(w, "400 Bad Request", http.StatusBadRequest)
        return
    }
    site := snap.Router.Match(host)
    if site == nil {
        a.metrics.UnmatchedHost.Inc()
        http.Error(w, "404 Not Found", http.StatusNotFound)
        return
    }

    ip := clientIP(r, snap.TrustedProxies)

    // ① 白名单：最高优先级，放行且不计限流
    if snap.IPWhite.Contains(ip) {
        a.proxyFor(site).ServeHTTP(w, r)
        return
    }

    // ② 黑名单 / 临时封禁
    if snap.IPBlack.Contains(ip) || snap.Bans.Contains(ip) {
        a.reject(w, r, site, ip, reqID, Verdict{Action: ActionBlock, Score: 100, Categories: []string{"ip_blacklist"}})
        return
    }

    // ③ 限流
    if !snap.Limiter.Allow(ip, r.URL.Path) {
        v := Verdict{Action: ActionChallenge, Score: 20, Categories: []string{"cc"}}
        a.reject(w, r, site, ip, reqID, v)
        return
    }
    if !snap.Rate.Allow() { // 全站保护
        a.reject(w, r, site, ip, reqID, Verdict{Action: ActionBlock, Score: 20, Categories: []string{"global_rate"}})
        return
    }

    // ④ 协议校验
    if !validMethod(r.Method) || len(r.URL.Path) > 2048 || len(r.Header) > 100 {
        a.reject(w, r, site, ip, reqID, Verdict{Action: ActionBlock, Score: 40, Categories: []string{"protocol"}})
        return
    }

    // ⑤ 读取 body（限长，超出部分流式透传）
    var body []byte
    if site.DetectBody && shouldInspectBody(r) {
        body, _ = proxy.BufferBody(r, site.MaxBodyBytes)
    }

    // ⑥ 检测
    verdict := snap.Engine.Inspect(&detect.Request{
        Method: r.Method,
        Host:   host,
        Path:   r.URL.Path,
        RawQuery: r.URL.RawQuery,
        Header: r.Header,
        Body:   body,
        RemoteIP: ip,
    })

    // ⑦ 观察模式降级
    if snap.Settings.Mode == ModeObserve && verdict.Action == ActionBlock {
        verdict.Observed = true
        verdict.Action = ActionLog
    }

    // ⑧ 决策
    if verdict.Action == ActionBlock || verdict.Action == ActionBan {
        if verdict.Action == ActionBan {
            snap.Bans.Add(ip, snap.Settings.BanTTL, verdict.Reason())
        }
        a.reject(w, r, site, ip, reqID, verdict)
        return
    }
    if verdict.Action == ActionChallenge && !snap.Challenge.Passed(r, ip) {
        a.challenge(w, r, site, ip, reqID, verdict)
        return
    }

    // ⑨ 转发，并记录响应状态
    rw := &responseRecorder{ResponseWriter: w, status: 200}
    a.proxyFor(site).ServeHTTP(rw, r)

    // ⑩ 异步记录
    a.emit(&Event{...})
}
```

**注意**：`responseRecorder` 必须实现 `http.Flusher`、`http.Hijacker`，否则 WebSocket 和 SSE 会失效。这是自己包 `ResponseWriter` 最常见的坑。

### 8.5 证书管理

```go
type CertManager struct {
    dir     string
    defName string
    mu      sync.RWMutex
    certs   map[string]*tls.Certificate
    mtime   map[string]time.Time
}

func (m *CertManager) GetCertificate(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
    name := strings.ToLower(strings.TrimSuffix(h.ServerName, "."))
    if c := m.lookup(name); c != nil {
        return c, nil
    }
    if i := strings.IndexByte(name, '.'); i > 0 { // 尝试一级通配符
        if c := m.lookup("*" + name[i:]); c != nil {
            return c, nil
        }
    }
    if c := m.lookup(m.defName); c != nil {
        return c, nil
    }
    return nil, fmt.Errorf("no certificate for %q", name)
}

func (m *CertManager) scan() error {
    entries, err := os.ReadDir(m.dir)
    if err != nil {
        return err
    }
    for _, e := range entries {
        if !e.IsDir() {
            continue
        }
        domain := e.Name()
        certPath := filepath.Join(m.dir, domain, "fullchain.pem")
        st, err := os.Stat(certPath)
        if err != nil {
            continue
        }
        m.mu.RLock()
        old, ok := m.mtime[domain]
        m.mu.RUnlock()
        if ok && !st.ModTime().After(old) {
            continue // 未变化
        }
        if err := m.load(domain); err != nil {
            log.Warn("load cert failed", "domain", domain, "err", err)
            continue
        }
        log.Info("cert loaded", "domain", domain, "not_after", m.expire(domain))
    }
    return nil
}

func (m *CertManager) load(domain string) error {
    dir := filepath.Join(m.dir, domain)
    certPEM, err := os.ReadFile(filepath.Join(dir, "fullchain.pem"))
    if err != nil {
        return err
    }
    keyPEM, err := os.ReadFile(filepath.Join(dir, "privkey.pem"))
    if err != nil {
        return err
    }
    cert, err := tls.X509KeyPair(certPEM, keyPEM)
    if err != nil {
        return err
    }
    m.mu.Lock()
    m.certs[domain] = &cert
    m.mtime[domain] = time.Now()
    m.mu.Unlock()
    return nil
}
```

启动时全量加载，之后每 60 秒 `scan()` 一次（比 fsnotify 简单可靠，证书文件一年才变几次，开销可忽略）。

### 8.6 Aho-Corasick 预筛

```go
package detect

type acNode struct {
    next map[byte]int32
    fail int32
    out  []int32 // 规则下标
}

type AC struct {
    nodes []acNode
}

func NewAC(patterns map[string][]int32) *AC {
    ac := &AC{nodes: []acNode{{next: map[byte]int32{}}}}
    for p, ids := range patterns {
        cur := int32(0)
        for i := 0; i < len(p); i++ {
            c := p[i]
            nxt, ok := ac.nodes[cur].next[c]
            if !ok {
                ac.nodes = append(ac.nodes, acNode{next: map[byte]int32{}})
                nxt = int32(len(ac.nodes) - 1)
                ac.nodes[cur].next[c] = nxt
            }
            cur = nxt
        }
        ac.nodes[cur].out = append(ac.nodes[cur].out, ids...)
    }

    // BFS 构建 fail 指针
    queue := make([]int32, 0, len(ac.nodes))
    for _, v := range ac.nodes[0].next {
        ac.nodes[v].fail = 0
        queue = append(queue, v)
    }
    for len(queue) > 0 {
        u := queue[0]
        queue = queue[1:]
        for c, v := range ac.nodes[u].next {
            f := ac.nodes[u].fail
            for f != 0 {
                if _, ok := ac.nodes[f].next[c]; ok {
                    break
                }
                f = ac.nodes[f].fail
            }
            if nxt, ok := ac.nodes[f].next[c]; ok && nxt != v {
                ac.nodes[v].fail = nxt
            }
            ac.nodes[v].out = append(ac.nodes[v].out, ac.nodes[ac.nodes[v].fail].out...)
            queue = append(queue, v)
        }
    }
    return ac
}

// Find 返回命中的规则下标（已去重）
func (ac *AC) Find(s string, buf []int32) []int32 {
    cur := int32(0)
    buf = buf[:0]
    for i := 0; i < len(s); i++ {
        c := s[i]
        for cur != 0 {
            if _, ok := ac.nodes[cur].next[c]; ok {
                break
            }
            cur = ac.nodes[cur].fail
        }
        if nxt, ok := ac.nodes[cur].next[c]; ok {
            cur = nxt
        } else {
            cur = 0
        }
        for _, id := range ac.nodes[cur].out {
            if !contains(buf, id) {
                buf = append(buf, id)
            }
        }
    }
    return buf
}
```

`buf` 由 `sync.Pool` 复用，避免每次请求分配切片。

### 8.7 限流与自动封禁

```go
type Guard struct {
    shards [16]struct {
        mu sync.Mutex
        m  map[string]*state
    }
    rps, burst int
    perMinute  int64
}

type state struct {
    lim  *rate.Limiter
    win  slidingWindow
    last time.Time
}

func (g *Guard) Allow(ip, path string) bool {
    s := &g.shards[fnv32(ip)%16]
    s.mu.Lock()
    st, ok := s.m[ip]
    if !ok {
        st = &state{lim: rate.NewLimiter(rate.Limit(g.rps), g.burst)}
        s.m[ip] = st
    }
    st.last = time.Now()
    ok = st.lim.Allow()
    n := st.win.Add(time.Now(), time.Minute)
    s.mu.Unlock()

    if !ok {
        return false
    }
    if n > int64(g.perMinute) {
        return false // 交给上层决定 challenge 还是 ban
    }
    return true
}
```

自动封禁的判定放在上层，根据超限倍数决定动作，并写入 `bans` 表（带过期时间），内存里用最小堆维护到期时间，每秒弹出过期项删除。

### 8.8 日志入库

```go
func (s *Store) InsertBatch(events []*Event) error {
    tx, err := s.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    stmt, err := tx.Prepare(`INSERT INTO events
        (ts, site, client_ip, method, host, path, query, status, action, score,
         categories, rule_ids, ua, referer, req_id, latency_us, upstream, country, bytes)
        VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
    if err != nil {
        return err
    }
    defer stmt.Close()

    for _, e := range events {
        if _, err := stmt.Exec(e.Ts, e.Site, e.ClientIP, e.Method, e.Host, e.Path, e.Query,
            e.Status, e.Action, e.Score, e.Categories, e.RuleIDs, e.UA, e.Referer,
            e.ReqID, e.LatencyUS, e.Upstream, e.Country, e.Bytes); err != nil {
            return err
        }
    }
    return tx.Commit()
}
```

单条 INSERT 每次都要 fsync，1 万条会慢到不可用；**必须用事务批量提交**，256 条一批，实测可到 5 万条/秒以上。

### 8.9 热更新

```go
func (a *App) Reload(ctx context.Context, operator string) error {
    next, err := a.build(ctx)
    if err != nil {
        return fmt.Errorf("build snapshot: %w", err)
    }
    old := a.cfg.Load()
    next.Version = old.Version + 1
    a.cfg.Store(next)

    a.audit(operator, "reload", "settings", "", old.Version, next.Version)
    log.Info("config reloaded", "version", next.Version)
    return nil
}

func (a *App) build(ctx context.Context) (*Snapshot, error) {
    sites, err := a.store.ListSites(ctx)
    if err != nil {
        return nil, err
    }
    rules, err := a.store.ListRules(ctx)
    if err != nil {
        return nil, err
    }
    ruleset, err := detect.Compile(rules) // 编译失败返回详细行号
    if err != nil {
        return nil, err
    }
    return &Snapshot{
        Router:  proxy.NewRouter(sites),
        Engine:  detect.NewEngine(ruleset),
        IPBlack: a.store.BlackMatcher(ctx),
        IPWhite: a.store.WhiteMatcher(ctx),
        Limiter: guard.New(...),
        Bans:    a.bans,
    }, nil
}
```

**编译失败必须保留旧快照**，并把错误返回给控制台，前端展示"第 3 条规则正则非法：missing closing )"。

### 8.10 控制面中间件（JWT / CORS / CSRF / Demo）

#### 8.10.1 统一响应封装

```go
package response

type Body struct {
    Code int    `json:"code"`
    Msg  string `json:"msg"`
    Data any    `json:"data,omitempty"`
}

// 注意 code 用 200 表示成功（对齐前端 ApiStatus.success）
func OK(c *gin.Context, data any) { c.JSON(200, Body{Code: 200, Msg: "ok", Data: data}) }

func Fail(c *gin.Context, httpStatus, code int, msg string) {
    c.AbortWithStatusJSON(httpStatus, Body{Code: code, Msg: msg})
}

// 分页响应，字段名对齐前端 Api.Common.PaginatedResponse
type Page[T any] struct {
    Records []T   `json:"records"`
    Current int   `json:"current"`
    Size    int   `json:"size"`
    Total   int64 `json:"total"`
}
```

#### 8.10.2 JWT 中间件（双通道）

```go
func Auth(secret []byte) gin.HandlerFunc {
    return func(c *gin.Context) {
        raw, fromCookie := extractToken(c)
        if raw == "" {
            response.Fail(c, 401, 401, "未登录")
            return
        }
        claims := &Claims{}
        tok, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
            if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
                return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
            }
            return secret, nil
        })
        if err != nil || !tok.Valid {
            response.Fail(c, 401, 401, "登录已过期")
            return
        }
        c.Set(CtxUserID, claims.UserID)
        c.Set(CtxUserName, claims.UserName)
        c.Set(CtxRole, claims.Role)
        c.Set(CtxAuthFrom, authFrom(raw, fromCookie)) // "header" | "cookie"
        c.Next()
    }
}

// 优先 Authorization 头，其次 Cookie；裸 token 与 "Bearer xxx" 都兼容
func extractToken(c *gin.Context) (token string, fromCookie bool) {
    if h := c.GetHeader("Authorization"); h != "" {
        if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
            return strings.TrimSpace(h[7:]), false
        }
        return strings.TrimSpace(h), false
    }
    if ck, err := c.Cookie("waf_token"); err == nil && ck != "" {
        return ck, true
    }
    return "", false
}
```

#### 8.10.3 CSRF 中间件（只拦 Cookie 通道）

```go
// 关键：带 Authorization 头的请求天然免疫 CSRF，直接放行。
// 只有"浏览器自动携带 Cookie"的写请求才做双提交校验。
func CSRF(cfg config.CSRF) gin.HandlerFunc {
    return func(c *gin.Context) {
        if !cfg.Enabled || isSafeMethod(c.Request.Method) {
            c.Next()
            return
        }
        if c.GetString(CtxAuthFrom) != "cookie" {
            c.Next()
            return
        }
        want, _ := c.Cookie(cfg.CookieName)
        got := c.GetHeader(cfg.Header)
        if want == "" || subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
            response.Fail(c, 403, 403, "CSRF 校验失败，请刷新页面后重试")
            return
        }
        c.Next()
    }
}
```

#### 8.10.4 CORS 中间件（手写，不引第三方）

```go
// 为什么不引 gin-contrib/cors：只有十几个头要处理，手写 30 行更可控，
// 而且能精确处理"allow_credentials=true 时不能回 *"这个坑。
func CORS(cfg config.CORS, isDemo bool) gin.HandlerFunc {
    allowed := make(map[string]struct{}, len(cfg.AllowOrigins))
    for _, o := range cfg.AllowOrigins {
        allowed[o] = struct{}{}
    }
    return func(c *gin.Context) {
        origin := c.GetHeader("Origin")
        if origin == "" {
            c.Next()
            return
        }
        // 演示模式允许任意来源只读访问，方便把面板嵌到介绍页里
        if _, ok := allowed[origin]; !ok && !(isDemo && !cfg.AllowCredentials) {
            c.Next() // 不回 CORS 头，浏览器自己拦，服务端不报错
            return
        }

        h := c.Writer.Header()
        h.Set("Vary", "Origin")
        h.Set("Access-Control-Allow-Origin", origin)
        h.Set("Access-Control-Allow-Methods", strings.Join(cfg.AllowMethods, ", "))
        h.Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowHeaders, ", "))
        h.Set("Access-Control-Expose-Headers", strings.Join(cfg.ExposeHeaders, ", "))
        h.Set("Access-Control-Max-Age", strconv.Itoa(int(cfg.MaxAge.Seconds())))
        if cfg.AllowCredentials {
            h.Set("Access-Control-Allow-Credentials", "true")
        }

        // 预检必须在这里结束，不能落到后面的 Auth，否则浏览器只看到 401
        if c.Request.Method == http.MethodOptions {
            c.AbortWithStatus(http.StatusNoContent)
            return
        }
        c.Next()
    }
}
```

**CORS 的四个必踩坑**（写在这里省得调试时再查）：

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| 浏览器报跨域，Network 里 OPTIONS 是 401 | 预检被 `Auth` 拦了 | `CORS` 中间件必须注册在 `Auth` 之前，且 OPTIONS 直接 `204` 返回 |
| 登录成功但后续请求不带 Cookie | 跨域 + `allow_credentials=false`，或前端 `VITE_WITH_CREDENTIALS=false` | 两边同时开；且 `allow_origins` 不能是 `*` |
| 自定义头 `X-CSRF-Token` 触发预检失败 | 没写进 `Allow-Headers` | 加到 `allow_headers` |
| 前端读不到响应头里的 token | 没写进 `Expose-Headers` | 加到 `expose_headers` |

#### 8.10.5 演示模式中间件

```go
// DemoGuard 放在 Auth 之后、Audit 之前。
// 只读模式：非安全方法一律 403，白名单除外。
func DemoGuard(readOnly bool, whitelist map[string]struct{}) gin.HandlerFunc {
    return func(c *gin.Context) {
        if !readOnly || isSafeMethod(c.Request.Method) {
            c.Next()
            return
        }
        if _, ok := whitelist[c.FullPath()]; ok {
            c.Next()
            return
        }
        response.Fail(c, 403, 40301, "演示环境已禁用该操作")
    }
}

// 白名单（这些请求不改数据）
var demoWhitelist = map[string]struct{}{
    "/api/auth/login":   {},
    "/api/auth/logout":  {},
    "/api/auth/refresh": {},
    "/api/rules/test":   {}, // 纯计算
}
```

#### 8.10.6 路由装配顺序

```go
func NewRouter(deps *Deps) *gin.Engine {
    gin.SetMode(gin.ReleaseMode)
    r := gin.New()

    r.Use(middleware.Recover(), middleware.RequestID(), middleware.AccessLog())
    r.Use(middleware.CORS(deps.Cfg.Admin.CORS, deps.Cfg.Demo.Enabled))
    r.Use(middleware.IPAllowlist(deps.Cfg.Admin.AllowedIPs))

    api := r.Group("/api")
    {
        // 公开：登录、刷新、CSRF
        api.POST("/auth/login", middleware.LoginLimit(deps.Limiter), deps.Auth.Login)
        api.POST("/auth/refresh", deps.Auth.Refresh)

        // 需登录
        auth := api.Group("", middleware.Auth(deps.Secret))
        {
            auth.GET("/auth/csrf", deps.Auth.CSRF)
            auth.GET("/user/info", deps.User.Info)
            auth.GET("/system/info", deps.System.Info)

            w := auth.Group("", middleware.CSRF(deps.Cfg.Admin.CSRF),
                middleware.DemoGuard(deps.Cfg.Demo.ReadOnly, demoWhitelist),
                middleware.Audit(deps.Audit))
            {
                w.POST("/ips", deps.IP.Create)
                w.PUT("/ips/:id", deps.IP.Update)
                w.DELETE("/ips/:id", deps.IP.Delete)
                w.POST("/rules", deps.Rule.Create)
                w.POST("/rules/test", deps.Rule.Test) // 演示白名单，仍要过 CSRF
                w.PUT("/settings", deps.Setting.Update)
                // ...
            }
        }
    }

    // SSE 单独一组：不能套 Audit，也不能套 CSRF
    r.GET("/api/stream/events", middleware.Auth(deps.Secret), deps.Event.Stream)

    mountAdmin(r) // 见 5.12
    return r
}
```

**`/api/rules/test` 同时出现在两处**：路由要经过 `CSRF`（Cookie 通道下仍需校验），但 `DemoGuard` 的 `FullPath()` 命中白名单后放行。顺序不能反——先 CSRF 再 DemoGuard，否则演示模式的越权请求连 CSRF 都不查。

### 8.11 构建脚本

```makefile
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: web sync-web build dev test lint

# 前端是独立 pnpm 工程（Frontend/），有上层 pnpm-workspace.yaml，必须 --ignore-workspace
web:
	cd Frontend && pnpm install --ignore-workspace && pnpm build

# go:embed 不能跨包引用，产物要同步进 internal/web/dist
sync-web: web
	rm -rf internal/web/dist && mkdir -p internal/web/dist
	cp -r Frontend/dist/. internal/web/dist/

build: sync-web
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath -ldflags "$(LDFLAGS)" -o bin/gowaf ./cmd/gowaf

# 只改后端时跳过前端构建（前端产物已在 internal/web/dist）
build-go:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-trimpath -ldflags "$(LDFLAGS)" -o bin/gowaf ./cmd/gowaf

dev:
	go run ./cmd/gowaf -c ./deploy/config.example.yaml

test:
	go test ./... -race -count=1

lint:
	golangci-lint run ./...
```

交叉编译产物约 15～25 MB（含前端 dist），`CGO_ENABLED=0` 能成立的前提是**两处都不能引 CGO**：

1. SQLite 驱动用 `github.com/glebarez/sqlite`（纯 Go），不是 `gorm.io/driver/sqlite`；
2. 密码哈希用 `golang.org/x/crypto/bcrypt`（纯 Go），不是依赖 openssl 的方案。

任何一处引入 CGO，`CGO_ENABLED=0` 编译会直接报错，而不是悄悄降级——所以这个错误在 CI 第一次跑就会暴露。


### 8.12 systemd

```ini
# /etc/systemd/system/gowaf.service
[Unit]
Description=Go-WAF (reverse proxy + WAF)
Documentation=https://github.com/yourname/gowaf
After=network-online.target apache.service
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=/www/server/gowaf
ExecStart=/www/server/gowaf/bin/gowaf -c /www/server/gowaf/config.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=3
LimitNOFILE=65535
# 只允许写数据与日志目录
ReadWritePaths=/www/server/gowaf/data /www/server/gowaf/logs
ProtectSystem=full
ProtectHome=true
PrivateTmp=true
StandardOutput=append:/www/server/gowaf/logs/stdout.log
StandardError=append:/www/server/gowaf/logs/stderr.log

[Install]
WantedBy=multi-user.target
```

`ExecReload` 发 `SIGHUP`，进程收到后调用 `Reload()` 重新加载规则（比 `systemctl restart` 温和，不断连接）。

### 8.13 优雅退出

```go
func (a *App) Shutdown() {
    a.logger.Sync()
    a.store.Flush()   // 把 channel 里剩余事件写完
    a.store.Close()
}
```

顺序很重要：先停止接收新请求（`http.Server.Shutdown`），再 flush 日志，最后关数据库。反过来会丢最后一批事件。


---

## 9. 前端控制台设计

> 前提：**不新建工程**。所有页面挂在已有的 `Frontend/`（Fssadmin + naive-ui）里，作为它的一个业务模块。本章只描述"往里加什么"，不重复框架本身的东西。

### 9.1 工程结构（增量）

```
Frontend/src/
├── api/
│   ├── auth.ts                  # 已有，不动
│   └── waf/                     # 新增：WAF 业务接口，一文件一域
│       ├── index.ts             # 统一出口
│       ├── dashboard.ts
│       ├── events.ts
│       ├── ips.ts
│       ├── rules.ts
│       ├── sites.ts
│       ├── settings.ts
│       └── types.ts             # 本模块的 DTO 类型
├── components/core/waf/         # 新增：业务组件，遵守 ArtXxx 命名
│   ├── art-stat-card/index.vue       # 指标卡
│   ├── art-trend-chart/index.vue     # ECharts 趋势（折线/面积）
│   ├── art-event-drawer/index.vue    # 事件详情抽屉
│   ├── art-rule-tester/index.vue     # 规则测试器
│   └── art-action-tag/index.vue      # block/challenge/log/pass 标签
├── views/waf/                   # 新增：页面
│   ├── dashboard/index.vue
│   ├── events/index.vue
│   ├── ips/index.vue
│   ├── bans/index.vue
│   ├── rules/index.vue
│   ├── sites/index.vue
│   ├── settings/index.vue
│   └── audit/index.vue
├── router/modules/waf.ts        # 新增：一个模块文件即接入（见 9.2）
├── store/modules/waf.ts         # 新增：只放 SSE 连接与全局筛选条件
└── locales/langs/{zh,en}.json   # 新增：waf.* 文案（两份同步）
```

**明确不改的目录**（改了就等于维护一个 fork，以后没法跟上游同步）：

| 目录 | 原因 |
| --- | --- |
| `src/router/core/*` | 路由加载、权限校验都在里面，动一处全站路由受影响 |
| `src/utils/http/*` | 统一拦截器；WAF 需要的行为已全部支持 |
| `src/store/modules/user.ts` | token 存取、登出清理是全站逻辑 |
| `src/layouts/*` | 侧边栏/顶栏由菜单配置驱动，不需要改代码 |

**接口层写法**（与 `src/api/auth.ts` 完全一致，参数用 `params` 而不是 `data`）：

```ts
// src/api/waf/events.ts
import request from '@/utils/http'

export interface EventQuery {
  current?: number
  size?: number
  action?: string
  category?: string
  ip?: string
  keyword?: string
  from?: number
  to?: number
}

/** 分页查询攻击/访问事件 */
export function fetchEventList(params: EventQuery) {
  return request.get<Api.Waf.Paginated<Api.Waf.EventItem>>({
    url: '/api/events',
    params
  })
}

/** 导出 CSV（blob 直传，拦截器不会解析） */
export function fetchEventExport(params: EventQuery) {
  return request.get<Blob>({
    url: '/api/events/export',
    params,
    responseType: 'blob'
  })
}
```

### 9.2 路由与菜单

在 `src/router/modules/waf.ts` 新增一个模块（`component` 是相对 `src/views` 的路径字符串，由 `ComponentLoader` 解析）：

```ts
import { AppRouteRecord } from '@/types/router'

export const wafRoutes: AppRouteRecord = {
  path: '/waf',
  name: 'Waf',
  component: '/index/index',
  meta: {
    title: 'menus.waf.title',
    icon: 'ri:shield-check-line',
    keepAlive: false
  },
  children: [
    { path: 'dashboard', name: 'WafDashboard', component: '/waf/dashboard', meta: { title: 'menus.waf.dashboard', icon: 'ri:dashboard-line' } },
    { path: 'events',    name: 'WafEvents',    component: '/waf/events',    meta: { title: 'menus.waf.events',    icon: 'ri:file-list-3-line', keepAlive: true } },
    { path: 'ips',       name: 'WafIps',       component: '/waf/ips',       meta: { title: 'menus.waf.ips',       icon: 'ri:shield-user-line' } },
    { path: 'bans',      name: 'WafBans',      component: '/waf/bans',      meta: { title: 'menus.waf.bans',      icon: 'ri:forbid-line' } },
    { path: 'rules',     name: 'WafRules',     component: '/waf/rules',     meta: { title: 'menus.waf.rules',     icon: 'ri:code-box-line' } },
    { path: 'sites',     name: 'WafSites',     component: '/waf/sites',     meta: { title: 'menus.waf.sites',     icon: 'ri:global-line' } },
    { path: 'settings',  name: 'WafSettings',  component: '/waf/settings',  meta: { title: 'menus.waf.settings',  icon: 'ri:settings-3-line' } },
    { path: 'audit',     name: 'WafAudit',     component: '/waf/audit',     meta: { title: 'menus.waf.audit',     icon: 'ri:history-line' } }
  ]
}
```

再到 `src/router/modules/index.ts` 把这个模块加进导出数组即可，不用碰 `core`。

| 路径 | 页面 | 说明 |
| --- | --- | --- |
| `/waf/dashboard` | 态势大屏 | WAF 模块首页 |
| `/waf/events` | 事件日志 | 支持 query 参数直达筛选 |
| `/waf/ips` | IP 名单 | 黑/白名单 tab |
| `/waf/bans` | 封禁管理 | 当前封禁 + 历史 |
| `/waf/rules` | 规则管理 | 内置/自定义 tab |
| `/waf/sites` | 站点管理 | |
| `/waf/settings` | 系统设置 | 观察模式、阈值、告警 |
| `/waf/audit` | 审计日志 | |

**登录与守卫不用重写**：`Frontend` 已有登录页（`src/views/auth/login/index.vue`）和 `beforeEach` 守卫，后端只要把 `/api/auth/login`、`/api/user/info` 实现正确，现有登录流程直接可用。要改的只是登录页文案（换成 WAF 产品的标题与演示账号提示）。

**按钮级权限——别用错 API**（这里很容易踩坑）：

| 方式 | 数据来源 | 适用模式 |
| --- | --- | --- |
| `useAuth().hasAuth('ip:ban')` | 前端模式读 `/api/user/info` 的 **`buttons`** | **本项目（`VITE_ACCESS_MODE=frontend`）用这个** |
| `v-auth="'ip:ban'"` 指令 | 路由 `meta.authList` | 仅后端权限模式；前端模式下 `authList` 为空，元素会被直接移除 |

原因在 `src/hooks/core/useAuth.ts`：`isFrontendMode` 为真时走 `info.buttons.includes(auth)`，否则才查 `route.meta.authList`。而 `src/directives/core/auth.ts` 里写死只读 `meta.authList`。所以 WAF 页面统一用 `hasAuth()` + `v-if`，**不要用 `v-auth`**，否则按钮会莫名消失。

约定 WAF 的按钮码（后端 `sys_users.buttons` 字段返回，前端只做展示控制，**后端每个写接口仍要独立校验**）：

| 按钮码 | 控制 |
| --- | --- |
| `ip:ban` | 手动封禁 / 解封 |
| `rule:write` | 新增、修改规则 |
| `rule:delete` | 删除规则 |
| `site:write` | 站点增改 |
| `settings:write` | 改系统设置 / 切 protect 模式 |
| `audit:read` | 查看审计日志 |


### 9.3 页面设计

#### 9.3.1 态势大屏 `/waf/dashboard`

从上到下四段：

```
┌─────────────────────────────────────────────────────────────┐
│ 时间范围 [1h|6h|24h|7d]   站点 [全部 ▼]     自动刷新 [30s] │
├──────────┬──────────┬──────────┬──────────┬─────────────────┤
│ 总请求   │ 已拦截   │ 挑战     │ 拦截率   │ 当前 QPS        │
│ 1,284,201│ 3,842    │ 1,207    │ 0.30%    │ 132             │
│ ↑12%     │ ↓8%      │ ↑3%      │ -        │ 峰值 421        │
├──────────┴──────────┴──────────┴──────────┴─────────────────┤
│  请求 / 拦截趋势（面积图，双 Y 轴，可缩放）                  │
├───────────────────────────────┬─────────────────────────────┤
│ 攻击类型分布（环形图）         │ Top 10 攻击 IP（横向条形）  │
├───────────────────────────────┼─────────────────────────────┤
│ Top 10 被攻击路径（表格）      │ 实时事件流（滚动，最近 20 条）│
└───────────────────────────────┴─────────────────────────────┘
```

**指标口径必须写清楚**（否则数字对不上会被自己怀疑）：

| 指标 | 定义 |
| --- | --- |
| 总请求 | 进入 WAF 且匹配到站点的请求数（含静态资源） |
| 已拦截 | `action = block` 的数量（含 ban 后的直接拒绝） |
| 挑战 | `action = challenge` 的数量 |
| 拦截率 | 已拦截 / 总请求 |
| 当前 QPS | 最近 10 秒的滑动平均，不是瞬时值 |
| 攻击类型 | 按 `categories` 去重计数（一个请求命中多类只算主类） |

图表用 ECharts，按需引入 `LineChart`、`PieChart`、`BarChart`、`GridComponent`、`TooltipComponent`、`DataZoomComponent`，不要全量引入（体积会从 300KB 涨到 1MB）。

**地域分布不要用中国地图**：地图数据涉及测绘合规，且需要额外的地图 JSON。用"国家/省份 TopN 横向条形图"表达同样的信息，零合规风险。

#### 9.3.2 事件日志 `/waf/events`

布局：顶部筛选条 + 表格 + 右侧详情抽屉。

筛选条件：

- 时间范围（快捷：最近 15 分钟 / 1 小时 / 24 小时 / 7 天 + 自定义）
- 站点
- 动作（block / challenge / log / pass）
- 类型（sqli / xss / scanner / cc / ...）
- IP（支持 CIDR）
- 路径（模糊）
- 关键字（在 UA、Referer、Query 中搜索）

表格列：

| 列 | 宽度 | 说明 |
| --- | --- | --- |
| 时间 | 160 | `MM-DD HH:mm:ss`，悬停显示毫秒 |
| 站点 | 100 | |
| 来源 IP | 140 | 点击复制；旁挂"封禁"按钮 |
| 方法 | 70 | GET/POST 彩色标签 |
| 路径 | 自适应 | 超长省略，悬停显示全文 |
| 动作 | 90 | 红/橙/灰标签 |
| 评分 | 70 | 进度条形式，≥60 红色 |
| 类型 | 160 | 多个标签 |
| 状态码 | 80 | |
| 耗时 | 80 | ms |
| 操作 | 100 | 详情 / 封禁 / 加白 |

详情抽屉内容：

- 基本信息（Request ID、时间、IP、UA、Referer、上游）
- 命中规则列表（规则 ID、名称、分值、命中位置、证据片段，证据用 `<mark>` 高亮）
- 原始请求（方法、路径、Query、Header，**敏感头打码**）
- 处置结果与建议（"该 IP 近 1 小时已触发 23 次，建议封禁 24 小时"）

**实时模式**：右上角开关，开启后通过 SSE 追加新事件，最多保留 500 条，自动滚动到顶部；关闭时走普通分页查询。

#### 9.3.3 IP 名单 `/waf/ips`

两个 tab：黑名单 / 白名单。

- 顶部：搜索框、批量导入、导出、新增
- 表格：IP/CIDR、范围（全局/站点）、原因、到期时间、创建人、创建时间、操作
- 新增弹窗：支持单个 IP、CIDR、范围（`1.2.3.4-1.2.3.10`）、备注、到期时间（永久/1 天/7 天/30 天/自定义）
- 批量导入：文本域，每行一个，支持 `IP # 备注` 格式，导入前预览去重与非法行

**防误封交互**：新增黑名单时，如果该 IP 在最近 10 分钟内有过成功请求（2xx），弹二次确认："该 IP 近期有 132 次正常访问，确认封禁？"

#### 9.3.4 封禁管理 `/waf/bans`

- 当前生效：IP、原因、来源（自动/手动）、命中次数、剩余时间（倒计时）、解封按钮
- 历史记录：分页表格
- 顶部红色按钮"清空所有临时封禁"，二次确认 + 输入 `CLEAR` 才执行
- 自动封禁开关（关闭后只记录不封）

#### 9.3.5 规则管理 `/waf/rules`

- 左侧分类树：全部 / SQLi / XSS / 路径穿越 / 命令注入 / 扫描器 / 上传 / 自定义
- 中间表格：ID、名称、分类、分值、目标、动作、状态开关、内置标识
- 右侧/弹窗编辑：
  - 基本信息（名称、分类、分值、动作）
  - 匹配目标（多选框：path/query/body/header/cookie/ua）
  - 关键词（每行一个，用于 AC 预筛；留空则对所有目标跑正则）
  - 正则（输入后实时校验，非法立即提示）
  - 备注
- **规则测试器**（弹窗，独立 tab）：左边填样例请求，右边实时显示归一化结果、命中规则、总分、最终动作。这是最有价值的功能——改规则前先自测，避免上线才发现误报或漏报。

#### 9.3.6 站点管理 `/waf/sites`

表格列：名称、域名（多个标签）、上游、Body 检测、限流阈值、状态、操作。

编辑弹窗分组：

- 基本：名称、域名（多行，一行一个）、上游地址
- 检测：是否检测 Body、最大检测字节数、白名单路径（多行）
- 限流：每秒请求数、突发、每分钟上限
- 测试：保存前可点"连通性测试"，返回状态码与耗时

#### 9.3.7 系统设置 `/waf/settings`

分组卡片：

| 分组 | 项 |
| --- | --- |
| 运行模式 | 观察模式 / 防护模式（切换需二次确认，说明影响） |
| 评分阈值 | 拦截阈值、挑战阈值（滑块 + 数字） |
| 限流 | 单 IP RPS、每分钟上限、自动封禁开关、封禁时长 |
| 日志 | 保留天数、采样率、归档路径（只读） |
| 告警 | Webhook 地址、拦截数阈值、测试发送按钮 |
| 控制面安全 | 允许访问的 IP/CIDR、会话时长、修改密码 |

#### 9.3.8 审计日志 `/waf/audit`

表格：时间、操作人、来源 IP、动作、对象、变更摘要、结果。变更摘要用 diff 形式展示（`score: 30 → 50`）。

### 9.4 数据获取策略

| 数据 | 方式 | 频率 |
| --- | --- | --- |
| 大屏概览 | 轮询 | 30 秒 |
| 大屏趋势 | 轮询（带 `range` 参数） | 60 秒 |
| 实时事件 | SSE | 持续 |
| 事件列表 | 按需查询 | 用户操作 |
| 规则/站点/设置 | 按需查询 | 页面进入时 |
| 系统信息 | 轮询 | 10 秒（仅系统页） |

**SSE 连接管理**放在 Pinia store 里，全局只建一条连接，页面切换不重连；断线后指数退避重连（1s/2s/4s/8s，最大 30s），并在顶栏显示"实时连接已断开"。

### 9.5 视觉规范

**不新建一套主题**，沿用 `Frontend` 的 `--app-*` 变量与 naive-ui 的 `themeOverrides`，只补 WAF 需要几个语义色：

| 语义 | 色值 | 用途 |
| --- | --- | --- |
| 主色 | `var(--app-color-primary)` = `#2563EB` | 按钮、选中态（沿用框架） |
| block | `#DC2626` | 拦截 |
| challenge | `#F59E0B` | 人机校验 |
| log / observe | `#64748B` | 只记录 |
| pass | `#16A34A` | 放行 |

- 圆角、间距、字体**全部继承框架**，不要单独写一套；
- 数字用等宽字体对齐（`font-variant-numeric: tabular-nums`），趋势图刻度才不会抖；
- 表格斑马纹关闭，hover 用框架默认；动作标签统一封装成 `ArtActionTag`，避免每个页面手写颜色；
- 响应式：≥1440 三列，1024～1440 两列，<1024 单列 + 侧边栏折叠；
- **禁止硬编码色值**，一律走 `var(--app-*)`，否则暗色主题下会出现白底白字。

### 9.6 打包与集成

`Frontend/vite.config.ts` 基本不用动，只需要确认两件事：

```ts
// Frontend/.env.development —— 开发时把 API 代理指到 WAF 控制面
VITE_API_URL = /
VITE_API_PROXY_URL = http://127.0.0.1:8899
```

```ts
// Frontend/vite.config.ts —— 只需保证 manualChunks 里有 naive / echarts
build: {
  outDir: 'dist',
  assetsDir: 'assets',
  chunkSizeWarningLimit: 1200,   // naive-ui + echarts 会超默认 500
  rollupOptions: {
    output: {
      manualChunks: {
        vue: ['vue', 'vue-router', 'pinia'],
        naive: ['naive-ui'],
        echarts: ['echarts']
      }
    }
  }
}
```

生产部署有两条路，**必须二选一，不能既让 Go embed 又让 Apache 托管**（会出现新旧两份 dist 同时可访问）：

| 方式 | 做法 | 适用 |
| --- | --- | --- |
| A. embed 进二进制（推荐） | `pnpm build` → 拷到 `internal/web/dist` → `go build` | 单二进制交付、版本一致 |
| B. 交给 Apache | `pnpm build` → 产物放 `/www/wwwroot/waf-admin` → Apache vhost 指向它 | 前端要独立发版、频繁改动 |

开发时：`pnpm dev`（端口 3006）→ Vite 代理 `/api` 到 `127.0.0.1:8899`，同源代理，不产生跨域，**CORS 配置在这个阶段不会被触发**。只有在前后端分别用不同域名/端口直连时才会走到 8.10.4 的 CORS 逻辑。

> 注意 `Frontend` 仓库的 `AGENTS.md` 明确规定：构建与浏览器端验证由维护者本人执行，AI 助手只做 `vue-tsc --noEmit` 与静态自查。文档里的 `pnpm build` 步骤是给人执行的。


---

## 10. 性能与容量

### 10.1 目标

| 场景 | 目标 | 测试条件 |
| --- | --- | --- |
| 纯转发 | ≥ 30,000 QPS | 4 核 8G，1KB 响应，keep-alive |
| 开启检测（无命中） | ≥ 12,000 QPS | 30 条规则，1KB 请求 |
| 开启检测（高频命中） | ≥ 6,000 QPS | 10% 请求命中规则 |
| 附加延迟 | P50 ≤ 0.5ms，P99 ≤ 3ms | 小请求 |
| 内存 | ≤ 400MB（10 万 IP 限流状态） | |
| 日志写入 | ≥ 20,000 条/秒（批量） | |

### 10.2 性能热点

按影响排序：

1. **正则匹配**：最大头。解决：AC 预筛、编译缓存、限制目标数量。
2. **Body 读取与 JSON 解析**：解决：限长 256KB、超过不检测、JSON 只遍历字符串值且限制节点数。
3. **每请求内存分配**：`[]byte`、`map`、`string` 拼接。解决：`sync.Pool`、预分配、避免 `fmt.Sprintf` 拼日志（用结构化 logger）。
4. **限流 map 锁竞争**：解决：16 分片、读写分离、定期清理。
5. **日志同步写**：解决：有界 channel + 批量事务 + 丢弃策略。
6. **反向代理连接未复用**：解决：`MaxIdleConnsPerHost` 调到 256。

### 10.3 优化清单

| 优化点 | 做法 | 预期收益 |
| --- | --- | --- |
| AC 预筛 | 关键词命中才跑正则 | 减少 90% 正则调用 |
| 正则预编译 | 加载时 `regexp.Compile` | 避免每请求编译（ms 级） |
| 对象池 | `sync.Pool` 复用 `[]byte`、`[]int32`、`Event` | 降低 GC 30%+ |
| 分片锁 | 16 个 shard，按 IP 哈希 | 锁竞争下降 10 倍 |
| 批量写库 | 256 条/事务 | 写入提升 50 倍 |
| 采样 | 正常请求 1% 入库 | 日志量下降 99% |
| 静态资源跳过 | 后缀白名单，只转发不检测 | 大站静态流量占比高，收益明显 |
| 零拷贝转发 | 不读 body 时直接 `io.Copy` | 大文件下载 CPU 下降 |
| 关闭 Debug 日志 | `gin.SetMode(ReleaseMode)` | 控制面开销可忽略 |
| HTTP/2 | TLS 下开启 h2 | 客户端多路复用，减少握手 |

### 10.4 参数基线

```yaml
# 内核参数（/etc/sysctl.d/99-gowaf.conf）
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.ipv4.ip_local_port_range = 1024 65535
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fin_timeout = 15
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216
fs.file-max = 1048576
```

```ini
# systemd
LimitNOFILE=65535
```

Go 运行时：

- 不设置 `GOMAXPROCS`，默认等于 CPU 核数；
- 若与 Apache、MySQL 混部，建议 `GOMAXPROCS=核数-1`，给系统留余量；
- `GOGC` 默认 100；内存紧张时可设 `GOGC=50`（CPU 换内存），一般不需要。

### 10.5 压测方法

```bash
# 安装 hey
go install github.com/rakyll/hey@latest

# 基线：直连 Apache（改 hosts 指向 8080）
hey -n 200000 -c 200 -H "Host: www.example.com" http://127.0.0.1:8080/api/ping

# 对比：经过 WAF
hey -n 200000 -c 200 -H "Host: www.example.com" http://127.0.0.1/api/ping

# 带攻击特征（测检测开销）
hey -n 50000 -c 100 "http://127.0.0.1/api/user?id=1%20union%20select%201,2"
```

观察指标：

```bash
# WAF 自身
curl -s http://127.0.0.1:8899/metrics | grep waf_
# 系统
top -H -p $(pidof gowaf)
ss -s
```

验收标准：**开启检测后的 QPS 不低于直连的 60%，P99 延迟增加不超过 3ms**。

### 10.6 容量规划

| 站点规模 | 日请求量 | 建议配置 | 说明 |
| --- | --- | --- | --- |
| 小（< 10 万/天） | < 10 万 | 2C2G | 与 Apache 混部即可 |
| 中（10～100 万/天） | 10～100 万 | 4C4G | 推荐起点 |
| 大（100～1000 万/天） | 100～1000 万 | 8C8G + SSD | 日志采样率降到 0.1% |
| 超大（> 1000 万/天） | > 1000 万 | 多实例 + 共享封禁 | 见 16.4 |

---

## 11. WAF 自身安全

WAF 是"守门人"，自己被攻破比不装更糟。必须做：

| 风险 | 加固措施 |
| --- | --- |
| 控制台暴露 | 默认 `127.0.0.1`；远程用 SSH 隧道；确需公网时前面加 Nginx + Basic Auth + IP 白名单 + TLS |
| 弱口令 | 初始密码随机 16 位；bcrypt cost 12；登录失败 5 次锁定 15 分钟；禁止 `admin/admin` |
| JWT 泄露 | 短有效期（2h）+ refresh 可吊销；密钥文件 600；Cookie 通道加 `Secure; SameSite=Strict`；支持一键失效所有会话（清 `sys_refresh_tokens`） |
| XSS 偷 token | 前端 token 存 localStorage，必须靠 CSP 兜底：`default-src 'self'; script-src 'self'; object-src 'none'; base-uri 'self'`；所有渲染用户可控内容（UA、Path、Referer）走文本插值，禁止 `v-html` |
| CSRF | Cookie 通道双提交 Token；写操作校验 `Origin`/`Referer` 与 `Host` 同源；Authorization 头通道天然免疫 |
| 越权 | `buttons` 按钮码后端也要校验（前端 disabled 只是体验，不是权限）；所有写接口过 `RequirePerm()` |
| 演示模式写穿 | `demo.db` 与生产库路径比对；`DemoGuard` 在路由层兜底，不依赖前端隐藏按钮 |
| SQL 注入（自己） | GORM 用参数绑定；`Where("path = ?", p)` 而非 `fmt.Sprintf`；原生 SQL 一律 `?` 占位；表名/字段名用白名单映射 |
| 命令注入（自己） | 不调用 `os/exec`；如必须，参数数组传参，绝不拼 shell |
| 路径穿越（静态文件） | `http.FileServer` + `fs.Sub`，禁止 `..`；不提供任意文件下载接口 |
| SSRF | 站点上游地址校验：禁止 `127.0.0.1:8899`、`169.254.169.254`、`file://`、`gopher://` |
| 日志注入 | 换行符转义，避免伪造日志行；UA/Path 长度截断 |
| 依赖漏洞 | `govulncheck ./...` 纳入 CI；前端 `npm audit` |
| 命令注入（自己） | 不调用 `os/exec`；如必须，参数数组传参，绝不拼 shell |
| 路径穿越（静态文件） | `http.FileServer` + `fs.Sub`，禁止 `..`；不提供任意文件下载接口 |
| SSRF | 站点上游地址校验：禁止 `127.0.0.1:8899`、`169.254.169.254`、`file://`、`gopher://` |
| 日志注入 | 换行符转义，避免伪造日志行；UA/Path 长度截断 |
| 依赖漏洞 | `govulncheck ./...` 纳入 CI；前端 `npm audit` |
| 二进制篡改 | 发布时附 SHA256；部署后 `sha256sum -c` 校验 |
| 权限过大 | systemd `ProtectSystem=full`、`ReadWritePaths` 限定目录；仅读证书私钥需要 root |
| 规则文件被改 | 规则存 DB，文件仅作种子；DB 文件 600 权限 |

另外两个容易忽略的点：

1. **控制面不能成为攻击入口**：Gin 的 `NoRoute` 对 `/api/*` 返回 JSON 404，不能把 SPA 的 `index.html` 返回给 API 请求，否则前端会把 HTML 当 JSON 解析，报错信息还可能泄露路径。
2. **错误页不泄露内部信息**：拦截页只显示 Request ID 和一句提示，不显示命中的规则名（避免攻击者据此调 payload）。规则详情只在控制台可见。

---

## 12. 可观测性

### 12.1 Prometheus 指标

| 指标 | 类型 | 标签 | 说明 |
| --- | --- | --- | --- |
| `waf_requests_total` | Counter | site, action, status | 请求总数 |
| `waf_request_duration_seconds` | Histogram | site | 端到端耗时 |
| `waf_detect_duration_seconds` | Histogram | — | 检测耗时 |
| `waf_blocked_total` | Counter | site, category | 拦截数 |
| `waf_challenged_total` | Counter | site | 挑战数 |
| `waf_banned_total` | Counter | reason | 封禁数 |
| `waf_active_connections` | Gauge | listener | 当前连接数 |
| `waf_upstream_errors_total` | Counter | site, kind | 上游错误（超时/拒绝/5xx） |
| `waf_event_dropped_total` | Counter | — | 日志丢弃数（关键告警项） |
| `waf_db_write_seconds` | Histogram | — | 批量写入耗时 |
| `waf_rule_hits_total` | Counter | rule_id | 单规则命中数（用于调优） |
| `waf_goroutines` | Gauge | — | Goroutine 数 |
| `waf_memory_bytes` | Gauge | — | 内存占用 |

### 12.2 健康检查

```
GET /__waf/health   （仅允许 127.0.0.1 访问）
{
  "status": "ok",
  "version": "1.2.3",
  "uptime": "72h13m",
  "snapshot_version": 42,
  "db": "ok",
  "upstreams": { "官网": "ok", "商城": "ok" }
}
```

不要在这个接口里做重操作（比如查数据库全表），否则健康检查本身会成为负担。DB 检查用 `SELECT 1`，上游检查用缓存的最近一次结果。

### 12.3 告警

| 条件 | 级别 | 动作 |
| --- | --- | --- |
| 5 分钟内拦截数 > 500 | P1 | Webhook |
| 单 IP 5 分钟内触发 > 100 次 | P2 | Webhook（附 IP 和路径） |
| `waf_event_dropped_total` 持续增长 | P1 | Webhook（日志写入跟不上） |
| 上游错误率 > 5% | P1 | Webhook |
| 证书 30 天内到期 | P2 | Webhook |
| 磁盘使用率 > 85% | P1 | Webhook |
| 进程重启 | P2 | systemd + Webhook |

Webhook 消息模板（企业微信机器人）：

```json
{
  "msgtype": "markdown",
  "markdown": {
    "content": "**WAF 告警**\n> 站点：官网\n> 类型：拦截激增\n> 5 分钟拦截：<font color=\"warning\">1,284</font> 次\n> Top IP：45.12.34.56（321 次）\n> 时间：2026-11-02 16:40"
  }
}
```

### 12.4 日志分级

| 级别 | 内容 | 保留 |
| --- | --- | --- |
| ERROR | 启动失败、DB 写入失败、panic、证书加载失败 | 90 天 |
| WARN | 上游超时、规则编译失败、日志丢弃 | 30 天 |
| INFO | 配置变更、封禁、启动/停止 | 30 天 |
| DEBUG | 单请求检测详情（默认关闭） | 7 天 |

用 `lumberjack` 做轮转：单文件 100MB，保留 7 个，压缩旧文件。


---

## 13. 部署与上线（宝塔实操）

> 本章按已选定的「WAF 前置 + Apache 后移」方案编写。全程建议在业务低峰期（凌晨 3～5 点）操作，预留 30 分钟窗口，实际中断约 10～20 秒。

### 13.1 前置检查

```bash
# 1. 确认 80/443 当前由谁监听
ss -lntp | grep -E ':(80|443)\s'

# 2. 确认 Apache 版本与模块
/www/server/apache/bin/httpd -v
/www/server/apache/bin/httpd -M | grep -E 'proxy|remoteip|ssl|rewrite|headers'

# 3. 列出现有站点
ls -1 /www/server/panel/vhost/apache/

# 4. 确认没有 Nginx 同时占用 80
systemctl status nginx 2>/dev/null | head -5

# 5. 磁盘与内存
df -h /www
free -m
```

必须存在的模块：`mod_proxy`、`mod_proxy_http`、`mod_ssl`、`mod_remoteip`、`mod_rewrite`、`mod_headers`。缺 `remoteip` 会导致后端拿不到真实 IP；若 `httpd -M` 里没有，在 `httpd.conf` 加一行 `LoadModule remoteip_module modules/mod_remoteip.so` 后重启。

### 13.2 目录规划

```bash
mkdir -p /www/server/gowaf/{bin,data,logs,deploy}
mkdir -p /www/server/gowaf/data/archive
chmod 700 /www/server/gowaf/data
```

```
/www/server/gowaf/
├── bin/gowaf                 # 二进制（已 embed 前端 dist）
├── config.yaml               # 静态配置
├── data/
│   ├── waf.db                # SQLite 主库
│   ├── demo.db               # 演示库（仅 demo.enabled=true 时生成）
│   ├── jwt.key               # JWT 密钥（600）
│   ├── initial-password.txt  # 首次启动生成，登录后删除
│   └── archive/              # 冷归档
├── logs/
│   ├── waf.log
│   ├── stdout.log
│   └── stderr.log
└── deploy/
    ├── gowaf.service
    ├── switch-to-waf.sh
    └── rollback.sh
```

> 前端不必单独部署：`dist` 已经 embed 进二进制，`/www/server/gowaf` 下没有 web 目录。如果选 9.6 的方式 B（Apache 托管前端），才需要额外放 `/www/wwwroot/waf-admin`，此时要确认二进制不是用带 dist 的版本编译的，否则会出现两份可访问的前端。

### 13.3 第一步：WAF 先在测试端口跑通

不要一上来就抢 80，先用 `8081/8443` 验证转发和证书。

```yaml
# /www/server/gowaf/config.test.yaml
server:
  listen: "127.0.0.1:8081"
  tls_listen: "127.0.0.1:8443"
  admin_listen: "127.0.0.1:8899"
tls:
  enabled: true
  cert_dir: "/www/server/panel/vhost/cert"
  default_cert: "www.example.com"
storage:
  dsn: "/www/server/gowaf/data/waf-test.db"
sites:
  - name: "测试站点"
    hosts: ["www.example.com"]
    upstream: "http://127.0.0.1:80"   # 此时 Apache 还在 80
engine:
  mode: "observe"
```

```bash
/www/server/gowaf/bin/gowaf -c /www/server/gowaf/config.test.yaml -t   # 只校验配置
/www/server/gowaf/bin/gowaf -c /www/server/gowaf/config.test.yaml       # 前台启动看日志
```

另开一个终端验证：

```bash
# HTTP 转发（Apache 此时还在 80）
curl -I -H "Host: www.example.com" http://127.0.0.1:8081/

# HTTPS + SNI（--resolve 把域名指到本机 8443）
curl -kI --resolve www.example.com:8443:127.0.0.1 https://www.example.com:8443/

# 攻击特征应被记录（observe 模式返回 200，但控制台能看到事件）
curl -s -o /dev/null -w '%{http_code}\n' \
  -H "Host: www.example.com" \
  "http://127.0.0.1:8081/api/user?id=1%20union%20select%201,2"
```

确认三件事再往下走：响应头有 `X-Request-Id`、页面内容正常、控制台能看到事件。

### 13.4 第二步：Apache 后移

**① 备份**

```bash
BK=/root/waf-backup-$(date +%F-%H%M)
mkdir -p $BK
cp -a /www/server/apache/conf $BK/
cp -a /www/server/panel/vhost/apache $BK/
cp -a /www/server/panel/vhost/cert $BK/
echo $BK > /root/waf-backup-path.txt
```

**② 改主配置**

编辑 `/www/server/apache/conf/httpd.conf`：

```apache
# 原来：Listen 80
Listen 127.0.0.1:8080

# 原来：Listen 443（如果有）
# Listen 443
```

如果 `httpd.conf` 末尾 `Include conf/extra/httpd-ssl.conf`，把这一行注释掉（该文件里通常还有 `Listen 443` 和默认 SSL vhost）。

**③ 改 vhost**

对 `/www/server/panel/vhost/apache/*.conf` 逐个处理：

```bash
cd /www/server/panel/vhost/apache
# 备份已在 $BK，先看有多少文件
grep -l "VirtualHost \*:80" *.conf
```

用编辑器把每个站点的：

```apache
<VirtualHost *:80>
```
改为
```apache
<VirtualHost 127.0.0.1:8080>
```

带 SSL 的站点，整段 `<VirtualHost *:443>` 注释掉（用 `<!-- -->` 不行，Apache 用 `#` 逐行注释，或直接删掉该段，备份里有）。

> **不要用 sed 批量替换 443 段**，vhost 里有多层嵌套，正则很容易误伤。一个站点一个站点改，改完 `httpd -t` 校验。

**④ 加真实 IP 与协议还原**

在 `httpd.conf` 末尾或每个 vhost 内加：

```apache
# 信任来自本机 WAF 的转发头
RemoteIPHeader X-Forwarded-For
RemoteIPTrustedProxy 127.0.0.1

# 让 PHP 的 $_SERVER['HTTPS'] 正确
SetEnvIf X-Forwarded-Proto "^https$" HTTPS=on

# 日志用真实 IP（mod_remoteip 会把 %a 替换为客户端 IP）
LogFormat "%a %l %u %t \"%r\" %>s %b \"%{Referer}i\" \"%{User-Agent}i\" %D" combined
```

**⑤ 校验并重启**

```bash
/www/server/apache/bin/httpd -t
systemctl restart httpd
ss -lntp | grep 8080     # 应该看到 127.0.0.1:8080
```

此时公网 80 已经不通了（Apache 退到内网），必须在几分钟内完成下一步。

### 13.5 第三步：WAF 接管 80/443

修改 `/www/server/gowaf/config.yaml`：

```yaml
server:
  listen: "0.0.0.0:80"
  tls_listen: "0.0.0.0:443"
  admin_listen: "127.0.0.1:8899"
  trusted_proxies: ["127.0.0.1/32"]

sites:
  - name: "官网"
    hosts: ["www.example.com", "example.com"]
    upstream: "http://127.0.0.1:8080"
    detect_body: true
    max_body_bytes: 262144
    whitelist_paths:
      - "/api/pay/notify"
      - "/.well-known/acme-challenge/*"
  - name: "商城"
    hosts: ["shop.example.com"]
    upstream: "http://127.0.0.1:8080"
    detect_body: true
    whitelist_paths: ["/api/wx/callback"]
```

> `sites` 只在**首次启动**时作为种子导入 SQLite，之后以控制台/数据库为准。改 YAML 不会覆盖线上配置，避免误操作。

```bash
/www/server/gowaf/bin/gowaf -c /www/server/gowaf/config.yaml -t
cp /www/server/gowaf/deploy/gowaf.service /etc/systemd/system/gowaf.service
systemctl daemon-reload
systemctl enable --now gowaf
systemctl status gowaf --no-pager
ss -lntp | grep -E ':(80|443|8899)\s'
```

期望输出：80/443 是 gowaf，8080 是 httpd，8899 是 gowaf（仅 127.0.0.1）。

### 13.6 第四步：验证清单

逐条打勾，任何一条不过都不要切 protect 模式。

| # | 检查项 | 命令 | 期望 |
| --- | --- | --- | --- |
| 1 | HTTP 首页 | `curl -I -H "Host: www.example.com" http://127.0.0.1/` | 200，有 `X-Request-Id` |
| 2 | HTTPS 首页 | `curl -kI --resolve www.example.com:443:127.0.0.1 https://www.example.com/` | 200，证书正确 |
| 3 | 静态资源 | `curl -I https://www.example.com/assets/app.js` | 200，`Content-Type` 正确 |
| 4 | API 转发 | `curl -X POST https://www.example.com/api/login -d '{}'` | 后端真实响应 |
| 5 | 真实 IP | 查看 Apache access.log | 记录的是客户端 IP，不是 127.0.0.1 |
| 6 | HTTPS 标识 | 后端输出 `$_SERVER['HTTPS']` | `on` |
| 7 | 拦截生效 | `curl "http://127.0.0.1/.env" -H "Host: www.example.com"` | 403（protect 模式） |
| 8 | 事件入库 | 控制台事件列表 | 能看到上面这条 |
| 9 | WebSocket | 打开站内实时功能 | 正常连接 |
| 10 | 大文件上传 | 上传 20MB 文件 | 成功，内存无明显上涨 |
| 11 | 证书续期路径 | `curl http://www.example.com/.well-known/acme-challenge/test` | 透传到 Apache（404 也算通） |
| 12 | 控制台隔离 | `curl http://<公网IP>:8899` | 连接被拒绝 |

### 13.7 回滚预案（60 秒内完成）

```bash
#!/bin/bash
# /www/server/gowaf/deploy/rollback.sh
set -e
BK=$(cat /root/waf-backup-path.txt)

systemctl stop gowaf

# 恢复 Apache 配置
cp -a $BK/conf/httpd.conf /www/server/apache/conf/httpd.conf
cp -a $BK/apache/*.conf /www/server/panel/vhost/apache/

/www/server/apache/bin/httpd -t
systemctl restart httpd

echo "已回滚，Apache 重新监听 80/443"
ss -lntp | grep -E ':(80|443)\s'
```

把这个脚本放在手边（本地也存一份），切换前先执行一次 `bash -n` 检查语法。

### 13.8 宝塔面板注意事项

| 事项 | 说明 |
| --- | --- |
| 面板端口 | 宝塔面板默认 `8888`，与 WAF 无关，不要动 |
| 网站 SSL 页 | 在面板里开关 SSL 会**重写 vhost**，把我们改的 `127.0.0.1:8080` 覆盖回 `*:80`。切换后不要再从面板操作 SSL |
| 配置文件保存 | 面板「配置文件」里保存同样会重写，改配置请直接改文件并 `httpd -t` |
| 防火墙 | 只放行 80/443；8899 绝对不要放行；宝塔「安全」页面里删除 8899 规则 |
| 计划任务 | 宝塔的「网站监控」可能探测 80 端口判断 Apache 状态，会误报"Apache 未启动"，忽略即可 |
| 证书续期 | 宝塔续期后文件会更新，WAF 60 秒内自动加载；续期验证走 80，已加白名单 |
| 日志切割 | 宝塔会切割 Apache 日志，WAF 日志用 lumberjack 自己切，互不影响 |
| 面板备份 | 宝塔「计划任务 → 备份网站」只备份 `wwwroot`，记得单独备份 `/www/server/gowaf/data/waf.db` |

**防止面板覆盖的兜底**：写一个 `deploy/reapply.sh`，在每次从面板操作过网站配置后执行，把 vhost 的 `VirtualHost *:80` 批量改回 `127.0.0.1:8080` 并重启 Apache。把它加到宝塔的计划任务里每天跑一次也无妨（幂等）。

### 13.9 灰度节奏

| 阶段 | 时长 | 模式 | 观察重点 |
| --- | --- | --- | --- |
| 1. 影子 | 3～7 天 | `observe` | 误报清单、规则命中分布、性能 |
| 2. 小流量 | 1～2 天 | `protect` + 只开扫描器/敏感路径 | 是否有正常请求被拦 |
| 3. 全量 | — | `protect` + 全规则 | 拦截率、5xx、上游错误 |
| 4. 调优 | 持续 | — | 按 `waf_rule_hits_total` 调整分值与阈值 |

**观察期的核心产出**是一张"误报清单"：把 `action=log` 且 `score>=30` 的事件按路径分组，人工看前 100 条，确认哪些规则在该站点不适用，加入站点白名单或降分。没有这一步，直接开 protect 大概率会打掉某个后台接口。

---

## 14. 测试方案

### 14.1 单元测试

| 模块 | 用例 |
| --- | --- |
| `normalize` | 单次/多次 URL 编码、HTML 实体、全角、SQL 注释、混合编码 |
| `AC` | 空串、重叠关键词、长文本、UTF-8 中文、未命中 |
| `engine` | 单规则命中、多规则叠加、去重、阈值边界（59/60/61） |
| `router` | 精确/通配/未知 Host、带端口、大小写、末尾点 |
| `limiter` | 令牌桶速率、突发、清理、分片并发 |
| `store` | 批量插入、分页游标、归档、迁移 |
| `auth` | 密码校验、JWT 过期、CSRF、IP 白名单 |

### 14.2 规则回归语料

建两个目录，纳入 CI：

```
testdata/
├── attacks/          # 必须命中（正样本）
│   ├── sqli.txt      # 每行一个 payload
│   ├── xss.txt
│   ├── traversal.txt
│   ├── cmdi.txt
│   └── scanner.txt
└── normal/           # 必须不命中（负样本）
    ├── api.txt
    ├── search.txt
    └── richtext.txt
```

```go
func TestAttackCorpus(t *testing.T) {
    eng := loadTestEngine(t)
    files, _ := filepath.Glob("testdata/attacks/*.txt")
    for _, f := range files {
        for _, line := range readLines(t, f) {
            v := eng.Inspect(queryRequest(line))
            if v.Action != ActionBlock {
                t.Errorf("%s: %q 未拦截，score=%d", filepath.Base(f), line, v.Score)
            }
        }
    }
}

func TestNormalCorpus(t *testing.T) {
    eng := loadTestEngine(t)
    files, _ := filepath.Glob("testdata/normal/*.txt")
    for _, f := range files {
        for _, line := range readLines(t, f) {
            v := eng.Inspect(queryRequest(line))
            if v.Action == ActionBlock {
                t.Errorf("%s: 误报 %q，命中 %v", filepath.Base(f), line, v.RuleIDs())
            }
        }
    }
}
```

负样本从真实 access.log 里抽样生成（去掉明显攻击的），比手写更贴近业务。

### 14.3 集成测试

用 `httptest.NewServer` 起一个假后端，覆盖：

- 正常 GET/POST 转发，Header 与 body 完整；
- 大 body（1MB/10MB）不丢数据；
- 上游超时返回 502；
- 上游返回 500 时事件记录状态码；
- WebSocket 握手（用 `httptest` + `gorilla/websocket` 客户端）；
- 客户端断开时不产生 5xx 事件。

### 14.4 压测

见 10.5。补充两个场景：

- **长连接**：`hey -n 100000 -c 500 -disable-keepalive=false`，观察 `TIME_WAIT` 数量；
- **慢请求**：后端 `sleep 5`，确认 WAF 不会因为 `ResponseHeaderTimeout` 误杀（按站点配置到 30s）。

### 14.5 攻击模拟（仅限自己的站点，需书面授权）

| 工具 | 用途 | 注意 |
| --- | --- | --- |
| `curl` 手工 payload | 验证单条规则 | 最可控 |
| `sqlmap -u "..." --batch --level=1` | 验证 SQLi 拦截 | 会打大量请求，低峰期跑 |
| `nikto -h https://...` | 验证扫描器识别 | 会被封 IP，先把自己加白 |
| `nuclei -u https://... -severity low` | 验证常见漏洞探测拦截 | 规则多，耗时长 |
| `ffuf -u https://.../FUZZ` | 验证目录爆破限流 | 控制并发，避免打挂 |

验收：攻击工具应在 10～20 个请求内被识别并封禁，且控制台能看到完整链路。

### 14.6 验收标准

| 指标 | 标准 |
| --- | --- |
| 已知攻击拦截率 | ≥ 95%（语料库） |
| 正常请求误报率 | ≤ 0.1%（观察期统计） |
| 附加延迟 P99 | ≤ 3ms |
| 吞吐衰减 | ≤ 40%（对比直连） |
| 连续运行 7 天 | 无内存泄漏（RSS 波动 < 10%）、无 panic |
| 配置热更新 | 生效 < 1s，不中断连接 |

---

## 15. 迭代计划

单人开发，按 6 周排：

| 周 | 目标 | 交付物 | 验收 |
| --- | --- | --- | --- |
| W1 | 骨架 + 代理 | 配置加载、站点路由、ReverseProxy、TLS 证书、systemd | 能代理真实站点，HTTPS 正常 |
| W2 | 基础防护 | IP 黑白名单、限流、事件模型、SQLite（原生 SQL 部分）、日志管道 | 能封 IP、能查日志 |
| W3 | 规则引擎 | 归一化、AC、规则编译、内置规则、评分 | 语料库通过率 ≥ 90% |
| W4 | 控制面骨架 | **GORM 建库 + AutoMigrate + 种子用户**；Gin 路由装配；**JWT 双令牌**；CORS / CSRF / 登录限流；`/api/auth/login`、`/api/user/info` 跑通 | 用 `curl` 完整走通「登录 → 拿 token → 查 info」 |
| W5 | 控制面业务 | 事件/规则/站点/IP/设置接口（GORM CRUD + 原生 SQL 查询）；SSE；审计；**演示模式** | Postman 全接口通；`demo.enabled=true` 下写操作全被拦 |
| W6 | 前端 + 上线 | `Frontend` 里加 `waf` 路由模块、8 个页面、`ArtXxx` 组件、i18n；压测、观察模式、灰度、调优 | 观察 3 天无误报，切 protect |

**W4 关键路径**：先把 `Frontend` 现有登录页跑通（它已经在调 `/api/auth/login` 和 `/api/user/info`），再去写业务接口。这一步通了，后面每加一个页面都是"照契约实现 + 联调"，不会卡在鉴权上返工。

**W5 与 W6 可以交错**：接口写一个、页面对一个，比"接口全写完再写页面"快，也更容易发现契约理解偏差。

**每周留半天做"还债"**：补测试、修 TODO、更新文档。否则第 6 周会被债务拖死。

如果时间紧，砍掉顺序：上传检测 → 响应检测 → 审计 → 规则测试器 → 人机校验（但规则测试器其实很省时间，建议保留）。

---

## 16. 风险与对策

### 16.1 技术风险

| 风险 | 概率 | 影响 | 对策 |
| --- | --- | --- | --- |
| 误报打掉正常业务 | 高 | 高 | 观察模式 ≥ 3 天；白名单；一键回滚；按路径降级为 log |
| 性能不达标 | 中 | 中 | AC 预筛、限流、静态资源跳过；压测前置；必要时加机器 |
| 规则被绕过（编码/分块/混淆） | 高 | 中 | 多层归一化；不追求 100%；配合业务修复根本漏洞 |
| 单点故障 | 中 | 高 | systemd 自动重启；`recover`；上游健康检查；回滚脚本 |
| 证书加载失败 | 低 | 高 | 启动校验；默认证书兜底；加载失败保留旧证书并告警 |
| SQLite 写放大 | 中 | 中 | 批量事务、WAL、采样、归档；监控 `waf_db_write_seconds` |
| 内存泄漏（限流 map） | 中 | 中 | 定期清理、分片、上限保护；7 天稳定性测试 |
| 宝塔覆盖配置 | 高 | 中 | 备份 + `reapply.sh` + 不从面板改 SSL |
| 日志爆盘 | 中 | 高 | 保留策略、归档、磁盘告警、采样 |
| 自身被攻破 | 低 | 极高 | 控制面回环、强密码、依赖扫描、最小权限 |
| CGO 依赖混入导致交叉编译失败 | 中 | 中 | 锁死 `glebarez/sqlite` + `x/crypto/bcrypt`；CI 第一步就跑 `CGO_ENABLED=0 go build` |
| GORM 用在事件热路径拖慢写入 | 中 | 高 | 明确边界：ORM 只碰配置表，`events`/`stats_1m` 走原生 SQL（见 6.1） |
| `AutoMigrate` 改列丢数据 | 中 | 高 | 只在首建库用；之后一律显式 `migrations/*.sql`（SQLite 改列是重建表） |
| 前后端契约漂移（`code`/`current`/`records` 对不上） | 高 | 中 | 后端按 7.1 实现；联调第一件事就是打通 `/api/auth/login` + `/api/user/info` |
| 演示模式误连生产库 | 低 | 极高 | 启动时比对 `demo.db_path` 与 `storage.dsn`，相同直接拒绝启动 |

### 16.2 运营风险

- **规则长期不更新**：新漏洞（如某框架 RCE）出现后，通用规则可能覆盖不到。对策：每月花 30 分钟看一次安全资讯，补充路径/特征规则；或定期同步 OWASP CRS 的子集。
- **只靠 WAF 不修代码**：WAF 是止血带，不是疫苗。SQLi 规则再全，也挡不住业务逻辑漏洞（越权、支付篡改）。必须同步做代码审计。
- **封禁误伤 NAT 用户**：公司/校园出口共享 IP，封一个影响一片。对策：优先挑战而非直接封；封禁时长从 5 分钟起，递增。

### 16.3 合规风险

- 日志中可能包含手机号、身份证、Cookie，属于个人信息，需脱敏后存储；
- 留存 6 个月是下限，不是上限；到期应删除，避免无限积累；
- 控制台账号密码、JWT 密钥不能写进代码仓库；
- 如果站点涉及等保测评，WAF 的拦截日志可作为"入侵防范"条款的证据，记得保留导出功能。

### 16.4 演进路径

单机跑稳之后，如果请求量继续上涨或多机部署，按这个顺序演进：

1. **多实例 + 共享封禁**：WAF 部署在每台 Web 前，封禁列表写 Redis（`SADD banned:ips` + TTL），各实例订阅变更；本地再缓存一层，避免每请求查 Redis。
2. **配置中心化**：SQLite 换成 MySQL/PostgreSQL，控制台独立部署，多实例从同一库拉配置。
3. **规则语义化**：从正则升级到语法分析（参考 libinjection 的思路），降低误报。
4. **指纹识别**：接入 JA3/JA4（TLS 指纹）、HTTP/2 指纹，识别改 UA 的扫描器。
5. **情报联动**：接入 IP 信誉库（微步、AbuseIPDB），对高风险 IP 直接挑战。

---

## 17. 附录

### A. 规则 YAML 完整示例

```yaml
# rules/builtin.yaml
version: 1
rules:
  - id: sqli-union
    name: SQL 联合查询注入
    category: sqli
    score: 50
    targets: [query, body, cookie]
    keywords: ["union select", "union all select", "union/**/select"]
    pattern: 'union\s+(all\s+)?select\b'
    action: block
    enabled: true
    remark: 覆盖最常见的联合查询注入

  - id: sqli-time
    name: SQL 时间盲注
    category: sqli
    score: 60
    targets: [query, body]
    keywords: ["sleep(", "benchmark(", "pg_sleep("]
    pattern: '(sleep|benchmark|pg_sleep)\s*\(\s*\d'
    action: block
    enabled: true

  - id: xss-script
    name: XSS script 标签
    category: xss
    score: 40
    targets: [query, body, header]
    keywords: ["<script", "<script>"]
    pattern: '<\s*script[^>]*>'
    action: block
    enabled: true

  - id: trav-dotdot
    name: 路径穿越
    category: traversal
    score: 45
    targets: [path, query]
    keywords: ["../", "..\\"]
    pattern: '(\.\./){2,}|(\.\.\\){2,}'
    action: block
    enabled: true

  - id: cmdi-shell
    name: 命令注入-管道执行
    category: cmdi
    score: 55
    targets: [query, body]
    keywords: [";wget", "|curl", "$(whoami", "`id`"]
    pattern: '[;|`]\s*(cat|ls|whoami|id|uname|curl|wget|nc|bash|sh)\b'
    action: block
    enabled: true

  - id: scan-path-env
    name: 探测 .env 文件
    category: sensitive_file
    score: 45
    targets: [path]
    keywords: ["/.env"]
    pattern: '^/\.env(\.|$)'
    action: block
    enabled: true

  - id: scan-ua-sqlmap
    name: 扫描器 UA - sqlmap
    category: scanner
    score: 30
    targets: [ua]
    keywords: ["sqlmap", "nikto", "nmap", "masscan", "gobuster", "dirsearch", "nuclei", "xray"]
    pattern: '(?i)(sqlmap|nikto|nmap|masscan|gobuster|dirsearch|nuclei|xray)'
    action: block
    enabled: true

  - id: upload-php
    name: 上传 PHP 可执行文件
    category: upload
    score: 80
    targets: [path, header]
    keywords: [".php", ".phtml", ".php5"]
    pattern: '\.(php[0-9]?|phtml|pht)(\.|$)'
    action: block
    enabled: true
    remark: 仅对上传接口生效，按站点路径白名单控制
```

字段说明：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `id` | 是 | 全局唯一，内置规则以 `类别-编号` 命名 |
| `targets` | 是 | `path/query/body/header/cookie/ua` 的组合 |
| `keywords` | 否 | AC 预筛关键词，**必须能在归一化后的文本中出现**，否则规则永不命中 |
| `pattern` | 否 | RE2 正则；与 keywords 同时存在时是"与"关系 |
| `score` | 是 | 命中后累加的分值 |
| `action` | 是 | `block` / `challenge` / `log` |

> 写规则时最容易犯的错：关键词写了大写或带空格变体，但归一化会把文本转小写并折叠空白，导致 AC 永远不命中，规则形同虚设。**加规则后必须用测试器验证一次。**

### B. 拦截页

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>请求被拦截</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      min-height: 100vh; display: grid; place-items: center;
      background: #f6f8fb; color: #1e293b;
      font: 15px/1.7 -apple-system, "PingFang SC", "Microsoft YaHei", sans-serif;
    }
    .box {
      max-width: 520px; padding: 48px 40px; background: #fff;
      border: 1px solid #e5e9f2; border-radius: 16px; text-align: center;
      box-shadow: 0 12px 40px rgba(30, 41, 59, .06);
    }
    .icon {
      width: 64px; height: 64px; margin: 0 auto 24px;
      display: grid; place-items: center; border-radius: 16px;
      background: #fef2f2; color: #dc2626; font-size: 28px;
    }
    h1 { font-size: 20px; margin-bottom: 12px; }
    p { color: #64748b; margin-bottom: 8px; }
    .id {
      margin-top: 24px; padding: 12px; background: #f8fafc;
      border-radius: 8px; font-family: ui-monospace, monospace;
      font-size: 13px; color: #475569; word-break: break-all;
    }
  </style>
</head>
<body>
  <div class="box">
    <div class="icon">!</div>
    <h1>请求已被安全策略拦截</h1>
    <p>如果这是误判，请联系站点管理员并提供下方编号。</p>
    <div class="id">Request ID: {{REQUEST_ID}}</div>
  </div>
</body>
</html>
```

> 页面里**不要**显示命中的规则名、评分、WAF 版本。这些信息会帮助攻击者判断规则边界。

### C. 常见扫描路径（节选）

```
/.env                     /.env.local               /.env.production
/.git/config              /.git/HEAD                /.svn/entries
/.DS_Store                /web.config               /config.php.bak
/backup.zip               /www.zip                  /db.sql
/phpinfo.php              /info.php                 /test.php
/adminer.php              /phpmyadmin/              /pma/
/wp-login.php             /wp-admin/                /xmlrpc.php
/actuator                 /actuator/env             /actuator/heapdump
/swagger-ui.html          /v2/api-docs              /v3/api-docs
/druid/index.html         /jmx-console/             /web-console/
/manager/html             /host-manager/html        /solr/admin/
/_cat/indices             /elasticsearch/           /console/
/server-status            /server-info              /.well-known/security.txt
/vendor/phpunit/          /vendor/phpunit/phpunit/src/Util/PHP/eval-stdin.php
/index.php?s=/Index/\think\app/invokefunction
/wp-content/plugins/...   /shell.php                /upload.php
```

### D. 常用命令速查

```bash
# 配置校验（不启动）
/www/server/gowaf/bin/gowaf -c config.yaml -t

# 登录取 token（裸 token，放进 Authorization 头）
TOKEN=$(curl -s -X POST http://127.0.0.1:8899/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"userName":"admin","password":"******"}' | jq -r '.data.token')

# 热加载规则（不重启）
curl -X POST -H "Authorization: $TOKEN" http://127.0.0.1:8899/api/settings/reload
# 或
kill -HUP $(pidof gowaf)

# 查看监听
ss -lntp | grep -E ':(80|443|8080|8899)\s'

# 查看实时日志
tail -f /www/server/gowaf/logs/waf.log | jq .

# 测试拦截
curl -s -o /dev/null -w '%{http_code}\n' -H "Host: www.example.com" "http://127.0.0.1/.env"

# 查看封禁列表
sqlite3 /www/server/gowaf/data/waf.db \
  "SELECT ip, reason, datetime(expires_at/1000,'unixepoch','localtime') FROM bans WHERE released_at IS NULL;"

# 备份数据库（热备，安全）
sqlite3 /www/server/gowaf/data/waf.db ".backup '/www/server/gowaf/data/waf-$(date +%F).db'"
```

### E. 术语表

| 术语 | 含义 |
| --- | --- |
| WAF | Web Application Firewall，Web 应用防火墙 |
| 反向代理 | 代理服务器接收客户端请求，转发给后端，再把响应返回客户端 |
| 数据面 / 控制面 | 前者处理业务流量，后者提供管理与配置能力 |
| CC 攻击 | Challenge Collapsar，通过大量请求耗尽应用资源的攻击 |
| AC 自动机 | Aho-Corasick，多模式字符串匹配算法，一次扫描找出所有关键词 |
| RE2 | Go 正则引擎，线性时间、无回溯，天然防 ReDoS |
| 观察模式 | 只记录不拦截，用于校准规则、评估误报 |
| 评分制 | 多个规则命中累加分数，达到阈值才处置，降低单条规则误报的影响 |
| SNI | TLS 握手时客户端明示的域名，用于选择证书 |
| SSE | Server-Sent Events，服务器单向推送事件流 |
| 真实 IP | 经过代理链后还原出的客户端原始 IP |
| 热更新 | 不重启进程，动态加载新配置/规则 |

### F. 参考实现与资料

- Go 标准库 `net/http/httputil.ReverseProxy`：代理实现的基础，重点看 `Rewrite`、`ModifyResponse`、`ErrorHandler`、`FlushInterval`
- OWASP CRS（Core Rule Set）：规则设计的事实标准，可把核心规则转成 YAML 导入
- libinjection：SQLi/XSS 词法分析思路，比纯正则更抗混淆
- Coraza：Go 实现的 WAF 引擎，值得对照其规则匹配与事务模型
- 雷池 SafeLine：控制台交互与产品化的参考
- `modernc.org/sqlite`：纯 Go SQLite 驱动，免 CGO
- `golang.org/x/time/rate`：令牌桶实现
- `github.com/golang-jwt/jwt/v5`：JWT

---

## 结语

这套方案的核心不是"写一个多强的规则库"，而是三件事：

1. **把流量变成可观测的数据**——先看得见，才谈得上防得住；
2. **把处置变成可回滚的动作**——观察模式、白名单、版本回滚，比多写 100 条规则更重要；
3. **把组件做成可运维的服务**——systemd、热更新、健康检查、告警，决定它能不能在线上活过三个月。

规则可以慢慢加，架构一开始就要留好口子。按第 15 章的节奏推进，第 3 周就能看到第一条拦截记录，第 6 周可以放心把 80 端口交给它。

---

## 18. 实现进度与踩坑记录

> 本节记录 v1.0 落地过程中的实际状态、偏离方案的地方和踩到的坑。
> 与前面章节冲突时，以本节为准（前面是设计意图，本节是已运行的事实）。

### 18.1 交付状态

| 模块 | 状态 | 说明 |
| --- | --- | --- |
| 数据面（检测/限流/封禁/转发） | ✅ 已实现 | `internal/edge`，标准库 + `httputil.ReverseProxy` |
| 规则引擎（归一化 + AC 预筛 + RE2 + 评分） | ✅ 已实现 | 30 条内置规则，`rules/builtin.yaml` |
| 控制面 API（鉴权/事件/规则/名单/站点/配置/审计） | ✅ 已实现 | Gin + GORM + 纯 Go SQLite |
| SSE 实时事件流 | ✅ 已实现 | `/api/stream/events`，事件帧与列表 DTO 同构 |
| 前端 WAF 模块（naive-ui 8 页） | ✅ 已实现 | 见 18.2 文件清单 |
| 前端内嵌单二进制 | ✅ 已实现 | `internal/web/dist` + `go:embed all:dist` |
| 版本注入与交叉编译 | ✅ 已实现 | `Makefile`，`CGO_ENABLED=0` |

### 18.2 前端新增文件清单

只新增、不改框架内核（`src/router/core/*`、`src/utils/http/*`、`src/store/modules/user.ts` 未改动）。

```
Frontend/src/
├── api/waf/
│   ├── index.ts            全部接口（按模块分组）
│   └── types.ts            与后端 DTO 一一对应的类型
├── components/core/waf/
│   ├── art-waf-action-tag/     处置结果标签（pass/log/challenge/block）
│   ├── art-waf-stat-card/      指标卡（渐变图标底 + 千分位数值）
│   ├── art-waf-trend-chart/    ECharts 三线趋势（复用 useChart 主题适配）
│   ├── art-waf-rule-tester/    规则在线测试器
│   └── art-waf-event-drawer/   事件详情抽屉 + 就地封禁
├── router/modules/waf.ts   模块路由，已注册进 modules/index.ts
├── store/modules/waf.ts    全局筛选 + 系统信息 + SSE 连接管理
└── views/waf/
    ├── dashboard/   安全总览（6 指标卡 / 趋势 / 分类 / Top3 / 系统状态 / 实时流）
    ├── events/      攻击事件（多维筛选 / 导出 CSV / 详情 / 封禁）
    ├── rules/       检测规则（CRUD / 启停 / 重载 / 在线测试器）
    ├── ips/         黑白名单（单条 / 批量 / CIDR 导入）
    ├── bans/        封禁管理（手动封禁 / 解封 / 清空）
    ├── sites/       防护站点（上游 / 域名 / 免检路径 / 连通性测试）
    ├── settings/    防护配置（阈值 / 限流 / 自动封禁 / 版本回滚）
    └── audit/       审计日志（按人 / 对象 / 时间筛选）
```

### 18.3 踩坑清单（按影响排序）

**① GORM 的 `default` tag 会吞掉零值——自建规则变成"内置规则"**

`model.Rule.Builtin` 原本带 `gorm:"default:1"`。GORM 对「有 default 且值为零值」的字段会**省略该列**，
于是 `RuleCreate` 里显式写的 `Builtin: 0` 被丢弃，落库成 1，导致自建规则删除时被
"内置规则不可删除" 拦下。

```go
// 错误：零值被 default 覆盖
Builtin int `gorm:"column:builtin;not null;default:1"`

// 正确：去掉 default，让零值真正落库
Builtin int `gorm:"column:builtin;not null"`
```

同样的问题适用于任何"零值有业务含义"的布尔/枚举列（如 `released_at = 0` 表示未解封）。
新增此类字段时，不要给 `default`，改由代码显式赋值。

**② 生产环境 `VITE_API_URL` 不能写死外部地址**

`Frontend/.env.production` 原本指向 Apifox mock。前端被 `go:embed` 内嵌后与 `/api` 同源，
若 baseURL 仍是远端地址，所有请求会打到 mock 上，表现为登录页报"无效的令牌"、路由守卫
"动态路由注册失败"。

```dotenv
# .env.production
VITE_API_URL = /
```

**③ 前端是 hash 路由，不是 history 路由**

`src/router/index.ts` 用 `createWebHashHistory()`，真实地址形如 `/#/waf/dashboard`。
影响两处：

- 部署/反代时不需要给 `try_files` 配 history fallback（请求 path 永远是 `/`）；
- 自动化测试、分享链接、埋点上报都要带 `#`。后端 `mountSPA` 的 fallback 保留着，
  只是为了兼容误输入的非 hash 路径，不是路由依赖。

**④ SSE 推送帧必须与列表 DTO 同源**

最初 `app.go` 里手写 `map[string]any` 拼 SSE 帧，导致：
- `categories` / `ruleIds` 在 model 里是 JSON **字符串**，被二次序列化成 `"[\"sqli\"]"`；
- 缺 `id` / `query` / `host` / `referer` / `latencyUs` / `upstream`；
- 前端按 `typeof payload.id === 'number'` 过滤时全部丢弃 → 实时流永远空白。

修复方式是让推送复用 `eventDTO`：

```go
type eventStreamMsg struct {
    eventDTO
    Type string `json:"type"`
}

func (s *Server) MarshalEvent(e model.Event) ([]byte, error) {
    return json.Marshal(eventStreamMsg{eventDTO: s.toDTO(e), Type: "event"})
}
```

注意事件是**异步批量落库**的，推送时主键可能尚未分配（`id = 0`），
前端须以 `reqId` 作为去重与列表 key。

**⑤ 规则检测目标的合法取值**

`validateRule` 只接受 `path` / `query` / `body` / `header` / `cookie` / `ua`。
写成 `url` 会返回 `400 未知检测目标: url`。前端下拉选项必须与此严格一致。

**⑥ `range` 参数的白名单**

`parseRange` 原本只认 `15m/1h/6h/24h/7d`，传 `30d` 会**静默回落到 1h**（不报错）。
已补上 `30d`，并调整 `bucketSeconds`：7d 用 1 小时桶（约 168 点），30d 用 6 小时桶（约 120 点）。

**⑦ naive-ui 组件宽度要用内联 style，不要用 Tailwind class**

给 `NSelect` / `NInput` 加 `class="w-32"` 不生效（组件内部根节点宽度由自身样式管理），
必须写 `style="width: 128px"`。同理，`NDataTable` 固定 `height` 会在数据少时留出大片空白，
改用 `:max-height` 让表格按内容自适应。

**⑧ `useMessage()` 在本项目不可用**

框架没有挂 `NMessageProvider`，调用 `useMessage()` 会抛错。统一用
`@/utils/http/error` 的 `showSuccess` / `showError`。

**⑨ SPA 的 `Page.navigate` 同 URL 不重载**

只改 hash 时 CDP 的 `Page.navigate` 属于 same-document 导航，不会重新加载 JS，
容易误判"改动没生效"。自动化验证要么加查询串强制新文档，要么用 `location.hash` 走站内路由。

### 18.4 本地运行

```bash
# 后端（数据面 8089 / 控制面 8899）
go run ./cmd/gowaf -c configs/config.yaml

# 前端开发（3006，代理到 8899）
cd Frontend && pnpm install --ignore-workspace && pnpm dev

# 前端改完后重新内嵌
make build          # = 构建前端 + 同步 dist + 编译二进制
```

首次启动的 admin 密码写在 `data/initial-password.txt`（随机 16 位），登录后请立即修改。

### 18.5 验收基线

交付时使用的验证矩阵，后续改动应保持同等覆盖：

- 编译：`go build` / `go vet` / `gofmt -l` 全部干净；`CGO_ENABLED=0` 交叉编译通过
- 前端：`npx vue-tsc --noEmit` 0 error
- 接口：登录 → 8 个页面全部接口 200，含分页、导出 CSV、PATCH 启停、配置保存与回滚
- 数据面：`.env` / `.git/config` / SQLi / 扫描器 UA 全部 403，且事件 `score` 与 `categories` 正确落库
- 浏览器：8 个页面真实渲染 0 控制台错误、0 个 4xx；SSE 连接后能收到新事件；规则测试器返回命中明细

