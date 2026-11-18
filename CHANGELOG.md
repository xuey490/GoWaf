# 更新日志

本项目的所有重要变更都会记录在这里，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

---

## [v1.6.2] - 2026-11-18 · 补丁：站点重复修复

修复「部署重装后站点列表出现重复记录」的问题。无数据库迁移、无设置键变更、
无配置格式变更，停服替换二进制即可升级；行为变化仅限启动时的站点种子写入。

### 修复

- **站点种子按域名交集去重（`internal/store/seed.go`）**：此前 `SeedSites` 只按 `name`
  判断站点是否已存在。宝塔环境的 vhost 会把 hash 前缀临时域名放在 `ServerName`、
  真实域名退为 `ServerAlias`，`deploy/install.sh` 生成的 `config.yaml` 站点名随之从
  `fast.example.com` 变成 `1ddf79d3.fast.example.com`；名字一变，种子就把已有站点
  当成新站点插入，旧记录留在库里，于是每重装一次多出一份。控制台按 `hosts` 展开标签，
  同一域名看起来出现 3 处，生产环境曾因此从 15 个站点膨胀到 28 条。
  修复后种子匹配分两层：先按 `name`（忽略大小写）匹配；未命中再按 `hosts` 交集匹配，
  命中则把新增域名并入既有站点，**不改名、不覆盖上游与防护配置**；
  通配符 `*` 与空域名不参与交集判定，兜底站点保持独立。

### 变更

- 升级后首次启动不再产生重复站点。已存在的历史重复记录不做自动删除
  （自动删数据风险过高），按下方「升级说明」人工核对后清理。
- 新增 3 条回归测试：同名幂等、主域名变更后合并别名、通配符不吞并。

### 升级说明

- 无迁移、无设置键变更，停服替换二进制即可回滚（还原上一版二进制）。
- 清理历史重复站点：先用 `sqlite3 data/waf.db ".backup backup/waf-before-dedupe-$(date +%Y%m%d-%H%M%S).db"`
  备份，再核对 `rule_exceptions` / `access_policies` / `cache_rules` 等表里 `site`
  指向的哪一个名字，把引用迁移到保留的那组，最后通过 `DELETE /api/sites/:id`
  删除多余站点（走管理接口可留审计并自动热重载）。

### 已知局限

- 去重只发生在启动播种阶段，不会整理运行期通过 API 创建的站点。
- `install.sh` 仍取 vhost 主域名作为站点名，宝塔环境下显示为 hash 前缀形式；
  这只影响可读性，不再造成重复。

---

## [v1.6.1] - 2026-11-18 · P1 追赶（对标雷池）第二批

本批完成「v1.6.0 与雷池社区版对比」新 P1 清单中的第 2~8 项共七项能力（第 1 项
「语义广度 20+ 类 + 万级样本库跑分」另行规划，建设路径见 `docs/P1-样本库建设方案.md`）。
含一次数据库迁移（`011_p1_features.sql`），升级方式为**停服替换二进制**，迁移在启动时自动执行。
所有新开关默认保守：防重放、等候室、日志外发默认关闭，JA4 采集默认开启但拦截默认关闭，
未显式配置时线上拦截行为与 v1.6.0 保持一致。

### 新增

- **JA4/TLS 客户端指纹（`guard.ja4.enabled` / `blocklist` / `block_builtin`）**：包装 TLS
  Listener 在握手阶段偷看 ClientHello，按 FoxIO 规范计算三段式指纹（`t13d1516h2_xxx_xxx`，
  GREASE 忽略、扩展按十六进制排序、仅剔除 SNI/ALPN 参与哈希），按连接四元组登记，
  HTTP/1.1 与 HTTP/2 均适用，连接关闭即清理。指纹写入事件库新增的 `ja4` 列并在事件 API
  返回；`guard.ja4.blocklist` 精确匹配即 403 拦截。内置清单（`internal/ja4/malicious_ja4.json`）
  仅收录两条本机真实采集种子，脚本栈指纹亦被正常工具使用，故 `block_builtin` 默认关闭。
- **日志外发（`logship.*`）**：拦截事件异步推送 SIEM，支持 Syslog RFC5424
  （STRUCTURED-DATA `SD-ID=gowaf@48577`，udp/tcp/tls，tcp/tls 为长连接失败自动重建）
  与 Kafka（kafka-go，key 为客户端 IP 保证同 IP 分区有序）双渠道，可同时开启。
  有界队列（默认 4096）+ 单 worker，满队列 fail-open 丢弃并计数；
  `logship.min_action` 支持 `all`/`log`/`block` 三档外发门槛。
- **请求防重放（`guard.replay.enabled` / `window` / `header`）**：携带 nonce 头
  （默认 `X-WAF-Nonce`）的请求首次放行并登记，窗口内同 nonce 重放直接 403（reason=`replay`）；
  不带该头的请求不受影响，内存表按窗口自动清理。
- **等候室（`waitroom.*`）**：上游并发阈值 FIFO 排队，`max_active` 限制同时打到上游的并发数，
  `timeout_ms` 内拿不到名额返回 429 + `Retry-After` + 自动刷新等候页。只约束真正转发的请求，
  白名单、缓存命中、Bot 放行与拦截早退不占用名额。
- **站点级自定义拦截页（`sites.block_page`）**：站点可配置 HTML 模板，支持
  `{{status}}`/`{{reason}}`/`{{reqid}}`/`{{site}}` 四个占位符（HTML 转义），上限 4096 字节；
  传入 `null` 回到内置模板，校验失败按 fail-open 退回内置页。
- **控制台 TOTP 两步验证（`/api/user/totp` 及 `setup`/`enable`/`disable`）**：RFC 6238
  实现（HMAC-SHA1、30 秒步长、±1 窗口容错），启用后登录需携带 `totpCode`，缺失或错误返回
  业务码 `40102` 并计入登录限流。API-first，控制台界面尚未提供入口。
- **人机校验降级链（`guard.challenge.chain` / `guard.captcha.*`）**：环境信号校验失败后的
  处置序列可配置为 `pow,captcha,ban` 的组合——纯拒绝（原行为）、降级到图形验证码、
  答错达阈值后封禁。验证码为确定性 SVG 算式，令牌绑定 IP 与过期时间，换 IP/篡改/过期一律拒绝。

### 变更

- 边缘管线插入 JA4 拦截、防重放、等候室与验证码端点；等候室放行期间计入 `waf_waitroom_active`。
- 新增指标：`waf_ja4_blocked_total`、`waf_replay_blocked_total`、`waf_captcha_fails_total`、
  `waf_captcha_passed_total`、`waf_waitroom_rejected_total`、`waf_waitroom_active`、
  `waf_logship_sent_total`、`waf_logship_dropped_total`。
- 优雅关闭时停止日志外发 worker 并关闭渠道连接。
- 维护协程按周期清理重放表与验证码失败计数。
- 文档：新增 `docs/P1-新特性-使用指南.md`（设置键、端点、验收口径对照与已知局限）、
  `docs/P1-样本库建设方案.md`。

