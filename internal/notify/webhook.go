// Package notify 告警通知：把「触发了封禁」的事件推给管理员（邮件 + webhook）。
//
// 本文件实现 Webhook 渠道：飞书 / 钉钉 / 企业微信群机器人 + 通用 JSON POST。
//
// 与邮件渠道共用同一套聚合窗口、冷却与最低分过滤（Notifier 主流程），
// 这里只负责"一个窗口的事件 → 各机器人消息格式 → HTTP POST"。
//
// 渠道语义：
//   - 飞书：自定义机器人，可选签名校验（timestamp + "\n" + secret 作 HMAC-SHA256 key）；
//   - 钉钉：自定义机器人，可选加签（timestamp + "\n" + secret 签名后拼进 URL）；
//   - 企业微信：群机器人，无签名机制，用 markdown 消息；
//   - 通用：POST 结构化 JSON {title, text, count, events}，方便接自建告警网关。
//
// 硬约束与邮件一致：任何失败只记日志与 notify_logs，绝不影响数据面。
package notify

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"gowaf/internal/store/model"
)

// WebhookConfig webhook 渠道配置，随 Config 现读现用。
type WebhookConfig struct {
	Enabled      bool   // 总开关
	FeishuURL    string // 飞书群机器人 webhook 地址
	FeishuSecret string // 飞书签名密钥（可选，未填不签名）
	DingURL      string // 钉钉群机器人 webhook 地址
	DingSecret   string // 钉钉签名密钥（可选，未填不签名）
	WecomURL     string // 企业微信群机器人 webhook 地址
	GenericURL   string // 通用 JSON POST 地址
}

// webhookTimeout 单个 webhook 的 HTTP 超时。群机器人一般毫秒级应答，
// 5 秒已足够宽松；聚合发送在后台协程，超时不影响主流程。
const webhookTimeout = 5 * time.Second

// webhookResult 单个目标的发送结果（测试接口逐条回显）。
type webhookResult struct {
	Target string `json:"target"` // 渠道名或 URL 摘要
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

// postJSON 向目标 URL 发送 JSON 消息体。
// 参数 rawURL：目标地址；参数 body：待序列化对象；参数 sign：URL 附加查询（钉钉加签用，可为空）。
// 返回 error：HTTP 非 2xx 或网络错误。
func postJSON(rawURL string, body any, sign url.Values) error {
	if sign != nil {
		if strings.Contains(rawURL, "?") {
			rawURL += "&" + sign.Encode()
		} else {
			rawURL += "?" + sign.Encode()
		}
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: webhookTimeout}
	resp, err := client.Post(rawURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// feishuSign 计算飞书自定义机器人的签名。
// 参数 secret：签名密钥；参数 ts：秒级时间戳。
// 返回 string：base64 编码的 HMAC-SHA256 摘要（key 为 "ts\nsecret"，内容为空串）。
func feishuSign(secret string, ts int64) string {
	mac := hmac.New(sha256.New, []byte(strconv.FormatInt(ts, 10)+"\n"+secret))
	mac.Write(nil)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// dingSign 计算钉钉加签参数。
// 参数 secret：签名密钥；参数 ts：毫秒时间戳。
// 返回 url.Values：timestamp 与 sign 两个查询参数。
func dingSign(secret string, ts int64) url.Values {
	stringToSign := strconv.FormatInt(ts, 10) + "\n" + secret
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stringToSign))
	out := url.Values{}
	out.Set("timestamp", strconv.FormatInt(ts, 10))
	out.Set("sign", url.QueryEscape(base64.StdEncoding.EncodeToString(mac.Sum(nil))))
	return out
}

// webhookText 把一批事件渲染成纯文本摘要（各机器人消息正文共用）。
// 参数 evs：本窗口的事件列表。
// 返回 string：按来源/IP 排序的纯文本摘要。
func webhookText(evs []Event) string {
	var b strings.Builder
	fmt.Fprintf(&b, "【GoWAF 封禁告警】%d 条事件（%s）\n", len(evs), evs[0].At.Format("2006-01-02 15:04:05"))
	// 按 IP 排序输出，重复 IP 只保留首次出现的原因，避免同 IP 刷屏。
	seen := map[string]bool{}
	sorted := append([]Event(nil), evs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].IP < sorted[j].IP })
	for _, e := range sorted {
		if seen[e.IP] {
			continue
		}
		seen[e.IP] = true
		fmt.Fprintf(&b, "· %s（%s）%s %s%s [%s] %s\n",
			e.IP, orDash(e.Country), orDash(e.Site), e.Method, e.Path, e.Source, e.Reason)
	}
	return strings.TrimRight(b.String(), "\n")
}

