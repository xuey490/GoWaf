// Package model 定义所有 GORM 模型。
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

// TableName 显式指定表名，避免 GORM 复数化规则带来的意外。
func (User) TableName() string { return "sys_users" }

// User 管理员账号。
type User struct {
	ID           uint   `gorm:"primaryKey;autoIncrement" json:"userId"`
	UserName     string `gorm:"column:user_name;uniqueIndex;size:64;not null" json:"userName"`
	PasswordHash string `gorm:"column:password_hash;size:128;not null" json:"-"`
	NickName     string `gorm:"column:nick_name;size:64" json:"nickName"`
	Email        string `gorm:"column:email;size:128" json:"email"`
	Avatar       string `gorm:"column:avatar;size:255" json:"avatar"`
	Role         string `gorm:"column:role;size:32;not null;default:admin" json:"role"`
	Buttons      string `gorm:"column:buttons;size:512" json:"-"`
	Status       int    `gorm:"column:status;not null;default:1" json:"status"`
	LastLoginAt  int64  `gorm:"column:last_login_at" json:"lastLoginAt"`
	LastLoginIP  string `gorm:"column:last_login_ip;size:64" json:"lastLoginIp"`
	CreatedAt    int64  `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt    int64  `gorm:"column:updated_at" json:"updatedAt"`
	DeletedAt    int64  `gorm:"column:deleted_at;index" json:"-"`
}

// TableName 显式表名。
func (RefreshToken) TableName() string { return "sys_refresh_tokens" }

// RefreshToken 可吊销的刷新令牌。
type RefreshToken struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint   `gorm:"column:user_id;index" json:"userId"`
	JTI       string `gorm:"column:jti;uniqueIndex;size:64;not null" json:"jti"`
	ExpiresAt int64  `gorm:"column:expires_at;not null" json:"expiresAt"`
	RevokedAt int64  `gorm:"column:revoked_at" json:"revokedAt"`
	ClientIP  string `gorm:"column:client_ip;size:64" json:"clientIp"`
	UserAgent string `gorm:"column:user_agent;size:255" json:"userAgent"`
	CreatedAt int64  `gorm:"column:created_at" json:"createdAt"`
}

// NormalizeBehindCDN 把各种写法收敛成两种取值：” 或 'cloudflare'。
//
// 只认 cloudflare：取真实 IP 依赖 trusted_proxies 里的具体网段，
// 随便填一个 CDN 名字不会带来任何行为，只会制造「以为配好了」的错觉。
// 放在 model 包里是为了让控制台接口与 YAML 种子数据共用同一套规则。
func NormalizeBehindCDN(v string) string {
	if strings.EqualFold(strings.TrimSpace(v), "cloudflare") {
		return "cloudflare"
	}
	return ""
}

// TableName 显式表名。
func (Site) TableName() string { return "sites" }

// Site 站点（Host → 上游）。
type Site struct {
	ID            uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	Name          string `gorm:"column:name;size:64;not null" json:"name"`
	Hosts         string `gorm:"column:hosts;size:512;not null" json:"-"` // JSON 数组
	Upstream      string `gorm:"column:upstream;size:255;not null" json:"upstream"`
	Enabled       int    `gorm:"column:enabled;not null;default:1" json:"enabled"`
	DetectBody    int    `gorm:"column:detect_body;not null;default:1" json:"detectBody"`
	MaxBodyBytes  int    `gorm:"column:max_body_bytes;not null;default:262144" json:"maxBodyBytes"`
	WhitelistPath string `gorm:"column:whitelist_paths;size:1024" json:"-"` // JSON 数组
	RateLimit     string `gorm:"column:rate_limit;size:255" json:"-"`       // JSON 对象
	// BehindCDN 标记站点是否在 CDN 回源之后：'' 直连 ｜ 'cloudflare'。
	// 不带 default tag：GORM 对「有 default 且值为零」的字段会省略该列，
	// 导致把 cloudflare 改回直连时写不进去（同 Rule.Builtin 的坑）。
	BehindCDN string `gorm:"column:behind_cdn;size:32;not null" json:"behindCdn"`
	// ProtectCfg 站点级防护覆盖配置：'' 跟随全局；否则 JSON（SiteProtect）。
	// 同样不带 default tag，理由同上——把自定义改回"跟随全局"时零值必须能落库。
	ProtectCfg string `gorm:"column:protect_cfg;size:1024;not null" json:"-"`
	CreatedAt  int64  `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt  int64  `gorm:"column:updated_at" json:"updatedAt"`
}

