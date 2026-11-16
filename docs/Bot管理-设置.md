# Bot 管理 设置

> 适用版本：v1.3.0 及以上（含事件归因与采样修复）
> 控制台路径：`#/waf/bots`
> 最后更新：2026-11-17

---

## 一、能力定位

Bot 管理解决的是「**这个请求是人还是程序**」的问题，与规则检测（看内容像不像攻击）互补：

| 能力 | 说明 |
|---|---|
| 四类显式策略 | 按 User-Agent、请求路径、客户端 IP、反向 DNS（PTR）匹配 |
| 正向-反向 DNS 验证 | UA 自称 Googlebot 时，校验 IP 的 PTR 与正查是否一致，识破伪造 |
| 蜜罐路径 | 正常用户不会访问的路径，命中即判恶意并封禁 |
| 行为评分 | 统计窗口内的请求频次，叠加可疑 UA 加权，超阈值按默认动作处置 |

判定产生的结果统称为「Verdict」，会落进事件表，分类为 `bot`，规则 ID 形如 `bot-<分类>`。

---

## 二、判定顺序（四步，先命中先返回）

| 顺序 | 阶段 | 命中后的动作 |
|---|---|---|
| 1 | 蜜罐路径 | 固定按恶意处理，**动作硬编码为 ban**，写封禁表 |
| 2 | 显式策略 | 按优先级取第一条命中，执行策略配置的动作 |
| 3 | 搜索引擎 rDNS 验证 | 仅当「全局开关 + 校验 rDNS」都开启：验证通过 → 放行；UA 冒充但验证失败 → 按全局默认动作处置（打分 90） |
| 4 | 行为评分 | 窗口内请求数 ≥ 阈值 → 基础分 60 + 超出部分（上限 40）；可疑 UA +30；总分 ≥ 60 才处置 |

> 关键推论：**显式策略在第 2 步就返回，会抢在 rDNS 验证之前**。因此不要给「 googlebot / bingbot 」这类 UA 建「放行」策略——那等于关掉伪造检测。真正的爬虫校验交给第 3 步。

---

## 三、正则表达式写法（最容易出错的部分）

### 3.1 三条底层规则

| 规则 | 说明 |
|---|---|
| **自动忽略大小写** | 引擎编译时会自动前置 `(?i)`。你写 `googlebot` 就能匹配 `GoogleBot`，**不需要**自己加；加了变成 `(?i)(?i)…` 也合法，不会报错 |
| **子串匹配，不是全串** | 底层是 Go 的 `MatchString`，只要串里有一段匹配就算命中。要精确匹配必须自己加 `^…$` |
| **RE2 语法** | 支持 `()` `\|` `[]` `{n,m}` `\d` `\b`；**不支持** lookahead `(?=)`、lookbehind `(?<=)`、反向引用 `\1` |

### 3.2 四个高频坑

| 写法 | 问题 | 正确做法 |
|---|---|---|
| `Goog*` | `*` 修饰的是前一个字符，本意是「任意字符」 | `Goog.*` |
| `.js` | `.` 是任意字符，`/aXjs` 也会命中 | `\.js` |
| `Googlebot`（想只匹配整串） | 子串匹配，`FakeGooglebotX` 也命中 | `^Mozilla/5\.0 \(compatible; Googlebot` |
| `(?=bot)` | RE2 不支持断言，保存时直接报「正则非法」 | 改写成 `bot` 或 `(bot\|crawler)` |

### 3.3 词边界的推荐写法

`nmap` 这类短词直接做子串匹配会误伤（`unmapped` 里就含 `nmap`）。用「左边界 + 右边界」包一层：

```regex
(^|[^a-z])(sqlmap|nikto|nmap|masscan|zgrab|nuclei|wpscan)([^a-z]|$)
```

- `^|[^a-z]`：开头或非字母字符（分隔符）
- `[^a-z]|$`：结尾或非字母字符
- 已自动忽略大小写，无需 `(?i)`

### 3.4 正则长度与转义

