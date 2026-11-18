// ja4probe 是 JA4 指纹采集工具（P1-3 交付物）。
//
// 在本机起一个套了 ja4 嗅探器的 TLS 服务，然后用一批真实客户端
// （Go / curl / python / node / PowerShell / openssl，各带配置变体）逐个
// 握手，输出每个客户端栈的 JA4 指纹 JSON。采集结果用于扩充
// internal/ja4/malicious_ja4.json 的「脚本/自动化工具栈参考清单」。
//
// 用法：
//
//	go run ./tools/ja4probe            # 全量电池
//	go run ./tools/ja4probe -only curl # 只跑名字含 curl 的项
//
// 诚实口径：所有条目都是本机真实握手的产物（经验证）；同一客户端的不同
// TLS 配置（版本/ALPN/密码套件）会产出不同指纹，各自独立成条。
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"gowaf/internal/ja4"
)

// probeResult 单次采集结果。
type probeResult struct {
	Client string `json:"client"` // 客户端与配置描述
	JA4    string `json:"ja4"`    // 采集到的指纹
	Err    string `json:"err,omitempty"`
}

// probe 一项采集：name + 触发函数（url 指向本机采集服务）。
type probe struct {
	name string
	run  func(url string) error
}

func main() {
	only := flag.String("only", "", "只跑名字含该子串的项")
	flag.Parse()

	addr, url, closer, err := startCaptureServer()
	if err != nil {
		fmt.Fprintln(os.Stderr, "启动采集服务失败:", err)
		os.Exit(1)
	}
	defer closer()
	fmt.Fprintf(os.Stderr, "采集服务: %s (%s)\n", addr, url)

	probes := battery()
	out := make([]probeResult, 0, len(probes))
	for _, p := range probes {
		if *only != "" && !strings.Contains(p.name, *only) {
			continue
		}
		// 清掉上一项的残留：失败的探针不应继承前一项的指纹
		capturedMu.Lock()
		captured["latest"] = ""
		capturedMu.Unlock()
		res := probeResult{Client: p.name}
		if err := p.run(url); err != nil {
			res.Err = err.Error()
		}
		res.JA4 = lookupFP(addr, 3*time.Second)
		out = append(out, res)
		fmt.Fprintf(os.Stderr, "%-34s %s %s\n", p.name, res.JA4, res.Err)
	}
	data, _ := json.MarshalIndent(dedupe(out), "", "  ")
	fmt.Println(string(data))
}

// dedupe 合并相同指纹的条目：不同客户端/配置出现同一 JA4 是常见现象
// （同一 TLS 栈的等价配置），合并成一个条目并保留全部客户端说明。
func dedupe(in []probeResult) []probeResult {
	var out []probeResult
	seen := map[string]*probeResult{}
	for _, r := range in {
		if r.JA4 == "" {
			continue // 失败项（客户端未完成握手）不入清单
		}
		if prev, ok := seen[r.JA4]; ok {
			prev.Client += " / " + r.Client
			continue
		}
		cp := r
		seen[r.JA4] = &cp
		out = append(out, cp)
	}
	return out
}

// captured 注册表：采集服务收到的握手（key = 客户端地址）。
var (
	capturedMu sync.Mutex
	captured   = map[string]string{}
)

// lookupFP 读取最近一次请求登记的指纹（电池逐项串行执行，latest 即当前项）。
func lookupFP(_ string, wait time.Duration) string {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		capturedMu.Lock()
		v := captured["latest"]
		capturedMu.Unlock()
		if v != "" {
			return v
		}
		time.Sleep(20 * time.Millisecond)
	}
	return ""
}

// startCaptureServer 起一个带 ja4 嗅探的 TLS 服务。
// 返回监听地址、访问 URL 与关闭函数。
func startCaptureServer() (string, string, func(), error) {
	cert, err := selfSigned()
	if err != nil {
		return "", "", nil, err
	}
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h2", "http/1.1"},
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", "", nil, err
	}
	wrapped := ja4.Wrap(ln)
	srv := &http.Server{
		TLSConfig: tlsCfg,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 与数据面一致：按连接远端地址取指纹（嗅探器在握手时登记）
			fp := ja4.Lookup(r.RemoteAddr)
			capturedMu.Lock()
			captured["latest"] = fp
			capturedMu.Unlock()
			w.WriteHeader(200)
			fmt.Fprintln(w, "ok")
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		_ = srv.ServeTLS(wrapped, "", "")
	}()
	url := "https://" + ln.Addr().String()
	return ln.Addr().String(), url, func() { srv.Close(); ln.Close() }, nil
}

