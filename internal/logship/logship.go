// Package logship 实现 WAF 事件的外发（对接 SIEM）：Syslog RFC5424 与 Kafka。
//
// 外发走独立队列与 worker 协程：热路径只做一次非阻塞入队（满了丢弃并计数，
// 与事件落库的 EventWriter 同一 fail-open 纪律），Syslog 发送失败不重试
// （SIEM 对单条丢失的容忍度远高于对 WAF 阻塞的容忍度），Kafka 交给
// kafka-go Writer 内部的批量与重试。
//
// RFC5424 帧格式：
//
//	<PRI>1 <ISO8601 时间> <hostname> gowaf - - [gowaf@48577 key="val" ...] <消息>
//
// 结构化数据 SD-ID 固定 gowaf@48577（IANA 私有企业号占位，可按注册号调整），
// 事件全部字段以 PARAM-NAME="VALUE" 形式承载，正文放「站点 动作 路径」摘要。
package logship

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"

	"gowaf/internal/store/model"
)

// SDID 结构化数据 ID（企业号 48577 为占位，如已有 IANA PEN 可替换）。
const SDID = "gowaf@48577"

// DefaultQueueSize 外发队列容量：与事件落库队列同量级，满即丢弃。
const DefaultQueueSize = 4096

// Config 外发配置快照：Reload 时由运行时编译，热生效。
type Config struct {
	Enabled   bool         // 总开关
	MinAction string       // 最低外发的动作级别：all 全部 / log 记录以上 / block 拦截以上
	Syslog    SyslogConfig // Syslog 渠道
	Kafka     KafkaConfig  // Kafka 渠道
}

// SyslogConfig Syslog 渠道配置。
type SyslogConfig struct {
	Enabled  bool   // 渠道开关
	Addr     string // 目标地址 host:port
	Proto    string // udp / tcp / tls
	Facility int    // facility（0~23），PRI = facility*8 + severity
	Hostname string // 帧内 hostname，空则用 "gowaf"
}

// KafkaConfig Kafka 渠道配置。
type KafkaConfig struct {
	Enabled bool     // 渠道开关
	Brokers []string // broker 列表
	Topic   string   // 目标 topic
}

// severityOf 按动作映射 syslog severity。
// block/ban → 4(warning)，challenge → 5(notice)，log → 6(informational)，pass → 6。
func severityOf(action string) int {
	switch action {
	case "block", "ban":
		return 4
	case "challenge":
		return 5
	default:
		return 6
	}
}

// Shipper 事件外发器：持配置快照、有界队列与发送 worker。
// 配置替换（Reload）时整体换成新实例；旧实例在后台自然耗尽退出。
type Shipper struct {
	cfg    Config           // 本次实例使用的配置快照
	ch     chan model.Event // 有界外发队列
	dropd  atomic.Int64     // 累计丢弃条数
	sent   atomic.Int64     // 累计成功外发条数
	done   chan struct{}    // worker 退出信号
	sysTCP net.Conn         // TCP/TLS 长连接（懒建，失败重建）
	kw     *kafka.Writer    // Kafka writer（启用时非 nil）
	closed atomic.Bool      // 是否已进入关闭流程
}

// New 创建外发器并启动 worker。
// 参数 cfg：配置快照；queueSize：队列容量（<=0 取默认）。
// 返回 *Shipper：就绪的外发器。
func New(cfg Config, queueSize int) *Shipper {
	if queueSize <= 0 {
		queueSize = DefaultQueueSize
	}
	s := &Shipper{
		cfg:  cfg,
		ch:   make(chan model.Event, queueSize),
		done: make(chan struct{}),
	}
	s.openChannels()
	go s.loop()
	return s
}

// openChannels 按配置建立底层通道（TCP 连接懒建；Kafka writer 即建）。
func (s *Shipper) openChannels() {
	if s.cfg.Kafka.Enabled && len(s.cfg.Kafka.Brokers) > 0 && s.cfg.Kafka.Topic != "" {
		s.kw = &kafka.Writer{
			Addr:         kafka.TCP(s.cfg.Kafka.Brokers...),
			Topic:        s.cfg.Kafka.Topic,
			BatchSize:    100,
			BatchTimeout: 500 * time.Millisecond,
			RequiredAcks: kafka.RequireOne,
		}
	}
}

// Send 非阻塞投递一条事件到外发队列。
// 参数 e：待外发事件。队列满或未启用时静默丢弃（fail-open）。
func (s *Shipper) Send(e model.Event) {
	if !s.cfg.Enabled || s.closed.Load() {
		return
	}
	if !passesMinAction(s.cfg.MinAction, e.Action) {
		return
	}
	select {
	case s.ch <- e:
	default:
		s.dropd.Add(1)
	}
}

// passesMinAction 判断事件动作是否达到外发级别。
// all → 全部；log → 除 pass 外；block → 仅 block/ban。
func passesMinAction(minAction, action string) bool {
	switch strings.ToLower(strings.TrimSpace(minAction)) {
	case "block":
		return action == "block" || action == "ban"
	case "log":
		return action != "pass"
	default: // all
		return true
	}
}

