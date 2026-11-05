// Package model 定义 WAF 的全部 GORM 数据模型。
//
// 覆盖管理员账号（User）、刷新令牌（RefreshToken）、站点（Site）、检测规则（Rule）、
// 规则例外（RuleException）、威胁情报源（IntelSource）、IP 黑白名单（IPRule）、
// 封禁（Ban）、计划任务（Task）、请求事件（Event）、分钟聚合（Stats1m）、
// 审计日志（AuditLog）、键值配置（Setting）、配置快照（ConfigVersion）、
// 通知记录（NotifyLog）、SecLang 规则包（SeclangPackage）等表结构。
//
// 约定：
//  1. 时间一律存 Unix 毫秒（int64），不用 time.Time，避免 SQLite 时区与格式歧义；
//  2. JSON 数组字段用 string 存，读写时由 service 层负责序列化；
//  3. 只有"配置类表"走 GORM；events / stats_1m 见 store 包的原生 SQL 实现。
package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

// TableName 指定 User 模型对应的数据库表名。
// 参数：无；返回：表名 "sys_users"。
func (User) TableName() string { return "sys_users" }

// User 管理员账号表。
type User struct {
	ID           uint   `gorm:"primaryKey;autoIncrement" json:"userId"`                        // 主键，自增；JSON 输出为 userId
	UserName     string `gorm:"column:user_name;uniqueIndex;size:64;not null" json:"userName"` // 登录名，列 user_name，唯一索引，最长 64 字符
	PasswordHash string `gorm:"column:password_hash;size:128;not null" json:"-"`               // 密码哈希（bcrypt），不出现在 JSON 中
	NickName     string `gorm:"column:nick_name;size:64" json:"nickName"`                      // 显示昵称
	Email        string `gorm:"column:email;size:128" json:"email"`                            // 邮箱
	Avatar       string `gorm:"column:avatar;size:255" json:"avatar"`                          // 头像地址
	Role         string `gorm:"column:role;size:32;not null;default:admin" json:"role"`        // 角色标识（如 admin 管理员 / demo 演示），默认 admin
	Buttons      string `gorm:"column:buttons;size:512" json:"-"`                              // 按钮级权限标识集合（JSON 数组，见 store.AllButtons），不出现在 JSON 中
	Status       int    `gorm:"column:status;not null;default:1" json:"status"`                // 账号状态：1 启用（可登录），非 1 拒绝登录；默认 1
	LastLoginAt  int64  `gorm:"column:last_login_at" json:"lastLoginAt"`                       // 上次登录时间（Unix 毫秒）
	LastLoginIP  string `gorm:"column:last_login_ip;size:64" json:"lastLoginIp"`               // 上次登录 IP
	CreatedAt    int64  `gorm:"column:created_at" json:"createdAt"`                            // 创建时间（Unix 毫秒）
	UpdatedAt    int64  `gorm:"column:updated_at" json:"updatedAt"`                            // 更新时间（Unix 毫秒）
	DeletedAt    int64  `gorm:"column:deleted_at;index" json:"-"`                              // 软删除标记（0=未删除），带索引；不出现在 JSON 中
}

// TableName 指定 RefreshToken 模型对应的数据库表名。
// 参数：无；返回：表名 "sys_refresh_tokens"。
func (RefreshToken) TableName() string { return "sys_refresh_tokens" }

