package policy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// baseReq 构造一个通用求值上下文，各用例按需覆盖字段。
func baseReq() *Request {
	return &Request{
		ClientIP: "1.2.3.4", Host: "example.com", Method: "GET",
		Path: "/index.html", Query: "id=7", UA: "Mozilla/5.0",
		Header: http.Header{}, Cookie: map[string]string{}, Args: map[string]string{},
	}
}

// TestCompileAndEvaluateAND 验证 AND 条件树：全部条件成立才命中。
func TestCompileAndEvaluateAND(t *testing.T) {
	set, err := Compile([]Definition{{
		ID: 1, Name: "封禁后台", Site: "shop", Priority: 1, Enabled: true, Action: ActionBlock,
		Conditions: Group{Logic: "and", Items: []Condition{
			{Field: "path", Op: "prefix", Value: "/admin"},
			{Field: "method", Op: "eq", Value: "POST"},
		}},
	}})
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	if set.Count() != 1 {
		t.Fatalf("策略数 = %d, 期望 1", set.Count())
	}

	// 全部成立：命中
	m, ok := set.Evaluate(&Request{Method: "POST", Path: "/admin/login"}, "shop")
	if !ok || m == nil || m.Action != ActionBlock || m.PolicyID != 1 {
		t.Fatalf("应命中 block，实际 ok=%v match=%+v", ok, m)
	}
	if !strings.Contains(m.Reason, "封禁后台") {
		t.Fatalf("Reason 应包含策略名: %q", m.Reason)
	}

	// 条件不成立：不命中
	if _, ok := set.Evaluate(&Request{Method: "GET", Path: "/admin/login"}, "shop"); ok {
		t.Fatal("method=GET 不该命中")
	}

	// 站点不匹配：绑定 shop 的策略对 blog 不生效
	if _, ok := set.Evaluate(&Request{Method: "POST", Path: "/admin/login"}, "blog"); ok {
		t.Fatal("站点不匹配不该命中")
	}

	// 站点匹配忽略大小写
	if _, ok := set.Evaluate(&Request{Method: "POST", Path: "/admin/login"}, "SHOP"); !ok {
		t.Fatal("站点名应忽略大小写")
	}
}

// TestEvaluateORAndNesting 验证 OR 组与嵌套分组的组合逻辑。
func TestEvaluateORAndNesting(t *testing.T) {
	set, err := Compile([]Definition{{
		ID: 2, Name: "管理接口", Priority: 1, Enabled: true, Action: ActionChallenge,
		Conditions: Group{Logic: "and", Items: []Condition{
			{Field: "path", Op: "prefix", Value: "/api"},
		}, Groups: []Group{
			{Logic: "or", Items: []Condition{
				{Field: "method", Op: "eq", Value: "DELETE"},
				{Field: "header.X-Admin", Op: "exists", Value: "true"},
			}},
		}},
	}})
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}

	// path 成立但 OR 组不成立：不命中
	if _, ok := set.Evaluate(&Request{Path: "/api/list", Header: http.Header{}}, "s"); ok {
		t.Fatal("无 DELETE 且无 X-Admin 头不该命中")
	}
	// OR 组第一个条件成立
	if _, ok := set.Evaluate(&Request{Path: "/api/list", Method: "DELETE", Header: http.Header{}}, "s"); !ok {
		t.Fatal("DELETE 应命中")
	}
	// OR 组第二个条件成立（header 存在）
	h := http.Header{}
	h.Set("X-Admin", "1")
	if _, ok := set.Evaluate(&Request{Path: "/api/list", Header: h}, "s"); !ok {
		t.Fatal("存在 X-Admin 头应命中")
	}
}

