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
type CertManager struct {
	mu          sync.RWMutex
	certDir     string
	defaultName string
	certs       map[string]*tls.Certificate // key: 小写域名
	fallback    *tls.Certificate
	minVersion  uint16
	hsts        bool
	explicit    []config.CertPair
}

// NewCertManager 创建并首次加载。
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
		m.minVersion = tls.VersionTLS13
	}
	if err := m.Reload(); err != nil {
		return nil, err
	}
	return m, nil
}

// Reload 重新扫描证书目录。加载失败的单个证书会被跳过并记为 fallback 之外的旧值。
func (m *CertManager) Reload() error {
	next := make(map[string]*tls.Certificate)

	for _, pair := range m.explicit {
		cert, err := tls.LoadX509KeyPair(pair.Cert, pair.Key)
		if err != nil {
			return fmt.Errorf("加载证书 %s: %w", pair.Cert, err)
		}
		names := certNames(&cert)
		for _, n := range names {
			next[strings.ToLower(n)] = &cert
		}
		if len(names) == 0 && m.defaultName != "" {
			next[m.defaultName] = &cert
		}
	}

	if m.certDir != "" {
		entries, err := os.ReadDir(m.certDir)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("读取证书目录 %s: %w", m.certDir, err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			domain := strings.ToLower(e.Name())
			full := filepath.Join(m.certDir, domain, "fullchain.pem")
			key := filepath.Join(m.certDir, domain, "privkey.pem")
			if _, err := os.Stat(full); err != nil {
				continue
			}
			if _, err := os.Stat(key); err != nil {
				continue
			}
			cert, err := tls.LoadX509KeyPair(full, key)
			if err != nil {
				log.Printf("[tls] 跳过证书 %s: %v", domain, err)
				continue
			}
			next[domain] = &cert
			for _, n := range certNames(&cert) {
				next[strings.ToLower(n)] = &cert
			}
		}
	}

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

// certNames 提取证书覆盖的域名（SAN + CN）。
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

// GetCertificate 实现 tls.Config.GetCertificate。
func (m *CertManager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	name := strings.ToLower(hello.ServerName)
	if cert, ok := m.certs[name]; ok {
		return cert, nil
	}
	// 泛域名匹配
	for pattern, cert := range m.certs {
		if strings.HasPrefix(pattern, "*.") {
			suffix := strings.TrimPrefix(pattern, "*")
			if strings.HasSuffix(name, suffix) {
				return cert, nil
			}
		}
	}
	if m.fallback != nil {
		return m.fallback, nil
	}
	return nil, fmt.Errorf("没有可用于 %q 的证书", hello.ServerName)
}

// TLSConfig 返回供 http.Server 使用的配置。
func (m *CertManager) TLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:     m.minVersion,
		GetCertificate: m.GetCertificate,
		NextProtos:     []string{"h2", "http/1.1"},
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

// HSTS 是否启用 HSTS 头。
func (m *CertManager) HSTS() bool { return m.hsts }

// Count 返回已加载证书数。
func (m *CertManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.certs)
}

// ParsePEMCertificate 供测试使用。
func ParsePEMCertificate(raw []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("不是合法的 PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}
