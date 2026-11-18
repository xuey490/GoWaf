package edge

import (
	"strings"
	"testing"

	"gowaf/internal/proxy"
	"gowaf/internal/store/model"
)

// TestRenderBlockPage 自定义拦截页占位符替换与转义。
func TestRenderBlockPage(t *testing.T) {
	tmpl := "<h1>{{status}}</h1><p>{{reason}}</p><span>{{reqid}}</span><i>{{site}}</i>"
	out := renderBlockPage(tmpl, 403, "命中<b>规则", "rid&1", "demo<script>")
	if !strings.Contains(out, "<h1>403</h1>") {
		t.Fatalf("状态码未替换: %s", out)
	}
	if strings.Contains(out, "命中<b>规则") || !strings.Contains(out, "命中&lt;b&gt;规则") {
		t.Fatalf("原因未转义: %s", out)
	}
	if !strings.Contains(out, "rid&amp;1") {
		t.Fatalf("请求 ID 未转义: %s", out)
	}
	if strings.Contains(out, "demo<script>") || !strings.Contains(out, "demo&lt;script&gt;") {
		t.Fatalf("站点名未转义: %s", out)
	}
}

// TestChallengeChain 降级链解析。
func TestChallengeChain(t *testing.T) {
	cases := []struct {
		v       string
		captcha bool
		banOn   bool
	}{
		{"pow", false, false},
		{"", false, false},
		{"pow,captcha", true, false},
		{"pow,captcha,ban", true, true},
		{"CAPTCHA", true, false},
		{"captcha, ban", true, true},
	}
	for _, c := range cases {
		captcha, ban := challengeChain(c.v)
		if captcha != c.captcha || ban != c.banOn {
			t.Fatalf("challengeChain(%q) = (%v,%v), 期望 (%v,%v)", c.v, captcha, ban, c.captcha, c.banOn)
		}
	}
}

// TestBlockPageFor 站点未配置自定义模板时回退内置页；配置启用时用模板。
func TestBlockPageFor(t *testing.T) {
	h := &Handler{}
	if out := h.blockPageFor(nil, 403, "x", "r1"); !strings.Contains(out, "Request ID") {
		t.Fatalf("无路由时应回退内置拦截页")
	}
	custom := &proxy.Route{
		SiteName:  "demo",
		BlockPage: &model.BlockPage{Enabled: 1, HTML: "<h1>{{status}} @ {{site}}</h1>"},
	}
	out := h.blockPageFor(custom, 403, "x", "r1")
	if out != "<h1>403 @ demo</h1>" {
		t.Fatalf("自定义模板渲染异常: %s", out)
	}
}