### 升级说明

- 迁移 `011_p1_features.sql` 新增三处列：`events.ja4`、`sys_users.totp_secret`/`totp_enabled`、
  `sites.block_page`，启动时自动执行，无需手工干预；老数据 `ja4` 为空属正常。
- 建议升级前备份 `data/waf.db*`（WAL 模式下需一并备份 `-wal`/`-shm`）与当前二进制。
- 回滚：停止服务 → 还原二进制 → 启动；已执行的迁移对 v1.6.0 为新增列，不影响旧版读取。

### 已知局限

- JA4 内置恶意指纹清单仅 2 条真实采集种子，非完整指纹库，拦截覆盖依赖管理员积累 blocklist。
- 防重放与等候室为单机内存态，多节点部署不共享状态；等候室无跨节点全局配额。
- 日志外发不做磁盘缓冲持久化，进程退出或队列满时可能丢事件（已计数）。
- TOTP 为 API-first，前端登录页尚无动态码输入框。

---

## [v1.6.0] - 2026-11-18 · P0 追赶（对标雷池）

本批改动完成「v1.5.0 与雷池社区版对比」清单中的全部四项 P0，无数据库迁移，替换二进制升级。
新能力除静态旁路外均默认保守：预筛加速默认关、语义通道分数压在拦截阈值之下，
升级不改变现有拦截行为。

### 新增

- **语义检测通道（第四引擎，`engine.sem.*`）**：在 libinjection（SQLi/XSS）之外补齐
  命令注入、远程代码执行、XXE、SSRF、SSTI 模板注入、表达式注入（含 Log4Shell JNDI）、
  反序列化七大类的结构级判定（`internal/detect/semantic.go`）。判定单元是「结构」而非
  关键词——元字符紧邻系统命令、DOCTYPE+ENTITY 成对、危险 scheme、模板运算/属性链、
  序列化魔数，配合 Normalize 的多层解码天然抗变形。单类默认 35~50 分（压在 block 60
  之下、challenge 30 之上），误报治理走例外规则（`sem-*` 规则 ID）。Header 额外扫
  JNDI/SpEL/OGNL（Log4Shell 经 UA/Referer 注入是经典路径）。
- **CRS 调度闸门（`engine.crs.prescreen/skip_static/body_text_only/exclude_paths/remove_ranges`）**：
  实测 CRS 是吞吐天花板（全引擎 4.3k → 关闭 17.1k RPS），闸门提供四个有界旁路：
  可疑度预筛（AC 自动机关键词取自 CRS 规则文件字面量 + 扫描器 UA + 注入结构词表，
  零命中跳过 Coraza 事务；默认关，开启后实测 3,998 → **12,836 RPS**，3.2 倍）、
  无参数静态资源旁路（默认开）、二进制 body 跳过 phase-2（默认开）、
  路径前缀豁免与编译期号段移除（如 `remove_ranges: "920"`）。旁路按原因打点
  `waf_crs_skipped_total{reason}`，可监控预筛命中率。
- **人机验证双层升级（`guard.challenge.env_check` / `min_elapsed_ms`）**：
  在 PoW 之上增加浏览器环境信号校验（navigator.webdriver、CPU 核数、屏幕、时区、
  WebGL 厂商/渲染器、页面耗时的结构合理性）与最小耗时校验（真实浏览器算 PoW 约
  0.3 秒 + 页面加载，脚本微秒级解 PoW 在此暴露）。真实浏览器完全无感；
  headless 自动化要么 webdriver=true 要么缺字段。
- **检测引擎独立调用（`server.detect_listen` / `detect_token`）**：WAF 以「检测引擎」
  形态被外部网关调用的接入面（对标雷池 T1K）：`POST /detect`（完整 JSON 协议）与
  `GET /check`（nginx auth_request / Traefik forwardAuth 鉴权式，2xx 放行 / 403 拦截 /
  401 需人机校验）。集成示例与协议说明见 `docs/integrations-detect-api.md`，
  示例配置在 `deploy/integrations/`（nginx lua-resty、Traefik forwardAuth）。

### 说明

- 预筛加速为「加速模式」：CRS 中少量无字面量的算子（`@detectSQLi` 等）会被旁路，
  已用高频注入结构词表（`1'='1`、`sleep(`、`<script` 等）覆盖主要盲区；默认关闭，
  由管理员权衡开启。回归样本：SQLi/命令注入/JNDI/SSTI/SSRF/XSS 在预筛开启时全部 403。
- 全量测试通过（`go vet` + `go test ./...`），新增语义引擎/闸门/环境信号/检测 API
  四组测试共 40+ 断言。

---

## [v1.5.1] - 2026-11-18

本版为控制台体验改进：**防护配置页按参数类别拆分为五个可切换 Tab**，各组独立保存、
热生效。纯前端改造（内嵌产物同步），无后端接口变更与数据库迁移，替换二进制即可升级。

### 变更

- **防护配置页（/waf/settings）Tab 化**
  原单页参数过多，按类别重构为 NTabs：
  **检测引擎**（引擎模式/阈值/超时熔断/失效策略、libinjection 语义通道、多层解码、
  CRS、事件采样率）｜**限流与封禁**（单 IP 频率、人机校验软阈值、自动封禁连击）｜
  **响应防护**（敏感信息过滤、动态防护）｜**网页防篡改**（开关/模式/路径 + 基线管理）｜
  **版本与回滚**（配置历史与一键回滚，只读操作）。
  - 每个 Tab 底部各自的「保存并热加载」：只提交**本组实际改动的键**，PUT 后热生效并
    联动刷新服务端快照/版本历史/防篡改运行态；空改动同样提示已保存；
  - 「重置本组」按组恢复服务器值，不影响其他 Tab 的未保存修改；
  - NTabs `display-directive=show`：切换 Tab 不丢未保存修改；顶部保留全局「全量重载」
    与演示模式提示；
  - 键覆盖完整性：5 组合计 20 字符串键 + 21 数值键，与原页面键集合双向差集为空，
    无参数丢失。
- i18n 中英同步（6 个新 key）。

### 说明

- v1.4.0 / v1.5.0 两个版本此前未随 tag 单独打包，本版为三者首次共同打包发布：
  v1.4.0（语义检测通道、引擎加固、响应侧能力、ACME、webhook 告警、OpenAPI+MCP、
  容器化）与 v1.5.0（P0 四件套：页面密码/自定义 CC/防篡改/多层解码）自本 tag 起一并
  进入发布线，升级路径 v1.3.2 → v1.5.1 覆盖迁移 009/010（启动时自动执行）。

---

## [v1.5.0] - 2026-11-18

本版完成对标雷池社区版的 P0 四件套：**页面密码/访问认证、自定义 CC 规则、网页防篡改、检测前多层解码**。
含一次数据库迁移（010_policy_config，启动时自动执行）。全部新能力默认关闭或保持现状，
升级不改变现有拦截行为。