// deliverWebhooks 把一个聚合窗口的事件发往全部已配置的 webhook 目标。
// 参数 cfg：通知配置（取其 Webhook 字段）；参数 evs：事件列表。
// 返回 []webhookResult：逐目标的发送结果；未启用或无目标时返回 nil。
// 每个目标独立落一条 notify_logs（channel 形如 webhook:feishu）。
func (n *Notifier) deliverWebhooks(cfg Config, evs []Event) []webhookResult {
	wc := cfg.Webhook
	if !wc.Enabled {
		return nil
	}
	text := webhookText(evs)
	subject := fmt.Sprintf("GoWAF 告警：%d 条封禁事件", len(evs))

	type target struct {
		name string // 渠道名（notify_logs.channel 与测试回显用）
		url  string
		send func() error
	}
	var targets []target
	if wc.FeishuURL != "" {
		feishuURL, secret := wc.FeishuURL, wc.FeishuSecret
		targets = append(targets, target{name: "webhook:feishu", url: feishuURL, send: func() error {
			body := map[string]any{"msg_type": "text", "content": map[string]string{"text": text}}
			if secret != "" {
				ts := time.Now().Unix()
				body["timestamp"] = strconv.FormatInt(ts, 10)
				body["sign"] = feishuSign(secret, ts)
			}
			return postJSON(feishuURL, body, nil)
		}})
	}
	if wc.DingURL != "" {
		dingURL, secret := wc.DingURL, wc.DingSecret
		targets = append(targets, target{name: "webhook:dingtalk", url: dingURL, send: func() error {
			var sign url.Values
			if secret != "" {
				sign = dingSign(secret, time.Now().UnixMilli())
			}
			return postJSON(dingURL, map[string]any{"msgtype": "text", "text": map[string]string{"content": text}}, sign)
		}})
	}
	if wc.WecomURL != "" {
		wecomURL := wc.WecomURL
		targets = append(targets, target{name: "webhook:wecom", url: wecomURL, send: func() error {
			return postJSON(wecomURL, map[string]any{"msgtype": "markdown", "markdown": map[string]string{"content": text}}, nil)
		}})
	}
	if wc.GenericURL != "" {
		genericURL := wc.GenericURL
		targets = append(targets, target{name: "webhook:generic", url: genericURL, send: func() error {
			return postJSON(genericURL, map[string]any{
				"title": subject, "text": text, "count": len(evs),
				"events": evs, "source": "gowaf",
			}, nil)
		}})
	}
	if len(targets) == 0 {
		return nil
	}

	results := make([]webhookResult, 0, len(targets))
	for _, t := range targets {
		err := t.send()
		res := webhookResult{Target: t.name, OK: err == nil}
		if err != nil {
			res.Error = err.Error()
			log.Printf("[notify] webhook %s 发送失败: %v", t.name, err)
		}
		results = append(results, res)
		if n.save != nil {
			n.save(model.NotifyLog{
				Channel: t.name, Target: truncateURL(t.url), Subject: subject,
				Summary: summarize(evs), Count: len(evs),
				Status: map[bool]string{true: "ok", false: "fail"}[err == nil],
				Error:  errString(err), CreatedAt: time.Now().UnixMilli(),
			})
		}
	}
	return results
}

// SendTestWebhook 用给定配置向全部目标各发一条测试消息。
// 参数 cfg：通知配置。
// 返回 []webhookResult：逐目标结果；渠道未启用或未配置任何地址时返回 nil。
func SendTestWebhook(cfg Config) []webhookResult {
	n := &Notifier{lastSent: map[string]time.Time{}, load: func() Config { return cfg }}
	return n.deliverWebhooks(cfg, []Event{{
		IP: "127.0.0.1", Site: "test", Source: "test",
		Reason: "这是一条测试告警（webhook 渠道验证）", At: time.Now(),
	}})
}

// truncateURL 发送记录里的目标地址截断（URL 可能带签名参数，不宜全存）。
// 参数 u：完整 URL。
// 返回 string：截到 120 字节的前缀。
func truncateURL(u string) string {
	if len(u) <= 120 {
		return u
	}
	return u[:120]
}

// errString 错误转字符串，nil 返回空串。
// 参数 err：错误。
// 返回 string：错误描述。
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
