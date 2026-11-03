// Package metrics 暴露 Prometheus 指标。
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// 全局指标。命名遵循 Prometheus 惯例：子系统_名称_单位。
var (
	// RequestsTotal 按站点/动作统计请求数。
	RequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "waf_requests_total",
		Help: "经 WAF 处理的请求总数",
	}, []string{"site", "action"})

	// RequestDuration 端到端耗时（秒）。
	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "waf_request_duration_seconds",
		Help:    "请求处理耗时",
		Buckets: []float64{0.001, 0.003, 0.01, 0.03, 0.1, 0.3, 1, 3},
	}, []string{"site"})

	// DetectDuration 仅检测阶段耗时。
	DetectDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "waf_detect_duration_seconds",
		Help:    "规则检测耗时",
		Buckets: []float64{0.00005, 0.0001, 0.0003, 0.001, 0.003, 0.01},
	})

	// RuleHits 规则命中次数。
	RuleHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "waf_rule_hits_total",
		Help: "规则命中次数",
	}, []string{"rule_id", "category"})

	// BlockedTotal 拦截总数。
	BlockedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "waf_blocked_total",
		Help: "拦截总数",
	}, []string{"site", "reason"})

	// EventDropped 日志丢弃数。
	EventDropped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "waf_event_dropped_total",
		Help: "因队列满而丢弃的事件数",
	})

	// BannedGauge 当前封禁数。
	BannedGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "waf_banned_ips",
		Help: "当前封禁 IP 数",
	})

	// UpstreamErrors 上游错误数。
	UpstreamErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "waf_upstream_errors_total",
		Help: "上游 5xx 与不可达次数",
	}, []string{"site"})
)