// selfSigned 生成临时自签证书。
func selfSigned() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "ja4probe.local"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"127.0.0.1", "localhost", "ja4probe.local"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tpl, &tpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}

// sniHost SNI 形态采集用的虚拟域名（自签证书已包含）。
const sniHost = "ja4probe.local"

// curlSNIRewrite 把 http://127.0.0.1:PORT 形态的 URL 改写成
// https://ja4probe.local:PORT 并加 --resolve 指回本机。
func curlSNIRewrite(url string) []string {
	addr := strings.TrimPrefix(url, "https://")
	host, port, _ := net.SplitHostPort(addr)
	resolve := fmt.Sprintf("%s:%s:%s", sniHost, port, host)
	target := fmt.Sprintf("https://%s:%s/", sniHost, port)
	return []string{"--resolve", resolve, target}
}

// sniURL 把 IP 形态 URL 的主机替换成 SNI 虚拟域名（客户端需配合
// --resolve / getaddrinfo 补丁 / servername 选项访问本机）。
func sniURL(url string) string {
	addr := strings.TrimPrefix(url, "https://")
	_, port, _ := net.SplitHostPort(addr)
	return fmt.Sprintf("https://%s:%s/", sniHost, port)
}

// battery 本机客户端采集矩阵。每个条目都是一次真实握手。
func battery() []probe {
	var ps []probe

	// —— Go 栈（含配置变体：h2 默认 / h1 / TLS1.2 / TLS1.3-only / RSA-only / ECDSA-only）——
	goVariants := []struct {
		name string
		cfg  func(c *tls.Config)
	}{
		{"go-http2-default(1.27)", func(c *tls.Config) {}},
		{"go-http1-only", func(c *tls.Config) { c.NextProtos = []string{"http/1.1"} }},
		{"go-tls12-max", func(c *tls.Config) { c.MaxVersion = tls.VersionTLS12; c.NextProtos = []string{"http/1.1"} }},
		{"go-tls13-min", func(c *tls.Config) { c.MinVersion = tls.VersionTLS13 }},
		{"go-rsa-ciphers-only", func(c *tls.Config) {
			c.CipherSuites = []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384, tls.TLS_RSA_WITH_AES_128_GCM_SHA256}
			c.NextProtos = []string{"http/1.1"}
		}},
		{"go-ecdsa-ciphers-only", func(c *tls.Config) {
			c.CipherSuites = []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305}
			c.NextProtos = []string{"http/1.1"}
		}},
		{"go-chacha-only", func(c *tls.Config) {
			c.CipherSuites = []uint16{tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305, tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305}
			c.NextProtos = []string{"http/1.1"}
		}},
		{"go-aes256-only", func(c *tls.Config) {
			c.CipherSuites = []uint16{tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384, tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384}
			c.NextProtos = []string{"http/1.1"}
		}},
		{"go-sni-domain-h2", func(c *tls.Config) { c.ServerName = sniHost }},
	}
	for _, v := range goVariants {
		vv := v
		ps = append(ps, probe{name: vv.name, run: func(url string) error {
			tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
			vv.cfg(tr.TLSClientConfig)
			client := &http.Client{Transport: tr, Timeout: 8 * time.Second}
			resp, err := client.Get(url)
			if err != nil {
				return err
			}
			return resp.Body.Close()
		}})
	}

	// —— curl（Windows 自带 Schannel 栈 + 配置变体 + SNI 形态）——
	// --resolve 让 curl 用域名访问本机服务，产生带 SNI 的 d 形态指纹。
	curlVariants := []struct {
		name string
		args []string
		sni  bool
	}{
		{"curl-schannel-default", []string{"-k"}, false},
		{"curl-schannel-no-alpn", []string{"-k", "--no-alpn"}, false},
		{"curl-schannel-sni", []string{"-k"}, true},
	}
	for _, v := range curlVariants {
		vv := v
		ps = append(ps, probe{name: vv.name, run: func(url string) error {
			args := append([]string{"-s", "-o", os.DevNull, "--max-time", "8"}, vv.args...)
			if vv.sni {
				args = append(args, curlSNIRewrite(url)...)
			} else {
				args = append(args, url)
			}
			return exec.Command("curl", args...).Run()
		}})
	}

	// —— python ssl（OpenSSL 栈 + 配置变体）——
	pyVariants := []struct {
		name string
		code string
	}{
		{"python-ssl-default", `
import ssl, urllib.request, sys
ctx = ssl._create_unverified_context()
print(urllib.request.urlopen(sys.argv[1], context=ctx, timeout=8).status)
`},
		{"python-tls12-max", `
import ssl, urllib.request, sys
ctx = ssl._create_unverified_context()
ctx.maximum_version = ssl.TLSVersion.TLSv1_2
print(urllib.request.urlopen(sys.argv[1], context=ctx, timeout=8).status)
`},
		{"python-aes256-ciphers", `
import ssl, urllib.request, sys
ctx = ssl._create_unverified_context()
ctx.set_ciphers("AES256")
print(urllib.request.urlopen(sys.argv[1], context=ctx, timeout=8).status)
`},
		{"python-sni-domain", `
import socket, ssl, urllib.request, sys
_orig = socket.getaddrinfo
host_name = sys.argv[2]
def _gi(host, *a, **k):
    if host == host_name:
        port = a[1] if len(a) > 1 else 443
        return [(socket.AF_INET, socket.SOCK_STREAM, 6, "", ("127.0.0.1", port))]
    return _orig(host, *a, **k)
socket.getaddrinfo = _gi
ctx = ssl._create_unverified_context()
print(urllib.request.urlopen(sys.argv[1], context=ctx, timeout=8).status)
`},
	}
	for _, v := range pyVariants {
		vv := v
		sni := vv.name == "python-sni-domain"
		ps = append(ps, probe{name: vv.name, run: func(url string) error {
			args := []string{"-I", "-c", vv.code}
			if sni {
				args = append(args, sniURL(url), sniHost)
			} else {
				args = append(args, url)
			}
			return exec.Command("python", args...).Run()
		}})
	}

	// —— node（OpenSSL 3 栈 + 配置变体）——
	nodeVariants := []struct {
		name string
		code string
	}{
		{"node-openssl-default", `
const https=require('https');
https.get(process.argv[1],{rejectUnauthorized:false},r=>{r.resume();process.exit(0)})
.on('error',e=>{console.error(e.message);process.exit(1)});
`},
		{"node-tls12-max", `
const https=require('https');
https.get(process.argv[1],{rejectUnauthorized:false,maxVersion:'TLSv1.2',ALPNProtocols:['http/1.1']},r=>{r.resume();process.exit(0)})
.on('error',e=>{console.error(e.message);process.exit(1)});
`},
		{"node-custom-ciphers", `
const https=require('https');
https.get(process.argv[1],{rejectUnauthorized:false,ciphers:'ECDHE-RSA-AES128-GCM-SHA256:ECDHE-RSA-AES256-GCM-SHA384:AES128-GCM-SHA256'},r=>{r.resume();process.exit(0)})
.on('error',e=>{console.error(e.message);process.exit(1)});
`},
		{"node-chacha-ciphers", `
const https=require('https');
https.get(process.argv[1],{rejectUnauthorized:false,ciphers:'ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305'},r=>{r.resume();process.exit(0)})
.on('error',e=>{console.error(e.message);process.exit(1)});
`},
		{"node-sni-domain", `
const https=require('https');
https.get({host:process.argv[2],port:process.argv[3],path:'/',servername:'ja4probe.local',rejectUnauthorized:false},r=>{r.resume();process.exit(0)})
.on('error',e=>{console.error(e.message);process.exit(1)});
`},
	}
	for _, v := range nodeVariants {
		vv := v
		sni := vv.name == "node-sni-domain"
		ps = append(ps, probe{name: vv.name, run: func(url string) error {
			args := []string{"-e", vv.code}
			if sni {
				addr := strings.TrimPrefix(url, "https://")
				host, port, _ := net.SplitHostPort(addr)
				args = append(args, host, port)
			} else {
				args = append(args, url)
			}
			return exec.Command("node", args...).Run()
		}})
	}

	// —— PowerShell/.NET Schannel（与 curl-Schannel 对比；PS5 兼容写法）——
	if runtime.GOOS == "windows" {
		ps = append(ps, probe{name: "powershell-dotnet-schannel", run: func(url string) error {
			cmd := fmt.Sprintf(
				"[Net.ServicePointManager]::SecurityProtocol=[Net.SecurityProtocolType]::Tls12;"+
					"add-type @\"\nusing System.Net;using System.Security.Cryptography.X509Certificates;\n"+
					"public class T { public static void V(object s,X509Certificate c,X509Chain ch,Net.SslPolicyErrors e){} }\n\"@;"+
					"[Net.ServicePointManager]::ServerCertificateValidationCallback=[T]::V;"+
					"(Invoke-WebRequest -UseBasicParsing -TimeoutSec 8 '%s').StatusCode", url)
			return exec.Command("powershell", "-NoProfile", "-Command", cmd).Run()
		}})
	}

	return ps
}
