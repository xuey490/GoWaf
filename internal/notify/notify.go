// Package notify 告警通知：把「触发了封禁」的事件推给管理员。
//
// 现在只实现邮件一个渠道，但结构按多渠道设计（Config 里有 Channel 语义、
// NotifyLog 有 channel 字段），以后加飞书机器人 / 短信 / 企业微信，
// 只需要新增一个 sender 并在这里注册，不必改调用方。
//
// 三条硬约束：
//  1. 不拖慢数据面：Send 只入队，真正发信在后台协程里做。
//  2. 不刷屏：同一 IP 在 cooldown 内只通知一次，一个窗口内的事件合成一封摘要。
//  3. 不影响业务：发信失败只记日志和 notify_logs，绝不向请求链路返回错误。
package notify

import (
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"gowaf/internal/store/model"
)

// Event 一条待通知的封禁事件。
type Event struct {
	IP      string    `json:"ip"`
	Site    string    `json:"site"`
	Country string    `json:"country"`
	Source  string    `json:"source"` // auto / rule_ban / intel / manual
	Reason  string    `json:"reason"`
	Method  string    `json:"method"`
	Path    string    `json:"path"`
	Score   int       `json:"score"`
	RuleIDs []string  `json:"ruleIds"`
	At      time.Time `json:"at"`
}

// Config 通知配置，每次发送前从设置表现读，改配置立即生效。
type Config struct {
	Enabled  bool
	Host     string
	Port     int
	User     string
	Pass     string
	From     string
	To       []string
	TLS      string // none | starttls | ssl
	Subject  string // 主题前缀，留空用默认
	MinScore int
	Window   time.Duration
	Cooldown time.Duration
}

// Notifier 聚合 + 异步发送。
type Notifier struct {
	mu       sync.Mutex
	pending  []Event
	firstAt  time.Time
	lastSent map[string]time.Time

	load  func() Config
	save  func(model.NotifyLog)
	stop  chan struct{}
	once  sync.Once
	wg    sync.WaitGroup
	mails int64 // 累计发送次数，仅用于日志
}

// New 构造通知器。load 负责从设置表读配置，save 负责写发送记录。
func New(load func() Config, save func(model.NotifyLog)) *Notifier {
	return &Notifier{
		lastSent: map[string]time.Time{},
		load:     load,
		save:     save,
		stop:     make(chan struct{}),
	}
}

// Start 启动后台冲刷协程。
func (n *Notifier) Start() {
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-n.stop:
				return
			case now := <-t.C:
				n.tick(now)
			}
		}
	}()
}

// Stop 停止后台协程（未发完的事件直接丢弃，避免关机卡住）。
func (n *Notifier) Stop() {
	n.once.Do(func() { close(n.stop) })
	n.wg.Wait()
}

// Send 投递一条事件。非阻塞，任何情况下都不会返回错误。
func (n *Notifier) Send(ev Event) {
	cfg := n.load()
	if !cfg.Enabled {
		return
	}
	if ev.At.IsZero() {
		ev.At = time.Now()
	}
	// 评分阈值只对「规则类」事件有意义：情报封禁的 score 天然是 0，
	// 不能因为阈值把情报告警整批吞掉，所以 score=0 时不做过滤。
	if cfg.MinScore > 0 && ev.Score > 0 && ev.Score < cfg.MinScore {
		return
	}

	n.mu.Lock()
	if cfg.Cooldown > 0 {
		if t, ok := n.lastSent[ev.IP]; ok && ev.At.Sub(t) < cfg.Cooldown {
			n.mu.Unlock()
			return
		}
	}
	n.lastSent[ev.IP] = ev.At
	if len(n.pending) == 0 {
		n.firstAt = ev.At
	}
	n.pending = append(n.pending, ev)
	n.mu.Unlock()
}

// tick 到点冲刷。窗口内的第一批事件凑够 window 才发，避免一次扫描刷出几十封邮件。
func (n *Notifier) tick(now time.Time) {
	cfg := n.load()
	if !cfg.Enabled {
		n.mu.Lock()
		n.pending, n.firstAt = nil, time.Time{}
		n.mu.Unlock()
		return
	}

	n.mu.Lock()
	if len(n.pending) == 0 {
		n.pruneLocked(now, cfg.Cooldown)
		n.mu.Unlock()
		return
	}
	if cfg.Window > 0 && now.Sub(n.firstAt) < cfg.Window {
		n.mu.Unlock()
		return
	}
	evs := n.pending
	n.pending, n.firstAt = nil, time.Time{}
	n.pruneLocked(now, cfg.Cooldown)
	n.mu.Unlock()

	n.deliver(cfg, evs)
}

// pruneLocked 清理过期的冷却记录，防止 map 无限增长。调用方需持有锁。
func (n *Notifier) pruneLocked(now time.Time, cooldown time.Duration) {
	if cooldown <= 0 || len(n.lastSent) < 512 {
		return
	}
	keep := cooldown * 3
	for ip, t := range n.lastSent {
		if now.Sub(t) > keep {
			delete(n.lastSent, ip)
		}
	}
}

func (n *Notifier) deliver(cfg Config, evs []Event) {
	if len(evs) == 0 {
		return
	}
	subject, body := RenderDigest(cfg, evs)
	err := SendMail(cfg, subject, body)

	lg := model.NotifyLog{
		Channel:   "email",
		Target:    strings.Join(cfg.To, ","),
		Subject:   subject,
		Summary:   summarize(evs),
		Count:     len(evs),
		Status:    "ok",
		CreatedAt: time.Now().UnixMilli(),
	}
	if err != nil {
		lg.Status = "fail"
		lg.Error = err.Error()
		log.Printf("[notify] 告警邮件发送失败: %v", err)
	} else {
		n.mu.Lock()
		n.mails++
		n.mu.Unlock()
		log.Printf("[notify] 告警邮件已发送: %d 条封禁事件 -> %s", len(evs), strings.Join(cfg.To, ","))
	}
	if n.save != nil {
		n.save(lg)
	}
}

// SendTest 用给定配置发一封测试邮件，供控制台「测试发送」按钮使用。
func SendTest(cfg Config) error {
	subject, body := RenderTest(cfg)
	return SendMail(cfg, subject, body)
}

func summarize(evs []Event) string {
	ips := make([]string, 0, 3)
	seen := map[string]bool{}
	for _, e := range evs {
		if !seen[e.IP] {
			seen[e.IP] = true
			ips = append(ips, e.IP)
		}
	}
	sort.Strings(ips)
	head := ips
	if len(head) > 5 {
		head = head[:5]
	}
	s := strings.Join(head, ", ")
	if len(ips) > len(head) {
		s += " 等"
	}
	return s
}
