// Package geo 提供离线 IP 归属地查询。
//
// 数据来自 ip2region 的 IPv4 段库，构建期压成「国家 + 省 + 市」的紧凑区间表
// （见 cmd/geo-gen），运行时只做一次二分查找，不联网、不依赖外部文件。
// 设计取舍：只保留归属地展示需要的字段，不存 ISP 和经纬度，把体积压到 1MB 内。
package geo

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io"
	"net/netip"
	"strings"
	"sync/atomic"
)

// magic 自定义二进制格式的魔数（Go WAF Geo v1），parse 用它校验数据合法性。
const magic = "GWG1"

// entry 是一个闭区间 [Start, End]，Region 是地区名在 region 表里的下标。
type entry struct {
	Start  uint32 // 区间起始 IPv4 地址（数值形式，1.2.3.4 → 0x01020304）
	End    uint32 // 区间结束 IPv4 地址（含端点）
	Region uint16 // 地区名在 regions 表中的下标
}

// DB 是不可变快照，可并发读。
type DB struct {
	ranges  []entry  // IP 区间表，按 Start 升序存放，供二分查找
	regions []string // 地区名表，entry.Region 指向其下标
	loaded  bool     // 是否成功解析过数据；false 时所有查询返回空串
}

var (
	// shared 进程级共享的单例；atomic.Pointer 保证并发访问下的安全发布。
	shared atomic.Pointer[DB]
)

// Default 返回进程级共享的单例实例，首次调用时解包内嵌数据。
// 参数：无。
// 返回 *DB：首次调用会解压并解析 packed 后缓存到 shared，之后直接返回缓存；
// 解析失败时返回 Ready 为 false 的空 DB（不会返回 nil）。
func Default() *DB {
	if d := shared.Load(); d != nil {
		return d
	}
	d := load()
	shared.Store(d)
	return d
}

// Load 显式解包一份独立的 DB。
// 参数：无。
// 返回 *DB：新解析出的实例，与 Default 的共享实例互不影响，主要供测试使用。
func Load() *DB { return load() }

// load 解压内嵌数据并解析为 DB。
// 参数：无（数据来自包级变量 packed）。
// 返回 *DB：解压、解析任一步失败都返回 Ready 为 false 的空 DB。
func load() *DB {
	// packed 是 gzip 流：包一层 Reader，把完整二进制一次性解压到内存
	r, err := gzip.NewReader(bytes.NewReader(packed))
	if err != nil {
		return &DB{}
	}
	defer r.Close()
	raw, err := io.ReadAll(r)
	if err != nil {
		return &DB{}
	}
	return parse(raw)
}

// parse 解析自定义二进制格式的地理数据库。
//
// 文件布局（除魔数外均为小端序，与 cmd/geo-gen 的写出端约定一致）：
//
//	4 字节    魔数 "GWG1"
//	4 字节    区间数 rangeCount（uint32）
//	2 字节    地区数 regionCount（uint16）
//	regionCount 个地区名：1 字节长度前缀 + UTF-8 内容
//	rangeCount 条区间：Start(4) + End(4) + Region(2)，每条固定 10 字节
//
// 参数 raw：解压后的完整二进制。
// 返回 *DB：任何校验失败都返回 Ready 为 false 的空 DB，不 panic。
func parse(raw []byte) *DB {
	// 长度下限 10 = 魔数 4 + 区间数 4 + 地区数 2；魔数不符即判非法
	if len(raw) < 10 || string(raw[:4]) != magic {
		return &DB{}
	}
	// p 是当前读取偏移，跳过魔数后依次读两个数量字段（小端序）
	p := 4
	rangeCount := int(binary.LittleEndian.Uint32(raw[p:]))
	p += 4
	regionCount := int(binary.LittleEndian.Uint16(raw[p:]))
	p += 2
	// 数量上限防御：脏数据不至于按它申请过大内存
	if rangeCount < 0 || regionCount < 0 || rangeCount > 1<<24 || regionCount > 1<<16 {
		return &DB{}
	}

	// 逐个读地区名：1 字节长度前缀 + 内容，存入 regions 表
	regions := make([]string, 0, regionCount)
	for i := 0; i < regionCount; i++ {
		if p >= len(raw) {
			return &DB{}
		}
		n := int(raw[p])
		p++
		if p+n > len(raw) {
			return &DB{}
		}
		regions = append(regions, string(raw[p:p+n]))
		p += n
	}

	// 每条区间固定 10 字节，先做整体长度校验再逐条读取（小端序）
	if p+rangeCount*10 > len(raw) {
		return &DB{}
	}
	ranges := make([]entry, rangeCount)
	for i := 0; i < rangeCount; i++ {
		ranges[i] = entry{
			Start:  binary.LittleEndian.Uint32(raw[p:]),
			End:    binary.LittleEndian.Uint32(raw[p+4:]),
			Region: binary.LittleEndian.Uint16(raw[p+8:]),
		}
		p += 10
	}
	return &DB{ranges: ranges, regions: regions, loaded: true}
}

