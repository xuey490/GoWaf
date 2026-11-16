package cache

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newEnabledManager 创建一个启用缓存的管理器并载入规则。
func newEnabledManager(t *testing.T, defs []RuleDef, st Settings) *Manager {
	t.Helper()
	st.Enabled = true
	m := NewManager()
	if err := m.Reload(defs, st); err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}
	return m
}

// TestMatchByKind 表格化验证五种路径规则匹配类型（guest 规则单独测）。
func TestMatchByKind(t *testing.T) {
	defs := []RuleDef{
		{ID: 1, Name: "后缀", Kind: KindSuffix, Pattern: ".css", Enabled: true},
		{ID: 2, Name: "精确", Kind: KindExact, Pattern: "/index.html", Enabled: true},
		{ID: 3, Name: "前缀", Kind: KindPrefix, Pattern: "/static/", Enabled: true},
		{ID: 4, Name: "正则", Kind: KindRegex, Pattern: `^/blog/[0-9]+$`, Enabled: true},
		{ID: 5, Name: "目录", Kind: KindDirectory, Pattern: "/blog", Enabled: true},
	}
	m := newEnabledManager(t, defs, Settings{})
	if m.RuleCount() != 5 {
		t.Fatalf("规则数 = %d, 期望 5", m.RuleCount())
	}

	cases := []struct {
		path string
		want bool
	}{
		{"/a/style.css", true},   // 后缀命中
		{"/a/style.cssx", false}, // 后缀不命中
		{"/index.html", true},    // 精确命中
		{"/index.htmlx", false},  // 精确不命中
		{"/static/app.js", true}, // 前缀命中
		{"/blog/123", true},      // 正则命中（也属目录内）
		{"/blog/abc", true},      // 目录前缀命中
		{"/blog", true},          // 目录本身命中
		{"/blogx", false},        // 目录不应误命中 /blogx
		{"/other", false},        // 无任何规则命中
	}
	for _, tc := range cases {
		r, ok := m.Match("shop", tc.path, true)
		if ok != tc.want {
			t.Errorf("Match(%q) = %v, 期望 %v", tc.path, ok, tc.want)
			continue
		}
		// 命中时默认 TTL 应补全为 300 秒
		if ok && r.TTLSecond != int(DefaultTTL) {
			t.Errorf("路径 %q 命中但 TTL 未补默认值: %d", tc.path, r.TTLSecond)
		}
	}
}

// TestMatchGuestRule 验证匿名全站规则：只对无 Cookie/Authorization 的访客生效。
func TestMatchGuestRule(t *testing.T) {
	m := newEnabledManager(t, []RuleDef{
		{ID: 1, Name: "匿名全站", Kind: KindGuest, Pattern: "*", Enabled: true},
	}, Settings{})
	if _, ok := m.Match("shop", "/anything", true); !ok {
		t.Fatal("匿名访客应命中 guest 规则")
	}
	if _, ok := m.Match("shop", "/anything", false); ok {
		t.Fatal("带 Cookie 的访客不该命中 guest 规则")
	}
	// guest 规则带路径前缀：只缓存前缀内的路径
	m = newEnabledManager(t, []RuleDef{
		{ID: 1, Name: "匿名限路径", Kind: KindGuest, Pattern: "/home", Enabled: true},
	}, Settings{})
	if _, ok := m.Match("shop", "/home/x", true); !ok {
		t.Fatal("前缀内的匿名请求应命中")
	}
	if _, ok := m.Match("shop", "/other", true); ok {
		t.Fatal("前缀外的匿名请求不该命中")
	}
}

// TestMatchRulePriority 验证多条规则命中时优先级低的先生效。
func TestMatchRulePriority(t *testing.T) {
	m := newEnabledManager(t, []RuleDef{
		{ID: 10, Name: "低优", Kind: KindPrefix, Pattern: "/a", Priority: 10, Enabled: true},
		{ID: 5, Name: "高优", Kind: KindPrefix, Pattern: "/a", Priority: 1, Enabled: true},
	}, Settings{})
	r, ok := m.Match("shop", "/a/b", true)
	if !ok || r.ID != 5 {
		t.Fatalf("优先级 1 应先生效: %+v ok=%v", r, ok)
	}
}

