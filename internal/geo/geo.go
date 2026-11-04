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

const magic = "GWG1"

// entry 是一个闭区间 [Start, End]，Region 是地区名在 region 表里的下标。
type entry struct {
	Start  uint32
	End    uint32
	Region uint16
}

// DB 是不可变快照，可并发读。
type DB struct {
	ranges  []entry
	regions []string
	loaded  bool
}

var (
	shared atomic.Pointer[DB]
)

// Default 返回进程级共享实例（首次调用时解包）。
func Default() *DB {
	if d := shared.Load(); d != nil {
		return d
	}
	d := load()
	shared.Store(d)
	return d
}

// Load 显式解包一份（测试用）。
func Load() *DB { return load() }

func load() *DB {
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

func parse(raw []byte) *DB {
	if len(raw) < 10 || string(raw[:4]) != magic {
		return &DB{}
	}
	p := 4
	rangeCount := int(binary.LittleEndian.Uint32(raw[p:]))
	p += 4
	regionCount := int(binary.LittleEndian.Uint16(raw[p:]))
	p += 2
	if rangeCount < 0 || regionCount < 0 || rangeCount > 1<<24 || regionCount > 1<<16 {
		return &DB{}
	}

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

// Ready 表示数据库是否成功加载。
func (d *DB) Ready() bool { return d != nil && d.loaded }

// Size 返回区间数量，供控制台展示。
func (d *DB) Size() int {
	if d == nil {
		return 0
	}
	return len(d.ranges)
}

// LookupString 是 Lookup 的字符串入口，解析失败返回空串。
func (d *DB) LookupString(s string) string {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return ""
	}
	return d.Lookup(addr)
}

// Lookup 返回「中国 广东 深圳」这样的归属地。内网/回环单独标注，
// 查不到返回空串（调用方展示为 "-"）。
func (d *DB) Lookup(addr netip.Addr) string {
	if !addr.IsValid() {
		return ""
	}
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

	ip := binary.BigEndian.Uint32(addr.AsSlice())
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

func cleanField(s string) string {
	s = strings.TrimSpace(s)
	if s == "0" {
		return ""
	}
	return s
}

func trimSuffix(s string, suffixes ...string) string {
	for _, suf := range suffixes {
		if strings.HasSuffix(s, suf) && len(s) > len(suf) {
			return strings.TrimSuffix(s, suf)
		}
	}
	return s
}
