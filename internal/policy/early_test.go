package policy

import (
	"net/http"
	"testing"
)

// earlyReq 构造一个用于提前求值的策略上下文：body 为空，其余字段按需覆盖。
// 参数 country：归属地文本；参数 path：请求路径。
// 返回 *Request：可直接传给 EvaluateDenyEarly 的上下文。
func earlyReq(country, path string) *Request {
	return &Request{
		ClientIP: "1.2.3.4", Country: country, Host: "example.com", Method: "GET",
		Path: path, Query: "", UA: "Mozilla/5.0",
		Header: http.Header{}, Cookie: map[string]string{}, Args: map[string]string{},
	}
}

// compileOne 编译单条策略，失败直接终止用例。
// 参数 t：测试句柄；参数 d：策略定义。
// 返回 *Set：编译后的快照。
func compileOne(t *testing.T, d Definition) *Set {
	t.Helper()
	set, err := Compile([]Definition{d})
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	return set
}

// TestEvaluateDenyEarlyBlocksCachedAsset 是缓存旁路缺陷的核心回归用例：
// 命中缓存的静态资源必须仍能被地域拦截策略拦下。请求不带 body，
// 策略也不依赖 body，因此必须在读 body 之前就给出结论。
func TestEvaluateDenyEarlyBlocksCachedAsset(t *testing.T) {
	set := compileOne(t, Definition{
		ID: 1, Name: "拦截-荷兰", Site: "", Priority: 100, Enabled: true,
		Action: ActionBlock,
		Conditions: Group{Logic: "and", Items: []Condition{
			{Field: "country", Op: "contains", Value: "荷兰"},
		}},
	})

	m, ok := set.EvaluateDenyEarly(earlyReq("荷兰", "/assets/index-DmplYGAM.js"), "")
	if !ok {
		t.Fatal("已缓存的静态资源应能被地域拦截策略提前拦下，实际未命中")
	}
	if m.Action != ActionBlock || m.PolicyID != 1 {
		t.Fatalf("命中结果不对：action=%s id=%d", m.Action, m.PolicyID)
	}

	// 非目标地域必须放行，避免「一句话把全站拦死」。
	if _, ok := set.EvaluateDenyEarly(earlyReq("中国 广东 深圳", "/assets/index-DmplYGAM.js"), ""); ok {
		t.Fatal("非荷兰来源不应命中")
	}
}

// TestEvaluateDenyEarlySkipsBodyDependent 验证依赖 body 的策略不会被提前执行：
// 第一条命中的策略引用了 body，就无法在读 body 之前下结论，必须交回完整求值。
// 这里用 body exists=false 构造一个「空 body 也能匹配」的策略，确保确实是
// needsBody 标记在起作用，而不是条件恰好没匹配上。
func TestEvaluateDenyEarlySkipsBodyDependent(t *testing.T) {
	g := Group{Logic: "and", Items: []Condition{
		{Field: "body", Op: "exists", Value: "false"},
		{Field: "path", Op: "prefix", Value: "/assets"},
	}}
	set := compileOne(t, Definition{
		ID: 1, Name: "按 body 拦截", Site: "", Priority: 1, Enabled: true,
		Action: ActionBlock, Conditions: g,
	})

	req := earlyReq("荷兰", "/assets/app.js")
	if _, ok := set.EvaluateDenyEarly(req, ""); ok {
		t.Fatal("依赖 body 的策略不应被提前求值")
	}
	// 完整求值（body 已读）时该策略确实能命中，说明上面的 false 来自延迟判定。
	if _, ok := set.Evaluate(req, ""); !ok {
		t.Fatal("完整求值应命中，否则用例前提不成立")
	}
}

