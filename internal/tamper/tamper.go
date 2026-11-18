// Package tamper 实现网页防篡改（对齐雷池「静态防护」能力的开源实现）。
//
// 场景：上游应用或主机被入侵后，攻击者篡改页面内容挂马/钓鱼。请求侧检测
// 管不住「上游自己吐出被改过的内容」，防篡改在响应链路上加一道完整性校验：
//
//  1. 管理员对受保护路径列表「生成基线」：WAF 主动向每个站点回源抓取响应，
//     记录 SHA-256 哈希与内容副本（有界存储）；
//  2. 之后每个经过的响应都与基线比对：
//     observe（默认）：不一致只记录事件（规则 ID tamper），内容原样下发；
//     block：不一致时用基线副本替换响应，客户端拿到的是可信内容，同时记录事件。
//
// 设计取舍（与文件级防篡改的差异）：
//   - WAF 是反代，拿不到源站文件系统，基线来自「回源抓取」——基线的可信度
//     等于生成基线那一刻的内容状态。因此基线只支持管理员手动刷新，
//     绝不做定时自动重抓：定时重抓会在内容已被篡改后把篡改版固化为新基线；
//   - 基线存内存（进程重启后需重新生成），单副本上限 max_bytes，条目总量
//     上限 256，总内存有界；
//   - 受保护路径跳过页面缓存：缓存 HIT 会绕过比对阶段，宁可放弃加速。
package tamper

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Settings 防篡改配置，Reload 时从设置表编译，数据面只读。
type Settings struct {
	Enabled  bool     // 总开关，默认关闭
	Mode     string   // observe 只记录 / block 替换为基线副本
	Paths    []string // 受保护的精确路径列表（如 /index.html、/static/logo.png）
	MaxBytes int      // 单个基线副本的字节上限，超过的路径只比对哈希不存副本
}

// 默认值。
const (
	DefaultMaxBytes   = 1 << 20  // 1MB：够覆盖绝大多数页面与静态资源
	MaxPaths          = 256      // 受保护路径条目上限（内存有界）
	MaxTotalCopyBytes = 64 << 20 // 基线副本总内存预算 64MB
)

// mode 常量。
const (
	ModeObserve = "observe"
	ModeBlock   = "block"
)

// DefaultSettings 返回默认配置（关闭、观察模式）。
func DefaultSettings() Settings {
	return Settings{Mode: ModeObserve, MaxBytes: DefaultMaxBytes}
}

// Baseline 一个受保护路径的可信基线。
type Baseline struct {
	Site        string `json:"site"`        // 站点名
	Path        string `json:"path"`        // 受保护路径（精确匹配）
	SHA256      string `json:"sha256"`      // 基线内容的哈希（hex）
	Size        int    `json:"size"`        // 基线内容字节数
	ContentType string `json:"contentType"` // 抓取时的 Content-Type
	FetchedAt   int64  `json:"fetchedAt"`   // 基线生成时间（Unix 毫秒）
	HasCopy     bool   `json:"hasCopy"`     // 是否存有内容副本（超上限的只比对哈希）

	copy []byte // 内容副本（block 模式替换用），不出 JSON
}

