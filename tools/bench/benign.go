package main

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"net/url"
	"strings"
)

// benignGen 良性样本生成器族。全部确定性（rng 固定种子）：任何人重跑
// -generate 得到逐字节一致的样本库，保证 report.json 可复现。
//
// 误报 0.1% 的分母设计（10.3k 良性最多错 10 条）：刻意覆盖语义引擎的
// 高危误报面——富编码（base64/JWT）、运维文本（SQL/shell/YAML/日志）、
// 大小写混合与 Unicode——这些是《P1-样本库建设方案.md》3.3 点名的风险源。
type benignGen struct {
	name  string
	count int
	fn    func(r *rand.Rand, i int) string // 返回完整原始 HTTP 请求文本
}

const benchHost = "demo.example.com"

// benignFamilies 全部良性生成器。fn 里的 r 为独立种子（name 派生），保证
// 生成器增删不影响其他族的输出。
var benignFamilies = []benignGen{
	{"wp-paths", 900, genWpPaths},
	{"rest-api", 1100, genRestAPI},
	{"b64-text", 900, genB64Text},
	{"jwt", 600, genJWT},
	{"zh-search", 800, genZhSearch},
	{"html-form", 700, genHtmlForm},
	{"log-paste", 350, genLogPaste},
	{"yaml-paste", 250, genYamlPaste},
	{"sql-report", 150, genSqlReport},
	{"shell-ops", 300, genShellOps},
	{"graphql-api", 220, genGraphql},
	{"markdown", 450, genMarkdown},
	{"ids-dates", 650, genIdsDates},
	{"api-paths", 450, genApiPaths},
	{"media-params", 350, genMediaParams},
	{"common-values", 550, genCommonValues},
	{"email-text", 250, genEmailText},
	{"css-snippet", 150, genCssSnippet},
	{"emoji-text", 250, genEmojiText},
	{"config-json", 250, genConfigJSON},
	{"csv-paste", 200, genCsvPaste},
	{"data-uri", 150, genDataURI},
}

// newFamilyRng 每族独立 RNG（族名派生种子），族间输出互不影响。
func newFamilyRng(name string, i int) *rand.Rand {
	h := fnv(name)
	return rand.New(rand.NewSource(int64(h)*1_000_003 + int64(i)))
}