// RefreshToken 可吊销的刷新令牌表。
type RefreshToken struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"id"`                 // 主键，自增
	UserID    uint   `gorm:"column:user_id;index" json:"userId"`                 // 所属用户 ID，带索引
	JTI       string `gorm:"column:jti;uniqueIndex;size:64;not null" json:"jti"` // JWT ID，令牌唯一标识，唯一索引，吊销时按它匹配
	ExpiresAt int64  `gorm:"column:expires_at;not null" json:"expiresAt"`        // 过期时间（Unix 毫秒）
	RevokedAt int64  `gorm:"column:revoked_at" json:"revokedAt"`                 // 吊销时间（0=未吊销）
	ClientIP  string `gorm:"column:client_ip;size:64" json:"clientIp"`           // 签发时的客户端 IP
	UserAgent string `gorm:"column:user_agent;size:255" json:"userAgent"`        // 签发时的 User-Agent
	CreatedAt int64  `gorm:"column:created_at" json:"createdAt"`                 // 签发时间（Unix 毫秒）
}

// NormalizeBehindCDN 把各种写法收敛成两种取值：” 或 'cloudflare'。
//
// 只认 cloudflare：取真实 IP 依赖 trusted_proxies 里的具体网段，
// 随便填一个 CDN 名字不会带来任何行为，只会制造「以为配好了」的错觉。
// 放在 model 包里是为了让控制台接口与 YAML 种子数据共用同一套规则。
// 参数 v：用户输入的 CDN 名称（可含首尾空白、大小写任意）。
// 返回：输入等价于 "cloudflare" 时返回 "cloudflare"，否则返回 ”（直连）。
func NormalizeBehindCDN(v string) string {
	if strings.EqualFold(strings.TrimSpace(v), "cloudflare") {
		return "cloudflare"
	}
	return ""
}

// TableName 指定 Site 模型对应的数据库表名。
// 参数：无；返回：表名 "sites"。
func (Site) TableName() string { return "sites" }

// Site 站点表（Host → 上游的转发配置）。
type Site struct {
	ID            uint   `gorm:"primaryKey;autoIncrement" json:"id"`                                // 主键，自增
	Name          string `gorm:"column:name;size:64;not null" json:"name"`                          // 站点名称，最长 64 字符
	Hosts         string `gorm:"column:hosts;size:512;not null" json:"-"`                           // 匹配的 Host 列表（JSON 数组），不出现在 JSON 中
	Upstream      string `gorm:"column:upstream;size:255;not null" json:"upstream"`                 // 上游目标地址
	Enabled       int    `gorm:"column:enabled;not null;default:1" json:"enabled"`                  // 是否启用：1 启用 / 0 停用，默认 1
	DetectBody    int    `gorm:"column:detect_body;not null;default:1" json:"detectBody"`           // 是否检测请求体：1 检测 / 0 跳过，默认 1
	MaxBodyBytes  int    `gorm:"column:max_body_bytes;not null;default:262144" json:"maxBodyBytes"` // body 检测上限（字节），默认 256KB，超过则不读不检
	WhitelistPath string `gorm:"column:whitelist_paths;size:1024" json:"-"`                         // 免检测路径白名单（JSON 数组，支持 * 前缀通配），不出现在 JSON 中
	RateLimit     string `gorm:"column:rate_limit;size:255" json:"-"`                               // 站点级限流配置（JSON 对象），不出现在 JSON 中
	// BehindCDN 标记站点是否在 CDN 回源之后：'' 直连 ｜ 'cloudflare'。
	// 不带 default tag：GORM 对「有 default 且值为零」的字段会省略该列，
	// 导致把 cloudflare 改回直连时写不进去（同 Rule.Builtin 的坑）。
	BehindCDN string `gorm:"column:behind_cdn;size:32;not null" json:"behindCdn"` // 是否在 CDN 之后：'' 直连 / 'cloudflare'；无 default 以保证零值能落库
	// ProtectCfg 站点级防护覆盖配置：'' 跟随全局；否则 JSON（SiteProtect）。
	// 同样不带 default tag，理由同上——把自定义改回"跟随全局"时零值必须能落库。
	ProtectCfg string `gorm:"column:protect_cfg;size:1024;not null" json:"-"` // 防护覆盖配置：'' 跟随全局 / JSON（SiteProtect）；不出现在 JSON 中
	CreatedAt  int64  `gorm:"column:created_at" json:"createdAt"`             // 创建时间（Unix 毫秒）
	UpdatedAt  int64  `gorm:"column:updated_at" json:"updatedAt"`             // 更新时间（Unix 毫秒）
}

// SiteProtect 站点级防护覆盖配置（以 JSON 形式存于 Site.ProtectCfg 列）。
//
// 语义：Enabled=1 时启用站点自定义；每个字段 0/"" 表示该维度跟随全局，
// 非 0 表示覆盖。启用后限流计数独立于全局（桶按"站点+IP"隔离），
// 避免十几个站点共享一个计数器把单 IP 指标挤爆。
type SiteProtect struct {
	Enabled        int    `json:"enabled"`                  // 1 启用站点自定义防护 / 0 关闭（跟随全局）
	Mode           string `json:"mode,omitempty"`           // 工作模式：observe 观察 / protect 拦截；空=跟随全局
	BlockScore     int    `json:"blockScore,omitempty"`     // 拦截分数线（1~1000，需大于 challengeScore）；0=跟随全局
	ChallengeScore int    `json:"challengeScore,omitempty"` // 人机挑战分数线（1~1000）；0=跟随全局
	IPRPS          int    `json:"ipRps,omitempty"`          // 单 IP 每秒请求数阈值；0=跟随全局
	IPBurst        int    `json:"ipBurst,omitempty"`        // 单 IP 突发容量阈值；0=跟随全局
	IPPerMinute    int    `json:"ipPerMinute,omitempty"`    // 单 IP 每分钟请求数阈值；0=跟随全局
}

// ParseSiteProtect 解析并校验 protect_cfg 列。
// 参数 raw：库中存储的原始字符串（” 或 "null" 视为未启用，跟随全局）。
// 返回：跟随全局时 (nil, nil)；配置合法时返回解析出的 SiteProtect；
// JSON 非法或字段校验失败时返回 (nil, error)。
func ParseSiteProtect(raw string) (*SiteProtect, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil, nil
	}
	var p SiteProtect
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, fmt.Errorf("protect_cfg JSON 非法: %w", err)
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// validate 校验站点防护覆盖配置的取值合法性。
// 参数：无（校验接收者 p 的各字段）。
// 返回：nil 表示合法；否则返回描述违规原因的错误
// （enabled 只能 0/1，mode 只能 observe/protect，数值范围 0~1000000，
// blockScore/challengeScore 需在 1~1000 且 blockScore > challengeScore）。
func (p *SiteProtect) validate() error {
	if p.Enabled != 0 && p.Enabled != 1 {
		return fmt.Errorf("enabled 只能是 0 或 1")
	}
	if p.Mode != "" && p.Mode != "observe" && p.Mode != "protect" {
		return fmt.Errorf("mode 只能是 observe/protect")
	}
	for k, v := range map[string]int{
		"blockScore": p.BlockScore, "challengeScore": p.ChallengeScore,
		"ipRps": p.IPRPS, "ipBurst": p.IPBurst, "ipPerMinute": p.IPPerMinute,
	} {
		if v < 0 || v > 1000000 {
			return fmt.Errorf("%s 超出范围", k)
		}
	}
	if p.BlockScore > 0 && p.BlockScore > 1000 {
		return fmt.Errorf("blockScore 必须是 1~1000")
	}
	if p.ChallengeScore > 0 && p.ChallengeScore > 1000 {
		return fmt.Errorf("challengeScore 必须是 1~1000")
	}
	if p.BlockScore > 0 && p.ChallengeScore > 0 && p.BlockScore <= p.ChallengeScore {
		return fmt.Errorf("必须满足 blockScore > challengeScore")
	}
	return nil
}

// TableName 指定 Task 模型对应的数据库表名。
// 参数：无；返回：表名 "tasks"。
func (Task) TableName() string { return "tasks" }

// Task 计划任务表。内置任务名白名单见 internal/tasks 包的 builtinTasks。
type Task struct {
	Name           string `gorm:"column:name;primaryKey;size:32" json:"name"`                       // 任务名，主键，最长 32 字符
	Enabled        int    `gorm:"column:enabled;not null;default:1" json:"enabled"`                 // 是否启用：1 启用 / 0 停用，默认 1
	IntervalH      int    `gorm:"column:interval_h;not null;default:6" json:"intervalH"`            // 执行间隔（小时），默认 6
	Params         string `gorm:"column:params;size:255;not null" json:"-"`                         // 任务参数（JSON 对象），不出现在 JSON 中
	LastRunMs      int64  `gorm:"column:last_run_ms;not null;default:0" json:"lastRunMs"`           // 上次执行时间（Unix 毫秒，0=从未执行）
	LastResult     string `gorm:"column:last_result;size:255;not null" json:"lastResult"`           // 上次执行结果摘要（成功描述或失败原因）
	LastDurationMs int64  `gorm:"column:last_duration_ms;not null;default:0" json:"lastDurationMs"` // 上次执行耗时（毫秒）
	TotalRuns      int64  `gorm:"column:total_runs;not null;default:0" json:"totalRuns"`            // 累计执行次数
	UpdatedAt      int64  `gorm:"column:updated_at" json:"updatedAt"`                               // 配置更新时间（Unix 毫秒）
}

// TableName 指定 Rule 模型对应的数据库表名。
// 参数：无；返回：表名 "rules"。
func (Rule) TableName() string { return "rules" }

// Rule 检测规则表。
type Rule struct {
	ID       string `gorm:"column:id;primaryKey;size:64" json:"id"`                     // 规则 ID，主键，最长 64 字符
	Name     string `gorm:"column:name;size:128;not null" json:"name"`                  // 规则名称
	Category string `gorm:"column:category;size:32;not null;index" json:"category"`     // 攻击类别（如 sqli/xss），带索引
	Score    int    `gorm:"column:score;not null;default:30" json:"score"`              // 命中后累加的风险分，默认 30
	Targets  string `gorm:"column:targets;size:128;not null" json:"targets"`            // 检测目标，逗号分隔：path,query,body,header,cookie,ua
	Keywords string `gorm:"column:keywords;size:1024" json:"-"`                         // 关键字列表（JSON 数组），供 AC 自动机预筛；不出现在 JSON 中
	Pattern  string `gorm:"column:pattern;size:2048" json:"pattern"`                    // 正则表达式
	Action   string `gorm:"column:action;size:16;not null;default:block" json:"action"` // 处置动作：block/challenge/log/ban（ban: 命中即封禁来源 IP），默认 block
	Enabled  int    `gorm:"column:enabled;not null;default:1" json:"enabled"`           // 是否启用：1 启用 / 0 停用，默认 1
	// Builtin 不带 default tag：GORM 对「有 default 且值为零」的字段会省略该列，
	// 导致自建规则被写成内置规则而无法删除，这里必须让零值真正落库。
	Builtin   int    `gorm:"column:builtin;not null" json:"builtin"`               // 1 内置规则（不可删除）/ 0 用户自建；无 default 以保证零值能落库
	Priority  int    `gorm:"column:priority;not null;default:100" json:"priority"` // 执行优先级，默认 100
	Remark    string `gorm:"column:remark;size:255" json:"remark"`                 // 备注说明
	UpdatedAt int64  `gorm:"column:updated_at" json:"updatedAt"`                   // 更新时间（Unix 毫秒）

	// 规则包字段：来源 / 维护方 / 版本 / 分组，支持按来源整组启停与升级对比。
	Source  string `gorm:"column:source;size:16;not null;default:builtin" json:"source"` // 规则来源标识（内置为 builtin，自建为 custom 等），默认 builtin
	Vendor  string `gorm:"column:vendor;size:32;not null;default:gowaf" json:"vendor"`   // 维护方标识，默认 gowaf
	Version string `gorm:"column:version;size:32;not null;default:1" json:"version"`     // 规则包版本，默认 "1"
	// Group 列名避开 SQL 关键字 group。
	Group string `gorm:"column:rule_group;size:64;not null;default:base" json:"group"` // 规则分组，列名 rule_group 避开 SQL 关键字 group，默认 base
	// Mode: inherit（跟随引擎）/ log（只记录不计分）/ block
	Mode string `gorm:"column:mode;size:16;not null;default:inherit" json:"mode"` // 生效模式：inherit 跟随引擎 / log 只记录不计分 / block 拦截，默认 inherit

	// 命中统计由后台任务定期回写，用于判断规则是否还有价值。
	Hits      int64 `gorm:"column:hits;not null;default:0" json:"hits"`             // 累计命中次数（后台任务定期回写）
	LastHitAt int64 `gorm:"column:last_hit_at;not null;default:0" json:"lastHitAt"` // 最近命中时间（Unix 毫秒，0=从未命中）
}

// RuleException 规则例外表：用于误报治理。
//
// 匹配语义（全部为空 = 全站全规则生效，谨慎使用）：
//   - Site 为空表示所有站点；
//   - RuleID 为空表示该路径/参数下的所有规则都跳过；
//   - PathPrefix 按前缀匹配请求路径；
//   - Param 命中参数名（query/body 的 key）时跳过。
type RuleException struct {
	ID         uint   `gorm:"primaryKey;autoIncrement" json:"id"`                     // 主键，自增
	Site       string `gorm:"column:site;size:64;not null" json:"site"`               // 生效站点名，空串=所有站点
	RuleID     string `gorm:"column:rule_id;size:64;not null" json:"ruleId"`          // 例外针对的规则 ID，空串=该范围内所有规则
	Category   string `gorm:"column:category;size:32;not null" json:"category"`       // 例外针对的攻击类别，空串=所有类别
	PathPrefix string `gorm:"column:path_prefix;size:255;not null" json:"pathPrefix"` // 请求路径前缀，空串=所有路径
	Param      string `gorm:"column:param;size:64;not null" json:"param"`             // 命中即跳过的参数名（query/body 的 key），空串=不按参数过滤
	Note       string `gorm:"column:note;size:255;not null" json:"note"`              // 例外原因备注
	Enabled    int    `gorm:"column:enabled;not null;default:1" json:"enabled"`       // 是否启用：1 启用 / 0 停用，默认 1
	CreatedBy  string `gorm:"column:created_by;size:64;not null" json:"createdBy"`    // 创建人
	CreatedAt  int64  `gorm:"column:created_at;not null;default:0" json:"createdAt"`  // 创建时间（Unix 毫秒）
}

// TableName 指定 RuleException 模型对应的数据库表名。
// 参数：无；返回：表名 "rule_exceptions"。
func (RuleException) TableName() string { return "rule_exceptions" }

// IntelSource 威胁情报源表：定时拉取 IP/CIDR 列表并落进封禁表。
//
// Kind=ip 时按纯文本列表解析（每行一条，支持 # 注释）；
// Kind=json 时抓 JSON 并递归提取其中的 IP/CIDR 字段（AbuseIPDB、CrowdSec CTI 等）。
// Headers 是随请求发送的自定义头，用来放 API Key；Scope=site 时只对指定站点生效。
type IntelSource struct {
	ID         uint   `gorm:"primaryKey;autoIncrement" json:"id"`                        // 主键，自增
	Name       string `gorm:"column:name;size:64;not null" json:"name"`                  // 情报源名称
	URL        string `gorm:"column:url;size:512;not null" json:"url"`                   // 拉取地址
	Kind       string `gorm:"column:kind;size:16;not null;default:ip" json:"kind"`       // 数据格式：ip 纯文本列表 / json JSON 递归提取，默认 ip
	Headers    string `gorm:"column:headers;not null;default:''" json:"headers"`         // 请求自定义头（JSON 对象，可放 API Key），默认空
	Scope      string `gorm:"column:scope;size:16;not null;default:global" json:"scope"` // 生效范围：global 全局 / site 指定站点，默认 global
	Site       string `gorm:"column:site;size:64;not null" json:"site"`                  // Scope=site 时的生效站点名
	TTLSeconds int64  `gorm:"column:ttl_seconds;not null;default:0" json:"ttlSeconds"`   // 情报缓存有效期（秒），即两次拉取的最小间隔，默认 0
	Enabled    int    `gorm:"column:enabled;not null;default:1" json:"enabled"`          // 是否启用：1 启用 / 0 停用，默认 1
	LastSyncAt int64  `gorm:"column:last_sync_at;not null;default:0" json:"lastSyncAt"`  // 上次同步时间（Unix 毫秒，0=从未同步）
	LastCount  int    `gorm:"column:last_count;not null;default:0" json:"lastCount"`     // 上次同步拉取到的条目数
	LastError  string `gorm:"column:last_error;size:255;not null" json:"lastError"`      // 上次同步的错误信息（空=成功）
	CreatedAt  int64  `gorm:"column:created_at;not null;default:0" json:"createdAt"`     // 创建时间（Unix 毫秒）
}

// TableName 指定 IntelSource 模型对应的数据库表名。
// 参数：无；返回：表名 "intel_sources"。
func (IntelSource) TableName() string { return "intel_sources" }

// TableName 指定 IPRule 模型对应的数据库表名。
// 参数：无；返回：表名 "ip_rules"。
func (IPRule) TableName() string { return "ip_rules" }

// IPRule IP 黑白名单表。
//
// Scope=global 时 Site 为空、对所有站点生效；Scope=site 时只在 Site 指定站点生效。
// Site 存站点名（events.site 同样存名字，数据面直接按 Route.SiteName 比对），
// 站点改名由 SiteUpdate 级联更新。
type IPRule struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"id"`                        // 主键，自增
	IP        string `gorm:"column:ip;size:64;not null" json:"ip"`                      // 单个 IP 或 CIDR 网段
	Kind      string `gorm:"column:kind;size:16;not null" json:"kind"`                  // 名单类型：black 黑名单 / white 白名单
	Scope     string `gorm:"column:scope;size:64;not null;default:global" json:"scope"` // 生效范围：global 全局 / site 指定站点，默认 global
	Site      string `gorm:"column:site;size:64;not null" json:"site"`                  // Scope=site 时的生效站点名
	Reason    string `gorm:"column:reason;size:255" json:"reason"`                      // 加入名单的原因
	ExpiresAt int64  `gorm:"column:expires_at;not null;default:0" json:"expiresAt"`     // 过期时间（Unix 毫秒，0=永不过期）
	CreatedBy string `gorm:"column:created_by;size:64" json:"createdBy"`                // 创建人
	CreatedAt int64  `gorm:"column:created_at" json:"createdAt"`                        // 创建时间（Unix 毫秒）
}

