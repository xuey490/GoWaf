# GoWAF 与雷池 SafeLine 开源版对比分析

> 对比基线：本文基于 **GoWAF v1.6.5（dev 分支源码）** 与 **雷池 SafeLine CE（个人版）9.4.4（2026-11-11 发布）** 的公开资料与官方文档。
> 调研日期：2026-11-19。
> 目的：客观呈现两个产品的优缺点，**学习别人的长处，补齐自己的短板**，形成可执行的改进路线。

---

## 1. 结论速览（TL;DR）

| 维度 | 一句话结论 |
|---|---|
| 总体 | 两者是**同一形态（反向代理 WAF）的不同实现路线**：SafeLine 靠「Rust 语义分析引擎 + 容器微服务」取胜于检测泛化与生态；GoWAF 靠「Go 单二进制 + 全环境接管」取胜于部署形态、开源完整度与运维自愈。 |
| GoWAF 最该学的 | **量化评测体系（BlazeHTTP）、检测规则的云端热更新、K8s/网关生态插件、产品化打磨**。 |
| GoWAF 已领先项 | 网页防篡改（SafeLine 收费）、地理位置访问控制（SafeLine 收费）、SIEM 日志外发、页面缓存加速、精细条件树访问控制、Windows/IIS/面板接管、观察模式灰度上线、核心代码全开源（SafeLine 检测引擎闭源）。 |
| SafeLine 最值得警惕的 | 检测率/误报率有公开量化背书（严格模式检出 76.17% / 误报 0.22%），且背后是商业公司的持续规则运营与 CVE 快速响应——这是纯社区项目的最大差距。 |

---

## 2. 产品定位对比

| 项目 | GoWAF | 雷池 SafeLine CE（个人版/开源版） |
|---|---|---|
| 定位 | 站在 Apache/Nginx/IIS 前面的**单二进制反向代理 WAF** | 站在业务前面的**容器化微服务 WAF**（Docker 一键部署） |
| 出品方 | 个人开源项目（xuey490） | 长亭科技（商业公司开源，社区版免费、个人版免费） |
| 当前版本 | v1.6.5 | 9.4.4（2026-11-11；2025-04 起 8.0.0 LTS） |
| 开源协议 | AGPL-3.0（代码 100% 开源） | GPL-3.0（**仅管理面开源**；核心检测引擎 detector 为闭源镜像分发） |
| GitHub 热度 | 小规模社区项目 | 23.3k stars / 1.6k forks，装机量宣称 70 万+ |
| 目标用户 | 中小站长、面板用户、Windows/IIS 环境 | 中小企业/开发者，Linux + Docker 环境 |
| 付费版本 | 无 | 专业版 ¥3600/年、商业版（等保合规/集群/防篡改等） |

---

## 3. 架构与技术栈对比

| 维度 | GoWAF | SafeLine CE |
|---|---|---|
| 进程形态 | **单体单进程**：数据面 + 管理面 + 存储同进程 | **微服务多容器**：tengine / detector / mgt / mgt-api / PostgreSQL / 前端 |
| 数据面 | Go `net/http` + `httputil.ReverseProxy`（自研反代） | **Tengine（Nginx 二次开发）** 作为网关，流量转发 |
| 检测引擎 | 三引擎并行：① 181 条自研 YAML 规则（AC 自动机预筛 + RE2 精匹配，评分制）② 内嵌 OWASP CRS v4（Coraza v3，PL1–4，协议类规则降权治理）③ libinjection 语义通道 | **自研语义分析引擎**（Rust 实现，闭源分发），语义分析 3.0，宣称无规则匹配思路、对 0day 有泛化能力；附带 Lua 辅助脚本、JA4 指纹 |
| 检测性能口径 | 未官方公布 TPS；引擎级 2s 超时熔断 + panic 隔离 | 宣称平均检测延迟 <1ms，单核 2000+ TPS |
| 存储 | SQLite（glebarez 纯 Go 驱动，WAL，顺序 SQL migration，当前 v6） | PostgreSQL（独立容器） |
| 管理面 | Gin v1.12 + GORM v1.31，JWT + CSRF + 权限位 + TOTP 两步验证 | mgt（Go）+ mgt-api（Go），前端 React；多管理员为专业版功能 |
| 前端 | Vue 3.5 + TypeScript + Vite + Naive UI + Pinia + vue-i18n（中英双语）+ ECharts + Tailwind 4，产物 `go:embed` 进二进制 | React 管理台，随镜像分发 |
| 热更新机制 | `atomic.Pointer` 不可变快照整体替换（规则/名单/CRS/策略读路径无锁） | mgt 通过消息推送下发配置，Tengine reload 热部署 |
| 交付物 | **1 个二进制文件**（前端 + IP 归属地库内嵌），Linux/Windows 双平台，CGO_ENABLED=0 交叉编译 | **1 套 Docker Compose**（约 4–5 个容器），仅 Linux（x86_64/arm64） |
| 资源占用 | 低（单进程 + SQLite） | 较高（PostgreSQL + 多容器，社区实测集群单节点 ~300MB 内存、CPU 25–32%） |