- 单条表达式上限 **512 字符**（超长会存不进）。
- 表达式按原文存储，**不需要为 JSON 额外转义**——但如果你用 API 调用，JSON 层仍要按 JSON 规则把 `\` 写成 `\\`，界面里直接填 `\.` 即可。

---

## 四、四种匹配方式

| 匹配方式 | 值的形式 | 语义 | 注意 |
|---|---|---|---|
| **User-Agent** | 正则 | 对 UA 整串做子串匹配 | 最常用；空 UA 需 `^$` |
| **请求路径** | 正则 | 对解码后的路径做子串匹配 | 想只在段首命中加 `(^|/)` |
| **客户端 IP** | IP 或 CIDR | 网段包含判断，支持裸 IP（按 /32） | 不写正则 |
| **反向 DNS** | 正则 | 对 PTR 名称做子串匹配 | ⚠️ 不要用 PTR 后缀做「放行」，PTR 由 IP 所有者自行设置，可伪造；放行请交给内置正反查验证 |

---

## 五、五种处置动作

| 动作 | 行为 | 建议用途 |
|---|---|---|
| **放行** | 跳过后续 WAF 检测，直接回源 | 已知监控探针、合作方采集 |
| **仅记录** | 不拦截，写事件并继续检测 | 灰度观察，上线前必做 |
| **人机校验** | 下发校验页，通过后 30 分钟内免校验 | AI 爬虫、无头浏览器 |
| **拦截** | 返回 403，不封 IP | SEO 抓取工具 |
| **封禁 IP** | 返回 403 并写入封禁表 | 扫描器、恶意采集 |

---

## 六、Bot 分类

分类是**纯标签**，只影响列表筛选、事件里的归因展示与统计，**不参与判定**：

| 值 | 界面文案 |
|---|---|
| `search_engine` | 搜索引擎 |
| `ai_crawler` | AI 爬虫 |
| `content grab` → `scraper` | 内容抓取 |
| `monitor` | 监控探针 |
| `malicious` | 恶意 Bot |
| `unknown` | 未知 |

---

## 七、优先级与冲突

- 数字**小的先命中**，同优先级按创建 ID 升序，**第一条命中即返回**。
- 推荐编排：放行类 `5–10` → 封禁类 `20–25` → 拦截类 `40` → 观察类 `50–62` → 中风险 `70–80` → 路径类 `90–95`。
- 分类相同不代表互斥：`AI 训练爬虫`(P60) 与 `AI 搜索类`(P62) 的 UA 列表必须不重叠，否则后一条永远不生效。

---

## 八、当前线上已配置的 14 条策略

| 优先级 | 分类 | 匹配 | 动作 | 正则（引擎自动忽略大小写） |
|---|---|---|---|---|
| 5 | 监控探针 | UA | 放行 | `(uptimerobot\|pingdom\|statuscake\|site24x7\|better ?uptime\|newrelicpinger\|nagios\|zabbix\|prometheus\|blackbox_exporter\|datadog\|grafana)` |
| 8 | 搜索引擎 | UA | 仅记录（**默认停用**） | `(googlebot\|bingbot\|baiduspider\|yandexbot\|duckduckbot\|sogou web spider\|360spider\|applebot)` |
| 20 | 恶意 Bot | UA | 封禁 IP | `(^\|[^a-z])(sqlmap\|nikto\|nmap\|masscan\|zgrab\|nuclei\|gobuster\|dirbuster\|ffuf\|wfuzz\|wpscan\|acunetix\|nessus\|openvas\|whatweb\|joomscan\|droopescan)([^a-z]\|$)` |
| 22 | 恶意 Bot | UA | 封禁 IP | `(httrack\|webcopier\|teleport pro\|zmeu\|crawler4j\|morfeus\|blackwidow\|winhttp\.request)` |
| 24 | 恶意 Bot | IP | 封禁 IP | `45.138.12.19` |
| 40 | 内容抓取 | UA | 拦截 | `(ahrefsbot\|semrushbot\|mj12bot\|dotbot\|blexbot\|dataforseo\|seznambot\|serpstatbot\|screaming ?frog\|sitebulb\|petalbot)` |
| 50 | 未知 | rDNS | 仅记录 | `(amazonaws\|digitalocean\|linode\|vultr\|ovh\|contabo\|hetzner\|hostinger\|choopa\|colocrossing)` |
| 52 | 未知 | rDNS | 仅记录 | `(googleusercontent\|azurewebsites\|cloudfront\|akamai\|fastly)` |
| 60 | AI 爬虫 | UA | 人机校验 | `(gptbot\|claudebot\|anthropic-ai\|claude-web\|ccbot\|bytespider\|meta-externalagent\|applebot-extended\|img2dataset\|cohere-ai\|diffbot\|omgili)` |
| 62 | AI 爬虫 | UA | 仅记录 | `(oai-searchbot\|chatgpt-user\|perplexitybot\|youbot\|google-extended)` |
| 70 | 监控探针 | UA | 人机校验 | `(headlesschrome\|phantomjs\|puppeteer\|playwright\|selenium\|webdriver\|electron/)` |
| 80 | 内容抓取 | UA | 仅记录 | `(^\|[^a-z])(scrapy\|python-requests\|aiohttp\|httpx\|go-http-client\|okhttp\|libwww-perl\|curl\|wget\|java/\|apache-httpclient)([^a-z]\|$)` |
| 90 | 恶意 Bot | 路径 | 封禁 IP | `(^\|/)(\.env\|\.git\|wp-login\.php\|xmlrpc\.php\|phpmyadmin\|adminer\.php\|\.ds_store\|\.svn\|\.aws\|\.ssh)(/\|$)` |
| 95 | 未知 | 路径 | 人机校验 | `(^\|/)(cgi-bin/\|vendor/phpunit\|solr/\|actuator/\|telescope\|_ignition/execute-solution\|druid/index)` |

**蜜罐路径**（命中即封禁 IP）：`/.env`、`/.git/config`、`/wp-login.php`

> P8 为什么默认停用：它会命中真实搜索引擎 UA 并**抢先于 rDNS 验证**返回，等于关掉伪造检测。只有在关闭「校验 rDNS」后才建议启用。

---

## 九、全局设置

| 设置项 | 键名 | 含义 | 建议 |
|---|---|---|---|
| 启用 | `bot.enabled` | 总开关，关闭时全部 Bot 判定（含蜜罐、评分）不执行 | 观察期后开启 |
| 校验 rDNS | `bot.verify_rdns` | 对自称搜索引擎的 UA 做正反查 | 保持开启 |
| 评分阈值 | `bot.score_threshold` | 窗口内请求次数达到该值开始计分 | 60（纯 API 站点建议调高） |
| 默认动作 | `bot.action` | 评分超限 / 伪造爬虫 的处置 | 人机校验 |
| 统计窗口 | `bot.window` | 行为计数窗口（秒） | 60 |
| rDNS 缓存 | `bot.cache_ttl` | PTR 结果缓存秒数 | 600 |

---

## 十、上线与验证

**第一步**：先全部用「仅记录」跑 1–2 天，按分类看命中量。

**第二步**：单条验证（把 UA 换成你策略里的关键字）：

```bash
curl -sI https://站点/ -H 'User-Agent: sqlmap/1.7'
# 期望：403，响应头 X-WAF-Action: ban / X-WAF-Reason: bot
```

**第三步**：事件页核对。筛选「类型 = Bot 管理」，或直接搜关键词 `bot-malicious`。带 `bot-` 前缀的规则 ID 就是归因。

**第四步**：确认无误后再开总开关。

---

## 十一、与其它模块的执行顺序

| 顺序 | 模块 | 关系 |
|---|---|---|
| 1–3 | 路由 / IP 白名单 / 封禁 | 先于 Bot |
| **4** | **Bot 判定** | **先于策略、先于缓存**：命中即处置，不会走到后面 |
| 5.6 | 人机校验 | 后于 Bot |
| 6 | 限流 | 后于 Bot |
| 6.4 | 策略提前判定 | 后于 Bot |
| 6.5 | 页面缓存 | Bot 命中的请求不会进缓存读取分支 |

> Bot 在缓存之前，所以「拦截/校验」对已缓存的静态资源同样生效；但「仅记录」不改变动作，请求仍会命中缓存，事件按缓存命中记录（`upstream=cache`），归因信息会一并带上。

---

## 十二、本次修复记录

| 问题 | 现象 | 修复 |
|---|---|---|
| 事件搜不到 Bot / 策略拦截 | 事件页关键词只搜 path/UA/query，`bot-<分类>`、`policy-<id>` 都在 `rule_ids` 字段里，搜不到 | 关键词扩展为 path / ua / query / rule_ids / categories / client_ip |
| 分类筛不出 | 分类下拉只有规则引擎八类，缺 `bot`、`policy`；分类标签显示原始 i18n key | 前端补两类选项 + 中英文案；动作下拉补 `ban` |
| 缓存命中丢失 Bot 归因 | 请求在 Bot 阶段命中但只记录，随后缓存命中直接返回，事件退化为一条无归因的 pass | 缓存命中分支带上 Bot 结果 |
| 「仅记录」几乎看不到 | 仅记录不改变动作，最终按 pass 以 `sample_rate`（生产 0.01）采样，99% 不入库 | 带归因（rule_ids 非空）的事件强制全量落库 |
| 新增时选「停用」被写成启用 | GORM 对 `default:1` 字段忽略零值 | 创建后显式回写 enabled |

---

## 十三、排障对照表

| 现象 | 可能原因 | 处理 |
|---|---|---|
| 策略配了完全没反应 | 总开关关闭 | 打开 `bot.enabled` |
| 保存时报「正则非法」 | 用了 RE2 不支持的语法（`(?=)`、`\1`） | 改写为等价表达式 |
| 误伤正常流量 | 用了短词子串（如 `bot`、`nmap`） | 加词边界或锚定 |
| 真实 Googlebot 被拦 | 建了 UA 拦截/校验策略，抢先于 rDNS 验证 | 删除该策略，交给验证 |
| rdns 放行被绕过 | PTR 可被 IP 所有者伪造 | 不要用 rdns 做放行判断 |
| 事件页看不到 Bot 事件 | 动作是「仅记录」且未升级版本 | 升级到含归因修复的版本；或改用「人机校验」观察 |
| 蜜罐误封了人 | 该路径确有真实访问 | 删除对应蜜罐，或改小范围 |