// TableName 指定 Ban 模型对应的数据库表名。
// 参数：无；返回：表名 "bans"。
func (Ban) TableName() string { return "bans" }

// Ban 封禁记录表。
//
// Scope=global 时 Site 为空，命中任何站点都拒绝；Scope=site 时只拒绝该站点。
// 自动封禁（auto）与高危规则封禁（rule_ban）一律全局，手动封禁可指定站点。
type Ban struct {
	ID         uint   `gorm:"primaryKey;autoIncrement" json:"id"`                        // 主键，自增
	IP         string `gorm:"column:ip;size:64;not null;index" json:"ip"`                // 被封禁的 IP，带索引
	Reason     string `gorm:"column:reason;size:255" json:"reason"`                      // 封禁原因
	Source     string `gorm:"column:source;size:16;not null" json:"source"`              // 封禁来源：auto 连击自动封禁 / manual 手动 / rule_ban 高危规则 / intel 威胁情报
	Scope      string `gorm:"column:scope;size:16;not null;default:global" json:"scope"` // 生效范围：global 全局 / site 指定站点，默认 global
	Site       string `gorm:"column:site;size:64;not null" json:"site"`                  // Scope=site 时的生效站点名
	HitCount   int    `gorm:"column:hit_count;not null;default:1" json:"hitCount"`       // 封禁期间的命中次数，默认 1
	CreatedAt  int64  `gorm:"column:created_at;not null" json:"createdAt"`               // 封禁开始时间（Unix 毫秒）
	ExpiresAt  int64  `gorm:"column:expires_at;not null" json:"expiresAt"`               // 封禁到期时间（Unix 毫秒）
	ReleasedAt int64  `gorm:"column:released_at" json:"releasedAt"`                      // 手动解封时间（0=未解封）
}