### 新增

- **页面密码 / 访问认证（访问策略 `auth` 动作）**
  给指定路径加进门密码：`page` 模式（密码页 + HMAC 通行证 Cookie，默认 8 小时、绑定 IP，
  Cookie 按策略独立 `waf_auth_<id>`）与 `basic` 模式（HTTP Basic，API 客户端天然可用）。
  密码只存 bcrypt 哈希（明文仅提交时经过内存，列表回显掩码）；门禁在缓存之前执行，
  已缓存资源同样要过认证；解锁端点 `/.waf-auth/<策略ID>` 先于所有闸门处理。
  详见 docs/页面密码-访问认证-设置.md。

- **自定义 CC 规则（访问策略 `rate_limit` 动作）**
  按条件树圈定范围（路径/UA/Referer/地域…）做频率限制：按「策略+IP」固定窗口计数
  （10 秒~1 天窗口，阈值可配），超限处置可选 429 拦截或转 PoW 人机校验（通过后
  30 分钟免检）。计数桶与自动封禁隔离（独立 HitTracker，48 小时无命中逐出）。
  详见 docs/自定义CC规则-设置.md。

- **网页防篡改（tamper）**
  响应链路完整性防护：管理员手动「生成基线」（对每个启用站点 × 受保护路径回源抓取，
  记录 SHA-256 与有界内容副本），之后每个响应与基线比对——`observe` 只记录事件
  （规则 ID `tamper`）、`block` 用基线副本替换被篡改的响应。基线**只支持手动刷新**
  （定时重抓会把篡改版固化为新基线，是刻意设计）；基线存内存，重启失效需重新生成；
  受保护路径自动跳过页面缓存。新增指标 `waf_tamper_hits_total` 与
  GET /api/tamper/status、POST /api/tamper/refresh 接口。
  详见 docs/防篡改-设置.md。

- **检测前多层解码（engine.decode.*）**
  补归一化层覆盖不到的三类绕过面：base64（按 query 参数值解码，最多 3 层）、
  压缩请求体（gzip/deflate/zlib 魔数识别解压）、JSON/JS unicode 转义（\u0027）。
  解码产物作为**附加检测文本**并入自研规则与 libinjection 语义通道（CRS 自带
  变换链不重复喂）；同一规则跨原文/变体只记一次命中。plausibility 检查 +
  单变体 64KB 上限防 DoS，默认开启。详见 docs/多层解码检测-设置.md。

### 变更

- 访问策略新增 `config` 列（迁移 010）：auth/rate_limit 两个动作的参数 JSON；
  编译期强校验（密码哈希必须是 bcrypt、窗口/阈值在合法区间），坏配置整体拒绝
  Reload 保留旧快照；门禁动作的条件树不允许依赖 body（编译期拒绝）。
- `policy.Evaluate` 跳过门禁动作（auth/rate_limit 已在缓存前的早期阶段执行，
  完整求值再触发会重复计数/重复弹窗）；`EvaluateDenyEarly` 的拒绝类集合扩为
  拒绝类 + 门禁类。
- Router 新增 `Sites()` 遍历方法（防篡改基线刷新使用）；maintenance 循环新增
  PolicyHits 清理（48 小时空闲逐出）。

### 说明

- 全部新能力默认关闭（auth/rate_limit 策略由管理员显式创建，tamper 默认关闭），
  多层解码默认开启（URL 多层解码本就由归一化覆盖，此处补的三类绕过面成本有界）。
- auth 事件按 `challenge` 动作落库（看板"人机校验"口径涵盖所有"先自证再放行"交互）；
  CC 超限的 429 计入 `waf_blocked_total{reason="cc"}`。

---

## [v1.4.0] - 2026-11-18

本版是对标雷池 SafeLine 社区版做的能力补齐：**语义检测通道、引擎加固、响应侧能力
（敏感信息过滤 + 动态防护）、ACME 自动签发、webhook 告警、OpenAPI + API Token +
MCP、容器化发布**一次性落地。含一次数据库迁移（009_api_tokens，启动时自动执行），
替换二进制升级即可；全部新引擎默认关闭或处于低敏配置，升级不改变现有拦截行为。

### 新增

- **libinjection 语义检测通道（第三引擎）**
  自研规则与 CRS 都是特征匹配，变形 payload 靠写规则追不完。新增基于
  libinjection 的词法/指纹通道：对 query 与 body 做语义级 SQLi/XSS 判定，
  分数并入统一评分体系（默认 sqli 45 / xss 35，压在默认拦截阈值 60 之下、
  挑战阈值 30 之上——误报代价是一次无感人机校验而非直接拦截）。
  误报治理走既有例外规则（规则 ID `libi-sqli` / `libi-xss`），
  开关与分值在「防护配置」页热改（`engine.libi.*`）。默认启用。

- **检测引擎加固：超时熔断 + panic 隔离 + 失效策略**
  检测与控制面同进程，此前任何一个引擎 panic 都会带倒整个数据面，病态正则
  或规则包能把单请求检测拖到几十秒。现在三个引擎（自研 / CRS / libi）与
  载荷解析统一套上保护：单引擎超过 `engine.timeout_ms`（默认 2000ms）熔断、
  panic 一律 recover 隔离，失效时按 `engine.fail_mode` 处理——`open`（默认）
  放行并记录事件（可搜 `engine-degraded` 归因），`closed` 拦截；观察模式下
  closed 也不真拦。新增指标 `waf_detect_errors_total`。

- **响应体敏感信息过滤（respfilter）**
  请求侧再严，也拦不住上游应用自己把敏感数据吐出去。新增响应链路扫描：
  中国手机号 / 身份证 / 银联卡 / 云密钥（AWS、OpenAI、GitHub、阿里云、Google、
  PEM 私钥块）/ JWT，`observe`（默认）只记录、`mask` 就地打码后下发。
  只处理 ≤1MB 的 2xx 文本类响应，压缩/流式/二进制一律透传；
  命中带归因（`resp-*` 规则 ID）进事件页与采样例外（必记录）。
  开关、模式、类别、排除路径均可热改（`respfilter.*`）。默认关闭。

- **动态防护：HTML/JS 每次访问动态加密（dprotect）**
  雷池同思路的反爬能力：服务端按请求派生一次性密钥，把 HTML 文本节点加密成
  乱码，页面尾部注入解密脚本，浏览器执行后还原；curl / requests 这类不跑 JS
  的客户端拿到的永远是密文。只处理 ≤2MB 的 200 text/html 响应，GET/HEAD 生效；
  script/style/textarea/noscript 与属性值不动，解析失败按原文透传（fail-open）。
  `dprotect.challenge` 开启后与**人机校验联动**：未通过 PoW 的访客先完成校验
  再拿正文，两层叠加才是完整的「动态加密 + 人机校验」语义。
  注意：动态防护适用的请求会整段跳过页面缓存（每请求密钥唯一，缓存密文页
  会全员解密失败）。默认关闭。