// TestPriorityOrdering 验证按优先级升序取第一条命中，同级按 ID 升序。
func TestPriorityOrdering(t *testing.T) {
	set, err := Compile([]Definition{
		{ID: 10, Name: "低优先", Priority: 10, Enabled: true, Action: ActionLog,
			Conditions: Group{Items: []Condition{{Field: "path", Op: "prefix", Value: "/x"}}}},
		{ID: 5, Name: "同级ID小", Priority: 1, Enabled: true, Action: ActionLog,
			Conditions: Group{Items: []Condition{{Field: "path", Op: "prefix", Value: "/x"}}}},
		{ID: 6, Name: "同级ID大", Priority: 1, Enabled: true, Action: ActionBlock,
			Conditions: Group{Items: []Condition{{Field: "path", Op: "prefix", Value: "/x"}}}},
	})
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	m, ok := set.Evaluate(&Request{Path: "/x/y"}, "s")
	if !ok || m.PolicyID != 5 {
		t.Fatalf("优先级 1 且 ID 最小的策略应先生效，实际 %+v", m)
	}

	// 禁用 ID 5 后应由 ID 6 接管
	set, err = Compile([]Definition{
		{ID: 5, Name: "已禁用", Priority: 1, Enabled: false, Action: ActionBlock,
			Conditions: Group{Items: []Condition{{Field: "path", Op: "prefix", Value: "/x"}}}},
		{ID: 6, Name: "生效", Priority: 1, Enabled: true, Action: ActionBlock,
			Conditions: Group{Items: []Condition{{Field: "path", Op: "prefix", Value: "/x"}}}},
	})
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	if m, _ := set.Evaluate(&Request{Path: "/x/y"}, "s"); m == nil || m.PolicyID != 6 {
		t.Fatalf("禁用策略应被跳过，实际 %+v", m)
	}
}

// TestConditionOperators 表格化验证全部操作符与字段解析。
func TestConditionOperators(t *testing.T) {
	cases := []struct {
		name string
		cond Condition
		req  *Request
		want bool
	}{
		{name: "eq命中", cond: Condition{Field: "method", Op: "eq", Value: "GET"}, req: baseReq(), want: true},
		{name: "eq忽略大小写", cond: Condition{Field: "method", Op: "eq", Value: "get", IgnoreCase: true}, req: baseReq(), want: true},
		{name: "ne命中", cond: Condition{Field: "method", Op: "ne", Value: "POST"}, req: baseReq(), want: true},
		{name: "contains", cond: Condition{Field: "ua", Op: "contains", Value: "Mozilla"}, req: baseReq(), want: true},
		{name: "suffix", cond: Condition{Field: "path", Op: "suffix", Value: ".html"}, req: baseReq(), want: true},
		{name: "regex", cond: Condition{Field: "path", Op: "regex", Value: `/index\.(html|htm)$`}, req: baseReq(), want: true},
		{name: "regex忽略大小写", cond: Condition{Field: "ua", Op: "regex", Value: `mozilla`, IgnoreCase: true}, req: baseReq(), want: true},
		{name: "cidr网段", cond: Condition{Field: "client_ip", Op: "cidr", Value: "1.2.0.0/16"}, req: baseReq(), want: true},
		{name: "cidr不在网段", cond: Condition{Field: "client_ip", Op: "cidr", Value: "10.0.0.0/8"}, req: baseReq(), want: false},
		{name: "cidr单IP补32", cond: Condition{Field: "client_ip", Op: "cidr", Value: "1.2.3.4"}, req: baseReq(), want: true},
		{name: "cidr IPv6映射v4", cond: Condition{Field: "client_ip", Op: "cidr", Value: "1.2.3.0/24"},
			req: &Request{ClientIP: "::ffff:1.2.3.4"}, want: true},
		{name: "in命中", cond: Condition{Field: "method", Op: "in", Value: "GET, POST"}, req: baseReq(), want: true},
		{name: "in不命中", cond: Condition{Field: "method", Op: "in", Value: "POST,PUT"}, req: baseReq(), want: false},
		{name: "gt数值", cond: Condition{Field: "arg.id", Op: "gt", Value: "5"}, req: withArgs("id", "7"), want: true},
		{name: "le数值", cond: Condition{Field: "arg.id", Op: "le", Value: "7"}, req: withArgs("id", "7"), want: true},
		{name: "非数值为假", cond: Condition{Field: "arg.id", Op: "gt", Value: "5"}, req: withArgs("id", "abc"), want: false},
		{name: "exists存在", cond: Condition{Field: "arg.id", Op: "exists", Value: "true"}, req: withArgs("id", "7"), want: true},
		{name: "exists不存在", cond: Condition{Field: "arg.missing", Op: "exists", Value: "false"}, req: baseReq(), want: true},
		{name: "header字段", cond: Condition{Field: "header.X-Token", Op: "eq", Value: "abc"}, req: withHeader("X-Token", "abc"), want: true},
		{name: "cookie字段", cond: Condition{Field: "cookie.session", Op: "exists", Value: "true"},
			req: &Request{Cookie: map[string]string{"session": "x"}, Header: http.Header{}, Args: map[string]string{}}, want: true},
		{name: "query字段同arg", cond: Condition{Field: "query.id", Op: "eq", Value: "7"}, req: withArgs("id", "7"), want: true},
		{name: "body字段", cond: Condition{Field: "body", Op: "contains", Value: "attack"},
			req: &Request{Body: "bad attack payload", Header: http.Header{}, Cookie: map[string]string{}, Args: map[string]string{}}, want: true},
		{name: "缺失字段exists false", cond: Condition{Field: "header.X-Nope", Op: "exists", Value: "true"},
			req: &Request{Header: http.Header{}, Cookie: map[string]string{}, Args: map[string]string{}}, want: false},
	}
	for _, tc := range cases {
		set, err := Compile([]Definition{{
			ID: 1, Name: "t", Enabled: true, Action: ActionLog,
			Conditions: Group{Items: []Condition{tc.cond}},
		}})
		if err != nil {
			t.Fatalf("%s: 编译失败: %v", tc.name, err)
		}
		if _, ok := set.Evaluate(tc.req, "s"); ok != tc.want {
			t.Errorf("%s: 期望 %v 实际 %v", tc.name, tc.want, ok)
		}
	}
}