// Dropped 返回累计丢弃条数。
func (s *Shipper) Dropped() int64 { return s.dropd.Load() }

// Sent 返回累计成功外发条数。
func (s *Shipper) Sent() int64 { return s.sent.Load() }

// Close 关闭队列并等待 worker 耗尽剩余事件（最多等 3 秒）。
func (s *Shipper) Close() {
	s.closed.Store(true)
	close(s.ch)
	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
	}
	if s.sysTCP != nil {
		_ = s.sysTCP.Close()
	}
	if s.kw != nil {
		_ = s.kw.Close()
	}
}

// loop worker 主循环：逐条发送，队列关闭后清空退出。
func (s *Shipper) loop() {
	defer close(s.done)
	for e := range s.ch {
		s.deliver(e)
	}
}

// deliver 把事件发到全部启用的渠道。
func (s *Shipper) deliver(e model.Event) {
	if s.cfg.Syslog.Enabled {
		s.sendSyslog(e)
	}
	if s.kw != nil {
		s.sendKafka(e)
	}
}

// ===== Syslog RFC5424 =====

// rfc5424 把事件组装成 RFC5424 帧。
// 参数 e：事件；参数 now：帧时间戳来源；参数 facility：syslog facility；
// 参数 hostname：帧内主机名。
// 返回 string：完整帧文本（含尾换行）。
func rfc5424(e model.Event, now time.Time, facility int, hostname string) string {
	pri := facility*8 + severityOf(e.Action)
	if hostname == "" {
		hostname = "gowaf"
	}
	ts := now.Format("2006-01-02T15:04:05.000Z07:00")
	sd := fmt.Sprintf(`[%s site="%s" client_ip="%s" action="%s" score="%d" method="%s" host="%s" path="%s" status="%d" rule_ids=%s categories=%s req_id="%s" ua="%s" country="%s"]`,
		SDID, esc(e.Site), esc(e.ClientIP), esc(e.Action), e.Score,
		esc(e.Method), esc(e.Host), esc(e.Path), e.Status,
		esc(e.RuleIDs), esc(e.Categories), esc(e.ReqID), esc(e.UA), esc(e.Country))
	msg := e.Site + " " + e.Action + " " + e.Method + " " + e.Path
	return fmt.Sprintf("<%d>1 %s %s gowaf - - %s %s\n", pri, ts, hostname, sd, msg)
}

// esc 转义 RFC5424 PARAM-VALUE 中的特殊字符（" \ ]）。
func esc(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	v = strings.ReplaceAll(v, `]`, `\]`)
	return v
}

// sendSyslog 发送一条 syslog；UDP 无连接直接发，TCP/TLS 懒建长连接、失败重建。
func (s *Shipper) sendSyslog(e model.Event) {
	frame := rfc5424(e, time.Now(), s.cfg.Syslog.Facility, s.cfg.Syslog.Hostname)
	switch strings.ToLower(s.cfg.Syslog.Proto) {
	case "udp":
		conn, err := net.DialTimeout("udp", s.cfg.Syslog.Addr, 3*time.Second)
		if err != nil {
			s.dropd.Add(1)
			return
		}
		_, _ = conn.Write([]byte(frame))
		_ = conn.Close()
		s.sent.Add(1)
	case "tcp", "tls":
		conn := s.sysTCP
		if conn == nil {
			c, err := s.dialSyslog()
			if err != nil {
				s.dropd.Add(1)
				return
			}
			conn, s.sysTCP = c, c
		}
		_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
		if _, err := conn.Write([]byte(frame)); err != nil {
			_ = conn.Close()
			s.sysTCP = nil
			s.dropd.Add(1)
			return
		}
		s.sent.Add(1)
	}
}

// dialSyslog 建立 TCP/TLS 连接。
func (s *Shipper) dialSyslog() (net.Conn, error) {
	addr := s.cfg.Syslog.Addr
	d := net.Dialer{Timeout: 3 * time.Second}
	if strings.EqualFold(s.cfg.Syslog.Proto, "tls") {
		return tls.DialWithDialer(&d, "tcp", addr, &tls.Config{ServerName: hostOf(addr)})
	}
	return d.Dial("tcp", addr)
}

// hostOf 从 host:port 拆出 host（TLS ServerName 用）。
func hostOf(addr string) string {
	if i := strings.LastIndexByte(addr, ':'); i > 0 {
		return addr[:i]
	}
	return addr
}

// ===== Kafka =====

// sendKafka 把事件作为 JSON 消息写入 Kafka。
// key 用客户端 IP（同源事件落同一分区，便于 SIEM 按会话聚合）。
func (s *Shipper) sendKafka(e model.Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	payload, err := json.Marshal(e)
	if err != nil {
		s.dropd.Add(1)
		return
	}
	err = s.kw.WriteMessages(ctx, kafka.Message{
		Key:   []byte(e.ClientIP),
		Value: payload,
		Time:  time.UnixMilli(e.TS),
	})
	if err != nil {
		log.Printf("[logship] kafka 发送失败: %v", err)
		s.dropd.Add(1)
		return
	}
	s.sent.Add(1)
}
