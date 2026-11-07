package detect

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	crsv4 "github.com/corazawaf/coraza-coreruleset/crs/v4"
)

/* ============================================================ CRS 上游版本检查 */

// CRS 规则集编译期内嵌在二进制里（coraza-coreruleset/crs/v4 依赖），运行期无法热更新；
// 这里提供「上游是否有新版本」的只读检查，供管理后台提示管理员升级依赖发版。
// 上游数据源按优先级回退：
//  1. GitHub release 接口：信息最全（版本号 + release 页 + 发布时间），未认证限 60 次/小时/IP；
//  2. Go 模块代理 @latest：只有版本号与发布时间；proxy.golang.org 在国内常不可达，
//     所以再挂一个 goproxy.cn 国内镜像兜底。

// crsReleaseAPI OWASP CRS 官方仓库的最新正式 release 接口（自动排除 draft/prerelease）。
// 测试里会被替换成 httptest 服务地址。
var crsReleaseAPI = "https://api.github.com/repos/coreruleset/coreruleset/releases/latest"

// crsProxyAPIs Go 模块代理兜底源，按顺序尝试：官方代理 → 国内镜像。
var crsProxyAPIs = []string{
	"https://proxy.golang.org/github.com/corazawaf/coraza-coreruleset/crs/v4/@latest",
	"https://goproxy.cn/github.com/corazawaf/coraza-coreruleset/crs/v4/@latest",
}

// crsReleasesPage 上游 release 列表页：兜底源拿不到单版链接时，给前端一个可跳转的地址。
const crsReleasesPage = "https://github.com/coreruleset/coreruleset/releases"

// crsCheckClient 检查更新专用 HTTP 客户端。
// 15 秒超时：源站不可达时让按钮尽快报错，而不是把面板请求挂住。
var crsCheckClient = &http.Client{Timeout: 15 * time.Second}

// crsUpdateTTL 检查结果缓存时长：面板按钮可能被反复点，10 分钟内的点击不重复出网。
const crsUpdateTTL = 10 * time.Minute

// 缓存状态：一次成功的检查结果与检查时刻；cache 为 nil 表示从未成功检查过。
// 失败不缓存——网络抖动时下次点击还能立刻重试。
var (
	crsUpdateMu      sync.Mutex
	crsUpdateCache   *CRSUpdateInfo
	crsUpdateChecked time.Time
)

// CRSUpdateInfo 一次 CRS 上游版本检查的结果。
type CRSUpdateInfo struct {
	Current         string `json:"current"`         // 当前内嵌版本（如 v4.30.0），来自编译期依赖
	Latest          string `json:"latest"`          // 上游最新正式版本号
	UpdateAvailable bool   `json:"updateAvailable"` // 上游版本是否比当前新
	ReleaseURL      string `json:"releaseUrl"`      // 上游 release 页面；走兜底源时退化为列表页
	PublishedAt     string `json:"publishedAt"`     // 上游发布时间（RFC3339）
	CheckedAt       string `json:"checkedAt"`       // 本次实际出网检查完成的时间（RFC3339）
	Cached          bool   `json:"cached"`          // true 表示命中缓存，未真正出网
}

// ghRelease GitHub releases/latest 接口只取需要的三个字段。
type ghRelease struct {
	TagName     string `json:"tag_name"`     // 版本 tag，如 v4.30.0
	HTMLURL     string `json:"html_url"`     // 单版 release 页面
	PublishedAt string `json:"published_at"` // 发布时间（RFC3339）
}

// goProxyLatest Go 模块代理 @latest 接口的响应（注意字段名是大写开头）。
type goProxyLatest struct {
	Version string `json:"Version"` // 最新版本号
	Time    string `json:"Time"`    // 该版本的提交时间
}

// CheckCRSUpdate 检查 OWASP CRS 上游是否有新版本。
// 参数 force：true 忽略缓存强制出网（面板「重新检查」按钮用）；false 时 10 分钟内直接回缓存。
// 返回 (结果, nil)：检查成功；返回 (nil, error)：GitHub 与全部模块代理都不可达。
// 并发安全：全局互斥，多个请求同时点按钮也只有第一个真正出网，其余等结果。
func CheckCRSUpdate(ctx context.Context, force bool) (*CRSUpdateInfo, error) {
	crsUpdateMu.Lock()
	defer crsUpdateMu.Unlock()
	if !force && crsUpdateCache != nil && time.Since(crsUpdateChecked) < crsUpdateTTL {
		info := *crsUpdateCache
		info.Cached = true
		return &info, nil
	}
	info, err := fetchCRSUpdate(ctx)
	if err != nil {
		return nil, err
	}
	crsUpdateCache = info
	crsUpdateChecked = time.Now()
	return info, nil
}

