// Package metrics 集中定义 WAF 的 Prometheus 监控指标。
//
// 各指标在包初始化时通过 promauto 注册到默认 Registry，业务代码
// 直接使用导出变量打点即可，无需关心注册与命名细节。
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// 全局指标集合。命名遵循 Prometheus 惯例：waf_名称_单位；
// promauto 在包加载时自动注册到默认 Registry，进程内只此一份。
var (
	// RequestsTotal 按站点/动作统计的请求总数（Counter）。
	// 标签 site：站点名；action：处理动作（pass/log/block/challenge 等）。
	RequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "waf_requests_total",
		Help: "经 WAF 处理的请求总数",
	}, []string{"site", "action"})

	// RequestDuration 请求端到端处理耗时分布（Histogram，单位秒）。
	// 标签 site：站点名。
	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "waf_request_duration_seconds",
		Help:    "请求处理耗时",
		Buckets: []float64{0.001, 0.003, 0.01, 0.03, 0.1, 0.3, 1, 3},
	}, []string{"site"})

	// DetectDuration 仅规则检测阶段的耗时分布（Histogram，单位秒），
	// 不含转发上游等其余环节，无标签。
	DetectDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "waf_detect_duration_seconds",
		Help:    "规则检测耗时",
		Buckets: []float64{0.00005, 0.0001, 0.0003, 0.001, 0.003, 0.01},
	})

	// RuleHits 规则命中次数（Counter）。
	// 标签 rule_id：规则 ID；category：规则分类。
	RuleHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "waf_rule_hits_total",
		Help: "规则命中次数",
	}, []string{"rule_id", "category"})

	// BlockedTotal 拦截总数（Counter）。
	// 标签 site：站点名；reason：拦截原因（如 rate_limit）。
	BlockedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "waf_blocked_total",
		Help: "拦截总数",
	}, []string{"site", "reason"})

	// EventDropped 因事件队列满而被丢弃的日志事件数（Counter），无标签。
	EventDropped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "waf_event_dropped_total",
		Help: "因队列满而丢弃的事件数",
	})

	// BannedGauge 当前封禁中的 IP 数量（Gauge），由封禁管理器直接 Set。
	BannedGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "waf_banned_ips",
		Help: "当前封禁 IP 数",
	})

	// UpstreamErrors 上游 5xx 与不可达次数（Counter）。
	// 标签 site：站点名。
	UpstreamErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "waf_upstream_errors_total",
		Help: "上游 5xx 与不可达次数",
	}, []string{"site"})

	// DetectErrors 检测引擎降级次数（超时 / panic 被隔离）（Counter）。
	// 标签 engine：引擎名（rules / crs / libi / payload）；reason：timeout / panic。
	DetectErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "waf_detect_errors_total",
		Help: "检测引擎降级（超时或 panic 被隔离）次数",
	}, []string{"engine", "reason"})

	// RespFilterHits 响应敏感信息命中次数（Counter）。
	// 标签 rule_id：响应规则 ID（如 resp-phone）；mode：当前过滤模式（observe/mask）。
	RespFilterHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "waf_respfilter_hits_total",
		Help: "响应敏感信息命中次数",
	}, []string{"rule_id", "mode"})

	// DProtectPages 动态防护页面加密次数（Counter），无标签。
	DProtectPages = promauto.NewCounter(prometheus.CounterOpts{
		Name: "waf_dprotect_pages_total",
		Help: "动态防护加密的 HTML 页面数",
	})

	// TamperHits 防篡改比对结果（Counter）。
	// 标签 site：站点名；outcome：match 一致 / mismatch 不一致（observe） /
	// replaced 不一致且已用基线副本替换（block）。
	TamperHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "waf_tamper_hits_total",
		Help: "防篡改完整性比对结果",
	}, []string{"site", "outcome"})

	// PolicyCCBlocks 自定义 CC 规则触发限流次数（Counter），无标签。
	PolicyCCBlocks = promauto.NewCounter(prometheus.CounterOpts{
		Name: "waf_policy_cc_blocks_total",
		Help: "自定义 CC 规则（rate_limit 动作）触发限流的次数",
	})
)