// SiteProtect 站点级防护覆盖配置。
//
// 语义：Enabled=1 时启用站点自定义；每个字段 0/"" 表示该维度跟随全局，
// 非 0 表示覆盖。启用后限流计数独立于全局（桶按"站点+IP"隔离），
// 避免十几个站点共享一个计数器把单 IP 指标挤爆。
type SiteProtect struct {
	Enabled        int    `json:"enabled"`
	Mode           string `json:"mode,omitempty"` // observe/protect，空=跟随全局
	BlockScore     int    `json:"blockScore,omitempty"`
	ChallengeScore int    `json:"challengeScore,omitempty"`
	IPRPS          int    `json:"ipRps,omitempty"`
	IPBurst        int    `json:"ipBurst,omitempty"`
	IPPerMinute    int    `json:"ipPerMinute,omitempty"`
}

// ParseSiteProtect 解析并校验 protect_cfg 列。
// 返回 (nil, nil) 表示跟随全局；配置非法时返回错误。
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

// TableName 显式表名。
func (Task) TableName() string { return "tasks" }

// Task 计划任务。内置任务名白名单见 app/tasks.go。
type Task struct {
	Name           string `gorm:"column:name;primaryKey;size:32" json:"name"`
	Enabled        int    `gorm:"column:enabled;not null;default:1" json:"enabled"`
	IntervalH      int    `gorm:"column:interval_h;not null;default:6" json:"intervalH"`
	Params         string `gorm:"column:params;size:255;not null" json:"-"` // JSON 对象
	LastRunMs      int64  `gorm:"column:last_run_ms;not null;default:0" json:"lastRunMs"`
	LastResult     string `gorm:"column:last_result;size:255;not null" json:"lastResult"`
	LastDurationMs int64  `gorm:"column:last_duration_ms;not null;default:0" json:"lastDurationMs"`
	TotalRuns      int64  `gorm:"column:total_runs;not null;default:0" json:"totalRuns"`
	UpdatedAt      int64  `gorm:"column:updated_at" json:"updatedAt"`
}

// TableName 显式表名。
func (Rule) TableName() string { return "rules" }

// Rule 检测规则。
type Rule struct {
	ID       string `gorm:"column:id;primaryKey;size:64" json:"id"`
	Name     string `gorm:"column:name;size:128;not null" json:"name"`
	Category string `gorm:"column:category;size:32;not null;index" json:"category"`
	Score    int    `gorm:"column:score;not null;default:30" json:"score"`
	Targets  string `gorm:"column:targets;size:128;not null" json:"targets"` // path,query,body,header,cookie,ua
	Keywords string `gorm:"column:keywords;size:1024" json:"-"`              // JSON 数组，AC 预筛
	Pattern  string `gorm:"column:pattern;size:2048" json:"pattern"`
	Action   string `gorm:"column:action;size:16;not null;default:block" json:"action"` // block/challenge/log/ban（ban: 命中即封禁来源 IP）
	Enabled  int    `gorm:"column:enabled;not null;default:1" json:"enabled"`
	// Builtin 不带 default tag：GORM 对「有 default 且值为零」的字段会省略该列，
	// 导致自建规则被写成内置规则而无法删除，这里必须让零值真正落库。
	Builtin   int    `gorm:"column:builtin;not null" json:"builtin"`
	Priority  int    `gorm:"column:priority;not null;default:100" json:"priority"`
	Remark    string `gorm:"column:remark;size:255" json:"remark"`
	UpdatedAt int64  `gorm:"column:updated_at" json:"updatedAt"`

	// 规则包字段：来源 / 维护方 / 版本 / 分组，支持按来源整组启停与升级对比。
	Source  string `gorm:"column:source;size:16;not null;default:builtin" json:"source"`
	Vendor  string `gorm:"column:vendor;size:32;not null;default:gowaf" json:"vendor"`
	Version string `gorm:"column:version;size:32;not null;default:1" json:"version"`
	// Group 列名避开 SQL 关键字 group。
	Group string `gorm:"column:rule_group;size:64;not null;default:base" json:"group"`
	// Mode: inherit（跟随引擎）/ log（只记录不计分）/ block
	Mode string `gorm:"column:mode;size:16;not null;default:inherit" json:"mode"`

	// 命中统计由后台任务定期回写，用于判断规则是否还有价值。
	Hits      int64 `gorm:"column:hits;not null;default:0" json:"hits"`
	LastHitAt int64 `gorm:"column:last_hit_at;not null;default:0" json:"lastHitAt"`
}

