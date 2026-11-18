package ja4

import (
	"crypto/tls"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

// buildHello 构造一条最小可解析的 ClientHello record 载荷，供黄金向量断言。
func buildHello(ciphers, exts []uint16, withSNI bool) []byte {
	var b []byte
	put16 := func(v uint16) { b = append(b, byte(v>>8), byte(v)) }
	put8 := func(v byte) { b = append(b, v) }

	put16(0x0303)                      // client version
	b = append(b, make([]byte, 32)...) // random
	put8(0)                            // session id len
	put16(uint16(len(ciphers) * 2))    // cipher len
	for _, c := range ciphers {
		put16(c)
	}
	put8(1)
	put8(0) // compression: null

	// 扩展列表（按需拼装，顺序即出现顺序）
	var exts2 []byte
	addExt := func(typ uint16, data []byte) {
		put16To(&exts2, typ)
		put16To(&exts2, uint16(len(data)))
		exts2 = append(exts2, data...)
	}
	if withSNI {
		// server_name: 列表长度(2) + 类型(1)=0 + 长度(2) + "example.com"
		host := "example.com"
		entry := []byte{0, byte(len(host))}
		entry = append(entry, host...)
		list := []byte{byte(len(entry) >> 8), byte(len(entry))}
		list = append(list, entry...)
		addExt(0x0000, list)
	}
	for _, e := range exts {
		switch e {
		case 0x002b: // supported_versions [0x0304]
			addExt(0x002b, []byte{2, 0x03, 0x04})
		case 0x0010: // ALPN "h2"
			addExt(0x0010, []byte{0, 2, 2, 'h', '2'})
		case 0x000d: // signature_algorithms [0x0403, 0x0804]
			addExt(0x000d, []byte{0, 4, 0x04, 0x03, 0x08, 0x04})
		}
	}
	put16(uint16(len(exts2)))
	b = append(b, exts2...)

	// 外层 handshake 头：type=1 + 长度
	full := []byte{1, byte(len(b) >> 16), byte(len(b) >> 8), byte(len(b))}
	full = append(full, b...)
	// 外层 record 头：handshake(0x16) + 0x0301 + 长度
	rec := []byte{0x16, 0x03, 0x01, byte(len(full) >> 8), byte(len(full))}
	return append(rec, full...)
}

func put16To(dst *[]byte, v uint16) {
	*dst = append(*dst, byte(v>>8), byte(v))
}

// TestJA4GoldenVector 黄金向量：手工构造 ClientHello，指纹应等于按规范
// 手工推导的结果（版本取 supported_versions、SNI=d、计数与哈希分段）。
func TestJA4GoldenVector(t *testing.T) {
	rec := buildHello([]uint16{0x1301, 0xc02f}, []uint16{0x002b, 0x0010, 0x000d}, true)
	fp := FromRecord(rec[5:])
	want := "t13d0204h2_" + hash12("1301,c02f") + "_" + hash12("000d,002b_0403,0804")
	if fp != want {
		t.Fatalf("JA4 = %s, 期望 %s", fp, want)
	}
}

// TestJA4GreaseAndCounts GREASE 值不参与计数与哈希；无 SNI 记 i；无 ALPN 记 00。
func TestJA4GreaseAndCounts(t *testing.T) {
	// 三个密码套件中 0x0a0a/0x1a1a 均为 GREASE，有效密码只剩 1 个；
	// 扩展中 GREASE 被剔除后只剩 supported_versions，版本取 0x0304 → 13
	rec := buildHello([]uint16{0x0a0a, 0x1302, 0x1a1a}, []uint16{0x0a0a, 0x002b}, false)
	fp := FromRecord(rec[5:])
	if !strings.HasPrefix(fp, "t13i010100_") {
		t.Fatalf("GREASE 未被剔除或分段错误: %s", fp)
	}
}

// TestJA4ALPNH1 ALPN 段规则：http/1.1 → h1、无 → 00、非字母数字 → hex 首尾。
func TestJA4ALPNH1(t *testing.T) {
	// 直接改造 ALPN 内容为 http/1.1：重建一次，扩展体由 buildHello 内部固定为 h2，
	// 这里用独立拼装验证 alpnSeg 的行为
	if got := alpnSeg([]byte("http/1.1")); got != "h1" {
		t.Fatalf("alpnSeg(http/1.1) = %s, 期望 h1", got)
	}
	if got := alpnSeg(nil); got != "00" {
		t.Fatalf("alpnSeg(nil) = %s, 期望 00", got)
	}
	if got := alpnSeg([]byte{0x20}); got != "20" {
		t.Fatalf("alpnSeg(0x20) = %s, 期望 20", got)
	}
}

// startTLSServer 启动经 Wrap 包装的 TLS 服务端（自签证书），返回监听器。
func startTLSServer(t *testing.T) net.Listener {
	t.Helper()
	cert := testCertPEMHolder
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	wrapped := Wrap(ln)
	go func() {
		cfg := &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"h2", "http/1.1"}}
		for {
			c, err := wrapped.Accept()
			if err != nil {
				return
			}
			tc := tls.Server(c, cfg)
			go func() {
				defer tc.Close()
				tc.SetDeadline(time.Now().Add(3 * time.Second))
				buf := make([]byte, 512)
				for {
					if _, err := tc.Read(buf); err != nil {
						return
					}
				}
			}()
		}
	}()
	return wrapped
}