// TestMatchSiteScoping 验证规则站点隔离与空站点全局生效。
func TestMatchSiteScoping(t *testing.T) {
	m := newEnabledManager(t, []RuleDef{
		{ID: 1, Name: "shop专属", Site: "Shop", Kind: KindPrefix, Pattern: "/s", Enabled: true},
	}, Settings{})
	if _, ok := m.Match("blog", "/s/x", true); ok {
		t.Fatal("shop 的规则不该命中 blog 站点")
	}
	// 站点名忽略大小写
	if _, ok := m.Match("SHOP", "/s/x", true); !ok {
		t.Fatal("站点匹配应忽略大小写")
	}
}

// TestMatchDisabled 验证总开关关闭时一律不命中。
func TestMatchDisabled(t *testing.T) {
	m := NewManager()
	if err := m.Reload([]RuleDef{{ID: 1, Kind: KindPrefix, Pattern: "/a", Enabled: true}}, Settings{}); err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}
	if m.Enabled() {
		t.Fatal("未启用时 Enabled() 应为 false")
	}
	if _, ok := m.Match("shop", "/a/b", true); ok {
		t.Fatal("总开关关闭时不该命中")
	}
}

// TestReloadValidation 验证规则编译校验。
func TestReloadValidation(t *testing.T) {
	// 空规则种类默认按 suffix
	m := NewManager()
	if err := m.Reload([]RuleDef{{ID: 1, Pattern: ".html", Enabled: true}}, Settings{Enabled: true}); err != nil {
		t.Fatalf("空 Kind 应默认 suffix: %v", err)
	}
	if r, ok := m.Match("s", "/a.html", true); !ok || r.Kind != KindSuffix {
		t.Fatalf("空 Kind 规则匹配异常: %+v ok=%v", r, ok)
	}

	// 非法种类整体拒绝
	if err := m.Reload([]RuleDef{{ID: 1, Kind: "contains", Pattern: "x", Enabled: true}}, Settings{}); err == nil {
		t.Fatal("非法 Kind 应报错")
	}
	// 非法正则整体拒绝
	if err := m.Reload([]RuleDef{{ID: 1, Kind: KindRegex, Pattern: "([a-", Enabled: true}}, Settings{}); err == nil {
		t.Fatal("非法正则应报错")
	}
	// 空规则被跳过
	if err := m.Reload([]RuleDef{{ID: 1, Pattern: "   ", Enabled: true}}, Settings{}); err != nil {
		t.Fatalf("空规则应被跳过: %v", err)
	}
	if m.RuleCount() != 0 {
		t.Fatalf("空规则不应编入: %d", m.RuleCount())
	}
}

// TestKeyDeterministic 验证缓存键生成：小写 Host、四段拼接。
func TestKeyDeterministic(t *testing.T) {
	k1 := Key("shop", "Example.COM", "/a", "x=1")
	k2 := Key("shop", "example.com", "/a", "x=1")
	if k1 != k2 {
		t.Fatalf("Host 大小写应归一: %q vs %q", k1, k2)
	}
	if k1 != "shop|example.com|/a?x=1" {
		t.Fatalf("键格式异常: %q", k1)
	}
	// 不同 query 不串味
	if Key("shop", "h", "/a", "x=1") == Key("shop", "h", "/a", "x=2") {
		t.Fatal("不同 query 应生成不同键")
	}
}

// TestPutGetRoundtrip 验证写入、命中、统计。
func TestPutGetRoundtrip(t *testing.T) {
	m := newEnabledManager(t, nil, Settings{})
	key := Key("shop", "h", "/a", "")
	body := []byte("hello cache")

	m.Put(key, &Entry{Status: 200, Header: nil, Body: body}, time.Minute)
	e, ok := m.Get(key, time.Now())
	if !ok || e == nil {
		t.Fatal("刚写入的对象应命中")
	}
	if e.Status != 200 || !bytes.Equal(e.Body, body) {
		t.Fatalf("命中对象内容异常: %+v", e)
	}
	st := m.Stats()
	if st.Hits != 1 || st.Misses != 0 || st.Entries != 1 {
		t.Fatalf("统计异常: %+v", st)
	}

	// 未命中的键计入 miss
	if _, ok := m.Get(Key("shop", "h", "/b", ""), time.Now()); ok {
		t.Fatal("不存在的键不该命中")
	}
	if st := m.Stats(); st.Misses != 1 {
		t.Fatalf("miss 计数异常: %+v", st)
	}
}

