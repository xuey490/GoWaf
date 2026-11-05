package guard

import "net/netip"

// cloudflareCIDRs 是 Cloudflare 回源网段，与 deploy/install.sh 的内置兜底列表保持一致。
//
// 为什么内置一份而不是运行时拉取：数据面启动时不能依赖外网可用，
// 而这份列表只用于「配置自检」——判断 trusted_proxies 是否覆盖了 CF，
// 即便个别网段过时，也只会让自检多报或少报一条提示，不影响取 IP 的正确性。
// 官方最新值：https://www.cloudflare.com/ips-v4 与 /ips-v6。
var cloudflareCIDRs = []string{
	"173.245.48.0/20",
	"103.21.244.0/22",
	"103.22.200.0/22",
	"103.31.4.0/22",
	"141.101.64.0/18",
	"108.162.192.0/18",
	"190.93.240.0/20",
	"188.114.96.0/20",
	"197.234.240.0/22",
	"198.41.128.0/17",
	"162.158.0.0/15",
	"104.16.0.0/13",
	"104.24.0.0/14",
	"172.64.0.0/13",
	"131.0.72.0/22",
	"2400:cb00::/32",
	"2606:4700::/32",
	"2803:f800::/32",
	"2405:b500::/32",
	"2405:8100::/32",
	"2a06:98c0::/29",
	"2c0f:f248::/32",
}

// cloudflarePrefixes 是解析后的只读快照，包初始化时构建一次，运行期不再变化。
var cloudflarePrefixes = parseAllPrefixes(cloudflareCIDRs)

// parseAllPrefixes 把 CIDR 字符串列表解析为前缀切片。
// 参数 list：CIDR 字符串列表。
// 返回 []netip.Prefix：解析成功且已 Masked（主机位清零）的前缀；非法条目静默跳过。
func parseAllPrefixes(list []string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(list))
	for _, s := range list {
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, p.Masked())
		}
	}
	return out
}

// CloudflarePrefixes 返回 Cloudflare 回源网段列表。
// 返回 []netip.Prefix：共享的只读切片，调用方只读，不要修改切片及其元素。
func CloudflarePrefixes() []netip.Prefix { return cloudflarePrefixes }

// IsCloudflare 判断地址是否属于 Cloudflare 回源网段。
// 参数 addr：待判定的连接对端地址。
// 返回 bool：命中任一 CF 网段返回 true；非法地址返回 false。
//
// 注意：判定的对象是「连接对端」，不是 XFF 里的访客 IP。
func IsCloudflare(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	// 去掉 IPv4-mapped IPv6 前缀，统一按原生地址比较
	addr = addr.Unmap()
	for _, p := range cloudflarePrefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// MissingCloudflare 返回 trusted 未完整覆盖的 Cloudflare 网段（已 Masked 的字符串）。
// 参数 trusted：trusted_proxies 配置解析出的可信网段列表。
// 返回 []string：未被完整覆盖的 CF 网段字符串，全部覆盖时长度为 0。
//
// 语义是「覆盖率」而不是「有交集」：trusted_proxies 里只写了 104.16.0.0/13
// 也算缺失，因为从 172.64.0.0/13 回源的流量仍会被当成直连访客。
func MissingCloudflare(trusted []netip.Prefix) []string {
	missing := make([]string, 0, 4)
	for _, cf := range cloudflarePrefixes {
		covered := false
		for _, t := range trusted {
			// 覆盖条件：可信段的位数不多于 CF 段，且包含 CF 段的起始地址。
			if t.Bits() <= cf.Bits() && t.Contains(cf.Addr()) {
				covered = true
				break
			}
		}
		if !covered {
			missing = append(missing, cf.String())
		}
	}
	return missing
}

// ParseTrusted 解析 trusted_proxies 配置，跳过非法条目。
// 参数 list：配置字符串列表，支持 CIDR（如 "10.0.0.0/8"）与单 IP（如 "127.0.0.1"）混写。
// 返回 []netip.Prefix：解析结果；单 IP 转为 /32（IPv4）或 /128（IPv6）前缀，非法条目被跳过。
func ParseTrusted(list []string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(list))
	for _, s := range list {
		// 优先按 CIDR 解析，并清零主机位
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, p.Masked())
			continue
		}
		// 再按单 IP 解析，转成覆盖单个地址的整段前缀
		if a, err := netip.ParseAddr(s); err == nil {
			out = append(out, netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()))
		}
	}
	return out
}
