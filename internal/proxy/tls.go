package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gowaf/internal/config"
)

// CertManager 负责多证书加载与 SNI 选择，支持热加载（宝塔续期后无需重启）。
// app 层每 6 小时定时调用 Reload 重扫证书来源并整体替换证书表。
type CertManager struct {
	mu          sync.RWMutex                // 保护 certs 与 fallback；握手读多、Reload 偶尔写
	certDir     string                      // 证书目录：其下每个「域名/fullchain.pem + privkey.pem」子目录视为一张证书
	defaultName string                      // 默认证书域名（小写），SNI 未命中时优先用它兜底
	certs       map[string]*tls.Certificate // key: 小写域名（含证书 SAN/CN 展开的全部名字）
	fallback    *tls.Certificate            // 兜底证书；Reload 后没有新的合适候选时保留旧值
	minVersion  uint16                      // 最低 TLS 版本，默认 1.2，配置 "1.3" 时升为 1.3
	hsts        bool                        // 是否下发 HSTS 响应头
	explicit    []config.CertPair           // 配置文件显式指定的证书对，优先于证书目录
}

// NewCertManager 创建证书管理器并完成首次加载。
// 参数 cfg：TLS 配置（证书目录、显式证书对、默认证书、最低版本、HSTS 开关）。
// 返回 (*CertManager, error)：首扫失败（含一张证书都没有）返回错误，调用方应拒绝以 HTTPS 启动。
func NewCertManager(cfg config.TLS) (*CertManager, error) {
	m := &CertManager{
		certDir:     cfg.CertDir,
		defaultName: strings.ToLower(cfg.DefaultCert),
		certs:       make(map[string]*tls.Certificate),
		explicit:    cfg.Certs,
		minVersion:  tls.VersionTLS12,
		hsts:        cfg.HSTS,
	}
	if cfg.MinVersion == "1.3" {
		// 仅识别 "1.3"，其余配置值一律保持默认的 TLS 1.2
		m.minVersion = tls.VersionTLS13
	}
	if err := m.Reload(); err != nil {
		return nil, err
	}
	return m, nil
}

// Reload 重扫证书来源并整体替换证书表，供定时热加载调用（宝塔续期后无需重启）。
// 显式证书加载失败直接报错中止；证书目录里单个证书损坏只记日志跳过，不影响其余证书。
// 证书表在锁内原子替换；扫描结果为空时返回错误，此时仅保留原 fallback 兜底。
// 无参数；返回 error：加载失败或一张证书都没有时返回原因，成功为 nil。
func (m *CertManager) Reload() error {
	// 先扫进临时表 next，全部就绪后再一次性换入，握手侧不会看到半成品
	next := make(map[string]*tls.Certificate)

	// 显式证书：加载失败视为致命错误，直接中止本次 Reload
	for _, pair := range m.explicit {
		cert, err := tls.LoadX509KeyPair(pair.Cert, pair.Key)
		if err != nil {
			return fmt.Errorf("加载证书 %s: %w", pair.Cert, err)
		}
		names := certNames(&cert)
		// 证书 SAN/CN 里的每个域名都注册成可用 key
		for _, n := range names {
			next[strings.ToLower(n)] = &cert
		}
		// 证书读不出域名时挂到默认域名下，保证至少有一条记录可被选中
		if len(names) == 0 && m.defaultName != "" {
			next[m.defaultName] = &cert
		}
	}

	// 证书目录：沿用宝塔/Let's Encrypt 的目录布局，子目录名即域名
	if m.certDir != "" {
		entries, err := os.ReadDir(m.certDir)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("读取证书目录 %s: %w", m.certDir, err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue // 只认子目录，散落的杂项文件忽略
			}
			domain := strings.ToLower(e.Name())
			full := filepath.Join(m.certDir, domain, "fullchain.pem")
			key := filepath.Join(m.certDir, domain, "privkey.pem")
			if _, err := os.Stat(full); err != nil {
				continue // 缺 fullchain 的目录跳过
			}
			if _, err := os.Stat(key); err != nil {
				continue // 缺私钥的目录跳过
			}
			cert, err := tls.LoadX509KeyPair(full, key)
			if err != nil {
				// 单张证书损坏只跳过，不拖垮其余证书
				log.Printf("[tls] 跳过证书 %s: %v", domain, err)
				continue
			}
			next[domain] = &cert
			for _, n := range certNames(&cert) {
				next[strings.ToLower(n)] = &cert
			}
		}
	}

	// 选兜底证书：优先默认证书，否则取扫描结果里的任意一张
	var fallback *tls.Certificate
	if m.defaultName != "" {
		fallback = next[m.defaultName]
	}
	if fallback == nil {
		for _, c := range next {
			fallback = c
			break
		}
	}

	m.mu.Lock()
	m.certs = next
	if fallback != nil {
		m.fallback = fallback
	}
	m.mu.Unlock()

	if len(next) == 0 {
		return fmt.Errorf("证书目录 %s 下没有可用证书", m.certDir)
	}
	return nil
}