**小结**：SafeLine 的 Tengine 网关继承了 Nginx 十余年打磨的连接处理与上游生态（负载均衡、健康检查成熟）；GoWAF 用纯 Go 反代换取了单二进制交付和 Windows 能力，代价是反代成熟度需自证。检测层 SafeLine 是「重引擎 + 轻规则」，GoWAF 是「三引擎 + 规则运营」，路线不同。

---

## 4. 部署与运维对比

| 维度 | GoWAF | SafeLine CE |
|---|---|---|
| 安装方式 | `install.sh` / `install.ps1` 一键接管 80/443；Docker compose 亦可 | 仅 Docker Compose 一键安装（离线包支持） |
| 支持环境 | 宝塔（Apache/Nginx 双端）、1Panel（OpenResty 容器）、小皮、原生 systemd、IIS；Linux + Windows 全覆盖 | 独立 Docker 容器部署，**不接管、不适配任何面板**；官方不支持 Windows |
| 部署前体检 | `doctor` 只读体检（面板/引擎/站点/证书/端口） | 官方安装脚本自检环境（Docker 版本/端口） |
| 回滚/卸载 | `rollback` / `uninstall` 自动还原引擎配置并交还端口；端口漂移 60 秒自愈（Linux resync 定时器） | 卸载即删容器，源站配置由用户自行恢复 |
| 证书管理 | 控制台证书列表（到期预警）+ ACME HTTP-01/DNS-01 签发热加载 | ACME 免费证书申请 + **自动续期（复用私钥，9.4.3）**，SSL 合规默认 TLS1.2/1.3 |
| IPv6 | 手动开关 `listen_v6`（考虑发行版 v6only 语义不默认双栈） | 9.x 支持 IPv6 识别（监听原生由 Nginx 承担） |
| HTTP/3 (QUIC) | 不支持（不接 UDP 443） | 不支持（Tengine 无 QUIC） |
| 升级体验 | 替换二进制 + 重启；CRS 规则可在线更新（`internal/detect/crs_update.go`），自研规则随版本发布 | **云规则热更新**：漏洞规则、语义引擎随云端持续推送，CVE 响应快（如 9.4.3 修复捆绑 tengine 的 CVE-2026-1642） |
| 守护方式 | systemd / Windows SCM 服务（`gowaf svc`，无 NSSM 依赖） | Docker restart 策略 |

**小结**：部署形态是 GoWAF 的**主场优势**——尤其「宝塔/小皮/IIS 用户的现有机器不动架构即可接入」和「rollback 全身而退」，SafeLine 完全没有对应能力。反过来，SafeLine 的**规则云更新与 CVE 快速响应**是 GoWAF 目前靠版本发布做不到的。

---

## 5. 功能模块逐项对比

图例：✅ 具备　⚠️ 部分具备/有降级　❌ 不具备（✅* 表示该功能在对方为付费项）

