// Command geo-gen 把 ip2region 的 ipv4_source.txt 编译成紧凑区间表。
//
// 用法：
//
//	go run ./cmd/geo-gen -in ipv4_source.txt -out internal/geo/data/geo.bin.gz
//
// 源文件格式： start|end|国家|省|市|ISP，每秒一行。这里只保留
// 「国家 + 省 + 市」并做相邻同地区合并，体积从 36MB 降到不到 1MB。
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"gowaf/internal/geo"
)

// span 表示源文件中一段连续的 IPv4 地址区间及其地区标签。
type span struct {
	start, end uint32 // 区间起止地址（闭区间），为 IPv4 地址的大端序 uint32 表示
	label      string // 归一化后的地区标签（国家+省+市）
}

// main 是 geo-gen 的入口：解析 -in/-out 命令行参数，逐行读取 ip2region 源文件，
// 归一化地区并合并相邻同地区区间，最后按自定义二进制格式 gzip 压缩输出。
// 参数：无（输入输出路径通过 -in/-out 命令行标志指定）。
// 返回：无（任一步骤失败时打印错误并以状态码 1 退出，成功时打印统计信息）。
func main() {
	in := flag.String("in", "", "ip2region ipv4_source.txt 路径")
	out := flag.String("out", "internal/geo/data/geo.bin.gz", "输出文件")
	flag.Parse()
	if *in == "" {
		fmt.Println("缺少 -in")
		os.Exit(1)
	}

	f, err := os.Open(*in)
	if err != nil {
		fmt.Println("打开源文件失败:", err)
		os.Exit(1)
	}
	defer f.Close()

	var spans []span
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20) // 缓冲区扩到 1MB，避免个别超长行导致扫描中止
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		parts := strings.Split(text, "|")
		if len(parts) < 4 { // 字段不足 4 个（缺国家或省份）的行无法定位地区，跳过
			continue
		}
		start, ok1 := parseIP(parts[0])
		end, ok2 := parseIP(parts[1])
		if !ok1 || !ok2 || start > end { // 地址解析失败或区间倒置的坏行直接跳过
			continue
		}
		city := ""
		if len(parts) > 4 {
			city = parts[4]
		}
		label := geo.NormalizeRegion(parts[2], parts[3], city)
		if label == "" || label == "未知" {
			continue
		}
		spans = append(spans, span{start: start, end: end, label: label})
	}
	if err := sc.Err(); err != nil {
		fmt.Println("读取失败:", err)
		os.Exit(1)
	}

	// 源数据已按 start 升序，这里同时合并相邻且同地区的段。
	merged := make([]span, 0, len(spans))
	for _, s := range spans {
		if n := len(merged); n > 0 {
			last := &merged[n-1]
			if last.label == s.label && last.end+1 >= s.start {
				if s.end > last.end {
					last.end = s.end
				}
				continue
			}
		}
		merged = append(merged, s)
	}

	// 建立「地区标签 → 编号」映射，地区去重后作为字符串表写入文件头
	regionIdx := map[string]uint16{}
	var regions []string
	for i := range merged {
		if _, ok := regionIdx[merged[i].label]; !ok {
			regionIdx[merged[i].label] = uint16(len(regions))
			regions = append(regions, merged[i].label)
		}
	}

	var buf bytes.Buffer
	// 文件头：4 字节魔数 "GWG1"（Go WAF Geo），供运行时加载时校验格式
	buf.WriteString("GWG1")
	// 紧接着写入段数（uint32）与地区数（uint16），均为小端序
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(merged)))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(len(regions)))
	for _, r := range regions {
		if len(r) > 120 { // 单个地区名最长 120 字节，超长部分截断（与读取端约定一致）
			r = r[:120]
		}
		buf.WriteByte(byte(len(r)))
		buf.WriteString(r)
	}
	// 逐段写入记录：起始地址（4 字节）+ 结束地址（4 字节）+ 地区编号（2 字节），小端序
	for _, s := range merged {
		_ = binary.Write(&buf, binary.LittleEndian, s.start)
		_ = binary.Write(&buf, binary.LittleEndian, s.end)
		_ = binary.Write(&buf, binary.LittleEndian, regionIdx[s.label])
	}

	if err := os.MkdirAll(dir(*out), 0o755); err != nil {
		fmt.Println("创建目录失败:", err)
		os.Exit(1)
	}
	of, err := os.Create(*out)
	if err != nil {
		fmt.Println("创建输出失败:", err)
		os.Exit(1)
	}
	gw, _ := gzip.NewWriterLevel(of, gzip.BestCompression) // 最高压缩级别；创建 Writer 不会失败，错误可忽略
	if _, err := gw.Write(buf.Bytes()); err != nil {
		fmt.Println("写入失败:", err)
		os.Exit(1)
	}
	_ = gw.Close()
	_ = of.Close()

	st, _ := os.Stat(*out)
	fmt.Printf("源行数 %d，合并后 %d 段，地区 %d 个，输出 %.1f MB\n",
		line, len(merged), len(regions), float64(st.Size())/1024/1024)
}

// parseIP 把源文件中的地址字段解析为 uint32，兼容点分十进制与纯整数两种写法。
// 参数 s：待解析的地址字符串，如 "1.0.1.0" 或十进制整数。
// 返回：地址对应的大端序 uint32 数值，以及解析是否成功（失败时数值无意义）。
func parseIP(s string) (uint32, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if !strings.Contains(s, ".") {
		v, err := strconv.ParseUint(s, 10, 32)
		return uint32(v), err == nil
	}
	addr, err := netip.ParseAddr(s)
	if err != nil || !addr.Is4() {
		return 0, false
	}
	b := addr.As4()
	return binary.BigEndian.Uint32(b[:]), true
}

// dir 提取路径中的目录部分，兼容正斜杠与反斜杠两种分隔符。
// 参数 p：文件路径。
// 返回：最后一个分隔符之前的部分；路径不含分隔符时返回当前目录 "."。
func dir(p string) string {
	if i := strings.LastIndexAny(p, "/\\"); i > 0 {
		return p[:i]
	}
	return "."
}