// TableName 指定 Event 模型对应的数据库表名。
// 参数：无；返回：表名 "events"。
func (Event) TableName() string { return "events" }

// Event 请求事件表（每次转发的访问与处置记录）。热路径，写入走原生 SQL。
type Event struct {
	ID         uint   `gorm:"primaryKey;autoIncrement" json:"id"`        // 主键，自增
	TS         int64  `gorm:"column:ts;not null;index" json:"ts"`        // 事件时间（Unix 毫秒），带索引
	Site       string `gorm:"column:site;size:64" json:"site"`           // 站点名
	ClientIP   string `gorm:"column:client_ip;size:64" json:"clientIp"`  // 客户端 IP
	Method     string `gorm:"column:method;size:8" json:"method"`        // HTTP 方法
	Host       string `gorm:"column:host;size:128" json:"host"`          // 请求 Host
	Path       string `gorm:"column:path;size:1024" json:"path"`         // 请求路径
	Query      string `gorm:"column:query;size:2048" json:"query"`       // 查询字符串
	Status     int    `gorm:"column:status" json:"status"`               // 响应状态码
	Action     string `gorm:"column:action;size:16;index" json:"action"` // 处置动作（pass 放行 / block 拦截 / challenge 挑战等），带索引
	Score      int    `gorm:"column:score" json:"score"`                 // 风险得分
	Categories string `gorm:"column:categories;size:255" json:"-"`       // 命中的攻击类别（JSON 数组），不出现在 JSON 中
	RuleIDs    string `gorm:"column:rule_ids;size:512" json:"-"`         // 命中的规则 ID（JSON 数组），不出现在 JSON 中
	UA         string `gorm:"column:ua;size:512" json:"ua"`              // User-Agent
	Referer    string `gorm:"column:referer;size:512" json:"referer"`    // Referer
	ReqID      string `gorm:"column:req_id;size:64" json:"reqId"`        // 请求 ID，用于链路追踪
	LatencyUS  int64  `gorm:"column:latency_us" json:"latencyUs"`        // 上游响应耗时（微秒）
	Upstream   string `gorm:"column:upstream;size:255" json:"upstream"`  // 实际转发的上游地址
	Country    string `gorm:"column:country;size:64" json:"country"`     // 客户端 IP 属地（ISO 国家代码）
	Bytes      int64  `gorm:"column:bytes" json:"bytes"`                 // 响应字节数
}