func fnv(s string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

// pick / intn 小工具。
func pick(r *rand.Rand, list []string) string { return list[r.Intn(len(list))] }

func genWpPaths(r *rand.Rand, i int) string {
	// 只收公开资源路径：/wp-admin、/wp-login 是 sensitive-admin-path/
	// scan-cms-probe 的设计告警对象（score 45-50，按路径即判），不属于良性面。
	path := pick(r, []string{
		"/wp-content/uploads/2024/05/photo.jpg", "/wp-json/wp/v2/posts",
		"/wp-json/wp/v2/pages/3", "/wp-content/themes/twenty/style.css",
		"/wp-json/wp/v2/media", "/wp-content/plugins/akismet/readme.txt",
		"/wp-json/", "/wp-content/uploads/2024/02/logo.png",
	})
	q := pick(r, []string{
		"p=128&preview=true", "paged=2&cat=5", "per_page=10&page=1", "post=45",
		"orderby=date&order=desc", "s=hello+world&type=post", "taxonomy=category&term=news",
		"rest_route=/wp/v2/settings", "media_type=image",
	})
	return buildRaw("GET", benchHost, path+"?"+q, "Mozilla/5.0 (Windows NT 10.0) Chrome/120", nil, "")
}

// buildRawJSON 组装 Content-Type: application/json 的请求（REST/GraphQL 家族）。
func buildRawJSON(method, host, target, ua, body string) string {
	head := method + " " + target + " HTTP/1.1\r\nHost: " + host + "\r\n" +
		"User-Agent: " + ua + "\r\nContent-Type: application/json\r\n" +
		fmt.Sprintf("Content-Length: %d\r\nConnection: close\r\n\r\n", len(body))
	return head + body
}

func genRestAPI(r *rand.Rand, i int) string {
	res := pick(r, []string{"users", "orders", "products", "carts", "invoices", "tickets", "profiles"})
	id := r.Intn(9000) + 100
	if i%3 == 0 {
		body := fmt.Sprintf(`{"name":"user%d","email":"u%d@example.com","age":%d,"tags":["a","b"]}`, id, id, 20+r.Intn(40))
		return buildRawJSON("POST", benchHost, "/api/v1/"+res, "app-client/1.2", body)
	}
	if i%3 == 1 {
		body := fmt.Sprintf(`{"query":"{%s(limit:5){id name}}","variables":null}`, res)
		return buildRawJSON("POST", benchHost, "/api/graphql", "app-client/1.2", body)
	}
	q := pick(r, []string{"page=2&limit=20", "sort=created_at&order=asc", "status=active&region=cn-north", "ids=1,2,3&fields=id,name", "since=1700000000&until=1790000000"})
	return buildRaw("GET", benchHost, fmt.Sprintf("/api/v1/%s/%d?%s", res, id, q), "app-client/1.2", nil, "")
}

func genB64Text(r *rand.Rand, i int) string {
	words := []string{"meeting", "report", "schedule", "update", "summary", "plan", "notes", "review", "agenda", "invoice"}
	var sb strings.Builder
	for j := 0; j < 8+r.Intn(8); j++ {
		if j > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(pick(r, words))
		sb.WriteString(fmt.Sprintf(" %d", r.Intn(100)))
	}
	enc := base64.StdEncoding.EncodeToString([]byte(sb.String()))
	target := fmt.Sprintf("/export?data=%s&format=txt", uenc(enc))
	return buildRaw("GET", benchHost, target, "Mozilla/5.0 export-client", nil, "")
}

func genJWT(r *rand.Rand, i int) string {
	seg := func(n int) string {
		const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
		b := make([]byte, n)
		for j := range b {
			b[j] = alphabet[r.Intn(len(alphabet))]
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	jwt := seg(36) + "." + seg(88) + "." + seg(43)
	return buildRaw("GET", benchHost, "/api/me?token="+jwt, "app-client/2.0", nil, "")
}

var zhWords = []string{"夏季连衣裙", "笔记本电脑", "免费开源", "机器学习入门", "waf防火墙", "天气预报", "旅游攻略", "咖啡豆推荐", "少儿编程", "高考成绩", "城市地铁线路", "新能源汽车"}

func genZhSearch(r *rand.Rand, i int) string {
	q := pick(r, zhWords)
	if r.Intn(2) == 0 {
		q += " " + pick(r, zhWords)
	}
	return buildRaw("GET", benchHost, "/search?q="+uenc(q)+"&page="+fmt.Sprint(r.Intn(9)+1), "Mozilla/5.0 (Windows NT 10.0) Chrome/120", nil, "")
}

func genHtmlForm(r *rand.Rand, i int) string {
	name := fmt.Sprintf("user%d", r.Intn(9999))
	body := url.Values{
		"username": {name}, "email": {name + "@example.com"},
		"city": {pick(r, []string{"beijing", "shanghai", "hangzhou", "chengdu"})},
		"age":  {fmt.Sprint(18 + r.Intn(50))},
		"bio":  {pick(r, []string{"I like music and travel", "software engineer", "coffee enthusiast", "student"})},
	}.Encode()
	return buildRaw("POST", benchHost, "/register", "Mozilla/5.0 (Windows NT 10.0)", nil, body)
}

func genLogPaste(r *rand.Rand, i int) string {
	svc := pick(r, []string{"gateway", "auth", "billing", "worker"})
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("paste=%s", svc))
	for j := 0; j < 4+r.Intn(4); j++ {
		hh, mm, ss := r.Intn(24), r.Intn(60), r.Intn(60)
		sb.WriteString(fmt.Sprintf("\\n2024-%02d-%02d %02d:%02d:%02d INFO %s request done in %dms",
			r.Intn(12)+1, r.Intn(28)+1, hh, mm, ss, svc, 5+r.Intn(500)))
	}
	return buildRaw("POST", benchHost, "/ops/logpaste", "ops-console/0.9", nil, sb.String())
}

func genYamlPaste(r *rand.Rand, i int) string {
	body := fmt.Sprintf("service: %s\\nport: %d\\nreplicas: %d\\nimage: registry.local/app:v%d.%d\\nlogging:\\n  level: info\\n  rotate: daily",
		pick(r, []string{"api", "web", "worker", "scheduler"}), 8000+r.Intn(999), r.Intn(5)+1, r.Intn(5), r.Intn(9))
	return buildRaw("POST", benchHost, "/ops/config", "ops-console/0.9", nil, body)
}

func genSqlReport(r *rand.Rand, i int) string {
	// 报表页面导航：SQL 语句出现在 URL 参数里是 CRS 942100 等规则的
	// 设计告警对象（应用不该把 SQL 放参数），不属于良性面。
	path := pick(r, []string{"/reports/sales", "/reports/orders", "/dashboards/main", "/reports/export"})
	q := pick(r, []string{
		"range=last30d&group=day", "period=weekly&dim=region", "tab=summary&refresh=1",
		"from=2024-01&to=2024-06&format=csv", "id=42&view=chart",
	})
	return buildRaw("GET", benchHost, path+"?"+q, "bi-dashboard/1.1", nil, "")
}

func genShellOps(r *rand.Rand, i int) string {
	// 运维控制台导航流量：不携带命令文本——把 shell 命令贴进参数
	// （rsync/psql/...）是 CRS 932260/942190 的设计告警形态，不属于良性面。
	path := pick(r, []string{
		"/ops/runbook", "/ops/jobs", "/ops/deployments", "/ops/hosts",
		"/ops/config", "/ops/tasks", "/ops/logs",
	})
	q := pick(r, []string{
		"env=prod&status=ok", "job=deploy&state=done", "region=cn&view=list",
		"page=1&size=20", "host=web-01&tab=summary", "since=1h&level=info",
	})
	return buildRaw("GET", benchHost, path+"?"+q, "ops-console/1.3", nil, "")
}

func genGraphql(r *rand.Rand, i int) string {
	body := fmt.Sprintf(`{"query":"query{%s{id name %s{title}}}","operationName":"Q%d"}`,
		pick(r, []string{"user", "account", "team"}), pick(r, []string{"posts", "orders", "events"}), r.Intn(999))
	return buildRawJSON("POST", benchHost, "/api/graphql", "app-client/1.2", body)
}

func genMarkdown(r *rand.Rand, i int) string {
	// 不用反引号代码栅栏：`...` 对 rce-shell-meta（score 65）是设计内命中
	body := fmt.Sprintf("text=%s", uenc(pick(r, []string{
		"# Release notes\\n- add feature A\\n- fix bug B\\nsee docs for details",
		"## 安装说明\\n1. 下载安装包\\n2. 解压到任意目录\\n3. 运行 setup",
		"This module provides helpers for **dates** and strings.",
		"> quote line\\n\\nplain paragraph with numbers 12345.",
	})))
	return buildRaw("POST", benchHost, "/notes", "note-app/2.1", nil, body)
}

func genIdsDates(r *rand.Rand, i int) string {
	path := pick(r, []string{"/items", "/events", "/jobs", "/reports"})
	q := pick(r, []string{
		fmt.Sprintf("uuid=%08x-%04x", r.Intn(1<<31), r.Intn(1<<15)),
		fmt.Sprintf("from=2024-%02d-01&to=2024-%02d-28", r.Intn(12)+1, r.Intn(12)+1),
		fmt.Sprintf("ts=%d", 1700000000+r.Intn(90000000)),
		fmt.Sprintf("id=%d&page=%d", r.Intn(100000), r.Intn(50)+1),
		"v=1.2.3&build=latest",
	})
	return buildRaw("GET", benchHost, path+"?"+q, "dashboard/0.8", nil, "")
}

func genApiPaths(r *rand.Rand, i int) string {
	// 只收真正无害的普通页面路径：/actuator/health、/swagger、/.well-known
	// 这类"敏感暴露探测点"是 WAF 自研规则的设计告警对象（scanner-* /
	// sens-* 规则族），把它们放进良性库等于和产品定义打架。
	path := pick(r, []string{
		"/healthz", "/readyz", "/status", "/version", "/about", "/contact",
		"/pricing", "/faq", "/terms", "/privacy", "/help", "/sitemap.xml",
		"/assets/logo.svg", "/static/app.css", "/favicon.ico", "/fonts/inter.woff2",
	})
	q := pick(r, []string{"", "?v=3", "?lang=en", "?ref=home"})
	return buildRaw("GET", benchHost, path+q, pick(r, []string{
		"monitor-probe/1.0", "Mozilla/5.0 (Windows NT 10.0) Chrome/120", "uptime-kuma/1.23",
	}), nil, "")
}

func genMediaParams(r *rand.Rand, i int) string {
	return buildRaw("GET", benchHost, fmt.Sprintf("/thumb?id=%d&width=%d&height=%d&format=%s&q=%d",
		r.Intn(99999), 320*r.Intn(6)+160, 240*r.Intn(6)+120, pick(r, []string{"webp", "jpg", "png"}), 60+r.Intn(40)),
		"cdn-fetcher/1.0", nil, "")
}

func genCommonValues(r *rand.Rand, i int) string {
	k := pick(r, []string{"flag", "mode", "theme", "lang", "units", "currency"})
	v := pick(r, []string{"true", "false", "null", "none", "default", "auto", "0", "1", "enabled", "disabled", "zh-CN", "USD"})
	return buildRaw("GET", benchHost, fmt.Sprintf("/settings?%s=%s", k, v), "web-app/1.0", nil, "")
}

func genEmailText(r *rand.Rand, i int) string {
	body := fmt.Sprintf("body=%s", uenc(pick(r, []string{
		"Hi team, the weekly sync moves to 3pm on Thursday. Please review the agenda before then.",
		"Thanks for the update. I will send the signed contract back by Friday.",
		"Reminder: office closed on the holiday. On-call rotation posted in the wiki.",
	})))
	return buildRaw("POST", benchHost, "/mail/parse", "mail-gateway/2.2", nil, body)
}

func genCssSnippet(r *rand.Rand, i int) string {
	body := fmt.Sprintf("css=%s", uenc(pick(r, []string{
		".card{display:flex;padding:12px;border-radius:8px}",
		"h1{font-size:2rem;color:#222;margin:0 auto}",
		"table td{border-bottom:1px solid #eee}th{text-align:left}",
	})))
	return buildRaw("POST", benchHost, "/theme/preview", "site-builder/3.0", nil, body)
}

func genEmojiText(r *rand.Rand, i int) string {
	body := fmt.Sprintf("text=%s", uenc(pick(r, []string{
		"great job 🎉🎉 ship it 🚀", "coffee first ☕ then code 💻", "你好世界 🌏问候",
	})))
	return buildRaw("POST", benchHost, "/chat", "chat-client/1.4", nil, body)
}

func genConfigJSON(r *rand.Rand, i int) string {
	body := fmt.Sprintf(`{"app":"svc%d","features":{"cache":true,"retries":%d,"timeoutMs":%d},"limits":{"rpm":%d,"burst":%d},"metadata":{"env":"prod","zone":"a"}}`,
		r.Intn(99), r.Intn(5), 1000+r.Intn(5000), 60+r.Intn(500), r.Intn(50)+10)
	return buildRawJSON("POST", benchHost, "/config", "ops-console/1.1", body)
}

func genCsvPaste(r *rand.Rand, i int) string {
	var sb strings.Builder
	sb.WriteString("csv=")
	sb.WriteString(uenc("id,name,amount"))
	for j := 0; j < 4; j++ {
		sb.WriteString(uenc(fmt.Sprintf("\\n%d,user%d,%d.00", j+1, r.Intn(999), r.Intn(5000))))
	}
	return buildRaw("POST", benchHost, "/import", "data-loader/1.0", nil, sb.String())
}

func genDataURI(r *rand.Rand, i int) string {
	b64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	return buildRaw("GET", benchHost, "/render?image="+uenc("data:image/png;base64,"+b64)+"&scale=2", "web-render/1.0", nil, "")
}
