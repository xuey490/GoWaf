// Package acme 实现证书自动签发（Let's Encrypt / 任意 ACME v2 服务）。
//
// 能力：
//   - 账户：ECDSA P-256 账户密钥持久化在 StateDir，按 ACME 目录区分（生产/测试各一把）；
//   - 下单：为每个域名创建授权（pre-authorization），返回 HTTP-01 / DNS-01 挑战凭据；
//   - 应答：HTTP-01 的 /.well-known/acme-challenge/<token> 由数据面 80 端口自动应答
//     （edge 在站点路由之前调用 ServeChallenge）；DNS-01 返回 TXT 记录值，由管理员
//     自行添加后触发验证；
//   - 签发：全部授权通过后用临时 ECDSA 密钥生成 CSR、CreateCert 拉取证书链，
//     写入 certDir/<主域名>/{fullchain.pem, privkey.pem}，回调 ReloadCerts 热生效。
//
// 实现基于 golang.org/x/crypto/acme 的 Authorize + CreateCert 组合（该版本未提供
// RFC8555 的 order 对象），语义等价：先逐域名预授权，再以 CSR 一次性签出。
//
// 已知取舍（v1）：
//   - 订单状态仅在内存：进程重启后进行中的订单丢失，重新下单即可（订单是分钟级
//     的短生命周期对象，落盘收益低）；账户密钥与已签发的证书都在磁盘上，不受影响；
//   - 泛域名必须走 DNS-01（ACME 规范如此），HTTP-01 只覆盖普通域名；
//   - 续期复用同一条链路：到期前重新 CreateOrder → ValidateOrder 即可，不做后台
//     自动续期循环（证书 6 小时热加载已存在，续期交由管理员或外部计划任务触发）。
package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme"
)

