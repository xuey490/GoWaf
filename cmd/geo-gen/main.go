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

type span struct {
	start, end uint32
	label      string
}

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
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		parts := strings.Split(text, "|")
		if len(parts) < 4 {
			continue
		}
		start, ok1 := parseIP(parts[0])
		end, ok2 := parseIP(parts[1])
		if !ok1 || !ok2 || start > end {
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

	regionIdx := map[string]uint16{}
	var regions []string
	for i := range merged {
		if _, ok := regionIdx[merged[i].label]; !ok {
			regionIdx[merged[i].label] = uint16(len(regions))
			regions = append(regions, merged[i].label)
		}
	}

	var buf bytes.Buffer
	buf.WriteString("GWG1")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(merged)))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(len(regions)))
	for _, r := range regions {
		if len(r) > 120 {
			r = r[:120]
		}
		buf.WriteByte(byte(len(r)))
		buf.WriteString(r)
	}
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
	gw, _ := gzip.NewWriterLevel(of, gzip.BestCompression)
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

// parseIP 兼容 "1.0.1.0" 与整数两种写法。
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

func dir(p string) string {
	if i := strings.LastIndexAny(p, "/\\"); i > 0 {
		return p[:i]
	}
	return "."
}