// RuleException 规则例外：误报治理。
//
// 匹配语义（全部为空 = 全站全规则生效，谨慎使用）：
//   - Site 为空表示所有站点；
//   - RuleID 为空表示该路径/参数下的所有规则都跳过；
//   - PathPrefix 按前缀匹配请求路径；
//   - Param 命中参数名（query/body 的 key）时跳过。
type RuleException struct {
	ID         uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	Site       string `gorm:"column:site;size:64;not null" json:"site"`
	RuleID     string `gorm:"column:rule_id;size:64;not null" json:"ruleId"`
	Category   string `gorm:"column:category;size:32;not null" json:"category"`
	PathPrefix string `gorm:"column:path_prefix;size:255;not null" json:"pathPrefix"`
	Param      string `gorm:"column:param;size:64;not null" json:"param"`
	Note       string `gorm:"column:note;size:255;not null" json:"note"`
	Enabled    int    `gorm:"column:enabled;not null;default:1" json:"enabled"`
	CreatedBy  string `gorm:"column:created_by;size:64;not null" json:"createdBy"`
	CreatedAt  int64  `gorm:"column:created_at;not null;default:0" json:"createdAt"`
}

// TableName 显式表名。
func (RuleException) TableName() string { return "rule_exceptions" }

// IntelSource 威胁情报源：定时拉取 IP/CIDR 列表并落进封禁表。
//
// Kind=ip 时按纯文本列表解析（每行一条，支持 # 注释）；
// Kind=json 时抓 JSON 并递归提取其中的 IP/CIDR 字段（AbuseIPDB、CrowdSec CTI 等）。
// Headers 是随请求发送的自定义头，用来放 API Key；Scope=site 时只对指定站点生效。
type IntelSource struct {
	ID         uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	Name       string `gorm:"column:name;size:64;not null" json:"name"`
	URL        string `gorm:"column:url;size:512;not null" json:"url"`
	Kind       string `gorm:"column:kind;size:16;not null;default:ip" json:"kind"`
	Headers    string `gorm:"column:headers;not null;default:''" json:"headers"`
	Scope      string `gorm:"column:scope;size:16;not null;default:global" json:"scope"`
	Site       string `gorm:"column:site;size:64;not null" json:"site"`
	TTLSeconds int64  `gorm:"column:ttl_seconds;not null;default:0" json:"ttlSeconds"`
	Enabled    int    `gorm:"column:enabled;not null;default:1" json:"enabled"`
	LastSyncAt int64  `gorm:"column:last_sync_at;not null;default:0" json:"lastSyncAt"`
	LastCount  int    `gorm:"column:last_count;not null;default:0" json:"lastCount"`
	LastError  string `gorm:"column:last_error;size:255;not null" json:"lastError"`
	CreatedAt  int64  `gorm:"column:created_at;not null;default:0" json:"createdAt"`
}

// TableName 显式表名。
func (IntelSource) TableName() string { return "intel_sources" }

// TableName 显式表名。
func (IPRule) TableName() string { return "ip_rules" }

// IPRule IP 黑白名单。
//
// Scope=global 时 Site 为空、对所有站点生效；Scope=site 时只在 Site 指定站点生效。
// Site 存站点名（events.site 同样存名字，数据面直接按 Route.SiteName 比对），
// 站点改名由 SiteUpdate 级联更新。
type IPRule struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	IP        string `gorm:"column:ip;size:64;not null" json:"ip"`
	Kind      string `gorm:"column:kind;size:16;not null" json:"kind"` // black/white
	Scope     string `gorm:"column:scope;size:64;not null;default:global" json:"scope"`
	Site      string `gorm:"column:site;size:64;not null" json:"site"`
	Reason    string `gorm:"column:reason;size:255" json:"reason"`
	ExpiresAt int64  `gorm:"column:expires_at;not null;default:0" json:"expiresAt"`
	CreatedBy string `gorm:"column:created_by;size:64" json:"createdBy"`
	CreatedAt int64  `gorm:"column:created_at" json:"createdAt"`
}