// TestEntryExpiry 验证 TTL 过期后返回未命中并移除对象。
func TestEntryExpiry(t *testing.T) {
	m := newEnabledManager(t, nil, Settings{})
	key := Key("s", "h", "/ttl", "")
	m.Put(key, &Entry{Status: 200, Body: []byte("x")}, 50*time.Millisecond)

	// 未过期：命中
	if _, ok := m.Get(key, time.Now()); !ok {
		t.Fatal("TTL 内应命中")
	}
	// 过期后：未命中，且对象已被移除
	if _, ok := m.Get(key, time.Now().Add(time.Second)); ok {
		t.Fatal("过期后不该命中")
	}
	if _, n := m.store.size(); n != 0 {
		t.Fatalf("过期对象应被移除，剩余 %d", n)
	}
}

// TestPutGuardrails 验证写入防线：未启用、空体、超单对象上限直接丢弃。
func TestPutGuardrails(t *testing.T) {
	// 未启用：不写
	m := NewManager()
	m.Put("k", &Entry{Status: 200, Body: []byte("x")}, time.Minute)
	if _, ok := m.Get("k", time.Now()); ok {
		t.Fatal("未启用时不该写入")
	}

	// 空体与 nil：不写
	m = newEnabledManager(t, nil, Settings{})
	m.Put("k1", &Entry{Status: 200, Body: nil}, time.Minute)
	m.Put("k2", nil, time.Minute)
	if _, ok := m.Get("k1", time.Now()); ok {
		t.Fatal("空体不该写入")
	}

	// 超过单对象上限：不写
	m = newEnabledManager(t, nil, Settings{MaxObjectBytes: 10})
	m.Put("big", &Entry{Status: 200, Body: bytes.Repeat([]byte("x"), 11)}, time.Minute)
	if _, ok := m.Get("big", time.Now()); ok {
		t.Fatal("超过单对象上限不该写入")
	}
	// 上限内的正常写入
	m.Put("ok", &Entry{Status: 200, Body: bytes.Repeat([]byte("x"), 10)}, time.Minute)
	if _, ok := m.Get("ok", time.Now()); !ok {
		t.Fatal("等于上限的对象应写入")
	}
}

// TestLRUEviction 验证容量超限时从最旧对象开始淘汰。
func TestLRUEviction(t *testing.T) {
	m := newEnabledManager(t, nil, Settings{MaxBytes: 100, MaxObjectBytes: 100})
	ent := func() *Entry { return &Entry{Status: 200, Body: bytes.Repeat([]byte("x"), 40)} }
	m.Put("k1", ent(), time.Minute)
	m.Put("k2", ent(), time.Minute)
	m.Put("k3", ent(), time.Minute) // 120 字节 > 100，最旧的 k1 被淘汰

	if _, ok := m.Get("k1", time.Now()); ok {
		t.Fatal("最旧的 k1 应被淘汰")
	}
	for _, k := range []string{"k2", "k3"} {
		if _, ok := m.Get(k, time.Now()); !ok {
			t.Fatalf("%s 不该被淘汰", k)
		}
	}
	if st := m.Stats(); st.Evictions != 1 {
		t.Fatalf("淘汰计数 = %d, 期望 1", st.Evictions)
	}

	// 访问 k3 再访问 k2（k2 成为最近使用）：新对象把最旧的 k3 挤出
	for _, k := range []string{"k3", "k2"} {
		if _, ok := m.Get(k, time.Now()); !ok {
			t.Fatalf("%s 不该被淘汰", k)
		}
	}
	m.Put("k2-again", &Entry{Status: 200, Body: ent().Body}, time.Minute) // 120 字节 > 100, 淘汰 k3
	if _, ok := m.Get("k3", time.Now()); ok {
		t.Fatal("最久未访问的 k3 应被淘汰")
	}
	if _, ok := m.Get("k2", time.Now()); !ok {
		t.Fatal("刚访问过的 k2 应保留")
	}
}

