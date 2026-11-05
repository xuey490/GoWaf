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

// Event 一条待通知的封禁事件，字段会渲染进告警邮件正文。
type Event struct {
	IP      string    `json:"ip"`      // 触发封禁的客户端 IP
	Site    string    `json:"site"`    // 命中的站点名
	Country string    `json:"country"` // IP 属地（国家/地区），可为空
	Source  string    `json:"source"`  // 事件来源：auto / rule_ban / intel / manual
	Reason  string    `json:"reason"`  // 封禁原因（人类可读文案）
	Method  string    `json:"method"`  // 触发时的 HTTP 方法
	Path    string    `json:"path"`    // 触发时的请求路径
	Score   int       `json:"score"`   // 风险评分；情报类事件天然为 0
	RuleIDs []string  `json:"ruleIds"` // 命中的规则 ID 列表
	At      time.Time `json:"at"`      // 事件发生时间
}

// Config 通知配置，每次发送前从设置表现读，改配置立即生效。
type Config struct {
	Enabled  bool          // 通知总开关
	Host     string        // SMTP 服务器地址
	Port     int           // SMTP 端口，<=0 时发送侧按 465 处理
	User     string        // SMTP 登录用户名（通常即发件邮箱）
	Pass     string        // SMTP 登录密码/授权码
	From     string        // 发件人地址，留空时回落为 User
	To       []string      // 收件人地址列表
	TLS      string        // 加密方式：none | starttls | ssl
	Subject  string        // 邮件主题前缀，留空用默认
	MinScore int           // 最低评分阈值，低于该值的事件不通知
	Window   time.Duration // 聚合窗口：首批事件入队后等满该时长才合并发送
	Cooldown time.Duration // 同一 IP 的冷却期，期内重复事件不再通知
}

// Notifier 聚合 + 异步发送。Send 只把事件放进缓冲队列，后台协程按聚合窗口
// 定期冲刷，真正发信与写记录都在协程里完成，绝不阻塞数据面。
type Notifier struct {
	mu       sync.Mutex           // 保护下面三个聚合状态
	pending  []Event              // 待发送的事件缓冲（一个聚合窗口的内容）
	firstAt  time.Time            // 本窗口第一条事件的时间，用于判断窗口是否凑满
	lastSent map[string]time.Time // IP -> 最近一次通知时间，实现冷却期

	load  func() Config         // 读配置回调（每次发送前现读，改配置即时生效）
	save  func(model.NotifyLog) // 写发送记录回调（落 notify_logs 表），可为 nil
	stop  chan struct{}         // 关闭即通知后台协程退出
	once  sync.Once             // 保证 stop 只被关闭一次
	wg    sync.WaitGroup        // 等待后台协程退出
	mails int64                 // 累计发送次数，仅用于日志
}

// New 构造通知器。
// 参数 load：读取最新配置的回调（从设置表现读），每次 Send/tick 都会调用。
// 参数 save：写发送记录的回调（落 notify_logs 表），可为 nil。
// 返回 *Notifier：可用通知器，需再调用 Start 启动后台冲刷协程。
func New(load func() Config, save func(model.NotifyLog)) *Notifier {
	return &Notifier{
		lastSent: map[string]time.Time{},
		load:     load,
		save:     save,
		stop:     make(chan struct{}),
	}
}

// Start 启动后台冲刷协程，每 2 秒 tick 一次，到点检查聚合窗口并发信。
// 无参数，无返回值；进程生命周期内应只调用一次，结束前用 Stop 回收。
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

// Stop 停止后台协程并阻塞等待其退出（未发完的事件直接丢弃，避免关机卡住）。
// 无参数，无返回值；内部用 sync.Once 保证重复调用安全。
func (n *Notifier) Stop() {
	n.once.Do(func() { close(n.stop) })
	n.wg.Wait()
}

// Send 投递一条事件到聚合队列。
// 参数 ev：封禁事件内容，At 为零值时自动补当前时间。
// 无返回值；非阻塞，任何情况下都不会向调用方返回错误（发信失败只记日志）。
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
	// 冷却期检查：同一 IP 距上次通知不足 cooldown 则丢弃本次事件
	if cfg.Cooldown > 0 {
		if t, ok := n.lastSent[ev.IP]; ok && ev.At.Sub(t) < cfg.Cooldown {
			n.mu.Unlock()
			return
		}
	}
	// 入队即记冷却起点，后续发送成败不影响冷却判定
	n.lastSent[ev.IP] = ev.At
	if len(n.pending) == 0 {
		n.firstAt = ev.At
	}
	n.pending = append(n.pending, ev)
	n.mu.Unlock()
}

// tick 到点冲刷，由后台协程每 2 秒调用一次。
// 参数 now：当前时间，用于判断聚合窗口是否凑满、清理过期冷却记录。
// 无返回值；窗口未凑满时本次不发，通知关闭时直接清空待发队列。
// 首批事件凑满 window 才发，避免一次扫描刷出几十封邮件。
func (n *Notifier) tick(now time.Time) {
	cfg := n.load()
	if !cfg.Enabled {
		// 已关通知：待发队列没必要保留，直接清空
		n.mu.Lock()
		n.pending, n.firstAt = nil, time.Time{}
		n.mu.Unlock()
		return
	}

	n.mu.Lock()
	if len(n.pending) == 0 {
		// 队列空也要顺手清理冷却表，防止 map 长期膨胀
		n.pruneLocked(now, cfg.Cooldown)
		n.mu.Unlock()
		return
	}
	if cfg.Window > 0 && now.Sub(n.firstAt) < cfg.Window {
		// 聚合窗口未凑满，继续攒事件
		n.mu.Unlock()
		return
	}
	evs := n.pending
	n.pending, n.firstAt = nil, time.Time{}
	n.pruneLocked(now, cfg.Cooldown)
	n.mu.Unlock()

	// 整批事件已在锁内摘走，发送放锁外做，避免持锁期间阻塞 Send
	n.deliver(cfg, evs)
}

// pruneLocked 清理过期的冷却记录，防止 map 无限增长。调用方需持有锁。
// 参数 now：当前时间；参数 cooldown：冷却时长，仅记录超过 3 倍冷却时长且
// 表内条目达到 512 才清理，避免高频遍历。
// 无返回值。
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

// deliver 把一批事件合成摘要邮件并发送，无论成败都落一条 notify_logs 记录。
// 参数 cfg：通知配置；参数 evs：本窗口聚合出的事件列表（空列表直接返回）。
// 无返回值；发信失败只记日志、状态记为 fail，不向调用方报错。
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
// 参数 cfg：待验证的邮件渠道配置。
// 返回 error：发送失败原因（连接/认证/投递任一步出错），成功为 nil。
func SendTest(cfg Config) error {
	subject, body := RenderTest(cfg)
	return SendMail(cfg, subject, body)
}

// summarize 生成发送记录里的摘要：事件按 IP 去重并排序，最多列 5 个，多余用「等」收尾。
// 参数 evs：本窗口的事件列表。
// 返回 string：形如 "1.1.1.1, 2.2.2.2 等" 的摘要文本。
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