| 功能模块 | GoWAF v1.6.5 | SafeLine CE 9.4.4 | 胜出方 |
|---|---|---|---|
| Web 攻击防护（SQLi/XSS/RCE/路径穿越/XXE/SSRF 等） | ✅ 三引擎评分合并判定 | ✅ 语义分析引擎（官方实测检出 76.17%/误报 0.22%） | SafeLine（泛化能力） |
| CC 防护 / 频率限制 | ✅ 全局 RPS/突发/每分钟 + **条件树自定义 CC**（按站点+IP 隔离计数，超限可 429 或转挑战） | ✅ 频率限制（IP/全局），HTTP Flood 规则（Pro 可自定义响应码） | GoWAF（策略表达力） |
| 人机验证 | ✅ PoW 工作量证明挑战（通过后 30 分钟免检），可与限流/策略联动 | ✅ 验证码人机验证（Anti-Bot 事件管理） | 各有侧重 |
| Bot 管理 | ✅ 蜜罐路径 + UA/Path/IP/rDNS 四类策略 + 搜索引擎正反向 DNS 双验证 + 行为评分 | ✅ 爬虫识别与拦截（语义 + 指纹，可运营粒度较粗） | GoWAF（可运营粒度） |
| 网站身份认证 | ✅ 页面密码（bcrypt + HMAC 通行证）+ HTTP Basic，门禁在缓存前执行 | ✅ 认证挑战（访客输密码访问） | 平手 |
| 动态防护（HTML/JS 加密） | ✅ 逐请求密钥加密文本节点 + 注入解密脚本（RC4-drop256） | ✅ 静态页面动态防护 | 平手（GoWAF 可与 PoW 联动是加分项） |
| **网页防篡改** | ✅ **开源**：SHA-256 基线 + 基线副本替换被篡改响应 | ❌ **商业版专属** | **GoWAF（显著）** |
| **地理位置访问控制** | ✅ **开源**：条件树 `country` 字段（内置 ip2region 离线库） | ❌ **专业版专属** | **GoWAF（显著）** |
| 精准访问控制 | ✅ 条件树（AND/OR 嵌套 4 层、64 条件）× 9 操作符 × 7 动作，Reload 编译为无锁快照 | ⚠️ IP/UA 黑白名单组 + Deny 规则（9.4.3 支持仅记录模式、9.4.4 支持 ASN 匹配）；无嵌套条件树 | GoWAF（表达力） |
| 页面缓存 / 加速 | ✅ LRU+TTL 六类规则、按客户端重协商压缩、JPEG/PNG→WebP | ❌ 无 | **GoWAF（独有）** |
| 上游负载均衡 | ✅ 多上游按健康度+优先级加权随机，fail-open；⚠️ 策略种类少（无 ip_hash/权重手动/主备显式区分） | ✅ 成熟：多上游 + 健康检查（9.4.1 并发执行、9.4.4 自定义探测路径、动态解析域名） | SafeLine（成熟度） |
| 响应敏感信息过滤 | ✅ 手机号/身份证/银行卡/云密钥/JWT 识别与打码（observe/mask） | ❌ 未见公开能力 | **GoWAF（独有）** |
| 上传文件 / 反序列化检测 | ✅ 危险扩展名/魔数不符/Java·.NET·pickle·PHP 序列化/原型链污染 | ✅ 漏洞规则持续更新（如 9.4.0 新增 .NET 反序列化、Kafka CVE、DedeCMS RCE） | 平手（SafeLine 漏洞规则运营更快） |
| 检测前多层解码 | ✅ base64（≤3 层）/压缩体/JSON-Unicode 转义，附加变体检测 | ⚠️ 语义引擎内建变形容忍（未公开细节） | 不好直接比较 |
| 威胁情报 | ✅ 订阅公共/自建 IP 清单（10 预设源 + AbuseIPDB/CrowdSec JSON），命中自动并入封禁 | ✅ **长亭社区恶意 IP 情报**（商业公司运营，质量与规模占优） | SafeLine（情报质量） |
| 限流与封禁管理 | ✅ 连击加倍→永久封禁、白名单、全局/站点作用域、统一封禁列表可释放 | ✅ 黑名单 + Deny 规则 + 监控模式 | GoWAF（封禁生命周期管理更细） |
| 告警通知 | ✅ 邮件时间窗聚合 + 飞书/钉钉/企微/通用 Webhook，逐渠道发送记录 | ✅ 通知管理（邮件/IM/Webhook） | 平手（GoWAF 聚合与去重设计更细） |
| SIEM / 日志外发 | ✅ **Syslog RFC5424 + Kafka**（logship，fail-open 队列） | ❌ 个人版无（商业版有系统日志审计） | **GoWAF（显著）** |
| 流量统计 / 可视化 | ✅ 仪表盘（趋势/类型分布/攻击源）+ SSE 实时事件 + Prometheus `/metrics` | ⚠️ 基础攻击事件；高级统计与实时大屏为专业版功能 | GoWAF（开源版内） |
| 审计日志 | ✅ 控制台操作审计 | ❌ 商业版专属 | **GoWAF** |
| 多用户 / RBAC | ✅ 用户管理 + 权限位（`rule:write`/`ip:ban`…）+ TOTP 两步验证 | ⚠️ 多管理员为专业版；未见开源 2FA | **GoWAF** |
| 多节点 / 集群 | ⚠️ 节点管理 + Agent 令牌心跳上报（无配置同步） | ⚠️ 个人版无原生集群（有主从同步教程）；商业版统一集群管理、9.4.0 主从规则同步 | 平手（各有半套） |
| OpenAPI / AI 集成 | ✅ API Token（SHA-256 摘要、读写作用域）+ **MCP 11 个工具**（ban_ip/rule_test…） | ✅ **MCP Server**（AI 辅助管理）+ 管理 API | 平手（方向一致） |
| 网关插件生态 | ✅ **detectapi**（`POST /detect` + `GET /check` auth_request/forwardAuth，对标 T1K）；⚠️ 尚无官方 lua/Kong/APISIX/Ingress 插件 | ✅ APISIX 插件、Kong 插件、Ingress-NGINX 插件（K8s） | SafeLine（生态落地） |
| K8s Ingress 形态 | ❌（README 明确「不做」，但 detectapi 已具备被 Ingress 调用的条件） | ✅ 官方插件 | SafeLine |
| 自定义拦截页面 | ⚠️ 内置默认拦截页（不可在线改模板） | ❌ 专业版专属 | 平手（开源版都没给全） |
| 观察模式灰度 | ✅ 默认 observe 3–7 天再切实时拦截；引擎失效 fail-open/closed 可选 | ⚠️ 9.4.3 Deny 规则支持仅记录模式（无整体默认观察态） | **GoWAF（独有理念）** |
| 误报治理 | ✅ 例外规则（站点/规则号/分类/路径/参数五维组合）+ 规则运营（分组统计/命中样例/批量启停/导入导出）+ 规则调试器 | ⚠️ 靠语义引擎低误报宣称 + 白名单组 | GoWAF（可运营工具链） |
| IP 归属地展示 | ✅ 内嵌离线库（21.6 万 IPv4 区间，零出网） | ⚠️ 有地理库（9.4.4 更新 IP 地理库），访问控制维度收费 | GoWAF（开源版内） |
| Cloudflare / CDN 适配 | ✅ 回源网段一键开启、真实 IP 还原、联网刷新官方网段 | ⚠️ 支持 CDN 场景配置（真实 IP） | 平手 |