- **ACME 证书自动签发 + 控制台证书管理页**
  新增 Let's Encrypt（或任意 ACME v2）签发：HTTP-01 由数据面 80 端口自动应答
  （边缘在站点路由之前拦截 `/.well-known/acme-challenge/*`），DNS-01 返回
  TXT 记录值由管理员添加后触发验证（泛域名必须走 DNS-01）；签出的证书写入
  `tls.cert_dir` 并立即热加载。控制台新增「证书管理」页：证书列表（到期时间
  <30 天标红）、上传/删除/手动重载、ACME 订单与挑战详情、staging 环境开关。
  ACME 账户密钥持久化在 `data/acme/`；订单状态仅存内存（重启后重新下单即可）。

- **告警 Webhook 渠道：飞书 / 钉钉 / 企业微信 / 通用**
  此前告警只有邮件。新增 webhook 渠道：三平台群机器人（飞书/钉钉支持可选
  签名）+ 通用 JSON POST（`{title, text, count, events}`，方便接自建告警网关），
  与邮件共用聚合窗口 / 冷却 / 最低分过滤；每目标独立落 notify_logs
  （channel 形如 `webhook:feishu`），「告警通知」页可逐渠道配置并测试发送。

- **OpenAPI（/api/v1）+ API Token + MCP Server**
  新增 API Token（SHA-256 摘要落库，明文仅创建时返回一次，`read` / `read,write`
  两种作用域）：Bearer 方式对既有 `/api/*` 与新增 `/api/v1` 别名组生效，
  read-only 令牌访问写接口一律 403（`RequireAPIScope`），令牌管理在
  `GET/POST /api/tokens`。`/api/v1` 提供外部集成用的稳定契约面
  （overview / timeseries / events / sites / bans / rules / settings / rules.test /
  bans 写接口等）。新增 `gowaf mcp` 子命令：stdio 上的 MCP（JSON-RPC 2.0）server，
  11 个工具（waf_overview / events / ban_ip / rule_test 等），凭
  `GOWAF_URL` + `GOWAF_TOKEN` 环境变量接入，AI 客户端（Claude 等）可直接管理面板。

- **容器化发布：Dockerfile + compose.yaml**
  单二进制 + 纯静态编译天然适合容器：多阶段构建（无需 node，前端产物已提交），
  运行镜像 alpine + CA 证书 + tzdata + 非 root 用户，健康检查 `/healthz`，
  全部持久化状态收敛到 `/data` 卷；`deploy/config.docker.yaml` 容器专用配置，
  `docker compose up -d --build` 一键起。

### 变更

- 数据面响应链路：有响应侧能力（respfilter / dprotect）适用时，会在上游与
  加速 writer 之间插入改写层，并把发往上游的 Accept-Encoding 置为 identity
  （客户端侧压缩不受影响，仍由 accelWriter 按客户端能力现算）。
- 写接口组新增 API Token 作用域中间件（JWT 会话不受任何影响）。
- 「防护配置」「告警通知」「设置」白名单与校验扩容：
  `engine.libi.*` / `engine.timeout_ms` / `engine.fail_mode` / `respfilter.*` /
  `dprotect.*` / `notify.webhook.*` / `acme.*`；飞书/钉钉签名密钥与 SMTP 密码
  同语义掩码回显。

### 说明

- 全部新引擎默认值保守：libi 开启但分数压在挑战阈值下、respfilter 关闭、
  dprotect 关闭、webhook 关闭、ACME 按需创建订单——升级不改变现有拦截行为。
- 响应侧命中（`resp-*`）永不参与评分与拦截，只记录或打码；事件里不保存命中
  原文（证据即敏感内容）。
- 动态防护的加密强度定位是「提高爬取成本」：密钥随页面下发（浏览器要能解密），
  按 JSON 字节数组混淆存放；配合 `dprotect.challenge` 的 PoW 门禁才是完整语义。
- MCP 客户端凭据走 API Token：创建 `read,write` 令牌 → 设 `GOWAF_URL` /
  `GOWAF_TOKEN` → 客户端接入 `gowaf mcp`。
- 数据库迁移 009（`api_tokens` 表）启动时自动执行；回滚到 v1.3.2 前无需清理
  （旧版本忽略该表）。

---

## [v1.3.2] - 2026-11-17

本版补上 v1.3.0 遗留的一处前端缺口：Bot 模块的六个全局参数此前只能改库或手工调接口，
控制台没有入口。仅前端改动，无数据库迁移与后端接口变更，替换二进制即可升级。

### 新增

- **Bot 管理 → 设置页签**
  此前 `bot.enabled` / `bot.score_threshold` / `bot.verify_rdns` / `bot.action` /
  `bot.window` / `bot.cache_ttl` 只存在于 `settings` 表，控制台没有任何入口
  （「防护配置」页只渲染 engine/guard/notify，「Bot 管理」页只有策略与蜜罐两个页签），
  只能改库或手工调 `PUT /api/settings`。现在「Bot 管理」新增第三个页签，六项可直接编辑，
  保存后热生效无需重启；页面挂载时读取当前值。
  写入只提交 `bot.*` 六个键，不会连带改动 SMTP 密码、引擎模式等其他设置项。
  需要 `settings:write` 权限。

### 说明

- `bot.score_threshold` 的语义是**单个 IP 在 `bot.window` 窗口内的请求次数**（默认 60），
  不是分数阈值。一个正常页面加载几十个静态资源，60 次/分钟容易被误判，界面文案与
  文档均建议按站点实际流量从 120–300 起步灰度。
- `bot.enabled` 为 `false` 时 `Check()` 在入口直接返回，**策略、蜜罐、行为评分全部跳过**。
  开启前建议先提高阈值，再观察「攻击事件」中 `bot` 分类的事件量。
- 本版默认值保持不变：`enabled=false` / `threshold=60`，升级不会自动打开拦截。

## [v1.3.1] - 2026-11-17

本版是 v1.3.0 的**上线后修正版**：修掉缓存链路上四个「配置看起来生效、实际没生效」的
问题（缓存从不命中、缓存绕过访问策略、仅记录动作被采样吞掉、事件搜不到策略/Bot 命中），
并补上缓存对象的可观测入口。无数据库迁移，替换二进制即可升级。

### 修复

- **页面缓存从不命中（数据面）**
  `httputil.ReverseProxy` 的 `FlushInterval: -1` 让每一段写入都立即 Flush，加速 writer
  在第一次 Flush 就降级为透传，`Close()` 里的入缓存与压缩逻辑永远不会执行——现象是
  `X-Cache` 永远是 `MISS`、缓存对象长期为 0。改为 `flushGrace = 300ms`：缓冲期内正常
  收集，超过宽限期立即恢复边收边发，慢响应与流式输出不受影响。

