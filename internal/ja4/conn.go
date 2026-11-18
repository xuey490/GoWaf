package ja4

import (
	"net"
	"sync"
)

// table 连接指纹登记表：key 为底层连接的 RemoteAddr 字符串（host:port，
// 每条连接唯一），value 为该连接的 JA4 指纹。
// 由包装后的 TLS Conn 在解析出 ClientHello 时写入、连接关闭时删除。
type table struct {
	m sync.Map
}

var globalTable = &table{}

// Lookup 按远端地址查询该连接的 JA4 指纹。
// 参数 remoteAddr：请求的 RemoteAddr（r.RemoteAddr）。
// 返回 string：指纹；非 TLS 连接、解析失败或连接已关闭返回空串。
func Lookup(remoteAddr string) string {
	v, ok := globalTable.m.Load(remoteAddr)
	if !ok {
		return ""
	}
	return v.(string)
}

// store 登记一条连接的指纹。
func store(remoteAddr, fp string) {
	if remoteAddr != "" {
		globalTable.m.Store(remoteAddr, fp)
	}
}

// remove 连接关闭时移除登记，防止表无限增长。
func remove(remoteAddr string) {
	if remoteAddr != "" {
		globalTable.m.Delete(remoteAddr)
	}
}

// Conn 包装 TLS 监听器产出的底层连接：首次 Read 时先偷看 ClientHello
// 计算指纹并登记，再把缓存字节回放给 crypto/tls 握手流程。
type Conn struct {
	net.Conn
	buf   []byte // 偷看到的 ClientHello record 字节（待回放）
	pos   int    // 已回放的偏移
	addr  string // 远端地址（登记 key）
	read0 bool   // 是否已完成首次偷看
}

// Read 实现 net.Conn：首次调用先读满一个完整 TLS record（ClientHello），
// 计算指纹登记后回放；后续调用直接透传。
// 参数 b：读缓冲。返回 (int, error)：读到的字节数与错误。
func (c *Conn) Read(b []byte) (int, error) {
	if !c.read0 {
		c.read0 = true
		c.buf = c.peekRecord()
		fp := ""
		if c.buf != nil && len(c.buf) > 5 {
			fp = FromRecord(c.buf[5:])
		}
		store(c.addr, fp)
	}
	if c.pos < len(c.buf) {
		n := copy(b, c.buf[c.pos:])
		c.pos += n
		return n, nil
	}
	return c.Conn.Read(b)
}

// peekRecord 从底层连接读出完整的首个 TLS record。
// 返回 []byte：record 字节（5 字节头 + 载荷）；连接过早关闭或数据非 TLS 返回 nil。
// 读取上限为单个 record 的声明长度（最大 64KB+5），恶意超长声明由该上限兜底。
func (c *Conn) peekRecord() []byte {
	head := make([]byte, 5)
	if err := readFull(c.Conn, head); err != nil {
		return nil
	}
	// 只处理 handshake record（0x16）；-alert/-close 等无需指纹
	if head[0] != 0x16 {
		return nil
	}
	n := int(head[3])<<8 | int(head[4])
	if n <= 0 || n > 1<<16 {
		return nil
	}
	payload := make([]byte, n)
	if err := readFull(c.Conn, payload); err != nil {
		return nil
	}
	return append(head, payload...)
}

// readFull 读满指定长度的缓冲（底层可能拆包到达）。
func readFull(c net.Conn, b []byte) error {
	n := 0
	for n < len(b) {
		m, err := c.Read(b[n:])
		if err != nil {
			return err
		}
		n += m
	}
	return nil
}

// Close 关闭底层连接并移除指纹登记。
func (c *Conn) Close() error {
	remove(c.addr)
	return c.Conn.Close()
}

// Listener 包装 TLS 监听器：Accept 返回的连接具备 ClientHello 指纹采集能力，
// 数据面经 Lookup(r.RemoteAddr) 读取。HTTP 服务需以 ServeTLS 方式使用该监听器。
type Listener struct {
	net.Listener
}

// Wrap 包装一个监听器，使 TLS 连接的 ClientHello 在交给 crypto/tls 之前
// 被解析并登记进指纹表。
// 参数 l：原始 TCP 监听器。返回 net.Listener：包装后的监听器。
func Wrap(l net.Listener) net.Listener { return &Listener{Listener: l} }

// Accept 接受新连接并包装为指纹采集连接。
func (l *Listener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &Conn{Conn: c, addr: c.RemoteAddr().String()}, nil
}