---

## 6. 检测能力专项对比

| 维度 | GoWAF | SafeLine CE |
|---|---|---|
| 技术路线 | 规则为主 + 语义为辅（libinjection），三引擎分数合并 | 语义为主（号称不依赖规则），规则/指纹为辅 |
| 可解释性 | 强：每条规则有 ID/分类/分数，事件可归因到规则 | 中：语义引擎输出可归因，但内部不公开 |
| 对未知变形的泛化 | 依赖 libinjection + 多层解码变体，泛化弱于真正的语法级语义分析 | 强（语义分析算法是其核心卖点，宣称可防 0day） |
| 规则更新 | CRS 可在线更新；自研规则随版本发版 | **云端热更新**（漏洞规则、检测逻辑随版本高频推送，2026 年 8–11 月连发 4 个版本含多条新 CVE 规则） |
| 公开评测 | 无公开基准（官方 README 未提供数据） | 有：BlazeHTTP 33669 样本实测（检出 76.17% / 误报 0.22% / 准确率 99.38%），对比 ModSecurity L1（69.74%/17.58%） |
| 误报治理 | 系统化（例外规则五维组合 + 协议类规则降权 + 观察模式） | 宣称低误报，治理手段未公开 |
| 引擎可靠性 | 引擎超时熔断 + panic 隔离 + fail-open/closed | 未公开（9.4.4 曾修复语义引擎崩溃） |