- **上游已压缩响应整体透传，缓存与二次压缩全部失效**
  源站（Apache / Nginx）通常已 gzip，直接透传等于把「压缩 + 缓存」两层都跳过。
  现在按 `Content-Encoding` 先解码（gzip / br / deflate）再入缓存，出站时按客户端
  `Accept-Encoding` 重新协商编码；并剔除带 `-gzip` 后缀的 ETag 与上游旧的
  `Content-Length`，避免客户端拿到长度不符的响应。一个对象同时服务 `br` 与 `gzip` 客户端。

- **缓存命中绕过精准访问控制**
  处理顺序是「缓存查找 → 读 body → 策略求值」，而缓存命中会直接返回响应，于是
  「拦截荷兰全境」「拦截某个 IP」这类策略对**已被缓存的静态资源**不生效（实测已缓存的
  js/css 对境外 IP 仍返回 200）。新增第 6.4 步提前判定 `EvaluateDenyEarly`：只对
  不依赖 body 且动作为 `block` / `ban` / `challenge` 的第一条命中策略生效；
  若第一条命中依赖 body、或是 `allow` / `log`，原样交回完整求值——保持「优先级第一条
  命中生效」的语义不变。观察模式下提前判定不拦截。

- **「仅记录」动作被采样吞掉，事件页几乎看不到数据**
  生产环境 `storage.sample_rate` 常设为 0.01，Bot 与访问策略的 `log` 动作不改变处置结果
  （最终仍是 pass），于是 99% 被采样丢弃，「先用仅记录观察一周」这种做法在事件页几乎
  没有样本。现在只要命中规则 / 策略 / Bot（`rule_ids` 非空）就强制落库，不受采样影响。

- **事件搜不到策略与 Bot 命中**
  关键词检索原先只覆盖 `path` / `ua` / `query`，而策略命中记录在 `rule_ids`（`policy-<id>`）、
  Bot 命中记录在 `categories` / `rule_ids`（`bot-<category>`）里，按策略名或规则 ID
  永远搜不出来。关键词扩展为 `path` / `ua` / `query` / `rule_ids` / `categories` /
  `client_ip`；事件页的动作筛选补 `ban`，分类筛选补 `bot` / `policy`。

- **缓存命中丢失 Bot 归因**
  命中缓存的分支只写了一条无分类、无规则 ID 的 `pass`，Bot「仅记录」的请求一旦资源在
  缓存里就完全失去线索。现在缓存命中路径会把 Bot 检测结果带进事件（动作仍记为
  `pass`，因为没拦截），无归因时传 `nil`，不再写入 `categories="null"` 的脏值。

- **新建策略 / Bot / 蜜罐 / 缓存规则时「停用」被写成「启用」**
  模型字段带 `default:1`，GORM 插入时会省略零值、交给数据库默认值，于是新增时选择
  停用（`enabled=0`）实际落库为启用——对拦截类配置是安全隐患（用户以为只是先存着不发，
  实际已对全网生效）。新增统一 `createRow()` 在创建后按意图纠正 `enabled`，
  覆盖策略 / Bot / 蜜罐 / 缓存规则 / 节点五个创建入口。

- **重复查询 IP 归属地**
  提前判定需要 `country`，原实现会在第 8 步再查一次；现在第 6.4 步算出的国家码沿用到
  完整求值，单请求少一次归属地查询。

### 新增

- **控制台「页面缓存 → 缓存列表」页签**
  展示当前内存中每个缓存对象的文件名、大小、路径、所属站点、命中次数、写入时间与剩余
  有效期（不足 1/4 标黄）；支持站点过滤、关键词（路径 / 文件名 / 域名）、按大小 /
  命中 / 剩余有效期排序与分页。后端接口 `GET /api/cache/entries`（仅需登录）。
  用于回答「规则明明配了，为什么没命中」「对象为什么很快只剩 1 个」这类问题——
  配合 `evictions` / `purges` 计数与 `ActiveEnterTimestamp` 可区分「被淘汰」「被清除」
  与「进程重启」三种情况（缓存是纯内存的，重启即清空）。

- **回归测试**
  `internal/edge/accelerate_test.go`（缓冲写入、flush 宽限、解码与重编码）、
  `internal/edge/policy_gate_test.go`（缓存命中不得绕过拒绝类策略）、
  `internal/edge/record_test.go`（带归因必落库、`categories` 归一化）、
  `internal/policy/early_test.go`（提前判定的语义边界）、
  `internal/cache/entries_test.go`（缓存列表快照与键拆分）、
  `internal/store/event_where_test.go`（关键词覆盖 `rule_ids` / `categories` / `client_ip`）。

- **文档**
  《精准访问控制 设置》《Bot 管理 设置》《缓存与CDN加速 设置》三份独立文档，
  含字段/操作符表、可直接照抄的规则配方与排障清单；README 中英双版补齐新模块。

### 升级说明

1. 无数据库迁移，schema 仍为 v8。停服务 → 替换 `gowaf-linux-<arch>` → 启动。
2. 升级后**已有的缓存规则立即开始真正命中**（v1.3.0 上的缓存基本没生效）；
   若此前为了「让缓存看起来工作」把 TTL 调得很短，可以调回正常值。
3. 之前已建但「以为停用了」的策略 / Bot / 蜜罐 / 缓存规则，升级后仍是**启用**状态
   （历史数据已被写成 `enabled=1`）。请到对应页面逐个确认开关，再用
   `sqlite3 data/waf.db "update access_policies set enabled=0 where id=…;"`
   这类方式或直接改开关纠正。
4. `bot.enabled` 仍默认 `false`，本次未改动默认值；开启前建议先把
   `bot.score_threshold` 提到 120~300 做灰度，避免行为评分误伤正常用户。
5. 缓存是单进程内存态，升级重启后缓存为空属正常；命中率会在几分钟内自然回升。

## [v1.3.0] - 2026-11-16

本版是「扩展能力」批次：在既有 WAF 检测之外补齐四个可组合的运营引擎——精准访问控制、
Bot 管理、页面缓存加速、站点多上游，外加节点管理与 IPv6 监听。全部默认关闭，
升级后请求路径行为与 v1.2.3 完全一致，在控制台逐个启用即可。

### 新增

- **精准访问控制**
  条件树（AND / OR / 嵌套分组，最深 4 层、单策略 ≤ 64 条件）在 Reload 时编译成不可变
  快照，热路径无锁、无 JSON 解析。字段覆盖 client_ip / country / region / host /
  method / path / query / ua / referer / content_type / body 及 `header.*` / `cookie.*` /
  `arg.*`；操作符 eq / ne / contains / prefix / suffix / regex / cidr（单 IP 自动补
  /32、/128，兼容 IPv4-mapped IPv6）/ in / 数值比较 / exists。动作 allow / block /
  challenge / log / ban，按优先级第一条命中生效；编译失败整体拒绝并保留旧快照，
  不会把站点放空。命中计数批量刷库，避免每个请求写一次 SQLite。
  控制台「访问策略」页 + `policy:write` 权限。

