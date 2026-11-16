package cache

import (
	"net/http"
	"testing"
	"time"
)

// TestEntriesSnapshotFields 验证「缓存列表」需要的关键字段都能从缓存键与对象还原：
// 站点、域名、路径、Query、文件名、大小、命中次数、写入/过期时间。
func TestEntriesSnapshotFields(t *testing.T) {
	m := newEnabledManager(t, nil, Settings{})
	now := time.Now()

	h := http.Header{}
	h.Set("Content-Type", "text/css; charset=utf-8")
	key := Key("shop", "Example.COM", "/assets/app.css", "v=2")
	m.Put(key, &Entry{Status: 200, Header: h, Body: []byte("body-8b!")}, 7200*time.Second)

	list := m.Entries(now)
	if len(list) != 1 {
		t.Fatalf("对象数 = %d, 期望 1", len(list))
	}
	e := list[0]
	if e.Site != "shop" || e.Host != "example.com" {
		t.Fatalf("站点/域名解析错误: site=%q host=%q", e.Site, e.Host)
	}
	if e.Path != "/assets/app.css" || e.Query != "v=2" {
		t.Fatalf("路径/Query 解析错误: path=%q query=%q", e.Path, e.Query)
	}
	if e.Filename != "app.css" {
		t.Fatalf("文件名 = %q, 期望 app.css", e.Filename)
	}
	if e.Size != int64(len("body-8b!")) {
		t.Fatalf("大小 = %d", e.Size)
	}
	if e.ContentType != "text/css; charset=utf-8" || e.Status != 200 {
		t.Fatalf("类型/状态异常: %q %d", e.ContentType, e.Status)
	}
	if e.TTLSeconds != 7200 {
		t.Fatalf("TTL 秒数 = %d, 期望 7200", e.TTLSeconds)
	}
	wantExpire := e.StoredAt + 7200*1000
	if e.ExpiresAt != wantExpire {
		t.Fatalf("过期时间 = %d, 期望 %d", e.ExpiresAt, wantExpire)
	}
	if e.RemainSeconds <= 7190 || e.RemainSeconds > 7200 {
		t.Fatalf("剩余有效期 = %d, 期望接近 7200", e.RemainSeconds)
	}
	if e.Hits != 0 {
		t.Fatalf("初始命中次数 = %d, 期望 0", e.Hits)
	}
}

// TestEntriesHitsAndOrder 验证命中次数累加与「最近写入在前」的排序。
func TestEntriesHitsAndOrder(t *testing.T) {
	m := newEnabledManager(t, nil, Settings{})
	old := Key("", "h", "/old.js", "")
	m.Put(old, &Entry{Status: 200, Body: []byte("x")}, time.Minute)
	// 拉开写入时间，避免同毫秒导致排序不确定
	time.Sleep(5 * time.Millisecond)
	fresh := Key("", "h", "/fresh.js", "")
	m.Put(fresh, &Entry{Status: 200, Body: []byte("y")}, time.Minute)

	for i := 0; i < 3; i++ {
		if _, ok := m.Get(old, time.Now()); !ok {
			t.Fatal("应命中")
		}
	}

	list := m.Entries(time.Now())
	if len(list) != 2 {
		t.Fatalf("对象数 = %d, 期望 2", len(list))
	}
	if list[0].Path != "/fresh.js" {
		t.Fatalf("排序错误，首条应为最近写入的 /fresh.js，实际 %s", list[0].Path)
	}
	if list[1].Path != "/old.js" || list[1].Hits != 3 {
		t.Fatalf("命中次数未累加: path=%s hits=%d", list[1].Path, list[1].Hits)
	}
}

// TestEntriesSkipExpired 验证已过期但未被后台清理的对象不出现在列表里。
func TestEntriesSkipExpired(t *testing.T) {
	m := newEnabledManager(t, nil, Settings{})
	key := Key("", "h", "/tmp.js", "")
	m.Put(key, &Entry{Status: 200, Body: []byte("z")}, 50*time.Millisecond)
	time.Sleep(80 * time.Millisecond)

	if list := m.Entries(time.Now()); len(list) != 0 {
		t.Fatalf("过期对象应被跳过，实际返回 %d 条", len(list))
	}
	// 统计也应以真实存储为准，过期对象此刻仍在（等 Cleanup），但列表不再展示
	if _, ok := m.Get(key, time.Now()); ok {
		t.Fatal("过期对象不应命中")
	}
}

// TestBaseName 验证文件名提取：目录型路径与根路径原样返回。
func TestBaseName(t *testing.T) {
	cases := map[string]string{
		"/assets/app.9f2c.js": "app.9f2c.js",
		"/":                   "/",
		"/blog/":              "/blog/",
		"/blog":               "blog",
		"":                    "/",
	}
	for in, want := range cases {
		if got := baseName(in); got != want {
			t.Fatalf("baseName(%q) = %q, 期望 %q", in, got, want)
		}
	}
}