**判断**：检测力差距的本质不是「规则 vs 语义」的路线之争，而是**检测质量有没有被量化度量、有没有持续运营**。SafeLine 每月拿真实 CVE 样本回归、拿 BlazeHTTP 出报告；GoWAF 的 181 条规则目前缺一个同等量级的评测闭环（仓库已有 `docs/P1-样本库建设方案.md` 的雏形，应尽快落地）。

---

## 7. 双方优缺点总结

### GoWAF 的优点（应保持并放大）

| # | 优点 | 说明 |
|---|---|---|
| 1 | 单二进制交付 | 前端、归属地库全内嵌，无运行时依赖，交叉编译三平台 |
| 2 | 全环境接管 | 宝塔/1Panel/小皮/IIS/原生，Linux+Windows 全覆盖，doctor 体检 + rollback 全身而退——SafeLine 无对应能力 |
| 3 | 开源功能完整度 | 防篡改、地理封锁、SIEM 外发、审计日志、RBAC、TOTP 在 SafeLine 开源版全部缺失或是付费项 |
| 4 | 策略表达力 | 条件树嵌套访问控制 + 自定义 CC + 页面密码 + 限流联动，可组合出 SafeLine 开源版做不了的策略 |
| 5 | 运维自愈 | 端口漂移 60 秒纠正、观察模式默认灰度、引擎熔断 fail-open、配置校验拒绝启动、防自锁 |
| 6 | 页面缓存加速 | 六类规则 + 重压缩 + WebP，WAF 兼 CDN 一体 |
| 7 | 双引擎可解释检测 | 自研规则可在线运营（调试器/命中样例/批量启停），CRS 随上游更新 |
| 8 | 中英双语控制台 + 中文文档 | 文档质量（原理/排障/适配细节）是明显强项 |

### GoWAF 的短板（需要补齐）

| # | 短板 | 后果 |
|---|---|---|
| 1 | 缺量化评测基准 | 检出率/误报率无公开数字，用户无法对比选择，自身改进也没有标尺 |
| 2 | 规则更新依赖发版 | 自研规则与版本绑定，新 CVE 出现到规则生效周期长；无云端规则订阅通道 |
| 3 | 语义检测深度有限 | libinjection 是词法级指纹，对强变形/编码嵌套的泛化不如语法级语义引擎 |
| 4 | 网关/K8s 生态未落地 | detectapi 已具备 T1K 形态但没有官方 lua-resty/Kong/APISIX/Ingress 插件与 Helm Chart |
| 5 | 反代成熟度需自证 | 纯 Go 反代在极限并发、上游协议广度（grpc_pass、缓冲策略、sendfile 等生态）不如 Tengine 十年打磨 |
| 6 | 多节点只有半套 | 节点心跳有了，配置同步、规则分发、集中日志没有 |
| 7 | 负载均衡策略简单 | 无 ip_hash/手动权重/主备切换/慢启动，健康探测路径不可自定义 |
| 8 | 证书自动续期未闭环 | ACME 签发已有，自动续期任务需补 |
| 9 | 无商业公司背书 | 规则运营、CVE 响应速度、社区规模与 SafeLine 有量级差距 |

### SafeLine 的优点（学什么）