// TestEvaluateDenyEarlyDefersAllowAndLog 验证 allow / log 不会被提前执行。
// 这两个动作不产生拒绝响应，提前处理只会打乱「第 8 步统一记录 log」的时序。
func TestEvaluateDenyEarlyDefersAllowAndLog(t *testing.T) {
	for _, action := range []Action{ActionAllow, ActionLog} {
		set := compileOne(t, Definition{
			ID: 1, Name: "非拒绝动作", Site: "", Priority: 1, Enabled: true,
			Action: action,
			Conditions: Group{Logic: "and", Items: []Condition{
				{Field: "country", Op: "eq", Value: "荷兰"},
			}},
		})
		if _, ok := set.EvaluateDenyEarly(earlyReq("荷兰", "/"), ""); ok {
			t.Fatalf("动作 %s 不应被提前执行", action)
		}
	}
}

// TestEvaluateDenyEarlyRespectsFirstMatchWins 验证优先级语义不被提前求值破坏：
// 优先级更靠前的 allow 策略命中后，后面的 block 策略不得提前生效。
func TestEvaluateDenyEarlyRespectsFirstMatchWins(t *testing.T) {
	set, err := Compile([]Definition{
		{
			ID: 2, Name: "放行静态资源", Site: "", Priority: 1, Enabled: true,
			Action: ActionAllow,
			Conditions: Group{Logic: "and", Items: []Condition{
				{Field: "path", Op: "prefix", Value: "/assets"},
			}},
		},
		{
			ID: 3, Name: "拦截-荷兰", Site: "", Priority: 100, Enabled: true,
			Action: ActionBlock,
			Conditions: Group{Logic: "and", Items: []Condition{
				{Field: "country", Op: "eq", Value: "荷兰"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	if _, ok := set.EvaluateDenyEarly(earlyReq("荷兰", "/assets/a.js"), ""); ok {
		t.Fatal("优先级更靠前的 allow 命中后，不应提前执行后面的 block")
	}
}

// TestEvaluateDenyEarlyFiltersBySite 验证站点维度的过滤：只对所属站点生效，
// 站点留空表示全站。
func TestEvaluateDenyEarlyFiltersBySite(t *testing.T) {
	set := compileOne(t, Definition{
		ID: 1, Name: "只拦 A 站", Site: "a.example.com", Priority: 1, Enabled: true,
		Action: ActionBlock,
		Conditions: Group{Logic: "and", Items: []Condition{
			{Field: "country", Op: "eq", Value: "荷兰"},
		}},
	})
	if _, ok := set.EvaluateDenyEarly(earlyReq("荷兰", "/"), "b.example.com"); ok {
		t.Fatal("策略只配了 A 站，B 站不应命中")
	}
	if _, ok := set.EvaluateDenyEarly(earlyReq("荷兰", "/"), "a.example.com"); !ok {
		t.Fatal("命中所属站点时应生效")
	}
}

// TestEvaluateDenyEarlyChallengeAndBan 验证 challenge / ban 同样可提前决策，
// 否则缓存命中路径上这两种动作也会失效。
func TestEvaluateDenyEarlyChallengeAndBan(t *testing.T) {
	for _, action := range []Action{ActionChallenge, ActionBan} {
		set := compileOne(t, Definition{
			ID: 9, Name: "动作 " + string(action), Site: "", Priority: 1, Enabled: true,
			Action: action,
			Conditions: Group{Logic: "and", Items: []Condition{
				{Field: "ua", Op: "contains", Value: "osint-scanner"},
			}},
		})
		m, ok := set.EvaluateDenyEarly(earlyReqWithUA("荷兰", "/", "Mozilla/5.0 (compatible; osint-scanner/1.0)"), "")
		if !ok || m.Action != action {
			t.Fatalf("动作 %s 应能提前决策，实际 ok=%v action=%v", action, ok, m)
		}
	}
}

// earlyReqWithUA 与 earlyReq 相同，但可指定 UA，用于验证以 UA 为条件的策略。
// 参数 country：归属地；参数 path：请求路径；参数 ua：User-Agent。
// 返回 *Request：策略上下文。
func earlyReqWithUA(country, path, ua string) *Request {
	req := earlyReq(country, path)
	req.UA = ua
	return req
}