// TestPurge 验证按站点与路径前缀清除。
func TestPurge(t *testing.T) {
	m := newEnabledManager(t, nil, Settings{})
	ent := &Entry{Status: 200, Body: []byte("x")}
	m.Put(Key("shop", "h", "/a", ""), ent, time.Minute)
	m.Put(Key("shop", "h", "/b", ""), ent, time.Minute)
	m.Put(Key("blog", "h", "/a", ""), ent, time.Minute)

	// 按站点清除
	if n := m.Purge("shop", ""); n != 2 {
		t.Fatalf("清除 shop 应删 2 个，实际 %d", n)
	}
	if _, ok := m.Get(Key("blog", "h", "/a", ""), time.Now()); !ok {
		t.Fatal("blog 的对象不应被清除")
	}
	// 按路径前缀清除（跨站点）
	if n := m.Purge("", "/a"); n != 1 {
		t.Fatalf("按前缀清除应删 1 个，实际 %d", n)
	}
	// 清空后 Entries 归零
	if st := m.Stats(); st.Entries != 0 {
		t.Fatalf("Entries = %d, 期望 0", st.Entries)
	}
}

// TestCleanupExpired 验证批量清理过期对象。
func TestCleanupExpired(t *testing.T) {
	m := newEnabledManager(t, nil, Settings{})
	ent := &Entry{Status: 200, Body: []byte("x")}
	m.Put(Key("s", "h", "/a", ""), ent, time.Minute)
	m.Put(Key("s", "h", "/b", ""), ent, time.Minute)
	// 时间快进到全部过期
	m.Cleanup(time.Now().Add(2 * time.Minute))
	if _, n := m.store.size(); n != 0 {
		t.Fatalf("过期对象应被清理，剩余 %d", n)
	}
}

// TestUpdateSettingsKeepsRules 验证只更新设置时规则保留。
func TestUpdateSettingsKeepsRules(t *testing.T) {
	m := NewManager()
	if err := m.Reload([]RuleDef{{ID: 1, Kind: KindPrefix, Pattern: "/a", TTL: 30 * time.Second, Enabled: true}}, Settings{}); err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}
	if err := m.UpdateSettings(Settings{Enabled: true}); err != nil {
		t.Fatalf("UpdateSettings 失败: %v", err)
	}
	if m.RuleCount() != 1 {
		t.Fatalf("规则应保留: %d", m.RuleCount())
	}
	if r, ok := m.Match("s", "/a/x", true); !ok || r.TTL != 30*time.Second {
		t.Fatalf("规则 TTL 应保留: %+v ok=%v", r, ok)
	}
}

// TestCompressible 验证可压缩内容类型判定。
func TestCompressible(t *testing.T) {
	yes := []string{
		"text/html; charset=utf-8", "text/plain", "text/css",
		"application/javascript", "application/json", "application/xml",
		"image/svg+xml", "application/xhtml+xml", "APPLICATION/JSON",
	}
	for _, ct := range yes {
		if !Compressible(ct) {
			t.Errorf("%q 应可压缩", ct)
		}
	}
	no := []string{"image/png", "image/jpeg", "video/mp4", "application/octet-stream", ""}
	for _, ct := range no {
		if Compressible(ct) {
			t.Errorf("%q 不该可压缩", ct)
		}
	}
}

