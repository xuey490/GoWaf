package handler

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/middleware"
	"gowaf/internal/admin/response"
	"gowaf/internal/proxy"
)

/* ================================================== 证书管理（本地证书 + ACME 签发） */

// certItem 一张证书条目的展示视图。
type certItem struct {
	Domain       string   `json:"domain"`       // 目录名（即主域名）；显式配置项为证书文件路径
	HasFullchain bool     `json:"hasFullchain"` // 证书文件是否存在
	HasKey       bool     `json:"hasKey"`       // 私钥文件是否存在
	NotAfter     int64    `json:"notAfter"`     // 到期时间（Unix 毫秒，0=文件缺失或解析失败）
	Subjects     []string `json:"subjects"`     // 证书覆盖的域名（SAN + CN）
	Issuer       string   `json:"issuer"`       // 签发者 CN
	IsDefault    bool     `json:"isDefault"`    // 是否为 SNI 兜底默认证书
}

// CertList 扫描证书配置与目录，返回每张证书的文件完整性与有效期。
// 参数 c：Gin 上下文，无查询参数。
// 返回：certDir 证书目录路径、tlsEnabled 是否启用 TLS、items 证书列表
// （显式 tls.certs 配置在前，目录扫描在后，按域名排序）。
// 路由：GET /api/certs。
func (s *Server) CertList(c *gin.Context) {
	items := make([]certItem, 0, 8)
	certDir := s.Cfg.TLS.CertDir

	// 显式证书对：来自配置文件 tls.certs，只读不写（删除接口会明确拒绝）。
	for _, pair := range s.Cfg.TLS.Certs {
		it := certItem{Domain: pair.Cert, Subjects: []string{}}
		it.HasFullchain, it.HasKey = fileExists(pair.Cert), fileExists(pair.Key)
		if raw, err := os.ReadFile(pair.Cert); err == nil {
			if leaf, perr := proxy.ParsePEMCertificate(raw); perr == nil {
				fillCertInfo(&it, leaf)
			}
		}
		items = append(items, it)
	}

	// 证书目录：沿用「域名/fullchain.pem + privkey.pem」的子目录布局。
	if certDir != "" {
		entries, err := os.ReadDir(certDir)
		if err == nil {
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				if e.IsDir() {
					names = append(names, e.Name())
				}
			}
			sort.Strings(names)
			for _, domain := range names {
				full := filepath.Join(certDir, domain, "fullchain.pem")
				key := filepath.Join(certDir, domain, "privkey.pem")
				it := certItem{Domain: domain, Subjects: []string{}, IsDefault: strings.EqualFold(domain, s.Cfg.TLS.DefaultCert)}
				it.HasFullchain, it.HasKey = fileExists(full), fileExists(key)
				if raw, err := os.ReadFile(full); err == nil {
					if leaf, perr := proxy.ParsePEMCertificate(raw); perr == nil {
						fillCertInfo(&it, leaf)
					}
				}
				items = append(items, it)
			}
		}
	}

	response.OK(c, gin.H{
		"certDir":    certDir,
		"tlsEnabled": s.Cfg.TLS.Enabled,
		"items":      items,
	})
}