// LetsEncryptDirectory 与 StagingDirectory 是默认的 ACME 目录地址。
const (
	LetsEncryptDirectory = "https://acme-v02.api.letsencrypt.org/directory"
	StagingDirectory     = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// challengePathPrefix HTTP-01 挑战的固定 URL 前缀（数据面按此拦截）。
const challengePathPrefix = "/.well-known/acme-challenge/"

// Conf ACME 客户端配置，每次下单前现读（设置表可热改）。
type Conf struct {
	Email     string // 账户联系邮箱，可为空
	Directory string // ACME 目录地址；指向 staging 即为测试环境
}

// Options 管理器装配参数。
type Options struct {
	StateDir    string       // 状态目录：账户密钥等（如 data/acme）
	CertDir     string       // 证书输出目录（tls.cert_dir）；为空表示 TLS 未启用
	LoadConf    func() Conf  // 现读配置回调
	ReloadCerts func() error // 签发成功后的证书热加载回调（TLS 未启用时为空操作）
}

// Challenge 一条授权挑战的对外视图。
type Challenge struct {
	Domain    string `json:"domain"`    // 授权的域名
	Type      string `json:"type"`      // http-01 / dns-01
	Token     string `json:"token"`     // 挑战令牌
	KeyAuth   string `json:"keyAuth"`   // HTTP-01 的应答正文（token.thumbprint）
	DNSRecord string `json:"dnsRecord"` // DNS-01 的记录名（如 _acme-challenge.example.com）
	DNSValue  string `json:"dnsValue"`  // DNS-01 的 TXT 值
}

// Order 一张签发订单的对外视图。
type Order struct {
	ID         string      `json:"id"`      // 订单号（waf 生成）
	Domains    []string    `json:"domains"` // SAN 列表（第一项为主域名）
	Status     string      `json:"status"`  // pending / valid / issued / failed
	Message    string      `json:"message"` // 进度说明或失败原因
	Staging    bool        `json:"staging"` // 是否测试环境订单
	CreatedAt  int64       `json:"createdAt"`
	Challenges []Challenge `json:"challenges"`

	// 内部句柄（不出 JSON）：预授权 URL 与挑战对象、账户、临时 CSR 密钥。
	authz []authzEntry
	// certURL 签发后的证书下载地址（仅记录，供排查）。
	certURL string
	// account 本订单使用的账户密钥。
	account *ecdsa.PrivateKey
	// csrKey 临时 CSR 密钥（签发后随证书写入 privkey.pem）。
	csrKey *ecdsa.PrivateKey
	// chType 选定的挑战类型。
	chType string
}

// authzEntry 一个域名的预授权与其选中的挑战。
type authzEntry struct {
	uri      string          // 授权 URL（轮询状态用）
	domain   string          // 授权的域名
	chal     *acme.Challenge // 选中的挑战对象（Accept 用）
	keyAuth  string          // HTTP-01 应答正文
	dnsValue string          // DNS-01 TXT 值
}

// Manager 管理订单生命周期与 HTTP-01 应答，互斥锁保护全部状态。
type Manager struct {
	opts Options

	mu         sync.Mutex                   // 保护下面三个表的互斥锁
	orders     map[string]*Order            // 订单表：进程内生命周期
	seq        int                          // 订单号自增
	httpTokens map[string]*httpTokenEntry   // HTTP-01 应答表：token -> 凭据 + 订单 ID
	accounts   map[string]*ecdsa.PrivateKey // 已注册账户：按目录地址缓存，避免重复注册
}

// httpTokenEntry HTTP-01 应答表条目。
type httpTokenEntry struct {
	keyAuth string
	orderID string
}

// New 创建管理器并确保状态目录存在。
// 参数 opts：装配参数。
// 返回 *acme.Manager：就绪的管理器（目录创建失败只记日志，下单时再报错）。
func New(opts Options) *Manager {
	if opts.StateDir != "" {
		if err := os.MkdirAll(opts.StateDir, 0o750); err != nil {
			log.Printf("[acme] 创建状态目录失败: %v", err)
		}
	}
	return &Manager{
		opts:       opts,
		orders:     map[string]*Order{},
		httpTokens: map[string]*httpTokenEntry{},
		accounts:   map[string]*ecdsa.PrivateKey{},
	}
}

// newClient 按目录地址构建 ACME 客户端（每张订单独立实例，避免共享连接状态）。
// 参数 dir：ACME 目录地址；参数 accountKey：账户私钥。
// 返回 *acme.Client：带 30 秒超时的客户端。
func newClient(dir string, accountKey *ecdsa.PrivateKey) *acme.Client {
	return &acme.Client{
		Key:          accountKey,
		DirectoryURL: dir,
		HTTPClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

// accountFor 取（或创建并注册）指定目录地址的账户密钥。
// 参数 dir：ACME 目录地址；参数 email：联系邮箱。
// 返回 (*ecdsa.PrivateKey, error)：账户密钥；创建或注册失败返回错误。
//
// 密钥按目录地址哈希命名，生产/测试环境各一把互不干扰；已注册过的账户
// 缓存在内存，进程重启后重新注册一次（ACME 对同 key 重复注册返回
// ErrAccountAlreadyExists，按成功处理）。
func (m *Manager) accountFor(dir, email string) (*ecdsa.PrivateKey, error) {
	m.mu.Lock()
	if key, ok := m.accounts[dir]; ok {
		m.mu.Unlock()
		return key, nil
	}
	m.mu.Unlock()

	name := fmt.Sprintf("%x", func() []byte {
		sum := sha256.Sum256([]byte(dir))
		return sum[:8]
	}())
	keyPath := filepath.Join(m.opts.StateDir, "account-"+name+".key")
	var key *ecdsa.PrivateKey
	if raw, err := os.ReadFile(keyPath); err == nil {
		block, _ := pem.Decode(raw)
		if block != nil {
			if parsed, perr := x509.ParseECPrivateKey(block.Bytes); perr == nil {
				key = parsed
			}
		}
	}
	if key == nil {
		var err error
		if key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader); err != nil {
			return nil, fmt.Errorf("生成账户密钥失败: %w", err)
		}
		der, derr := x509.MarshalECPrivateKey(key)
		if derr != nil {
			return nil, fmt.Errorf("序列化账户密钥失败: %w", derr)
		}
		if werr := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600); werr != nil {
			return nil, fmt.Errorf("写入账户密钥失败: %w", werr)
		}
	}

	// 注册或复用已有账户。注册结果缓存：失败不缓存，下次重试。
	client := newClient(dir, key)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var contact []string
	if email != "" {
		contact = []string{"mailto:" + email}
	}
	if _, err := client.Register(ctx, &acme.Account{Contact: contact}, nil); err != nil {
		// 账户已存在不算错误：同 key 重复注册时 ACME 会拒绝，密钥可用即可。
		if err != acme.ErrAccountAlreadyExists {
			return nil, fmt.Errorf("注册 ACME 账户失败: %w", err)
		}
	}

	m.mu.Lock()
	m.accounts[dir] = key
	m.mu.Unlock()
	return key, nil
}

// nextSeq 生成下一个订单号。内部自持锁，可从任意上下文调用。
// 无参数；返回 int：自增后的序号。
func (m *Manager) nextSeq() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	return m.seq
}