// TestAcceptsEncoding 验证 Accept-Encoding 解析（含 q 值与通配符）。
func TestAcceptsEncoding(t *testing.T) {
	cases := []struct {
		header, enc string
		want        bool
	}{
		{"gzip", "gzip", true},
		{"GZIP, deflate", "gzip", true},
		{"br, gzip", "br", true},
		{"br;q=0", "br", false},
		{"gzip;q=0.000", "gzip", false},
		{"*;q=0.5", "gzip", true},
		{"*;q=0", "gzip", false},
		{"deflate", "gzip", false},
		{"", "gzip", false},
	}
	for _, tc := range cases {
		if got := AcceptsEncoding(tc.header, tc.enc); got != tc.want {
			t.Errorf("AcceptsEncoding(%q, %q) = %v, 期望 %v", tc.header, tc.enc, got, tc.want)
		}
	}
}

// chaoticBytes 生成确定性伪随机字节（LCG），用于不可压缩数据。
func chaoticBytes(n int) []byte {
	out := make([]byte, n)
	var x uint32 = 1
	for i := range out {
		x = x*1664525 + 1013904223
		out[i] = byte(x >> 24)
	}
	return out
}

// TestCompress 验证压缩选择：brotli 优先、gzip 回落、不可压缩跳过。
func TestCompress(t *testing.T) {
	body := bytes.Repeat([]byte("abcdefghij"), 500) // 高度可重复

	// 允许 br 且客户端接受：优先 brotli
	data, enc, ok := Compress(body, "br, gzip", true)
	if !ok || enc != "br" || len(data) >= len(body) {
		t.Fatalf("brotli 压缩异常: ok=%v enc=%q %d/%d", ok, enc, len(data), len(body))
	}
	// preferBrotli=false：回落 gzip
	data, enc, ok = Compress(body, "br, gzip", false)
	if !ok || enc != "gzip" || len(data) >= len(body) {
		t.Fatalf("gzip 压缩异常: ok=%v enc=%q", ok, enc)
	}
	// 只接受 gzip：即便 preferBrotli=true 也走 gzip
	data, enc, ok = Compress(body, "gzip", true)
	if !ok || enc != "gzip" {
		t.Fatalf("应回落 gzip: ok=%v enc=%q", ok, enc)
	}
	// 什么都不接受：原样返回
	data, enc, ok = Compress(body, "identity", false)
	if ok || enc != "" || !bytes.Equal(data, body) {
		t.Fatalf("不接受压缩时应原样返回: ok=%v enc=%q", ok, enc)
	}
	// 伪随机数据压缩后反而更大：放弃压缩
	raw := chaoticBytes(4096)
	data, enc, ok = Compress(raw, "gzip, br", true)
	if ok || !bytes.Equal(data, raw) {
		t.Fatalf("不可压缩数据应原样返回: ok=%v", ok)
	}
}

// TestWebPConvertible 验证可转换图片类型判定。
func TestWebPConvertible(t *testing.T) {
	for _, ct := range []string{"image/jpeg", "image/jpg", "image/png", "IMAGE/PNG", "image/png; charset=x"} {
		if !WebPConvertible(ct) {
			t.Errorf("%q 应可转 WebP", ct)
		}
	}
	for _, ct := range []string{"image/gif", "image/webp", "text/html", ""} {
		if WebPConvertible(ct) {
			t.Errorf("%q 不该可转 WebP", ct)
		}
	}
}

// TestConvertWebP 验证 PNG → WebP 实际转换。
func TestConvertWebP(t *testing.T) {
	// 纯色大图：PNG 有逐行结构开销，WebP 无损应显著更小
	img := image.NewRGBA(image.Rect(0, 0, 512, 512))
	for x := 0; x < 512; x++ {
		for y := 0; y < 512; y++ {
			img.Set(x, y, color.RGBA{R: 240, G: 120, B: 60, A: 255})
		}
	}
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatalf("生成 PNG 失败: %v", err)
	}
	webp, ok := ConvertWebP(pngBuf.Bytes())
	if !ok {
		t.Fatalf("PNG 应转换成功 (png=%d bytes)", pngBuf.Len())
	}
	if len(webp) >= pngBuf.Len() {
		t.Fatalf("WebP 应小于 PNG: %d vs %d", len(webp), pngBuf.Len())
	}
	// WebP 魔数：RIFF....WEBP
	if len(webp) < 12 || string(webp[0:4]) != "RIFF" || string(webp[8:12]) != "WEBP" {
		t.Fatal("输出不是合法 WebP 魔数")
	}

	// 非图片数据：失败回退
	if _, ok := ConvertWebP([]byte("not an image at all")); ok {
		t.Fatal("非图片数据不该转换成功")
	}
}