// TableName 指定 Stats1m 模型对应的数据库表名。
// 参数：无；返回：表名 "stats_1m"。
func (Stats1m) TableName() string { return "stats_1m" }

// Stats1m 分钟级流量聚合表（每分钟 × 每站点一行）。
type Stats1m struct {
	Minute     int64  `gorm:"column:minute;primaryKey;autoIncrement:false" json:"minute"` // 分钟时间戳（Unix 毫秒取整到分钟），联合主键
	Site       string `gorm:"column:site;primaryKey;size:64" json:"site"`                 // 站点名，联合主键
	Total      int64  `gorm:"column:total" json:"total"`                                  // 总请求数
	Passed     int64  `gorm:"column:passed" json:"passed"`                                // 放行请求数
	Blocked    int64  `gorm:"column:blocked" json:"blocked"`                              // 拦截请求数
	Challenged int64  `gorm:"column:challenged" json:"challenged"`                        // 下发人机挑战的请求数
	Status5xx  int64  `gorm:"column:status_5xx" json:"status5xx"`                         // 上游 5xx 响应数
	Categories string `gorm:"column:categories;size:512" json:"-"`                        // 各攻击类别分布（JSON），不出现在 JSON 中
	TopIPs     string `gorm:"column:top_ips;size:512" json:"-"`                           // 本分钟请求量最高的 IP 摘要（JSON），不出现在 JSON 中
	TopPaths   string `gorm:"column:top_paths;size:512" json:"-"`                         // 本分钟请求量最高的路径摘要（JSON），不出现在 JSON 中
}

