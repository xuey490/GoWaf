# 更新日志

本项目的所有重要变更都会记录在这里，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

---

## [未发布]

暂无待发布变更。

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