// CertUpload 上传（替换）一张证书：校验 PEM 配对后写入证书目录并热加载。
// 参数 c：Gin 上下文，请求体 {domain, fullchain, key}，PEM 文本直接粘贴。
// 返回：成功返回 200 与到期时间；域名校验/PEM 校验失败返回 400。
// 路由：POST /api/certs/upload（要求 settings:write 权限，经 CSRF/DemoGuard/审计）。
func (s *Server) CertUpload(c *gin.Context) {
	var in struct {
		Domain    string `json:"domain" binding:"required"`
		Fullchain string `json:"fullchain" binding:"required"`
		Key       string `json:"key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "domain、fullchain、key 均必填")
		return
	}
	domain := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(in.Domain), "."))
	if domain == "" || strings.HasPrefix(domain, "*") || strings.ContainsAny(domain, "/\\?#:") {
		response.BadRequest(c, "域名非法（不允许通配符、路径与端口，泛域名请走 ACME DNS-01）")
		return
	}
	// 先验配对再落盘：fullchain 与 key 不匹配是最常见的上传错误，绝不能写出坏证书。
	pair, err := tls.X509KeyPair([]byte(in.Fullchain), []byte(in.Key))
	if err != nil {
		response.BadRequest(c, "证书与私钥不匹配或格式非法: "+err.Error())
		return
	}
	if len(pair.Certificate) == 0 {
		response.BadRequest(c, "证书内容为空")
		return
	}
	if s.Cfg.TLS.CertDir == "" {
		response.BadRequest(c, "tls.cert_dir 未配置，无法写入证书（请先在配置文件启用 TLS）")
		return
	}

	outDir := filepath.Join(s.Cfg.TLS.CertDir, domain)
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		s.failDB(c, err)
		return
	}
	if err := os.WriteFile(filepath.Join(outDir, "fullchain.pem"), []byte(in.Fullchain), 0o644); err != nil {
		s.failDB(c, err)
		return
	}
	// 私钥按 0600 落盘：证书目录与数据目录同盘，严于证书本身。
	if err := os.WriteFile(filepath.Join(outDir, "privkey.pem"), []byte(in.Key), 0o600); err != nil {
		s.failDB(c, err)
		return
	}
	if err := s.reloadCerts(); err != nil {
		// 文件已写入，加载失败通常是权限/磁盘问题；如实告知但不回滚——
		// 6 小时定时重扫与下一次手动重载都会再尝试。
		response.Internal(c, "证书已写入但热加载失败: "+err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "cert")
	response.OK(c, gin.H{"domain": domain, "notAfter": pair.Leaf.NotAfter.UnixMilli()})
}

// CertDelete 删除证书目录中的一张证书并热加载。
// 参数 c：Gin 上下文，路由参数 domain。
// 返回：成功返回 200；目录不存在返回 404；显式配置（tls.certs）里的证书拒绝并说明。
// 路由：DELETE /api/certs/domain/:domain（要求 settings:write 权限）。
//
// 路由用了 /domain/ 前缀：gin 的路由树不允许「同级既有 :domain 参数段又有 acme
// 静态段」，加一段前缀让参数段独占一层，ACME 路由得以共存。
func (s *Server) CertDelete(c *gin.Context) {
	domain := strings.TrimSpace(c.Param("domain"))
	if domain == "" {
		response.BadRequest(c, "domain 必填")
		return
	}
	if s.Cfg.TLS.CertDir == "" {
		response.BadRequest(c, "tls.cert_dir 未配置")
		return
	}
	// 显式证书对来自配置文件，删了文件下次 Reload 还会再加载，直接拒绝并说明。
	for _, pair := range s.Cfg.TLS.Certs {
		if strings.EqualFold(filepath.Base(filepath.Dir(pair.Cert)), domain) {
			response.BadRequest(c, "该证书来自配置文件 tls.certs，请修改配置文件后重启")
			return
		}
	}
	outDir := filepath.Join(s.Cfg.TLS.CertDir, domain)
	if _, err := os.Stat(outDir); err != nil {
		response.NotFound(c, "证书不存在")
		return
	}
	if err := os.RemoveAll(outDir); err != nil {
		s.failDB(c, err)
		return
	}
	if err := s.reloadCerts(); err != nil {
		response.Internal(c, "证书已删除但热加载失败: "+err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "cert")
	response.OK(c, gin.H{"domain": domain})
}

// CertReload 手动触发证书重扫（宝塔续期、手动替换文件后无需等 6 小时定时任务）。
// 参数 c：Gin 上下文，无请求体。
// 返回：成功返回 200 与当前加载的证书条目数；未启用 TLS 返回 400。
// 路由：POST /api/certs/reload（要求 settings:write 权限）。
func (s *Server) CertReload(c *gin.Context) {
	if s.RT.Certs == nil {
		response.BadRequest(c, "TLS 未启用，无证书可加载")
		return
	}
	if err := s.RT.Certs.Reload(); err != nil {
		response.Internal(c, "重载失败（保留旧证书）: "+err.Error())
		return
	}
	response.OK(c, gin.H{"count": s.RT.Certs.Count()})
}

// AcmeOrders 返回当前进程内的全部 ACME 订单。
// 参数 c：Gin 上下文，无查询参数。
// 返回：items 订单列表（含挑战凭据；订单仅存内存，进程重启后未完成订单消失）。
// 路由：GET /api/certs/acme/orders。
func (s *Server) AcmeOrders(c *gin.Context) {
	response.OK(c, gin.H{"items": s.RT.ACME.ListOrders()})
}

// AcmeCreateOrder 创建一张 ACME 签发订单。
// 参数 c：Gin 上下文，请求体 {domains: [], email?, staging?, challenge?}。
// 返回：订单对象（含挑战凭据；http-01 由 WAF 在 80 端口自动应答，
// dns-01 需管理员按 dnsRecord/dnsValue 添加 TXT 后再触发验证）。
// 路由：POST /api/certs/acme/order（要求 settings:write 权限）。
func (s *Server) AcmeCreateOrder(c *gin.Context) {
	var in struct {
		Domains   []string `json:"domains" binding:"required"`
		Email     string   `json:"email"`
		Staging   bool     `json:"staging"`
		Challenge string   `json:"challenge"` // http-01（默认）/ dns-01（泛域名必选）
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "domains 必填（域名数组）")
		return
	}
	order, err := s.RT.ACME.CreateOrder(in.Domains, in.Challenge, in.Staging)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "cert")
	response.OK(c, order)
}

// AcmeValidateOrder 触发验证并等待签发完成（HTTP-01 自动应答；DNS-01 前提是 TXT 已生效）。
// 参数 c：Gin 上下文，路由参数 id。
// 返回：订单最新状态；status=issued 表示证书已写入并热加载；
// status=failed 时 message 带原因（HTTP 层仍是 200，前端按状态渲染）。
// 路由：POST /api/certs/acme/order/:id/validate（要求 settings:write 权限）。
func (s *Server) AcmeValidateOrder(c *gin.Context) {
	order, err := s.RT.ACME.ValidateOrder(c.Param("id"))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "cert")
	response.OK(c, order)
}

// AcmeCancelOrder 取消（删除）一张订单。
// 参数 c：Gin 上下文，路由参数 id。
// 返回：成功返回 200；订单不存在返回 404。
// 路由：DELETE /api/certs/acme/order/:id（要求 settings:write 权限）。
func (s *Server) AcmeCancelOrder(c *gin.Context) {
	if !s.RT.ACME.CancelOrder(c.Param("id")) {
		response.NotFound(c, "订单不存在")
		return
	}
	response.OK(c, gin.H{"ok": true})
}

// reloadCerts 统一的证书热加载入口：TLS 未启用时是空操作。
// 返回 error：加载失败原因；TLS 未启用返回 nil。
func (s *Server) reloadCerts() error {
	if s.RT.Certs == nil {
		return nil
	}
	return s.RT.Certs.Reload()
}

// fileExists 判断文件是否存在。
// 参数 path：文件路径。
// 返回 bool：存在返回 true。
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// fillCertInfo 把解析出的证书信息填进展示条目。
// 参数 it：目标条目；参数 leaf：证书内容（proxy.ParsePEMCertificate 的产物）。
// 返回值无。
func fillCertInfo(it *certItem, leaf *x509.Certificate) {
	it.NotAfter = leaf.NotAfter.UnixMilli()
	it.Subjects = append([]string{}, leaf.DNSNames...)
	if leaf.Subject.CommonName != "" && !containsString(it.Subjects, leaf.Subject.CommonName) {
		it.Subjects = append(it.Subjects, leaf.Subject.CommonName)
	}
	it.Issuer = leaf.Issuer.CommonName
}

// containsString 判断列表是否包含字符串。
// 参数 list：字符串列表；参数 v：目标值。
// 返回 bool：包含返回 true。
func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