| # | 优点 | 值得学的点 |
|---|---|---|
| 1 | 语义分析引擎 | 对未知攻击/变形 payload 的泛化是硬实力；提示我们应在 libinjection 之上引入更深的语法级解析 |
| 2 | 量化评测体系 | BlazeHTTP 数据集 + 公开检出率/误报率报告，把检测力变成可验证的数字 |
| 3 | 云规则更新 | 漏洞规则热更新通道 + 高频发版节奏（2026 年 8–11 月 4 版，含多条 CVE 检测） |
| 4 | 网关生态插件 | APISIX/Kong/Ingress-NGINX 官方插件，把「检测能力」独立成可被任何网关调用的服务（GoWAF 的 detectapi 方向正确，缺落地） |
| 5 | 产品化打磨 | 安装体验、控制台交互、文档站、Discord 社区、7 天试用、版本对比页——商业化运营链路完整 |
| 6 | 证书自动续期成熟 | ACME 复用私钥自动续期、SSL 合规默认 TLS1.2/1.3 |
| 7 | 负载均衡成熟 | 健康检查自定义路径/间隔、域名动态解析、应用级并发探测 |
| 8 | CVE 快速响应 | 捆绑组件漏洞（如 CVE-2026-1642）主动修复——单二进制同样要管好供应链 |

### SafeLine 的缺点（GoWAF 的差异化机会）

| # | 缺点 | 说明 |
|---|---|---|
| 1 | 核心引擎闭源 | GPL-3.0 只覆盖管理面，detector 是闭源镜像——「开源版」其实半开源；GoWAF 代码 100% 开源且可审计 |
| 2 | 仅 Docker + Linux | 不支持 Windows、不适配面板；宝塔/IIS/小皮用户群完全覆盖不到 |
| 3 | 开源版功能阉割明显 | 防篡改（商业版）、地理访问控制（专业版）、高级统计（专业版）、审计（商业版）、多管理员（专业版）全收费 |
| 4 | 多容器资源与运维成本 | PostgreSQL + 4~5 容器，升级/备份/迁移复杂度高于单二进制 |
| 5 | 与现有引擎不共生 | 不接管、不改造现有 Nginx/Apache 配置体系；面板用户的站点管理仍割裂 |
| 6 | 数据面不可编程扩展 | 自研 Tengine 内核闭源，第三方难以参与数据面开发；GoWAF 全栈可改 |

---

## 8. 借鉴路线图：学长处、补短板

优先级依据：**对检测力和用户信任的影响** > **生态覆盖** > **产品体验**。

### P0 —— 检测力（决定产品生死）

| 优先级 | 行动 | 对标 SafeLine 的什么 | 现状与落点 |
|---|---|---|---|
| P0-1 | **建立量化评测闭环**：落地 `docs/P1-样本库建设方案.md`，引入 BlazeHTTP 样本集（33k+）跑三引擎基线，把检出率/误报率写进 README 与 CHANGELOG，纳入 CI 回归 | BlazeHTTP 公开报告 | 仓库已有样本库方案与 `bench.exe`，缺数据集与自动化 |
| P0-2 | **规则热更新通道**：自研规则库独立版本化（Git tag + 校验签名），WAF 定时拉取可选私有源；新 CVE 规则不发版即可推送 | 云规则热更新 | `crs_update.go` 已有在线更新框架可复用 |
| P0-3 | **语义检测增强**：在 libinjection 之上加一层轻量语法归一化（SQL/JS/HTML token 级，Go 生态可选 gsv/AST 类库），扩展 `internal/detect/semantic.go`；先把多层解码的变体空间做厚 | Rust 语义分析引擎 | 已有 `semantic.go`/`libi.go` 骨架 |
| P0-4 | **JA4 指纹运营化**：`internal/ja4` 已有采集 + `malicious_ja4.json`，补策略页（按指纹封禁/放行/观察）与情报订阅接口 | JA4 指纹（9.x 引入） | 采集与识别已实现，缺运营面 |

### P1 —— 生态（决定覆盖面）

| 优先级 | 行动 | 对标 SafeLine 的什么 | 现状与落点 |
|---|---|---|---|
| P1-1 | **官方网关插件**：detectapi（`/detect` + `/check`）已对标 T1K，补 lua-resty SDK、Kong/APISIX 插件仓库与一键脚本，写接入文档 | APISIX/Kong 插件 | `internal/detectapi` 已实现，`docs/integrations-detect-api.md` 已有 |
| P1-2 | **Ingress-NGINX 接入形态**：detectapi 模式 B + auth_request 即可组成 Helm Chart（`modes B` 文档已有），低成本拿到 K8s 入口 | Ingress-NGINX 插件 | README 明确「不做 K8s」，但 detectapi 已让这件事只需打包 |
| P1-3 | **MCP 对齐**：把 11 个 MCP 工具对齐雷池 MCP 工具集语义，补 `mcp.json` 一键配置示例与 AI 运维手册 | MCP Server | `internal/mcp` 已有，补文档即可 |