// TableName 指定 AuditLog 模型对应的数据库表名。
// 参数：无；返回：表名 "audit_logs"。
func (AuditLog) TableName() string { return "audit_logs" }

// AuditLog 配置变更审计日志表。
type AuditLog struct {
	ID       uint   `gorm:"primaryKey;autoIncrement" json:"id"`                      // 主键，自增
	TS       int64  `gorm:"column:ts;not null;index" json:"ts"`                      // 操作时间（Unix 毫秒），带索引
	Operator string `gorm:"column:operator;size:64;not null" json:"operator"`        // 操作人
	ClientIP string `gorm:"column:client_ip;size:64" json:"clientIp"`                // 操作来源 IP
	Action   string `gorm:"column:action;size:32;not null" json:"action"`            // 动作类型（create/update/delete 等）
	Object   string `gorm:"column:object;size:32;not null" json:"object"`            // 操作对象类型（site/rule/setting 等）
	ObjectID string `gorm:"column:object_id;size:64" json:"objectId"`                // 操作对象 ID
	Before   string `gorm:"column:before;size:4096" json:"before"`                   // 变更前快照（JSON）
	After    string `gorm:"column:after;size:4096" json:"after"`                     // 变更后快照（JSON）
	Result   string `gorm:"column:result;size:16;not null;default:ok" json:"result"` // 操作结果：ok 成功 / 其他为失败描述，默认 ok
}