// Ready 报告数据库是否成功加载。
// 参数：无。
// 返回 bool：成功解析出有效数据为 true；接收者为 nil 或加载失败为 false。
func (d *DB) Ready() bool { return d != nil && d.loaded }

// Size 返回区间数量，供控制台展示。
// 参数：无。
// 返回 int：ranges 的长度；接收者为 nil 时返回 0。
func (d *DB) Size() int {
	if d == nil {
		return 0
	}
	return len(d.ranges)
}

// LookupString 是 Lookup 的字符串入口。
// 参数 s：点分十进制 IPv4 或 IPv6 地址字符串。
// 返回 string：归属地文本；地址解析失败返回空串。
func (d *DB) LookupString(s string) string {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return ""
	}
	return d.Lookup(addr)
}

// Lookup 返回「中国 广东 深圳」这样的归属地。内网/回环单独标注，
// 查不到返回空串（调用方展示为 "-"）。
// 参数 addr：任意合法 IP，IPv4-mapped IPv6 会先还原成 IPv4。
// 返回 string：归属地标签；非法地址或未命中区间返回空串。
func (d *DB) Lookup(addr netip.Addr) string {
	if !addr.IsValid() {
		return ""
	}
	// IPv4-mapped IPv6（如 ::ffff:1.2.3.4）还原为 IPv4，统一走 v4 查询路径
	addr = addr.Unmap()
	if addr.IsLoopback() {
		return "本机"
	}
	if addr.IsPrivate() {
		return "内网"
	}
	if addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() {
		return "内网"
	}
	if !addr.Is4() {
		return "IPv6"
	}
	if d == nil || !d.loaded {
		return ""
	}

	// 地址 4 字节按大端转成数值：1.2.3.4 → 0x01020304，与区间表的排序一致
	ip := binary.BigEndian.Uint32(addr.AsSlice())
	// ranges 按 Start 升序存放，标准二分查找命中闭区间 [Start, End]
	lo, hi := 0, len(d.ranges)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		e := d.ranges[mid]
		switch {
		case ip < e.Start:
			hi = mid - 1
		case ip > e.End:
			lo = mid + 1
		default:
			if int(e.Region) < len(d.regions) {
				return d.regions[e.Region]
			}
			return ""
		}
	}
	return ""
}

// NormalizeRegion 把 ip2region 的 国家|省|市 压成展示用短标签。
//
// 国内精确到省市（数据集本身是中文），海外只到国家（英文名翻成中文），
// 港澳台按「中国香港 / 中国澳门 / 中国台湾」展示。
// 参数 country、province、city：ip2region 的原始字段，"0" 或空白视为空。
// 返回 string：归一化后的展示文本；国家为空时返回 "未知"，
// 未收录的海外国家名原样返回。
func NormalizeRegion(country, province, city string) string {
	country = cleanField(country)
	province = cleanField(province)
	city = cleanField(city)
	if country == "" {
		return "未知"
	}

	if country == "中国" || country == "China" || country == "CN" {
		switch province {
		case "香港", "香港特别行政区", "Hong Kong":
			return "中国香港"
		case "澳门", "澳门特别行政区", "Macau", "Macao":
			return "中国澳门"
		case "台湾", "台湾省", "Taiwan":
			if city != "" {
				return "中国台湾 " + trimSuffix(city, "市", "县")
			}
			return "中国台湾"
		}
		if zh, ok := provinceNames[province]; ok {
			province = zh
		}
		province = trimSuffix(province, "省", "市", "自治区", "特别行政区", "维吾尔", "壮族", "回族")
		city = trimSuffix(city, "市", "地区", "自治州", "盟")
		switch {
		case province != "" && city != "" && city != province:
			return "中国 " + province + " " + city
		case province != "":
			return "中国 " + province
		default:
			return "中国"
		}
	}

	if zh, ok := countryNames[country]; ok {
		return zh
	}
	return country
}

// cleanField 清理 ip2region 原始字段：去首尾空白，"0" 视为空。
// 参数 s：原始字段值。
// 返回 string：清理后的值，无有效内容时为空串。
func cleanField(s string) string {
	s = strings.TrimSpace(s)
	if s == "0" {
		return ""
	}
	return s
}

// trimSuffix 依次尝试用给定后缀裁剪 s，命中且不会裁成空串时立即返回。
// 参数 s：原字符串；suffixes：候选后缀列表（如 "省"、"市"）。
// 返回 string：首个成功裁剪的结果；无一命中则原样返回。
func trimSuffix(s string, suffixes ...string) string {
	for _, suf := range suffixes {
		if strings.HasSuffix(s, suf) && len(s) > len(suf) {
			return strings.TrimSuffix(s, suf)
		}
	}
	return s
}