### P2 —— 产品化（决定体验）

| 优先级 | 行动 | 对标 SafeLine 的什么 | 现状与落点 |
|---|---|---|---|
| P2-1 | 负载均衡补全：手动权重/ip_hash/主备 + 健康探测路径与间隔自定义 | 上游健康检查自定义路径（9.4.4） | `internal/proxy` 多上游已有加权随机 |
| P2-2 | 证书自动续期：ACME 续期计划任务 + 到期自动重签 | ACME 自动续期复用私钥（9.4.3） | ACME 签发已有，`internal/tasks` 可挂 |
| P2-3 | 自定义拦截/挑战页面模板：控制台可编辑 HTML（SafeLine 收费，GoWAF 可做成开源亮点） | 自定义拦截页（专业版） | 内置默认页，可加模板配置 |
| P2-4 | 多节点配置同步：节点心跳之上加配置快照推送/拉取与规则分发 | 主从规则同步（9.4.0）/商业集群 | `internal/app` 节点管理已有半套 |
| P2-5 | 供应链自检：单二进制内嵌 CRS/Tengine 同类风险自查（Go 模块 CVE 扫描进 CI） | 主动修复捆绑组件 CVE（9.4.3） | 无 |
| P2-6 | IPv6 默认安全双栈：探测内核 `v6only` 语义后自动决定是否绑 `::` | Nginx 原生 IPv6 | 手动开关已有 |

### 反向清单 —— GoWAF 已领先、不要丢的

| 能力 | 保持理由 |
|---|---|
| 网页防篡改（开源） | SafeLine 收费项，中小企业刚需 |
| 地理位置访问控制（开源） | SafeLine 收费项，已可与条件树组合 |
| SIEM 外发（Syslog+Kafka） | 商业版才有的能力，对接已有安全体系的门票 |
| 页面缓存加速、响应脱敏 | SafeLine 完全没有，差异化卖点 |
| 观察模式默认灰度 + 引擎熔断 fail-open | 「不上错规则、不当机」的运维安全感，SafeLine 无整体默认观察态 |
| Windows/IIS/面板接管 + rollback | 覆盖 SafeLine 触达不到的全部存量用户 |
| 代码 100% 可审计 | 对比 detector 闭源，是面向安全敏感用户的根本差异 |

---

## 9. 信息来源

- GoWAF：仓库源码（`README.zh-CN.md`、`go.mod`、`internal/*` 模块注释），v1.6.5
- SafeLine 官网与版本对比：[waf-ce.chaitin.cn](https://waf-ce.chaitin.cn) / [版本对比页](https://waf-ce.chaitin.cn/version)
- SafeLine 帮助文档：[help.waf-ce.chaitin.cn](https://help.waf-ce.chaitin.cn)（产品介绍/进阶教程/付费选型）
- SafeLine GitHub：[chaitin/SafeLine](https://github.com/chaitin/SafeLine)（README、Releases 9.4.0–9.4.4）
- SafeLine 技术架构：[官方架构文档](https://help.waf-ce.chaitin.cn/node/01973fc6-dfeb-7821-9076-8b3696b00db4)、[社区架构分析（CSDN）](https://blog.csdn.net/2301_79518550/article/details/160683499)、[知乎：雷池社区版结构](https://zhuanlan.zhihu.com/p/645083487)、[GitHub Discussions #176 各服务职责](https://github.com/chaitin/SafeLine/issues/176)

> 注：SafeLine 侧数据来自官方口径与公开资料（宣称值），检测率/误报率为官方在 BlazeHTTP 样本集上的自测结果；GoWAF 侧结论来自源码审读。两边的「性能宣称」均未做独立复测，仅供决策参考。