// CreateOrder 创建签发订单并返回挑战凭据。
// 参数 domains：SAN 列表（至少一个）；参数 chType：http-01 或 dns-01；
// 参数 staging：true 使用 Let's Encrypt 测试环境（无速率限制，证书不受浏览器信任）。
// 返回 (*Order, error)：订单对象；参数非法或与 ACME 目录交互失败返回错误。
func (m *Manager) CreateOrder(domains []string, chType string, staging bool) (*Order, error) {
	if len(domains) == 0 {
		return nil, fmt.Errorf("至少需要一个域名")
	}
	for _, d := range domains {
		if err := validateDomain(d); err != nil {
			return nil, err
		}
	}
	if chType == "" {
		chType = "http-01"
	}
	if chType != "http-01" && chType != "dns-01" {
		return nil, fmt.Errorf("挑战类型只能是 http-01 或 dns-01")
	}
	// 泛域名只有 DNS-01 能签（ACME 规范），HTTP-01 直接给出明确提示。
	if chType == "http-01" {
		for _, d := range domains {
			if strings.HasPrefix(d, "*.") {
				return nil, fmt.Errorf("泛域名 %s 必须使用 dns-01 挑战", d)
			}
		}
	}

	conf := m.opts.LoadConf()
	if conf.Directory == "" {
		conf.Directory = LetsEncryptDirectory
	}
	dir := conf.Directory
	if staging {
		dir = StagingDirectory
	}
	account, err := m.accountFor(dir, conf.Email)
	if err != nil {
		return nil, err
	}

	client := newClient(dir, account)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	o := &Order{
		ID:        fmt.Sprintf("wo%d", m.nextSeq()),
		Domains:   append([]string(nil), domains...),
		Status:    "pending",
		Message:   "等待完成挑战验证",
		Staging:   staging,
		CreatedAt: time.Now().UnixMilli(),
		account:   account,
		chType:    chType,
	}

	// 逐域名创建预授权并选定挑战，生成对外凭据。HTTP-01 的应答表登记后，
	// 数据面即可在 80 端口应答（先登记再 Accept，保证探测时能拿到正文）。
	for _, domain := range domains {
		az, err := client.Authorize(ctx, domain)
		if err != nil {
			return nil, fmt.Errorf("创建授权 %s 失败: %w", domain, err)
		}
		var chosen *acme.Challenge
		for _, ch := range az.Challenges {
			if ch.Type == chType {
				chosen = ch
				break
			}
		}
		if chosen == nil {
			return nil, fmt.Errorf("授权 %s 不支持 %s 挑战", domain, chType)
		}
		entry := authzEntry{uri: az.URI, domain: domain, chal: chosen}
		cv := Challenge{Domain: domain, Type: chType, Token: chosen.Token}
		if chType == "http-01" {
			keyAuth, err := client.HTTP01ChallengeResponse(chosen.Token)
			if err != nil {
				return nil, fmt.Errorf("计算 HTTP-01 应答失败: %w", err)
			}
			entry.keyAuth = keyAuth
			cv.KeyAuth = keyAuth
		} else {
			value, err := client.DNS01ChallengeRecord(chosen.Token)
			if err != nil {
				return nil, fmt.Errorf("计算 DNS-01 记录失败: %w", err)
			}
			entry.dnsValue = value
			cv.DNSRecord = "_acme-challenge." + strings.TrimPrefix(domain, "*.")
			cv.DNSValue = value
		}
		o.authz = append(o.authz, entry)
		o.Challenges = append(o.Challenges, cv)
		if chType == "http-01" {
			m.mu.Lock()
			m.httpTokens[chosen.Token] = &httpTokenEntry{keyAuth: cv.KeyAuth, orderID: o.ID}
			m.mu.Unlock()
		}
	}

	m.mu.Lock()
	m.orders[o.ID] = o
	m.mu.Unlock()

	return o, nil
}