// FetchResult 一次基线抓取的结果（控制台刷新接口回显）。
type FetchResult struct {
	Site   string `json:"site"`
	Path   string `json:"path"`
	OK     bool   `json:"ok"`
	SHA256 string `json:"sha256,omitempty"`
	Size   int    `json:"size,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Manager 基线仓库。读多写少（只有刷新写），RWMutex 保护。
type Manager struct {
	mu        sync.RWMutex
	baselines map[string]*Baseline // key: site + "|" + path
}

// New 创建管理器。
func New() *Manager {
	return &Manager{baselines: map[string]*Baseline{}}
}

// key 基线的存储键。
func key(site, path string) string { return site + "|" + path }

// Store 写入（替换）一个基线。由基线刷新流程调用。
// 参数 site：站点名；path：受保护路径；body：抓取到的响应体；contentType：响应类型。
// 返回 error：路径条目超限或总内存超预算时返回错误（基线保持旧值）。
func (m *Manager) Store(site, path string, body []byte, contentType string) error {
	if len(body) == 0 {
		return fmt.Errorf("基线内容为空")
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.baselines[key(site, path)]; !exists && len(m.baselines) >= MaxPaths {
		return fmt.Errorf("基线条目已达上限 %d，请先清理不再需要的路径", MaxPaths)
	}
	// 总内存预算：新增副本前算一笔账，超额则拒绝（保旧值，宁可保护少不可撑爆）
	if len(body) > MaxTotalCopyBytes {
		return fmt.Errorf("基线内容超过总内存预算")
	}
	total := 0
	for _, b := range m.baselines {
		total += len(b.copy)
	}
	if total+len(body) > MaxTotalCopyBytes {
		return fmt.Errorf("基线副本总内存超过 %dMB 预算，请减少路径或调低 max_bytes", MaxTotalCopyBytes/1024/1024)
	}

	hasCopy := true // 能走到这里说明 body 非空且有界，副本一定可存
	m.baselines[key(site, path)] = &Baseline{
		Site: site, Path: path, SHA256: hash,
		Size: len(body), ContentType: contentType,
		FetchedAt: time.Now().UnixMilli(), HasCopy: hasCopy,
		copy: append([]byte(nil), body...),
	}
	return nil
}

// BaselineFor 取一个基线（比对阶段用）。
// 参数 site：站点名；path：请求路径。
// 返回 (*Baseline, bool)：存在返回 true。
func (m *Manager) BaselineFor(site, path string) (*Baseline, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.baselines[key(site, path)]
	return b, ok
}

// Copy 返回基线的内容副本（block 模式替换响应用）。
// 参数 b：基线。
// 返回 []byte：副本的拷贝（防止调用方改写内部状态）；无副本返回 nil。
func (m *Manager) Copy(b *Baseline) []byte {
	if b == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(b.copy) == 0 {
		return nil
	}
	return append([]byte(nil), b.copy...)
}

// Matches 比对响应体与基线是否一致。
// 参数 b：基线；参数 body：当前响应体。
// 返回 bool：哈希一致返回 true。
func (m *Manager) Matches(b *Baseline, body []byte) bool {
	if b == nil {
		return false
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]) == b.SHA256
}

// List 返回全部基线（按站点+路径排序）。
func (m *Manager) List() []Baseline {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Baseline, 0, len(m.baselines))
	for _, b := range m.baselines {
		// 不导出 copy 字段，只导出元信息
		out = append(out, Baseline{
			Site: b.Site, Path: b.Path, SHA256: b.SHA256,
			Size: b.Size, ContentType: b.ContentType,
			FetchedAt: b.FetchedAt, HasCopy: len(b.copy) > 0,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Site != out[j].Site {
			return out[i].Site < out[j].Site
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// Count 返回基线条目数。
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.baselines)
}

// Clear 清空全部基线（删除站点/停用功能后调用，避免残留失效基线）。
func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.baselines = map[string]*Baseline{}
}

// Applies 判断本请求是否受防篡改保护（响应侧 stage 与缓存跳过共用）。
// 参数 s：当前设置；参数 method：HTTP 方法；参数 path：请求路径。
// 返回 bool：未启用返回 false；非 GET/HEAD 返回 false（写请求响应无完整性基线意义）；
// 命中受保护路径（精确匹配）返回 true。
func Applies(s *Settings, method, path string) bool {
	if s == nil || !s.Enabled || len(s.Paths) == 0 {
		return false
	}
	if method != "GET" && method != "HEAD" {
		return false
	}
	for _, p := range s.Paths {
		if p == path {
			return true
		}
	}
	return false
}

// HashShort 哈希的短展示（控制台列表用）。
// 参数 hash：完整哈希。
// 返回 string：前 16 位；过短则原样返回。
func HashShort(hash string) string {
	if len(hash) >= 16 {
		return hash[:16]
	}
	return hash
}

// protectableContentTypes 值得做完整性比对的响应类型前缀（小写比较）。
var protectableContentTypes = []string{
	"text/html", "text/plain", "text/css", "application/javascript",
	"application/json", "application/xml", "text/xml", "image/", "font/",
}

// Transformable 判断响应是否在比对范围（状态码 + Content-Type）。
// 参数 status：响应状态码；参数 contentType：响应 Content-Type。
// 返回 bool：200 且类型在受保护集合返回 true。
func Transformable(status int, contentType string) bool {
	if status != 200 {
		return false
	}
	ct := strings.ToLower(contentType)
	for _, p := range protectableContentTypes {
		if strings.HasPrefix(ct, p) {
			return true
		}
	}
	return false
}