- **Bot 管理**
  UA / Path / IP / rDNS 四类策略 + 蜜罐路径（正常用户不会访问，命中即 ban）+
  行为评分（窗口请求频率 + 可疑 UA，超阈值按配置动作处置）。搜索引擎 Bot 走
  正向-反向 DNS 双重验证：真 Googlebot 放行且不占限流额度，伪造 UA 直接按可疑处置；
  DNS 结果带 TTL 缓存，不在热路径上阻塞。已验证爬虫在限流与人机校验之前放行。
  控制台「Bot 管理」页 + `bot:write` 权限。

- **页面缓存与加速**
  六类缓存规则（后缀 / 精确 / 前缀 / 正则 / 目录 / 匿名访客全站）；LRU + TTL，
  总容量与单对象上限可配，超过上限立即转流式透传，绝不把大文件读进内存；只缓存
  匿名 GET/HEAD，带 Set-Cookie / private / no-store 的响应一律不缓存，SSE /
  WebSocket 绕过。回源响应按客户端 Accept-Encoding 做 gzip / brotli 压缩（q 值感知、
  优先 brotli），JPEG/PNG 转 WebP——全部纯 Go 实现，CGO_ENABLED=0 可编译。
  支持按站点 / 路径前缀手动清除与命中统计。控制台「页面缓存」页 + `cache:write` 权限。

- **站点多上游**
  单站点可配多上游：按健康度与优先级加权随机选择，连接失败自动切换下一条
  （仅可重放请求重试，大文件 / 流式上传不换目标）；健康探测周期可配（`upstream.health_interval`）；
  全部目标不健康时 fail-open 退回全部目标，防止误判把站点打成 502。
  上游组合按站点缓存 ReverseProxy，连接池复用。

- **节点管理与 Agent 上报**
  新增 nodes 表与节点 CRUD / 令牌签发；Agent 免登录上报接口
  `GET /api/agent/config`、`POST /api/agent/heartbeat`（handler 内校验
  `node.agent_token`），心跳超时（`node.heartbeat_ttl`）视为离线。
  控制台「节点管理」页 + `node:write` 权限。

- **IPv6 额外监听**
  `server.listen_v6` / `server.tls_listen_v6`：与 v4 共用数据面 Handler 的独立监听。
  不默认双栈——部分发行版 v6only 语义不同，强行绑定 `::` 可能与已有 v4 监听冲突，
  导致老部署升级后启动失败。IPv6 监听端口一并纳入自环防护。

- **上传与反序列化检测**
  multipart 解析：危险扩展名、双扩展名绕过（shell.php.jpg）、文件名攻击特征、
  声明图片与魔数不符、可执行文件头（ELF / MZ / Java Class）与脚本标记；
  请求体反序列化特征：Java 原生流、.NET BinaryFormatter、Python pickle、PHP 序列化、
  Fastjson/Jackson `@type`、YAML 不安全标签、Node 原型链污染——合并进统一
  detect.Result 按分数处置。

### 变更

- 控制台新增「扩展中心」相关页面：访问策略 / Bot 管理 / 页面缓存 / 节点管理；
  站点编辑支持多上游配置。
- 权限按钮全集新增 `policy:write` / `bot:write` / `cache:write` / `node:write`。
- 内嵌前端构建产物同步（新增 bots / cache / extensions / policies / nodes chunk）。
- 四个引擎补齐单元测试约 1650 行：条件树操作符与编译校验、Bot 判定与评分、
  缓存 LRU/TTL 与压缩 / WebP 实际转换、上传与反序列化特征逐类验证。

### 升级说明

1. 停服务 → 替换 `gowaf-linux-<arch>` 与 `rules/builtin.yaml` → 启动。
   `configs/` 与 `data/` 保留。
2. **本版新增迁移 008（schema v7 → v8）**：新增 access_policies、bot_policies、
   bot_honeypots、cache_rules、site_upstreams、nodes 六张表。首次启动自动执行，
   无需手动操作；回滚到 v1.2.3 不受影响（旧版本不读这些表）。
3. 所有新引擎默认关闭：`bot.enabled=false`、`cache.enabled=false`、无默认精准策略、
   站点多上游为空——升级后请求路径行为与 v1.2.3 一致，在控制台逐个启用即可。
4. 需要 IPv6 时在 `config.yaml` 配置 `server.listen_v6`（HTTPS 还需 `server.tls_listen_v6`），
   重启生效。
5. Bot 行为评分默认 `bot.score_threshold=30` 次 / 窗口、超限动作 challenge；
   搜索引擎验证默认关闭（`bot.verify_rdns=false`），开启前确认服务器能做 DNS 反查。

## [v1.2.3] - 2026-11-08

本版把人机校验从「有模块但从不生效」补成真正的工作入口，并修正白名单语义：
白名单 IP 现在真的不被任何规则与封禁影响。另外补上封禁原因反查触发事件、
安全总览实时事件流默认开启，README 改为英文优先。

### 新增

- **人机校验适配 Cloudflare 等 CDN 回源**
  挑战页的内联脚本加上 `data-cfasync="false"`：CF 的 Rocket Loader 会默认延迟/重排
  页面内联脚本，被它接管后工作量证明可能根本不执行，用户会一直卡在「校验中」。
  安全响应同时补了 `CDN-Cache-Control: no-store`（站点开了 Cache Everything 时，
  普通 Cache-Control 会被边缘规则覆盖，挑战页一旦被缓存，后续访客会拿到别人的副本）
  与 `X-Robots-Tag: noindex`（403 的校验页不该被搜索引擎收录）。
  取 IP 侧早已支持 `CF-Connecting-IP`（优先于 XFF）与 `trusted_proxies` 网段校验，
  并在站点标记 CDN 后给出配置缺口提示；`deploy/install.sh cloudflare on` 可一次补齐网段。

- **人机校验（自动校验）真正接管可疑流量**
  校验页改为「JS 执行 + SHA-256 工作量证明」：页面自动算出 16 位前导零的答案换取
  30 分钟通行证（绑定 IP），用户无感；不执行 JS 的爬虫/扫描器拿不到通行证。
  新增两类触发条件，都在硬限流（429）之前生效，把「可疑但不确定」降级成一次自证：
  - 跨站来路：从外站跳转进来的页面导航先校验一次，站内跳转/直接访问/搜索引擎来路不校验
    （允许清单 `guard.challenge_referer_allow` 可配）；
  - 频率软阈值：`guard.challenge_rps`（默认 10 req/s）、`guard.challenge_per_minute`
    （默认 300 次/分）超限先校验，硬限流仍兜底。
  只对页面导航（`Sec-Fetch-Dest: document` 或 `Accept: text/html` 的 GET）生效：
  API/XHR 无法执行 JS，挑战等于封禁，这类流量继续走 429。
  控制台「设置 → 防护」新增四项可热改配置。

### 修复