// ValidateOrder 触发挑战验证并等待签发完成。
// 参数 id：订单号。
// 返回 (*Order, error)：最新订单状态；订单不存在返回错误，验证/签发失败时
// 订单置为 failed 并把原因写入 Message（HTTP 层按业务状态回给前端）。
//
// 流程：Accept 全部挑战 → 轮询授权（最长 180 秒）→ 生成 CSR → CreateCert →
// 写入 certDir → 回调 ReloadCerts。
func (m *Manager) ValidateOrder(id string) (*Order, error) {
	m.mu.Lock()
	o, ok := m.orders[id]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("订单不存在")
	}

	dir := LetsEncryptDirectory
	if o.Staging {
		dir = StagingDirectory
	}
	client := newClient(dir, o.account)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// 1) 通知服务端挑战已就绪（HTTP-01 此刻已可被 80 端口应答）。
	//    Accept 失败不直接判死：授权可能在下单到验证之间已被其它客户端通过
	//    （状态快照过期），交给 WaitAuthorization 按服务端真实状态裁决。
	for _, e := range o.authz {
		if e.chal.Status == "pending" {
			if _, err := client.Accept(ctx, e.chal); err != nil {
				log.Printf("[acme] 提交挑战 %s 失败（交由授权轮询裁决）: %v", e.domain, err)
			}
		}
	}

	// 2) 轮询授权状态。DNS-01 下管理员可能刚加完记录、传播需要一点时间，
	//    服务端轮询容忍度由 ACME 服务端决定，这里整体给 300 秒预算。
	for _, e := range o.authz {
		if _, err := client.WaitAuthorization(ctx, e.uri); err != nil {
			return m.failOrder(o, fmt.Errorf("授权验证未通过（%s）: %w", e.domain, err)), nil
		}
	}

	// 3) 生成临时密钥与 CSR，签发并拉取证书链。
	if o.csrKey == nil {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return m.failOrder(o, fmt.Errorf("生成证书私钥失败: %w", err)), nil
		}
		o.csrKey = key
	}
	tpl := &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: o.Domains[0]},
		DNSNames: o.Domains,
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, tpl, o.csrKey)
	if err != nil {
		return m.failOrder(o, fmt.Errorf("生成 CSR 失败: %w", err)), nil
	}
	chain, certURL, err := client.CreateCert(ctx, csrDER, 0, true)
	if err != nil {
		return m.failOrder(o, fmt.Errorf("签发失败: %w", err)), nil
	}
	o.certURL = certURL

	// 4) 落盘：certDir/<主域名>/。certDir 为空（TLS 未启用）时只报状态不落盘。
	if m.opts.CertDir == "" {
		return m.failOrder(o, fmt.Errorf("tls.cert_dir 未配置，证书已签发但无处写入（请启用 TLS 后重试）")), nil
	}
	outDir := filepath.Join(m.opts.CertDir, o.Domains[0])
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return m.failOrder(o, fmt.Errorf("创建证书目录失败: %w", err)), nil
	}
	var fullchain strings.Builder
	for _, der := range chain {
		fullchain.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	}
	keyDER, err := x509.MarshalECPrivateKey(o.csrKey)
	if err != nil {
		return m.failOrder(o, fmt.Errorf("序列化私钥失败: %w", err)), nil
	}
	if err := os.WriteFile(filepath.Join(outDir, "fullchain.pem"), []byte(fullchain.String()), 0o644); err != nil {
		return m.failOrder(o, fmt.Errorf("写入证书失败: %w", err)), nil
	}
	if err := os.WriteFile(filepath.Join(outDir, "privkey.pem"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return m.failOrder(o, fmt.Errorf("写入私钥失败: %w", err)), nil
	}

	// 5) 热加载证书：SNI 表整体替换，无需重启。
	if m.opts.ReloadCerts != nil {
		if err := m.opts.ReloadCerts(); err != nil {
			log.Printf("[acme] 证书热加载失败（文件已写入，等 6 小时定时任务重扫）: %v", err)
		}
	}

	m.mu.Lock()
	o.Status = "issued"
	o.Message = "证书已签发并写入 " + outDir
	m.mu.Unlock()
	return o, nil
}