// TableName 显式表名。
func (Ban) TableName() string { return "bans" }

// Ban 封禁记录。
//
// Scope=global 时 Site 为空，命中任何站点都拒绝；Scope=site 时只拒绝该站点。
// 自动封禁（auto）与高危规则封禁（rule_ban）一律全局，手动封禁可指定站点。
type Ban struct {
	ID         uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	IP         string `gorm:"column:ip;size:64;not null;index" json:"ip"`
	Reason     string `gorm:"column:reason;size:255" json:"reason"`
	Source     string `gorm:"column:source;size:16;not null" json:"source"` // auto/manual/rule
	Scope      string `gorm:"column:scope;size:16;not null;default:global" json:"scope"`
	Site       string `gorm:"column:site;size:64;not null" json:"site"`
	HitCount   int    `gorm:"column:hit_count;not null;default:1" json:"hitCount"`
	CreatedAt  int64  `gorm:"column:created_at;not null" json:"createdAt"`
	ExpiresAt  int64  `gorm:"column:expires_at;not null" json:"expiresAt"`
	ReleasedAt int64  `gorm:"column:released_at" json:"releasedAt"`
}

// TableName 显式表名。
func (Event) TableName() string { return "events" }

// Event 请求事件。热路径，写入走原生 SQL。
type Event struct {
	ID         uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	TS         int64  `gorm:"column:ts;not null;index" json:"ts"`
	Site       string `gorm:"column:site;size:64" json:"site"`
	ClientIP   string `gorm:"column:client_ip;size:64" json:"clientIp"`
	Method     string `gorm:"column:method;size:8" json:"method"`
	Host       string `gorm:"column:host;size:128" json:"host"`
	Path       string `gorm:"column:path;size:1024" json:"path"`
	Query      string `gorm:"column:query;size:2048" json:"query"`
	Status     int    `gorm:"column:status" json:"status"`
	Action     string `gorm:"column:action;size:16;index" json:"action"`
	Score      int    `gorm:"column:score" json:"score"`
	Categories string `gorm:"column:categories;size:255" json:"-"` // JSON 数组
	RuleIDs    string `gorm:"column:rule_ids;size:512" json:"-"`   // JSON 数组
	UA         string `gorm:"column:ua;size:512" json:"ua"`
	Referer    string `gorm:"column:referer;size:512" json:"referer"`
	ReqID      string `gorm:"column:req_id;size:64" json:"reqId"`
	LatencyUS  int64  `gorm:"column:latency_us" json:"latencyUs"`
	Upstream   string `gorm:"column:upstream;size:255" json:"upstream"`
	Country    string `gorm:"column:country;size:64" json:"country"`
	Bytes      int64  `gorm:"column:bytes" json:"bytes"`
}

// TableName 显式表名。
func (Stats1m) TableName() string { return "stats_1m" }

// Stats1m 分钟级聚合。
type Stats1m struct {
	Minute     int64  `gorm:"column:minute;primaryKey;autoIncrement:false" json:"minute"`
	Site       string `gorm:"column:site;primaryKey;size:64" json:"site"`
	Total      int64  `gorm:"column:total" json:"total"`
	Passed     int64  `gorm:"column:passed" json:"passed"`
	Blocked    int64  `gorm:"column:blocked" json:"blocked"`
	Challenged int64  `gorm:"column:challenged" json:"challenged"`
	Status5xx  int64  `gorm:"column:status_5xx" json:"status5xx"`
	Categories string `gorm:"column:categories;size:512" json:"-"`
	TopIPs     string `gorm:"column:top_ips;size:512" json:"-"`
	TopPaths   string `gorm:"column:top_paths;size:512" json:"-"`
}

// TableName 显式表名。
func (AuditLog) TableName() string { return "audit_logs" }

