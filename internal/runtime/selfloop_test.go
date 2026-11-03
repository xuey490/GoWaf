package runtime

import (
	"testing"

	"gowaf/internal/store/model"
)

func TestSelfListenPorts(t *testing.T) {
	ports := selfListenPorts("0.0.0.0:80", "[::]:443", "127.0.0.1:8899", "", "bogus")
	for _, p := range []string{"80", "443", "8899"} {
		if !has(ports, p) {
			t.Fatalf("端口 %s 应在集合中", p)
		}
	}
	if len(ports) != 3 {
		t.Fatalf("应恰好 3 个端口, got %v", ports)
	}
}

func TestUpstreamHitsPorts(t *testing.T) {
	ports := map[string]struct{}{"80": {}, "443": {}, "8899": {}}
	cases := []struct {
		raw  string
		want bool
	}{
		{"http://127.0.0.1:80", true},     // 事故场景：兜底站点自环
		{"127.0.0.1:80", true},            // 无 scheme 也必须识别
		{"http://127.0.0.1:8080", false},  // 正常 Apache 上游
		{"http://127.0.0.1:9797/", false}, // 后端直连
		{"http://example.com", true},      // 无端口 http 默认 80 → 命中
		{"ftp://127.0.0.1:80", true},      // scheme 无关，端口命中即拒绝
		{"http://127.0.0.1:8080/api", false},
		{"", false},
	}
	for _, c := range cases {
		if got := upstreamHitsPorts(c.raw, ports); got != c.want {
			t.Errorf("upstreamHitsPorts(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}

func TestDropSelfLoopSites(t *testing.T) {
	ports := map[string]struct{}{"80": {}, "443": {}}
	sites := []model.Site{
		{Name: "正常", Upstream: "http://127.0.0.1:8080"},
		{Name: "自环", Upstream: "http://127.0.0.1:80"},
		{Name: "兜底", Upstream: "http://127.0.0.1:80"},
	}
	got := dropSelfLoopSites(sites, ports)
	if len(got) != 1 || got[0].Name != "正常" {
		t.Fatalf("应只保留正常站点, got %v", got)
	}
	if got := dropSelfLoopSites(sites, map[string]struct{}{}); len(got) != 3 {
		t.Fatalf("空集合不应过滤")
	}
}
