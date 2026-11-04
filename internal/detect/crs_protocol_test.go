package detect

import "testing"

// TestCRSProtocolRulesDoNotBlock 回归：线上事故——正常登录 POST 被 crs-920620
// （协议合规类）判 60 分直接 403，5 次后触发自动封禁，全站挂掉。
// 协议类规则必须降权且总和封顶，单靠它们不能越过默认校验阈值 30。
func TestCRSProtocolRulesDoNotBlock(t *testing.T) {
	eng, err := NewCRSEngine(CRSOptions{Mode: "block", Paranoia: 1})
	if err != nil {
		t.Fatalf("加载 CRS 失败: %v", err)
	}

	// 模拟真实浏览器/前端发出的报文：有 Host、Content-Length、Accept 等，
	// 否则"缺少 Host"这类规则本来就会正常命中，测不出误报问题。
	realHeaders := "Host: demo.local\n" +
		"Content-Length: 39\n" +
		"Accept: application/json, text/plain, */*\n" +
		"Accept-Language: zh-CN,zh;q=0.9\n" +
		"Connection: keep-alive\n" +
		"Content-Type: application/json\n" +
		"User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/131.0.0.0 Safari/537.36\n"

	cases := []struct {
		name string
		in   *Input
	}{
		{"登录POST带JSON", &Input{
			Method: "POST", Host: "demo.local", Path: "/prod/api/core/login",
			Body:    `{"username":"admin","password":"x"}`,
			Headers: realHeaders,
		}},
		{"表单登录", &Input{
			Method: "POST", Host: "demo.local", Path: "/prod/api/schooie/parent/login",
			Body: "username=admin&password=123456",
			Headers: "Host: demo.local\nContent-Type: application/x-www-form-urlencoded\n" +
				"Content-Length: 33\nUser-Agent: Mozilla/5.0 Chrome/131.0.0.0\n",
		}},
		{"重复ContentType", &Input{
			Method: "POST", Host: "demo.local", Path: "/prod/api/core/login",
			Body: `{"a":1}`,
			Headers: "Host: demo.local\nContent-Length: 7\n" +
				"Content-Type: application/json\nContent-Type: text/plain\n" +
				"User-Agent: Mozilla/5.0 Chrome/131.0.0.0\n",
		}},
	}

	for _, c := range cases {
		res := eng.Inspect(c.in)
		if res.Score >= 30 {
			var ids []string
			for _, h := range res.Hits {
				ids = append(ids, h.RuleID)
			}
			t.Errorf("%s: 协议类误报导致评分 %d（>=30 会被判成挑战/拦截），命中=%v", c.name, res.Score, ids)
		}
		for _, h := range res.Hits {
			if h.Category == "protocol" && h.Score >= 60 {
				t.Errorf("%s: 协议规则 %s 权重未降级（%d）", c.name, h.RuleID, h.Score)
			}
		}
	}
}

// TestCRSStillBlocksSQLi 降权只针对协议类，内容类规则不能被削弱。
func TestCRSStillBlocksSQLi(t *testing.T) {
	eng, err := NewCRSEngine(CRSOptions{Mode: "block", Paranoia: 1})
	if err != nil {
		t.Fatalf("加载 CRS 失败: %v", err)
	}
	res := eng.Inspect(&Input{
		Method: "GET", Path: "/", Query: "id=1%20union%20select%201,2,3",
		Headers: "User-Agent: Mozilla/5.0\n",
	})
	if res.Score < 60 {
		t.Fatalf("SQLi 应得分 >= 60，实际 %d", res.Score)
	}
}

// TestCRSMethodEnforcementREST 回归：线上事故——前后端分离的 REST API 用
// PUT 更新资源，CRS 默认 tx.allowed_methods 只有 GET HEAD POST OPTIONS，
// 911100 判 CRITICAL 60 分，叠加 920620（15 分）正好达到拦截阈值 75，
// 管理后台整屏 API 全 403，随后自动封禁连锁放大。
func TestCRSMethodEnforcementREST(t *testing.T) {
	eng, err := NewCRSEngine(CRSOptions{Mode: "block", Paranoia: 2})
	if err != nil {
		t.Fatalf("加载 CRS 失败: %v", err)
	}
	hdr := "Host: demo.local\nContent-Length: 2\n" +
		"Content-Type: application/json\n" +
		"User-Agent: Mozilla/5.0 Chrome/131.0.0.0\n"
	for _, m := range []string{"PUT", "DELETE", "PATCH"} {
		res := eng.Inspect(&Input{
			Method: m, Host: "demo.local", Path: "/prod/api/flow/category/update/2",
			Body: `{"id":2}`, Headers: hdr,
		})
		for _, h := range res.Hits {
			if h.RuleID == "crs-911100" {
				t.Errorf("%s 请求命中 911100（方法白名单未覆盖 REST 方法），score=%d", m, res.Score)
			}
		}
		if res.Action == "block" {
			t.Errorf("%s 正常 REST 更新被拦截: score=%d hits=%v", m, res.Score, res.RuleIDs)
		}
	}

	// 收紧白名单后 PUT 应重新命中
	eng2, err := NewCRSEngine(CRSOptions{Mode: "block", Paranoia: 1, AllowedMethods: "GET HEAD POST OPTIONS"})
	if err != nil {
		t.Fatalf("加载 CRS 失败: %v", err)
	}
	res := eng2.Inspect(&Input{
		Method: "PUT", Host: "demo.local", Path: "/x", Headers: hdr,
	})
	found := false
	for _, h := range res.Hits {
		if h.RuleID == "crs-911100" {
			found = true
		}
	}
	if !found {
		t.Errorf("收紧白名单后 PUT 未命中 911100")
	}
	if eng2.Methods() != "GET HEAD POST OPTIONS" {
		t.Errorf("Methods() 不符: %q", eng2.Methods())
	}

	// sanitizeMethods 防注入
	if got := sanitizeMethods("GET' HEAD,\\ DELETE\"; SecRuleEngine On"); got != "GET HEAD DELETE SECRULEENGINE ON" {
		t.Errorf("sanitizeMethods 异常: %q", got)
	}
	if sanitizeMethods("") != DefaultCRSMethods || sanitizeMethods("!!!") != DefaultCRSMethods {
		t.Errorf("空/非法输入应回退默认集")
	}
}