// AuditLog 配置变更审计。
type AuditLog struct {
	ID       uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	TS       int64  `gorm:"column:ts;not null;index" json:"ts"`
	Operator string `gorm:"column:operator;size:64;not null" json:"operator"`
	ClientIP string `gorm:"column:client_ip;size:64" json:"clientIp"`
	Action   string `gorm:"column:action;size:32;not null" json:"action"`
	Object   string `gorm:"column:object;size:32;not null" json:"object"`
	ObjectID string `gorm:"column:object_id;size:64" json:"objectId"`
	Before   string `gorm:"column:before;size:4096" json:"before"`
	After    string `gorm:"column:after;size:4096" json:"after"`
	Result   string `gorm:"column:result;size:16;not null;default:ok" json:"result"`
}

// TableName 显式表名。
func (Setting) TableName() string { return "settings" }

// Setting 键值配置，带版本号用于热更新检测。
type Setting struct {
	Key       string `gorm:"column:key;primaryKey;size:64" json:"key"`
	Value     string `gorm:"column:value;size:4096" json:"value"`
	Version   int64  `gorm:"column:version;not null;default:1" json:"version"`
	UpdatedAt int64  `gorm:"column:updated_at" json:"updatedAt"`
}

// TableName 显式表名。
func (SchemaMeta) TableName() string { return "schema_meta" }

// SchemaMeta 迁移版本记录。
type SchemaMeta struct {
	Key   string `gorm:"column:key;primaryKey;size:64" json:"key"`
	Value string `gorm:"column:value;size:1024" json:"value"`
}

// TableName 显式表名。
func (ConfigVersion) TableName() string { return "config_versions" }

// ConfigVersion 配置快照，用于回滚。
type ConfigVersion struct {
	Version   int64  `gorm:"column:version;primaryKey;autoIncrement" json:"version"`
	Snapshot  string `gorm:"column:snapshot;not null" json:"snapshot"`
	Operator  string `gorm:"column:operator;size:64" json:"operator"`
	Remark    string `gorm:"column:remark;size:255" json:"remark"`
	CreatedAt int64  `gorm:"column:created_at" json:"createdAt"`
}

// NotifyLog 告警通知发送记录。
//
// 只记「发了什么、发给谁、成没成」，不存完整正文（邮件正文可能含大量 IP，
// 存库收益低）。Channel 预留 email/feishu/sms/wecom，方便后续扩展多渠道。
type NotifyLog struct {
	ID        uint   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Channel   string `gorm:"column:channel;size:16;not null;default:email" json:"channel"`
	Target    string `gorm:"column:target;size:255;not null" json:"target"`
	Subject   string `gorm:"column:subject;size:255;not null" json:"subject"`
	Summary   string `gorm:"column:summary;not null" json:"summary"`
	Count     int    `gorm:"column:count;not null;default:0" json:"count"`
	Status    string `gorm:"column:status;size:16;not null;default:ok" json:"status"`
	Error     string `gorm:"column:error;not null" json:"error"`
	CreatedAt int64  `gorm:"column:created_at;not null;default:0" json:"createdAt"`
}

// TableName 显式表名。
func (NotifyLog) TableName() string { return "notify_logs" }

// SeclangPackage SecLang 规则包：第三方 SecLang/CRS 插件包的元信息。
//
// 包内容（.conf 文本）落盘在 data/seclang/<slug>.conf，由 Coraza 与内嵌
// CRS 一起编译执行；本表只管生命周期（启停/计数/错误）。
type SeclangPackage struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string `gorm:"column:name;size:128;not null" json:"name"`
	Slug      string `gorm:"column:slug;size:64;not null;uniqueIndex" json:"slug"`
	Vendor    string `gorm:"column:vendor;size:64;not null;default:''" json:"vendor"`
	Version   string `gorm:"column:version;size:32;not null;default:'1'" json:"version"`
	RuleCount int    `gorm:"column:rule_count;not null;default:0" json:"ruleCount"`
	Enabled   int    `gorm:"column:enabled;not null;default:1" json:"enabled"`
	LastError string `gorm:"column:last_error;size:255;not null;default:''" json:"lastError"`
	Note      string `gorm:"column:note;size:255;not null;default:''" json:"note"`
	CreatedBy string `gorm:"column:created_by;size:64;not null;default:''" json:"createdBy"`
	CreatedAt int64  `gorm:"column:created_at;not null;default:0" json:"createdAt"`
	UpdatedAt int64  `gorm:"column:updated_at;not null;default:0" json:"updatedAt"`
}

// TableName 显式表名。
func (SeclangPackage) TableName() string { return "seclang_packages" }
