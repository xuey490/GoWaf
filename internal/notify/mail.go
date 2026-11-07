package notify

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"html"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// SendMail 用给定配置发一封 HTML 邮件。
//
// 支持三种加密方式，覆盖国内常见邮箱：
//   - ssl       465 端口，连上就是 TLS（QQ/163/企业邮箱默认）
//   - starttls  587 端口，明文连上再升级
//   - none      25 端口，内网自建 MTA
//
// 参数 cfg：邮件渠道配置（服务器、端口、加密方式、账号、收件人）。
// 参数 subject：邮件主题，写入报文前会做 RFC 2047 编码。
// 参数 htmlBody：HTML 正文，写入报文前会做 quoted-printable 编码。
// 返回 error：连接、握手、认证、投递任一步失败的原因，成功为 nil。
func SendMail(cfg Config, subject, htmlBody string) error {
	if strings.TrimSpace(cfg.Host) == "" {
		return errors.New("未配置 SMTP 服务器地址")
	}
	if len(cfg.To) == 0 {
		return errors.New("未配置收件人")
	}
	port := cfg.Port
	if port <= 0 {
		port = 465 // 未配置端口时按 SSL 惯例端口处理
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(port))
	// ServerName 用于证书域名校验；最低 TLS 1.2，拒绝老旧不安全版本
	tlsCfg := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}

	var (
		conn net.Conn
		err  error
		c    *smtp.Client
	)
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	switch strings.ToLower(strings.TrimSpace(cfg.TLS)) {
	case "ssl", "tls", "":
		// 隐式 TLS：先完成 TLS 握手再进 SMTP 会话（465 端口），空值也按 SSL 处理
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("连接 SMTP(SSL) 失败: %w", err)
		}
		c, err = smtp.NewClient(conn, cfg.Host)
	default:
		// 明文连入（587/25 端口），starttls 时协商升级
		conn, err = dialer.Dial("tcp", addr)
		if err != nil {
			return fmt.Errorf("连接 SMTP 失败: %w", err)
		}
		c, err = smtp.NewClient(conn, cfg.Host)
		if err == nil && strings.EqualFold(cfg.TLS, "starttls") {
			// 服务器未宣告 STARTTLS 能力时保持明文继续，不视为错误
			if ok, _ := c.Extension("STARTTLS"); ok {
				err = c.StartTLS(tlsCfg)
			}
		}
	}
	if err != nil {
		return fmt.Errorf("SMTP 握手失败: %w", err)
	}
	defer c.Close()

	if cfg.User != "" {
		// 配置了账号且服务器宣告支持 AUTH 才认证，兼容免认证的内网 MTA
		if ok, _ := c.Extension("AUTH"); ok {
			if err := c.Auth(smtp.PlainAuth("", cfg.User, cfg.Pass, cfg.Host)); err != nil {
				return fmt.Errorf("SMTP 认证失败（用户名/授权码是否正确？）: %w", err)
			}
		}
	}

	// 发件人未配置时用登录账号代替
	from := strings.TrimSpace(cfg.From)
	if from == "" {
		from = cfg.User
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("设置发件人失败: %w", err)
	}
	for _, to := range cfg.To {
		if err := c.Rcpt(to); err != nil {
			return fmt.Errorf("设置收件人 %s 失败: %w", to, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("开始投递失败: %w", err)
	}

	msg, err := buildMessage(cfg, from, subject, htmlBody)
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("写入邮件内容失败: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("结束投递失败: %w", err)
	}
	return c.Quit()
}

// buildMessage 组装完整 MIME 报文（头部 + quoted-printable 正文）。
//
// 正文必须走 quoted-printable：HTML 压缩后常常是超长单行，用 8bit 直发会撞上
// RFC 5321 的 998 octet 行上限（QQ 邮箱直接回 500 Line too long），
// QP 编码器会自动按 76 字符软换行，顺带解决非 ASCII 正文的 8bit 兼容性。
//
// 参数 cfg：取收件人列表；参数 from：发件人地址；参数 subject：主题；参数 htmlBody：HTML 正文。
// 返回 []byte：完整报文（头部与正文均为 CRLF 行尾）；返回 error：编码失败原因。
func buildMessage(cfg Config, from, subject, htmlBody string) ([]byte, error) {
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", foldAddressList(cfg.To))
	// 中文主题必须走 RFC 2047 编码，否则很多邮箱会显示成乱码
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", subject))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString("\r\n")

	qp := quotedprintable.NewWriter(&b)
	if _, err := qp.Write([]byte(htmlBody)); err != nil {
		return nil, fmt.Errorf("编码邮件内容失败: %w", err)
	}
	if err := qp.Close(); err != nil {
		return nil, fmt.Errorf("编码邮件内容失败: %w", err)
	}
	return b.Bytes(), nil
}

// foldAddressList 把收件人列表折行，避免收件人很多时头部过宽。
// 折行点只在逗号之后（RFC 5322 允许在逗号处折行），单行控制在 72 字符内。
// 参数 addrs：收件人地址列表。
// 返回 string：可能含 CRLF 续行的 To 头部值，还原折行后与原列表等价。
func foldAddressList(addrs []string) string {
	var sb strings.Builder
	col := 0
	for i, a := range addrs {
		if i > 0 {
			// 预留 ", " 两个字符，放不下就先折行
			if col+2+len(a) > 72 {
				sb.WriteString(",\r\n ")
				col = 1 + len(a)
				sb.WriteString(a)
				continue
			}
			sb.WriteString(", ")
			col += 2
		}
		sb.WriteString(a)
		col += len(a)
	}
	return sb.String()
}

// RenderDigest 生成摘要邮件。多个封禁事件合成一封，按 IP 去重后列出。
// 参数 cfg：通知配置（取主题前缀）；参数 evs：待渲染的事件列表，正文最多列 50 条。
// 返回 (string, string)：邮件主题与 HTML 正文。
func RenderDigest(cfg Config, evs []Event) (string, string) {
	prefix := strings.TrimSpace(cfg.Subject)
	if prefix == "" {
		prefix = "【GoWAF 告警】"
	}
	ips := map[string]bool{}
	for _, e := range evs {
		ips[e.IP] = true
	}
	subject := fmt.Sprintf("%s%d 个 IP 触发封禁", prefix, len(ips))
	if len(evs) == 1 {
		// 单事件时主题直接写明 IP 与原因，一眼可读
		subject = fmt.Sprintf("%s%s 触发封禁（%s）", prefix, evs[0].IP, evs[0].Reason)
	}

	var b strings.Builder
	b.WriteString(emailHead("GoWAF 封禁告警"))
	b.WriteString(`<h2 style="margin:0 0 6px;font-size:18px;color:#111827;">检测到封禁事件</h2>`)
	fmt.Fprintf(&b, `<p style="margin:0 0 16px;color:#6b7280;font-size:13px;">共 <b>%d</b> 条事件，涉及 <b>%d</b> 个 IP；生成时间 %s</p>`,
		len(evs), len(ips), time.Now().Format("2006-01-02 15:04:05"))

	b.WriteString(`<table style="width:100%;border-collapse:collapse;font-size:13px;">`)
	b.WriteString(`<thead><tr style="background:#f3f4f6;color:#374151;text-align:left;">`)
	for _, h := range []string{"时间", "IP", "属地", "站点", "来源", "原因", "评分"} {
		fmt.Fprintf(&b, `<th style="padding:8px 10px;border-bottom:1px solid #e5e7eb;font-weight:600;">%s</th>`, h)
	}
	b.WriteString(`</tr></thead><tbody>`)
	limit := len(evs)
	if limit > 50 {
		limit = 50 // 正文最多渲染 50 条，防止事件堆积时撑爆邮件
	}
	for _, e := range evs[:limit] {
		fmt.Fprintf(&b, `<tr>`)
		fmt.Fprintf(&b, `<td style="padding:7px 10px;border-bottom:1px solid #f3f4f6;color:#6b7280;white-space:nowrap;">%s</td>`, e.At.Format("01-02 15:04:05"))
		fmt.Fprintf(&b, `<td style="padding:7px 10px;border-bottom:1px solid #f3f4f6;font-family:monospace;">%s</td>`, esc(e.IP))
		fmt.Fprintf(&b, `<td style="padding:7px 10px;border-bottom:1px solid #f3f4f6;">%s</td>`, esc(orDash(e.Country)))
		fmt.Fprintf(&b, `<td style="padding:7px 10px;border-bottom:1px solid #f3f4f6;">%s</td>`, esc(orDash(e.Site)))
		fmt.Fprintf(&b, `<td style="padding:7px 10px;border-bottom:1px solid #f3f4f6;">%s</td>`, esc(sourceLabel(e.Source)))
		fmt.Fprintf(&b, `<td style="padding:7px 10px;border-bottom:1px solid #f3f4f6;">%s`, esc(e.Reason))
		if len(e.RuleIDs) > 0 && len(e.RuleIDs) <= 5 {
			fmt.Fprintf(&b, `<br><span style="color:#9ca3af;font-family:monospace;font-size:11px;">%s</span>`, esc(strings.Join(e.RuleIDs, ", ")))
		}
		fmt.Fprintf(&b, `</td>`)
		fmt.Fprintf(&b, `<td style="padding:7px 10px;border-bottom:1px solid #f3f4f6;">%d</td>`, e.Score)
		b.WriteString(`</tr>`)
	}
	b.WriteString(`</tbody></table>`)
	if len(evs) > limit {
		// 超出上限的事件不渲染，只在末尾提示剩余条数
		fmt.Fprintf(&b, `<p style="color:#9ca3af;font-size:12px;">另有 %d 条未列出。</p>`, len(evs)-limit)
	}
	b.WriteString(`<p style="margin:18px 0 0;color:#9ca3af;font-size:12px;">本邮件由 GoWAF 自动发送。可在「告警通知」页调整阈值、收件人与冷却时间。</p>`)
	b.WriteString(emailFoot())
	return subject, b.String()
}

// RenderTest 生成测试邮件，用来验证 SMTP 配置是否可用。
// 参数 cfg：邮件渠道配置，正文会回显服务器/发件人/收件人等信息。
// 返回 (string, string)：邮件主题与 HTML 正文。
func RenderTest(cfg Config) (string, string) {
	prefix := strings.TrimSpace(cfg.Subject)
	if prefix == "" {
		prefix = "【GoWAF 告警】"
	}
	subject := prefix + "测试邮件"

	var b strings.Builder
	b.WriteString(emailHead("GoWAF 告警测试"))
	b.WriteString(`<h2 style="margin:0 0 8px;font-size:18px;color:#111827;">邮件通道测试成功</h2>`)
	b.WriteString(`<p style="margin:0 0 16px;color:#374151;font-size:14px;">能收到这封邮件，说明告警通知已经配通。之后凡是触发封禁的事件，会按你设置的窗口合并后发到这个邮箱。</p>`)
	b.WriteString(`<table style="font-size:13px;color:#374151;">`)
	for _, kv := range [][2]string{
		{"SMTP 服务器", fmt.Sprintf("%s:%d（%s）", cfg.Host, cfg.Port, cfg.TLS)},
		{"发件人", orDash(cfg.From)},
		{"收件人", strings.Join(cfg.To, ", ")},
		{"发送时间", time.Now().Format("2006-01-02 15:04:05")},
	} {
		fmt.Fprintf(&b, `<tr><td style="padding:4px 12px 4px 0;color:#6b7280;">%s</td><td style="padding:4px 0;">%s</td></tr>`, esc(kv[0]), esc(kv[1]))
	}
	b.WriteString(`</table>`)
	b.WriteString(emailFoot())
	return subject, b.String()
}

// emailHead 返回 HTML 邮件的公共开头：文档骨架 + 居中卡片容器。
// 参数 title：页面标题（写入前做 HTML 转义）。
// 返回 string：直接拼接在正文最前面的 HTML 片段。
func emailHead(title string) string {
	return `<!doctype html><html><head><meta charset="utf-8"><title>` + esc(title) + `</title></head>` +
		`<body style="margin:0;padding:24px;background:#f9fafb;font-family:-apple-system,'PingFang SC','Microsoft YaHei',sans-serif;">` +
		`<div style="max-width:760px;margin:0 auto;background:#fff;border:1px solid #e5e7eb;border-radius:10px;padding:22px 24px;">`
}

// emailFoot 返回 HTML 邮件的公共结尾：分隔线、产品署名与闭合标签。
// 无参数；返回 string：拼接在正文末尾的 HTML 片段。
func emailFoot() string {
	return `<hr style="border:none;border-top:1px solid #f3f4f6;margin:20px 0 10px;">` +
		`<p style="color:#9ca3af;font-size:12px;margin:0;">GoWAF · 反向代理型 Web 应用防火墙</p>` +
		`</div></body></html>`
}

// esc 对文本做 HTML 转义。事件字段来自请求内容（路径/UA 等），进正文前必须
// 经过这里，防止 HTML 注入。
// 参数 s：原始文本；返回 string：转义后的安全文本。
func esc(s string) string { return html.EscapeString(s) }

// orDash 把空文本统一显示为 "-"，避免表格里出现空白单元格。
// 参数 s：原始文本；返回 string：空白（含纯空白字符）时返回 "-"，否则原样返回。
func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// sourceLabel 把事件来源代码翻译成中文标签，未知代码原样显示。
// 参数 s：来源代码（auto / rule_ban / intel / manual）。
// 返回 string：对应的中文名称（自动封禁/高危规则/威胁情报/手动封禁）。
func sourceLabel(s string) string {
	switch s {
	case "auto":
		return "自动封禁"
	case "rule_ban":
		return "高危规则"
	case "intel":
		return "威胁情报"
	case "manual":
		return "手动封禁"
	default:
		return orDash(s)
	}
}