// withArgs 返回带查询参数的上下文。
func withArgs(k, v string) *Request {
	r := baseReq()
	r.Args[k] = v
	return r
}

// withHeader 返回带头部的上下文。
func withHeader(k, v string) *Request {
	r := baseReq()
	r.Header.Set(k, v)
	return r
}

// TestCompileRejectsInvalid 表格化验证编译期校验：非法配置整体拒绝。
func TestCompileRejectsInvalid(t *testing.T) {
	one := func(c Condition) []Definition {
		return []Definition{{ID: 1, Name: "t", Enabled: true, Action: ActionBlock,
			Conditions: Group{Items: []Condition{c}}}}
	}
	cases := []struct {
		name string
		defs []Definition
	}{
		{name: "空条件", defs: []Definition{{ID: 1, Name: "t", Enabled: true,
			Conditions: Group{Logic: "and"}}}},
		{name: "未知字段", defs: one(Condition{Field: "password", Op: "eq", Value: "x"})},
		{name: "未知操作符", defs: one(Condition{Field: "path", Op: "~=", Value: "x"})},
		{name: "空比较值", defs: one(Condition{Field: "path", Op: "eq", Value: ""})},
		{name: "非法正则", defs: one(Condition{Field: "path", Op: "regex", Value: "([a-"})},
		{name: "非法CIDR", defs: one(Condition{Field: "client_ip", Op: "cidr", Value: "999.1.1.1/8"})},
		{name: "in列表为空", defs: one(Condition{Field: "method", Op: "in", Value: " , "})},
		{name: "数值非法", defs: one(Condition{Field: "arg.id", Op: "gt", Value: "abc"})},
		{name: "exists值非法", defs: one(Condition{Field: "arg.id", Op: "exists", Value: "maybe"})},
		{name: "动作非法", defs: []Definition{{ID: 1, Name: "t", Enabled: true, Action: "destroy",
			Conditions: Group{Items: []Condition{{Field: "path", Op: "prefix", Value: "/"}}}}}},
		{name: "名称为空", defs: []Definition{{ID: 1, Enabled: true,
			Conditions: Group{Items: []Condition{{Field: "path", Op: "prefix", Value: "/"}}}}}},
		{name: "嵌套超深", defs: func() []Definition {
			// 逐层嵌套 5 层 AND 组，超过 4 层上限
			g := Group{Items: []Condition{{Field: "path", Op: "prefix", Value: "/d"}}}
			for i := 0; i < 5; i++ {
				g = Group{Logic: "and", Groups: []Group{g}}
			}
			return []Definition{{ID: 1, Name: "深嵌套", Enabled: true, Conditions: g}}
		}()},
		{name: "条件超64", defs: func() []Definition {
			items := make([]Condition, 65)
			for i := range items {
				items[i] = Condition{Field: "arg.a" + itoaForTest(i), Op: "exists", Value: "false"}
			}
			return []Definition{{ID: 1, Name: "超量", Enabled: true, Conditions: Group{Items: items}}}
		}()},
	}
	for _, tc := range cases {
		if s, err := Compile(tc.defs); err == nil {
			t.Errorf("%s: 应编译失败，实际成功且 count=%d", tc.name, s.Count())
		}
	}
}

// itoaForTest 生成 a0..a64 参数名。
func itoaForTest(i int) string {
	return string(rune('a'+i/10)) + string(rune('0'+i%10))
}

