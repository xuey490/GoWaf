package dprotect

// RC4-drop256 密钥流：动态防护的加解密原语。
//
// 为什么不用 AES-GCM：动态防护的对抗对象是"不执行 JS 的爬虫/采集器"，密钥本身
// 会随解密脚本一起下发到浏览器（详见 dprotect.go 的设计注释），加密强度再高
// 也不构成机密性——这是一层"看不懂"，不是一层"解不开"。既然强度需求不高，
// 就选了能在浏览器里用 20 行 JS 精确复现的 RC4：
//   - Go 侧与 JS 侧的实现必须逐字节一致，RC4 的 KSA/PRGA 只有加法与交换，
//     没有端序/浮点/大整数陷阱，跨语言对齐的出错面最小；
//   - drop256 丢弃前 256 字节输出，规避 RC4 最初的密钥偏置。

// rc4Cipher 是一个 RC4 密钥流生成器实例。
type rc4Cipher struct {
	s    [256]byte // 置换表
	i, j uint8     // PRGA 游标
}

// newRC4 用密钥初始化置换表（KSA）。
// 参数 key：任意长度密钥；空密钥按 1 字节 0 处理，避免死循环。
// 返回 *rc4Cipher：就绪的密钥流生成器。
func newRC4(key []byte) *rc4Cipher {
	c := &rc4Cipher{}
	for idx := range c.s {
		c.s[idx] = byte(idx)
	}
	if len(key) == 0 {
		key = []byte{0}
	}
	j := uint8(0)
	for idx := 0; idx < 256; idx++ {
		j = j + c.s[idx] + key[idx%len(key)]
		c.s[idx], c.s[j] = c.s[j], c.s[idx]
	}
	return c
}

// next 产生密钥流的下一个字节（PRGA 单步）。
// 无参数；返回 byte：密钥流字节。
func (c *rc4Cipher) next() byte {
	c.i++
	c.j += c.s[c.i]
	c.s[c.i], c.s[c.j] = c.s[c.j], c.s[c.i]
	return c.s[(c.s[c.i]+c.s[c.j])&0xff]
}

// rc4Drop256 生成 n 字节密钥流：KSA 后先丢弃 256 字节再输出。
// 参数 key：密钥；参数 n：需要的字节数。
// 返回 []byte：长度为 n 的密钥流。
func rc4Drop256(key []byte, n int) []byte {
	c := newRC4(key)
	out := make([]byte, n)
	for idx := 0; idx < 256; idx++ {
		_ = c.next() // 丢弃初始偏置段
	}
	for idx := range out {
		out[idx] = c.next()
	}
	return out
}

// cryptXOR 用密钥流对明文做异或，加解密共用同一个函数。
// 参数 text：明文/密文；参数 key：RC4 密钥。
// 返回 []byte：异或结果（新切片，不修改入参）。
func cryptXOR(text, key []byte) []byte {
	ks := rc4Drop256(key, len(text))
	out := make([]byte, len(text))
	for idx := range text {
		out[idx] = text[idx] ^ ks[idx]
	}
	return out
}