- **人机校验计数恒为 0**
  挑战事件用 `sample=false` 记录，只进 Prometheus 指标、从不写 events 表，
  而看板「人机校验」统计的是 `action=challenge` 的事件，所以永远是 0。
  现改为全量落库；同时阈值带宽（30~59 分）实测几乎不命中——单条 CRS 规则就 100+ 分，
  新增的频率与来路触发才是这个模块真正的工作入口。
- **通行证可被一行请求头绕过**
  原实现只要请求带 `X-WAF-JS: 1` 就无条件签发通行证，等于校验形同虚设。
  现在必须提交服务端签名的挑战令牌与正确答案，缺一不可。
- **挑战签名密钥用错了内容**
  原代码把 `admin.jwt_secret_file` 的**路径字符串**当 HMAC 密钥，等于用公开常量签名；
  改为读取密钥文件内容（缺失时退化为进程内随机密钥，重启后旧通行证失效）。
- **白名单未真正豁免（语义与实现不一致）**
  数据面判定顺序原为「黑名单 → 封禁 → 白名单」，导致「先被封禁、后加白名单」的 IP
  仍然 403，必须手动解封才恢复。现调整为「白名单 → 黑名单 → 封禁 → 情报 → 限流 → 检测」：
  白名单 IP 不再经过任何拦截规则，也不会被自动封禁 / 情报封禁。
  配套：白名单 IP 拒绝手动封禁（`POST /api/bans` 返回 400 并提示先移出白名单）；
  新增 / 导入 / 改成白名单时自动解除该 IP 现存生效封禁（`unbanned` 计数回传前端）；
  白名单条目按 `expires_at` 到期失效（此前只有黑名单受过期约束，带到期时间的白名单
  会变成永久放行）。回归用例：`TestIPListWhitePrecedence`、`TestIPListWhiteExpiry`。

### 变更

- **实时事件流默认开启**
  安全总览挂载时自动建立 SSE 连接，不再需要先点「连接」；用户手动断开后保持断开，
  刷新页面恢复默认开启。切换站点筛选会按新站点重建流（旧流的 `site` 参数已失效）。
  空列表在暂停时给出「已暂停」提示，避免和「没有事件」混淆。
- **封禁管理支持反查触发事件**
  「封禁原因」列改为可点击入口：新增 `GET /api/bans/:id/events`（仅需登录），按
  同 IP + 封禁创建时间窗口（默认前 15 分钟、后 1 分钟，`?window=` 可调，上限 24 小时）
  检索事件，站点封禁只看该站点。抽屉内展示封禁摘要、关联事件列表与「最近一次命中」，
  点任一行可继续打开事件详情（复用事件页同一个抽屉）。
  规则类封禁（`auto` / `rule_ban`）只返回真正命中规则的事件：封禁生效后的请求在数据面
  第一道就被封禁表挡下，落库时无规则、0 分，时间却最新，会把「是什么触发的封禁」挤到后面。
  情报 / 手动封禁没有规则命中可看，保持全量；老数据过滤后为空时自动回退全量。

### 文档

- **README 改为英文优先**
  `README.md` 换成面向国际读者的精简英文版，只保留六块内容：简介与架构图、功能与模块、
  部署（构建 / Linux / Windows）、控制台如何访问、开发机调试、项目结构与已知限制。
  原有完整中文文档原样迁移到 `README.zh-CN.md`（含适配细节、场景指南、排障经验），
  两份顶部互相链接，细节口径以中文版为准。

---

## [v1.2.2] - 2026-11-07

本版合并 v1.2.0 之后 dev 分支的全部迭代：CRS 升级到 v4.30.0、支持导入 SecLang 规则包、
管理后台可检查 CRS 上游版本，并修复前后端分离站点批量 403 的线上事故。

### 新增

- **内嵌 CRS 升级 v4.0.0 → v4.30.0**
  `coraza-coreruleset` 改用 multi-module 布局（`crs/v4` 提供 CRS 规则、`coraza/v3` 提供
  `@coraza.conf-recommended`，`mergefs` 合并根 FS）。协议降权号段扩展至 922xxx
  （v4.30 新增 multipart 合规检查，全部 CRITICAL，与 920/921 同类降权封顶 29 分）。
  打包仓库 v4.30.0 遗漏 `web-shells-php.data`（上游 tag 存在），新增 `crspatch`
  补丁包以 base64 入库、运行时内存解码补入根 FS——该文件含真实 webshell 特征，
  明文落盘会被 Windows Defender 查杀，故不入库明文。
- **导入规则包支持 SecLang（第三方 .conf 包）**
  检测规则模块可直接粘贴/上传 SecLang 文本（`SecRule`/`SecAction`），服务端独立编译
  校验（失败整包拒绝；禁止包内改 `SecRuleEngine` 等引擎全局开关），落盘
  `data/seclang/<slug>.conf` 并随 CRS 一起编译执行。同 slug 重复导入视为升级覆盖。
  新增「SecLang 规则包」管理面板：启停 / 删除 / 错误回显（migration 007 新表
  `seclang_packages`，接口 `/api/seclang/*`）。包规则命中以 `seclang-<id>` 透出，
  未标 severity 默认计 40 分。隔离机制：单包编译失败自动跳过并记录 `last_error`；
  多包组合编译失败（如 rule id 冲突）退回纯 CRS，绝不让一个坏包打瘫引擎。
- **管理后台支持检查 CRS 上游新版本**
  新增 `GET /api/crs/check-update`（需登录，只读）：数据源依次回退 GitHub release →
  Go 模块代理（proxy.golang.org → goproxy.cn），结果服务端缓存 10 分钟（`?force=1`
  强制重查），语义化版本比较判定 `updateAvailable`。设置页「CRS 规则版本」行新增
  「检查更新」按钮：已是最新显示绿色标签；有新版给出 release 链接与升级提示
  （CRS 内嵌于二进制，升级需更新依赖重新构建发版，不做在线热更新）。

### 修复

- **前后端分离站点批量 403（线上事故）**
  CRS 默认 `tx.allowed_methods` 只有 `GET HEAD POST OPTIONS`，管理 API 的
  `PUT`/`DELETE`/`PATCH` 全部命中 911100（CRITICAL 60），叠加 920620 重复
  `Content-Type`（协议降权 15）正好达到拦截阈值 75；SPA 一屏十几个请求全 403 后
  触发 `guard.auto_ban`，误报被放大成整站不可用。新增 `detect.CRSOptions.AllowedMethods`，
  在 901-INITIALIZATION 之前写入 TX 覆盖 CRS 默认值（默认 REST 常规集，`sanitizeMethods`
  防指令注入），并新增设置项 `engine.crs.methods`（纯表单业务可收紧为
  `GET HEAD POST OPTIONS`）。回归：`PUT`/`DELETE`/`PATCH` 不再命中 911100，
  收紧后 `PUT` 恢复命中，SQLi 仍正常拦截。

---