// certNames 提取证书覆盖的域名（SAN DNS 名称 + CN）。
// 参数 cert：已加载的证书对，Leaf 未预解析时现场解析第一段证书。
// 返回 []string：域名列表（大小写未规整，由调用方转小写）；解析失败返回 nil。
func certNames(cert *tls.Certificate) []string {
	var leaf *x509.Certificate
	if cert.Leaf != nil {
		leaf = cert.Leaf
	} else if len(cert.Certificate) > 0 {
		parsed, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return nil
		}
		leaf = parsed
	}
	if leaf == nil {
		return nil
	}
	names := make([]string, 0, len(leaf.DNSNames)+1)
	names = append(names, leaf.DNSNames...)
	if leaf.Subject.CommonName != "" {
		names = append(names, leaf.Subject.CommonName)
	}
	return names
}

// GetCertificate 实现 tls.Config.GetCertificate，按 SNI 选证书。
// 参数 hello：客户端握手信息，ServerName 为其请求的域名。
// 返回 (*tls.Certificate, error)：精确匹配 -> 泛域名匹配 -> 兜底证书；
// 连兜底都没有时报错。读取全程只持读锁，热加载替换不影响并发握手。
func (m *CertManager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	name := strings.ToLower(hello.ServerName)
	if cert, ok := m.certs[name]; ok {
		return cert, nil
	}
	// 泛域名匹配：模式剥掉 * 取后缀，请求域名以后缀结尾即命中
	for pattern, cert := range m.certs {
		if strings.HasPrefix(pattern, "*.") {
			suffix := strings.TrimPrefix(pattern, "*")
			if strings.HasSuffix(name, suffix) {
				return cert, nil
			}
		}
	}
	// SNI 未命中（含 IP 直连、未携带 SNI）时退回兜底证书
	if m.fallback != nil {
		return m.fallback, nil
	}
	return nil, fmt.Errorf("没有可用于 %q 的证书", hello.ServerName)
}

// TLSConfig 返回供 http.Server 使用的 TLS 配置。
// 无参数；返回 *tls.Config：证书由 GetCertificate 按 SNI 动态选取，
// 同时协商 h2 与 http/1.1 两个应用层协议。
func (m *CertManager) TLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:     m.minVersion,
		GetCertificate: m.GetCertificate,
		NextProtos:     []string{"h2", "http/1.1"},
		// 只保留 ECDHE 套件（具备前向保密）且全为 AEAD/ChaCha20，剔除 CBC 等弱算法
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		},
	}
}

// HSTS 报告是否启用 HSTS 响应头。
// 无参数；返回 bool：取自 TLS 配置的 hsts 项，edge 层据此决定是否加 Strict-Transport-Security。
func (m *CertManager) HSTS() bool { return m.hsts }

// Count 返回当前已加载的证书条目数（按域名 key 计，同一张多域名证书会计多条）。
// 无参数；返回 int：读取持读锁，并发安全。
func (m *CertManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.certs)
}

// ParsePEMCertificate 解析单段 PEM 编码的 X.509 证书，供测试与调试使用。
// 参数 raw：PEM 文本（如 fullchain.pem 的第一段证书）。
// 返回 (*x509.Certificate, error)：输入不是 PEM 或证书内容损坏时返回错误。
func ParsePEMCertificate(raw []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("不是合法的 PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}