// TestJA4RealHandshake 用真实 TLS 握手验证端到端采集：
// 指纹格式正确，且 Lookup(客户端 LocalAddr) 能取回本连接指纹。
func TestJA4RealHandshake(t *testing.T) {
	ln := startTLSServer(t)
	defer ln.Close()

	conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2"},
		ServerName:         "example.com",
	})
	if err != nil {
		t.Fatalf("TLS 连接失败: %v", err)
	}
	defer conn.Close()
	_, _ = conn.Write([]byte("ping"))

	fp := Lookup(conn.LocalAddr().String())
	if fp == "" {
		t.Fatalf("未采集到指纹")
	}
	t.Logf("Go 标准库客户端 JA4 (h2): %s", fp)
	// Go 客户端默认走 TLS 1.3 + SNI 域名 + ALPN h2
	if !strings.HasPrefix(fp, "t13d") || !strings.HasSuffix(strings.Split(fp, "_")[0], "h2") {
		t.Fatalf("指纹段不符合预期: %s", fp)
	}
	seg := strings.Split(fp, "_")
	if len(seg) != 3 || len(seg[1]) != 12 || len(seg[2]) != 12 {
		t.Fatalf("哈希段长度异常: %s", fp)
	}
}

// TestConnCloseRemovesEntry 连接关闭后指纹登记应被移除。
func TestConnCloseRemovesEntry(t *testing.T) {
	ln := startTLSServer(t)
	defer ln.Close()
	conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{InsecureSkipVerify: true, ServerName: "x.com"})
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	local := conn.LocalAddr().String()
	_, _ = conn.Write([]byte("x"))
	if Lookup(local) == "" {
		t.Fatalf("连接期间应能查到指纹")
	}
	_ = conn.Close()
	time.Sleep(50 * time.Millisecond)
	if Lookup(local) != "" {
		t.Fatalf("连接关闭后登记应被移除")
	}
}

// TestParseHelloRejectsGarbage 非 ClientHello 输入应返回空指纹而不是 panic。
func TestParseHelloRejectsGarbage(t *testing.T) {
	if FromRecord([]byte{0x01, 0x02}) != "" {
		t.Fatalf("过短输入应返回空")
	}
	if FromRecord(make([]byte, 64)) != "" {
		t.Fatalf("全零输入应返回空")
	}
}

var _ = binary.BigEndian