## [v1.2.0] - 2026-11-04

威胁情报闭环 + 告警通知能力落地，同时修掉三处会导致「整站被拦」的严重缺陷。
本版把 v1.1.3 ~ v1.1.6 的迭代合并发布。

### 新增

- **威胁情报订阅支持自定义 Header / Token**
  新增 `intel_sources.headers` 字段，支持 JSON 对象或 `Key: Value` 逐行（`#` 开头为注释），
  填报 `X-Api-Key` / `Authorization` 即可对接 CrowdSec CTI、AbuseIPDB 等需要鉴权的源。
  同时新增 `kind=json`，按递归遍历解析 JSON 列表（不绑定字段名），
  `{"data":[{"ipAddress":"1.2.3.4"}]}` 这类结构可以直接消费。
  控制台回显时把 `key|auth|token|secret|cookie` 类头部掩码为 `******`，原样提交不会覆盖真实值。
- **预设情报源一键添加**
  `GET/POST /api/intel/presets`，内置 10 个公共源：Spamhaus DROP、FireHOL Level1、
  blocklist.de、ipsum Level3、CINS、GreenSnow、Binary Defense、ET Compromised、
  AbuseIPDB、CrowdSec CTI，附带预估条数、误报风险等级、是否需要 Key。
  按 URL 去重，需要 Key 的源缺 token 时拒绝写入。
- **情报命中自动纳入封禁管理**
  命中情报的请求会写入 `bans`（`source=intel`，作用域跟随情报源为全局或站点级），
  可在「封禁管理」统一查看与释放。开关 `intel.ban_on_hit`（默认开启），
  封禁时长 `intel.ban_ttl`（默认 3600 秒）。
- **邮件告警通知**（新菜单「告警通知」）
  支持 SMTP `ssl`（465 直连 TLS）/ `starttls`（587）/ `none`（25）；
  事件按时间窗聚合成一封摘要邮件（默认 60 秒），同 IP 冷却去重（默认 3600 秒），
  评分阈值可过滤低危事件（`score=0` 的情报封禁不会被阈值吞掉）；
  内置「发送测试邮件」与投递日志（成功/失败原因均落库）。
  后续按同一通道抽象扩展飞书、短信、企业微信。

### 修复

- **CRS 协议合规规则导致正常登录被自动封禁**（严重）
  `920000–922000` 段的协议合规规则在正常 POST 上可累计到 60 分，等于 `engine.block_score`，
  连续 5 次即触发自动封禁。现对整段单独降权：单条上限 15 分、该段合计上限 29 分，
  永远够不到默认的 challenge 阈值 30。SQLi/XSS 等真实攻击的评分不受影响。
- **观察模式未真正「只看不拦」**（严重）
  `engine.mode=observe` 原先只阻止「新封禁」，已存在的黑名单、封禁记录、情报命中仍在拦截。
  现整条拦截链路（黑名单 → 封禁 → 情报）退化为仅记录，限流不受影响。
- **情报源混入 bogon 导致反向代理场景自锁**（严重）
  多来源聚合清单可能包含 `127.0.0.0/8`、`10.0.0.0/8` 等特殊用途网段；
  反代场景下上游未传 `X-Forwarded-For` 时客户端 IP 退化为 `127.0.0.1`，
  命中即全站 403。现在 `intel.Match` 对特殊用途地址直接短路，解析阶段也丢弃整段 bogon
  （仅丢弃 `bits >= 8` 的段，`0.0.0.0/1` 这类覆盖公网的大聚合保留）。
- **告警通知「收件人」输入框整个不渲染**
  `waf.notify.toPlaceholder` 文案含裸 `@`，vue-i18n 将其识别为链接消息语法
  （`@:key`）并在编译期抛 `Invalid linked format`，Vue 吞掉异常后整个表单项消失，
  用户无处填写收件人却又被「请先填写」拦住。文案改用 `{'@'}` 字面量转义，
  并新增 `Frontend/scripts/check-i18n.mjs` 守卫（扫描语言包裸 `@`，已挂到 `npm run build` 前置）。
- **QQ 邮箱拒收测试邮件（500 Line too long）**
  正文以 `8bit` 直发，HTML 压缩后是超长单行，超过 RFC 5321 的 998 octet 行上限，
  QQ 在 DATA 结束阶段直接拒收。正文改用 `quoted-printable`（自动 76 字符软换行），
  收件人列表按 72 字符折行，并补 `mail_test.go` 回归。
- 告警通知表单不再随「启用」开关禁用（原先必须先启用才能填、启用又要求已填完，形成死锁）；
  启用开关校验失败时回退，避免界面显示已启用但未落库。
- 设置保存后右侧状态卡片立即刷新，不再需要手动刷新页面。
- 前端 5xx 错误优先展示服务端返回的具体原因，不再一律显示「服务器内部错误」。
- 邮件测试失败改为返回 400（配置类问题），避免前端拦截器对 5xx 重试造成重复投递。

### 变更

- 内置检测规则由 49 条扩充至 **181 条**
  （sensitive_file 45 / scanner 25 / rce 22 / protocol 20 / xss 19 / upload 19 / sqli 18 / traversal 13）。
- 数据面流水线明确为：黑名单 → 封禁 → 白名单 → 威胁情报 → 限流 → 静态跳过 → 检测 → 转发。
- 数据库迁移至 schema v6（`006_intel_notify.sql`：新增 `notify_logs` 表、`intel_sources.headers` 列）。
- README 按当前技术栈与已交付能力全面重写。

### 升级说明

- 直接替换二进制与 `rules/builtin.yaml` 即可，`configs/` 与 `data/` 保留，迁移会自动执行。
- 告警通知默认关闭（`notify.enabled=false`），需要在「告警通知」页填写 SMTP 后手动启用。
- 升级后建议确认「防护配置」中的模式与 CRS 偏执等级符合预期（`settings` 表优先于 `config.yaml`）。

---

## [v1.1.1] - 2026-10

- 规则运营能力：分组统计、命中样例、批量启停、导入/导出。
- 例外规则：按「站点 / 规则号 / 分类 / 路径前缀 / 参数名」组合放过，同时作用于自研引擎与 CRS。
- 攻击事件新增 IP 归属地列（离线 ip2region 库，构建期生成，无运行时依赖）。
- 审计日志、配置版本与回滚。

## [v1.1.0] - 2026-09

- 接入 OWASP CRS v4（Coraza 内嵌第二引擎），支持 `SecRuleEngine On/DetectionOnly` 与偏执等级 1–4。
- 检测规则支持在线增删改、评分热更新、高危规则直封（`action: ban`）。

## [v1.0.0] - 2026-08

- 首个正式版：反向代理与多站点、TLS/SNI 多证书、自研检测引擎（AC 自动机 + RE2）、
  限流与自动封禁、黑白名单、Cloudflare 真实 IP、计划任务、Vue 3 控制台、
  Linux/Windows 一键部署脚本与自愈机制。
