# 更新日志

本项目的所有重要变更都会记录在这里，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

---

## [未发布]

### 新增

- **Bot 管理 → 设置页签**
  此前 `bot.enabled` / `bot.score_threshold` / `bot.verify_rdns` / `bot.action` /
  `bot.window` / `bot.cache_ttl` 只存在于 `settings` 表，控制台没有任何入口
  （「防护配置」页只渲染 engine/guard/notify，「Bot 管理」页只有策略与蜜罐两个页签），
  只能改库或手工调 `PUT /api/settings`。现在「Bot 管理」新增第三个页签，六项可直接编辑，
  保存后热生效无需重启；页面挂载时读取当前值。
  写入只提交 `bot.*` 六个键，不会连带改动 SMTP 密码、引擎模式等其他设置项。

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