// TestMediaType 验证媒体类型提取。
func TestMediaType(t *testing.T) {
	if mt := MediaType("text/html; charset=utf-8"); mt != "text/html" {
		t.Fatalf("MediaType = %q", mt)
	}
	if mt := MediaType("application/json"); mt != "application/json" {
		t.Fatalf("MediaType = %q", mt)
	}
}

// TestDecodeSettings 验证设置表字符串解析与缺省回落。
func TestDecodeSettings(t *testing.T) {
	values := map[string]string{
		"cache.enabled":            "true",
		"cache.max_bytes":          "104857600",
		"cache.max_object_bytes":   "5242880",
		"cache.compress":           "false",
		"cache.brotli":             "false",
		"cache.webp":               "true",
		"cache.compress_min_bytes": "2048",
		"cache.default_ttl":        "120",
	}
	get := func(k string) string { return values[k] }
	st := DecodeSettings(get, Settings{Compress: true, Brotli: true})

	if !st.Enabled || st.MaxBytes != 104857600 || st.MaxObjectBytes != 5242880 {
		t.Fatalf("数值键解析异常: %+v", st)
	}
	if st.Compress || st.Brotli || !st.WebP {
		t.Fatalf("布尔键解析异常: %+v", st)
	}
	if st.CompressMinBytes != 2048 || st.DefaultTTL != 120*time.Second {
		t.Fatalf("其余键解析异常: %+v", st)
	}

	// 非法值回落缺省
	bad := DecodeSettings(func(k string) string {
		return map[string]string{
			"cache.enabled": "yes!", "cache.max_bytes": "-5",
			"cache.default_ttl": "abc",
		}[k]
	}, Settings{Enabled: false, MaxBytes: DefaultMaxBytes, DefaultTTL: 300 * time.Second})
	if bad.Enabled {
		t.Fatal("非法布尔值应回落缺省")
	}
	if bad.MaxBytes != DefaultMaxBytes {
		t.Fatal("负数应回落缺省")
	}
	if bad.DefaultTTL != 300*time.Second {
		t.Fatal("非法 TTL 应回落缺省")
	}
}

// TestEntryHelpers 验证 Entry 的 WriteTo / CopyBody / MarshalJSON。
func TestEntryHelpers(t *testing.T) {
	h := map[string][]string{"X-Custom": {"v1", "v2"}}
	e := &Entry{Status: 200, Header: h, Body: []byte("payload"), TTL: time.Minute, StoredAt: time.Now()}

	rec := httptest.NewRecorder()
	e.WriteTo(rec)
	if rec.Code != 200 || rec.Body.String() != "payload" {
		t.Fatalf("WriteTo 输出异常: code=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Values("X-Custom"); len(got) != 2 {
		t.Fatalf("多值头应完整写出: %v", got)
	}

	// CopyBody 返回副本：修改副本不影响缓存
	cp := e.CopyBody()
	cp[0] = 'X'
	if e.Body[0] != 'p' {
		t.Fatal("CopyBody 不该共享底层数组")
	}
	if e.CopyBody() == nil && e != nil {
		t.Fatal("非 nil Entry 的 CopyBody 不该为 nil")
	}

	// MarshalJSON 不携带 Body 全文
	js, err := e.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 失败: %v", err)
	}
	s := string(js)
	if strings.Contains(s, "payload") {
		t.Fatal("JSON 不该包含响应体全文")
	}
	if !strings.Contains(s, `"status":200`) {
		t.Fatalf("JSON 应含状态码: %s", s)
	}
	// nil Entry 序列化为 null
	if js, err := (*Entry)(nil).MarshalJSON(); err != nil || string(js) != "null" {
		t.Fatalf("nil Entry 应序列化为 null: %s err=%v", js, err)
	}
}