// TestCompileEmptySetOk 空定义集合法：返回空快照而不是错误。
func TestCompileEmptySetOk(t *testing.T) {
	set, err := Compile(nil)
	if err != nil || set.Count() != 0 {
		t.Fatalf("空定义集应编译成功: err=%v count=%d", err, set.Count())
	}
	// 空快照求值永远不命中
	if _, ok := set.Evaluate(baseReq(), "s"); ok {
		t.Fatal("空快照不该命中")
	}
}

// TestHitStatsAccounting 验证命中统计的记录、快照与归零。
func TestHitStatsAccounting(t *testing.T) {
	set, _ := Compile([]Definition{{ID: 7, Name: "t", Enabled: true,
		Conditions: Group{Items: []Condition{{Field: "path", Op: "prefix", Value: "/"}}}}})
	set.RecordHit(7, 1000)
	set.RecordHit(7, 2000)
	set.RecordHit(99, 3000) // 不存在的 ID 静默忽略

	stats := set.HitStats()
	if len(stats) != 1 || stats[0].ID != 7 {
		t.Fatalf("统计快照异常: %+v", stats)
	}
	if stats[0].Hits != 2 || stats[0].LastHit != 2000 {
		t.Fatalf("命中数/时间异常: %+v", stats[0])
	}

	set.ResetHits(7, 2)
	if st := set.HitStats()[0]; st.Hits != 0 {
		t.Fatalf("归零后 Hits = %d", st.Hits)
	}
	// 非正增量不产生任何效果
	set.ResetHits(7, 0)
	if st := set.HitStats()[0]; st.Hits != 0 {
		t.Fatalf("增量 0 不该改变计数")
	}
}

// TestDecodeGroup 验证条件树 JSON 的解析与缺省值。
func TestDecodeGroup(t *testing.T) {
	// 空串：返回空 AND 组
	g, err := DecodeGroup("")
	if err != nil || g.Logic != "and" || len(g.Items) != 0 {
		t.Fatalf("空串应返回空 AND 组: %+v err=%v", g, err)
	}
	// 正常 JSON
	g, err = DecodeGroup(`{"logic":"or","items":[{"field":"path","op":"prefix","value":"/a"}]}`)
	if err != nil || g.Logic != "or" || len(g.Items) != 1 || g.Items[0].Field != "path" {
		t.Fatalf("JSON 解析异常: %+v err=%v", g, err)
	}
	// logic 缺省补 and
	g, err = DecodeGroup(`{"items":[{"field":"path","op":"prefix","value":"/a"}]}`)
	if err != nil || g.Logic != "and" {
		t.Fatalf("logic 缺省应为 and: %+v err=%v", g, err)
	}
	// 非法 JSON 报错
	if _, err := DecodeGroup("{not json"); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}

// TestNewRequestFromHTTP 验证从 http.Request 构造上下文：Cookie/Args 展开、表单合并。
func TestNewRequestFromHTTP(t *testing.T) {
	body := strings.NewReader("user=tom&from=form")
	req := httptest.NewRequest("POST", "/login?id=7&tag=q", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Custom", "cv")
	req.AddCookie(&http.Cookie{Name: "sid", Value: "s1"})

	r := NewRequest(req, []byte("user=tom&from=form"), "9.9.9.9", "CN", "BJ")

	if r.ClientIP != "9.9.9.9" || r.Country != "CN" || r.Region != "BJ" {
		t.Fatalf("IP/归属地异常: %+v", r)
	}
	if r.Path != "/login" || r.Query != "id=7&tag=q" || r.Method != "POST" {
		t.Fatalf("路径/查询/方法异常: %+v", r)
	}
	if r.Cookie["sid"] != "s1" {
		t.Fatalf("Cookie 未展开: %+v", r.Cookie)
	}
	if r.Args["id"] != "7" || r.Args["tag"] != "q" {
		t.Fatalf("query 参数未展开: %+v", r.Args)
	}
	if r.Args["user"] != "tom" || r.Args["from"] != "form" {
		t.Fatalf("表单体参数未合并: %+v", r.Args)
	}
	if r.Header.Get("X-Custom") != "cv" {
		t.Fatal("Header 应保留原 http.Header")
	}
	if r.Body != "user=tom&from=form" {
		t.Fatalf("Body 异常: %q", r.Body)
	}

	// query 参数优先于表单同名参数
	req2 := httptest.NewRequest("POST", "/login?user=query", strings.NewReader("user=body"))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if r2 := NewRequest(req2, []byte("user=body"), "", "", ""); r2.Args["user"] != "query" {
		t.Fatalf("query 与表单重名时 query 应优先: %v", r2.Args)
	}
}