// TableName 指定 Setting 模型对应的数据库表名。
// 参数：无；返回：表名 "settings"。
func (Setting) TableName() string { return "settings" }

// Setting 键值配置表，带版本号用于热更新检测。
type Setting struct {
	Key       string `gorm:"column:key;primaryKey;size:64" json:"key"`         // 配置键，主键
	Value     string `gorm:"column:value;size:4096" json:"value"`              // 配置值
	Version   int64  `gorm:"column:version;not null;default:1" json:"version"` // 版本号，用于热更新检测，默认 1
	UpdatedAt int64  `gorm:"column:updated_at" json:"updatedAt"`               // 更新时间（Unix 毫秒）
}

// TableName 指定 SchemaMeta 模型对应的数据库表名。
// 参数：无；返回：表名 "schema_meta"。
func (SchemaMeta) TableName() string { return "schema_meta" }

// SchemaMeta 数据库迁移版本记录表。
type SchemaMeta struct {
	Key   string `gorm:"column:key;primaryKey;size:64" json:"key"` // 元数据键（如迁移项名称），主键
	Value string `gorm:"column:value;size:1024" json:"value"`      // 元数据值（如迁移版本号）
}

// TableName 指定 ConfigVersion 模型对应的数据库表名。
// 参数：无；返回：表名 "config_versions"。
func (ConfigVersion) TableName() string { return "config_versions" }