// fetchCRSUpdate 实际出网检查：先打 GitHub release，失败逐个回退模块代理。
// 无参数；返回填充好的结果；GitHub 与全部代理都失败时返回最后一个错误链。
func fetchCRSUpdate(ctx context.Context) (*CRSUpdateInfo, error) {
	info := &CRSUpdateInfo{
		Current:    crsv4.Version, // 编译期内嵌的版本，与引擎是否启用无关
		ReleaseURL: crsReleasesPage,
	}

	rel, err := fetchGHRelease(ctx, crsReleaseAPI)
	if err == nil {
		info.Latest = rel.TagName
		info.PublishedAt = rel.PublishedAt
		if rel.HTMLURL != "" {
			info.ReleaseURL = rel.HTMLURL
		}
	} else {
		// GitHub 不可达：逐个试模块代理，任一成功即用其版本号
		for _, u := range crsProxyAPIs {
			p, perr := fetchGoProxy(ctx, u)
			if perr == nil {
				info.Latest = p.Version
				info.PublishedAt = p.Time
				break
			}
			err = fmt.Errorf("%w; %s: %v", err, proxyHost(u), perr)
		}
		if info.Latest == "" {
			return nil, fmt.Errorf("全部数据源不可达: %w", err)
		}
	}

	info.UpdateAvailable = crsVersionLess(info.Current, info.Latest)
	info.CheckedAt = time.Now().UTC().Format(time.RFC3339)
	return info, nil
}

// fetchGHRelease 拉取并解析 GitHub releases/latest 响应。
// 参数 ctx：请求上下文（随面板请求取消）；url：release API 地址。
// 返回 (release, nil)：解析成功且 tag_name 非空；(nil, error)：网络/状态码/解析失败。
func fetchGHRelease(ctx context.Context, url string) (*ghRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := crsCheckClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// 403 多为未认证限流（60 次/小时/IP），直接把状态码透给上层拼错误消息
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var rel ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&rel); err != nil {
		return nil, err
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("响应缺少 tag_name")
	}
	return &rel, nil
}

// fetchGoProxy 拉取并解析 Go 模块代理的 @latest 响应。
// 参数 ctx：请求上下文；url：模块代理地址。
// 返回 (latest, nil)：解析成功且 Version 非空；(nil, error)：网络/状态码/解析失败。
func fetchGoProxy(ctx context.Context, url string) (*goProxyLatest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := crsCheckClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var p goProxyLatest
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<10)).Decode(&p); err != nil {
		return nil, err
	}
	if p.Version == "" {
		return nil, fmt.Errorf("响应缺少 Version")
	}
	return &p, nil
}

// proxyHost 取 URL 的主机部分用于错误消息；解析失败原样返回。
// 参数 u：代理地址。返回：主机名或原始串。
func proxyHost(u string) string {
	if i := strings.Index(u, "//"); i >= 0 {
		rest := u[i+2:]
		if j := strings.Index(rest, "/"); j >= 0 {
			return rest[:j]
		}
	}
	return u
}

// crsVersionLess 判断 a 是否小于 b：语义化版本逐段数值比较。
// 参数 a、b：形如 "v4.30.0" 的版本号，容忍前导 v、缺失段（v4 等价 v4.0.0）与 "-rc1" 后缀。
// 返回 bool：a < b 时为 true。任一解析失败时退化为归一化字符串不等——
// 宁可多报一次「有更新」也不能漏报，字符串相等则视为没有更新。
func crsVersionLess(a, b string) bool {
	pa, oka := parseCRSVer(a)
	pb, okb := parseCRSVer(b)
	if !oka || !okb {
		return crsNormVer(a) != crsNormVer(b)
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return false
}

// parseCRSVer 把 "v4.30.0" 解析成三段整数。
// 参数 v：原始版本号，容忍前后空白、前导 v 与 "-" 后的第一个后缀段。
// 返回 ([3]int, bool)：解析结果与是否成功；任何一段含非数字即为失败。
func parseCRSVer(v string) ([3]int, bool) {
	var out [3]int
	v = strings.SplitN(strings.TrimSpace(v), "-", 2)[0] // 去掉 -rc1/-beta 之类的后缀
	v = strings.TrimPrefix(v, "v")
	if v == "" {
		return out, false
	}
	parts := strings.Split(v, ".")
	if len(parts) > 3 {
		return out, false
	}
	for i, p := range parts {
		if p == "" {
			return out, false
		}
		n := 0
		for _, r := range p {
			if r < '0' || r > '9' {
				return out, false
			}
			n = n*10 + int(r-'0')
		}
		out[i] = n
	}
	return out, true
}

// crsNormVer 版本号的归一化形式：去空白、统一小写、去前导 v。
// 仅在版本解析失败退化字符串比较时使用。
// 参数 v：原始版本号。返回：归一化后的字符串。
func crsNormVer(v string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(v)), "v")
}