// failOrder 把订单置为 failed 并携带原因。
// 参数 o：订单；参数 err：失败原因。
// 返回 *Order：状态更新后的订单（返回值非 error：HTTP 层按业务状态回给前端）。
func (m *Manager) failOrder(o *Order, err error) *Order {
	m.mu.Lock()
	o.Status = "failed"
	o.Message = err.Error()
	m.mu.Unlock()
	return o
}

// ListOrders 返回全部订单（按创建时间倒序）。
// 无参数；返回 []*Order：订单快照列表。
func (m *Manager) ListOrders() []*Order {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Order, 0, len(m.orders))
	for _, o := range m.orders {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// GetOrder 按订单号取订单。
// 参数 id：订单号。
// 返回 (*Order, bool)：存在返回 true。
func (m *Manager) GetOrder(id string) (*Order, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.orders[id]
	return o, ok
}

// CancelOrder 删除订单并清掉其 HTTP-01 应答表条目。
// 参数 id：订单号。
// 返回 bool：订单存在且已删除返回 true。
func (m *Manager) CancelOrder(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.orders[id]; !ok {
		return false
	}
	delete(m.orders, id)
	for token, e := range m.httpTokens {
		if e.orderID == id {
			delete(m.httpTokens, token)
		}
	}
	return true
}

// ServeChallenge 应答 HTTP-01 挑战请求。由数据面在站点路由之前调用（仅 80 端口明文流量）。
// 参数 w：响应写出器；参数 r：客户端请求。
// 返回 bool：本次请求已被本包处理返回 true；非挑战路径或未知令牌返回 false，
// 调用方继续原有流程（可能是误配的业务路径）。
func (m *Manager) ServeChallenge(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, challengePathPrefix) {
		return false
	}
	token := strings.TrimPrefix(r.URL.Path, challengePathPrefix)
	m.mu.Lock()
	entry, ok := m.httpTokens[token]
	m.mu.Unlock()
	if !ok {
		return false
	}
	// 挑战应答绝不能被缓存：缓存住了后续真实请求会拿到过期凭据。
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(entry.keyAuth))
	return true
}

// validateDomain 校验域名的最小合法性：非空、无路径、无端口、无 scheme。
// 参数 d：待校验域名（泛域名允许）。
// 返回 error：非法返回原因。
func validateDomain(d string) error {
	d = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(d), "."))
	if d == "" {
		return fmt.Errorf("域名为空")
	}
	if strings.ContainsAny(d, "/\\?#") {
		return fmt.Errorf("域名 %s 含非法字符", d)
	}
	if strings.Contains(d, ":") {
		return fmt.Errorf("域名 %s 不应包含端口", d)
	}
	if !strings.HasPrefix(d, "*.") && len(strings.Split(d, ".")) < 2 {
		return fmt.Errorf("域名 %s 缺少顶级域", d)
	}
	return nil
}