// ConfigVersion 配置快照表，用于回滚。
type ConfigVersion struct {
	Version   int64  `gorm:"column:version;primaryKey;autoIncrement" json:"version"` // 快照版本号，主键，自增
	Snapshot  string `gorm:"column:snapshot;not null" json:"snapshot"`               // 配置快照内容（JSON）
	Operator  string `gorm:"column:operator;size:64" json:"operator"`                // 创建快照的操作人
	Remark    string `gorm:"column:remark;size:255" json:"remark"`                   // 快照备注
	CreatedAt int64  `gorm:"column:created_at" json:"createdAt"`                     // 创建时间（Unix 毫秒）
}

// NotifyLog 告警通知发送记录表。
//
// 只记「发了什么、发给谁、成没成」，不存完整正文（邮件正文可能含大量 IP，
// 存库收益低）。Channel 预留 email/feishu/sms/wecom，方便后续扩展多渠道。
type NotifyLog struct {
	ID        uint   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`                 // 主键，自增
	Channel   string `gorm:"column:channel;size:16;not null;default:email" json:"channel"` // 发送渠道：email/feishu/sms/wecom 等，默认 email
	Target    string `gorm:"column:target;size:255;not null" json:"target"`                // 接收方（如收件邮箱）
	Subject   string `gorm:"column:subject;size:255;not null" json:"subject"`              // 通知标题
	Summary   string `gorm:"column:summary;not null" json:"summary"`                       // 内容摘要（不含完整正文）
	Count     int    `gorm:"column:count;not null;default:0" json:"count"`                 // 本次通知聚合的事件条数，默认 0
	Status    string `gorm:"column:status;size:16;not null;default:ok" json:"status"`      // 发送结果：ok 成功 / fail 失败，默认 ok
	Error     string `gorm:"column:error;not null" json:"error"`                           // 失败原因（空=成功）
	CreatedAt int64  `gorm:"column:created_at;not null;default:0" json:"createdAt"`        // 发送时间（Unix 毫秒）
}

// TableName 指定 NotifyLog 模型对应的数据库表名。
// 参数：无；返回：表名 "notify_logs"。
func (NotifyLog) TableName() string { return "notify_logs" }

// SeclangPackage SecLang 规则包表：第三方 SecLang/CRS 插件包的元信息。
//
// 包内容（.conf 文本）落盘在 data/seclang/<slug>.conf，由 Coraza 与内嵌
// CRS 一起编译执行；本表只管生命周期（启停/计数/错误）。
type SeclangPackage struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"id"`                              // 主键，自增
	Name      string `gorm:"column:name;size:128;not null" json:"name"`                       // 规则包显示名
	Slug      string `gorm:"column:slug;size:64;not null;uniqueIndex" json:"slug"`            // 规则包短标识（对应 data/seclang/<slug>.conf 文件名），唯一索引
	Vendor    string `gorm:"column:vendor;size:64;not null;default:''" json:"vendor"`         // 维护方/来源标识，默认空
	Version   string `gorm:"column:version;size:32;not null;default:'1'" json:"version"`      // 规则包版本，默认 "1"
	RuleCount int    `gorm:"column:rule_count;not null;default:0" json:"ruleCount"`           // 包内规则条数，默认 0
	Enabled   int    `gorm:"column:enabled;not null;default:1" json:"enabled"`                // 是否启用：1 启用 / 0 停用，默认 1
	LastError string `gorm:"column:last_error;size:255;not null;default:''" json:"lastError"` // 上次加载/编译的错误信息（空=正常）
	Note      string `gorm:"column:note;size:255;not null;default:''" json:"note"`            // 备注，默认空
	CreatedBy string `gorm:"column:created_by;size:64;not null;default:''" json:"createdBy"`  // 安装人，默认空
	CreatedAt int64  `gorm:"column:created_at;not null;default:0" json:"createdAt"`           // 安装时间（Unix 毫秒）
	UpdatedAt int64  `gorm:"column:updated_at;not null;default:0" json:"updatedAt"`           // 更新时间（Unix 毫秒）
}

// TableName 指定 SeclangPackage 模型对应的数据库表名。
// 参数：无；返回：表名 "seclang_packages"。
func (SeclangPackage) TableName() string { return "seclang_packages" }
