package edge

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"gowaf/internal/policy"
)

// buildSet 编译一组策略，供数据面接入层用例复用。
// 参数 t：测试句柄；参数 defs：策略定义。
// 返回 *policy.Set：编译后的快照。
func buildSet(t *testing.T, defs ...policy.Definition) *policy.Set {
	t.Helper()
	set, err := policy.Compile(defs)
	if err != nil {
		t.Fatalf("策略编译失败: %v", err)
	}
	return set
}

// TestEarlyPolicyVerdictCoversCachedAssets 是「缓存绕过策略」缺陷的接入层回归。
//
// 缺陷成因：数据面顺序为「缓存查找 → 读 body → 完整策略求值」，缓存命中直接
// 返回响应，完整求值永远走不到。修复后由 earlyPolicyVerdict 在缓存查找之前
// 给出结论，本用例锁定该行为：同一请求在接入层必须拿到拦截结论。
func TestEarlyPolicyVerdictCoversCachedAssets(t *testing.T) {
	set := buildSet(t, policy.Definition{
		ID: 1, Name: "拦截-荷兰", Site: "", Priority: 100, Enabled: true,
		Action: policy.ActionBlock,
		Conditions: policy.Group{Logic: "and", Items: []policy.Condition{
			{Field: "country", Op: "contains", Value: "荷兰"},
		}},
	})

	r := httptest.NewRequest(http.MethodGet, "http://goframe.example/assets/index-DmplYGAM.js", nil)
	m, ok := earlyPolicyVerdict(set, r, "45.138.12.19", "荷兰", "goframe.example", false)
	if !ok {
		t.Fatal("已缓存静态资源的请求应被提前拦下，实际未命中")
	}
	if m.Action != policy.ActionBlock {
		t.Fatalf("动作应为 block，实际 %s", m.Action)
	}

	// 非目标地域照常放行，确保修复没有把正常流量一起拦掉。
	if _, ok := earlyPolicyVerdict(set, r, "1.2.3.4", "中国 广东 深圳", "goframe.example", false); ok {
		t.Fatal("非荷兰来源不应命中")
	}
}

// TestEarlyPolicyVerdictRespectsObserveMode 验证观察模式下不提前拦截：
// 观察模式的语义是「只记录不拦」，若在这里返回拦截结论就会把观察模式变成真拦。
func TestEarlyPolicyVerdictRespectsObserveMode(t *testing.T) {
	set := buildSet(t, policy.Definition{
		ID: 1, Name: "拦截-荷兰", Site: "", Priority: 100, Enabled: true,
		Action: policy.ActionBlock,
		Conditions: policy.Group{Logic: "and", Items: []policy.Condition{
			{Field: "country", Op: "eq", Value: "荷兰"},
		}},
	})
	r := httptest.NewRequest(http.MethodGet, "http://a.example/", nil)
	if _, ok := earlyPolicyVerdict(set, r, "45.138.12.19", "荷兰", "a.example", true); ok {
		t.Fatal("观察模式下不应提前拦截")
	}
}

// TestEarlyPolicyVerdictHandlesEmptySet 验证空策略集与 nil 快照不会 panic。
// 这两个分支在服务刚启动、策略尚未加载时是常态。
func TestEarlyPolicyVerdictHandlesEmptySet(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://a.example/", nil)
	if _, ok := earlyPolicyVerdict(nil, r, "1.2.3.4", "荷兰", "a.example", false); ok {
		t.Fatal("nil 快照不应命中")
	}
	set := buildSet(t)
	if _, ok := earlyPolicyVerdict(set, r, "1.2.3.4", "荷兰", "a.example", false); ok {
		t.Fatal("空策略集不应命中")
	}
}

// TestEarlyPolicyVerdictSkipsBodyDependent 验证接入层不会替依赖 body 的策略做决定：
// 这类策略必须留给第 8 步的完整求值，否则会出现「正文没读就按 body 条件拦人」。
func TestEarlyPolicyVerdictSkipsBodyDependent(t *testing.T) {
	set := buildSet(t, policy.Definition{
		ID: 1, Name: "按 body 拦截", Site: "", Priority: 1, Enabled: true,
		Action: policy.ActionBlock,
		Conditions: policy.Group{Logic: "and", Items: []policy.Condition{
			{Field: "body", Op: "contains", Value: "union select"},
			{Field: "path", Op: "prefix", Value: "/api"},
		}},
	})
	r := httptest.NewRequest(http.MethodPost, "http://a.example/api/login", nil)
	if _, ok := earlyPolicyVerdict(set, r, "1.2.3.4", "荷兰", "a.example", false); ok {
		t.Fatal("依赖 body 的策略不应在接入层提前决策")
	}
}

// TestEarlyPolicyVerdictBlockSpecificIP 验证按 IP 封禁同样覆盖缓存路径，
// 这是最常用的一类策略，且完全不依赖 body。
func TestEarlyPolicyVerdictBlockSpecificIP(t *testing.T) {
	set := buildSet(t, policy.Definition{
		ID: 2, Name: "拦截-指定IP", Site: "", Priority: 20, Enabled: true,
		Action: policy.ActionBlock,
		Conditions: policy.Group{Logic: "and", Items: []policy.Condition{
			{Field: "client_ip", Op: "eq", Value: "96.126.130.106"},
		}},
	})
	r := httptest.NewRequest(http.MethodGet, "http://a.example/assets/app.css", nil)
	if _, ok := earlyPolicyVerdict(set, r, "96.126.130.106", "美国", "a.example", false); !ok {
		t.Fatal("指定 IP 的拦截策略应覆盖缓存命中的静态资源")
	}
	if _, ok := earlyPolicyVerdict(set, r, "96.126.130.107", "美国", "a.example", false); ok {
		t.Fatal("相邻 IP 不应被误拦")
	}
}
