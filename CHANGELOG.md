# 更新日志

本项目的所有重要变更都会记录在这里，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

---

## [未发布]

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
