package edge

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"gowaf/internal/guard"
)

// newRequestID 生成 16 位十六进制请求 ID，用于日志/响应头/面板串联。
// 返回 string：8 字节随机数的十六进制编码（16 字符）；随机源失败时退化为纳秒
// 时间戳的 16 位十六进制表示，保证 ID 始终可用。
func newRequestID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

// blockPage 返回拦截页 HTML。不暴露具体规则，避免帮攻击者调 payload。
// 参数 status：HTTP 状态码（大字显示）；参数 text：提示文案（HTML 转义后输出）；
// 参数 reqID：请求 ID（供用户反馈误报时定位日志）。
// 返回 string：完整 HTML 页面文本，由调用方自行写出。
func blockPage(status int, text, reqID string) string {
	return `<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + fmt.Sprint(status) + ` 请求被拦截</title>
<style>
body{font-family:-apple-system,"PingFang SC","Microsoft YaHei",sans-serif;background:#F6F8FB;
color:#334155;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}
.box{background:#fff;border:1px solid #E5E9F2;border-radius:12px;padding:40px 48px;text-align:center;
max-width:460px;box-shadow:0 4px 24px rgba(15,23,42,.06)}
.code{font-size:44px;font-weight:700;color:#DC2626;margin:0 0 8px}
h1{font-size:20px;margin:0 0 12px}
p{color:#64748B;margin:0 0 4px;font-size:14px}
.rid{margin-top:16px;font-size:12px;color:#94A3B8;font-family:ui-monospace,Menlo,monospace}
</style></head><body><div class="box">
<div class="code">` + fmt.Sprint(status) + `</div>
<h1>请求被安全策略拦截</h1>
<p>` + html.EscapeString(text) + `</p>
<p>如果这是误报，请把下方请求 ID 提供给管理员。</p>
<div class="rid">Request ID: ` + html.EscapeString(reqID) + `</div>
</div></body></html>`
}

// noStore 禁止浏览器与 CDN 缓存安全相关响应。
//
// 挑战页里带着目标地址，拦截页里带着请求 ID，两者都是按请求动态生成的。
// 站点前面挂了 CDN（Cloudflare 的 Cache Everything 规则会把 403 一起缓存）时，
// 少了这个头就会有「一次 403 被缓存、之后所有访客都拿到同一份挑战页」的问题，
// 表现是页面永远卡在校验中，或者正常用户莫名被拦。
// 参数 w：响应写出器，直接写入 Cache-Control/Pragma/Expires 三个响应头。
// 返回值无。
func noStore(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	// CDN-Cache-Control 是 Cloudflare 等 CDN 单独识别的缓存指令：
	// 站点若开了 Cache Everything，普通 Cache-Control 会被边缘规则覆盖。
	// 一旦挑战页被缓存，后面所有访客都会拿到同一份「校验中」的副本；
	// 拦截页被缓存则更糟——正常用户会莫名看到别人的 403。
	h.Set("CDN-Cache-Control", "no-store")
	h.Set("Pragma", "no-cache")
	h.Set("Expires", "0")
	// 校验页是 403 状态码，别让搜索引擎把「校验中」当成站点内容收录。
	h.Set("X-Robots-Tag", "noindex, nofollow")
}

// challengePassed 判断是否已通过人机校验。
// 参数 w：响应写出器（当前实现未使用，保留以备扩展）；参数 r：客户端请求；
// 参数 ip：客户端 IP（通行证与其绑定，换 IP 即失效）。
// 返回 bool：请求携带有效挑战 Cookie 且签名校验通过返回 true，否则 false。
func (h *Handler) challengePassed(w http.ResponseWriter, r *http.Request, ip netip.Addr) bool {
	ck, err := r.Cookie(h.rt.Chall.CookieName())
	if err != nil || ck.Value == "" {
		return false
	}
	return h.rt.Chall.Verify(ck.Value, ip.String(), time.Now())
}

// serveChallenge 返回挑战页。
//
// 挑战分两层，对用户都是「无感」的：
//  1. 页面必须能执行 JS —— 不跑 JS 的爬虫/扫描器拿不到通行证；
//  2. 页面要算出一道轻量工作量证明（SHA-256 前导零），服务端才签发通行证。
//
// 第 2 层是这次补上的关键：原实现只校验一个固定的 X-WAF-JS 请求头，
// 任何脚本加一行 header 就能拿到 30 分钟的通行证，等于没有校验。
// 现在令牌由服务端按 IP + 过期时间 + 难度签名，客户端必须真的算出答案。
// 参数 w：响应写出器；参数 r：客户端请求；参数 ip：客户端 IP。
// 返回值无：命中通行证路径时交给 issuePass 校验令牌，否则写出挑战页。
func (h *Handler) serveChallenge(w http.ResponseWriter, r *http.Request, ip netip.Addr) {
	if r.URL.Path == h.rt.Chall.PassPath() {
		h.issuePass(w, r, ip)
		return
	}

	token := h.rt.Chall.ChallengeToken(ip.String(), time.Now(), guard.ChallengePoWBits)
	// 令牌与目标地址都要放进 <script> 里的 JS 字符串：用 JSON 编码而不是 HTML 转义，
	// 否则 URL 里的 & 会被转成 &amp; 导致跳转地址错乱；再把 "</" 写成 "<\/"
	// 防止请求路径里带 </script> 提前闭合脚本块（XSS）。
	tokenJS := scriptString(token)
	targetJS := scriptString(r.URL.RequestURI())
	passJS := scriptString(h.rt.Chall.PassPath())

	noStore(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = io.WriteString(w, `<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>安全校验</title>
<style>
body{font-family:-apple-system,"PingFang SC",sans-serif;background:#F6F8FB;color:#334155;
display:flex;align-items:center;justify-content:center;height:100vh;margin:0}
.box{background:#fff;border:1px solid #E5E9F2;border-radius:12px;padding:40px 48px;
text-align:center;max-width:440px;box-shadow:0 4px 24px rgba(15,23,42,.06)}
h1{font-size:18px;margin:0 0 10px}p{color:#64748B;font-size:14px;margin:0}
.spin{width:28px;height:28px;border:3px solid #E5E9F2;border-top-color:#2563EB;
border-radius:50%;margin:0 auto 18px;animation:s .8s linear infinite}
.tip{margin-top:10px;font-size:12px;color:#94A3B8}
@keyframes s{to{transform:rotate(360deg)}}
</style></head><body><div class="box">
<div class="spin"></div>
<h1>正在进行安全校验</h1>
<p>请稍候，页面会自动跳转。</p>
<p class="tip" id="tip">校验浏览器环境…</p>
<noscript><p class="tip">当前浏览器禁用了 JavaScript，无法完成校验。</p></noscript>
</div>
<script data-cfasync="false">
(function(){
  var to=`+string(targetJS)+`, tok=`+string(tokenJS)+`, pass=`+string(passJS)+`, bits=`+strconv.Itoa(guard.ChallengePoWBits)+`;
  var tip=document.getElementById("tip");
  function rrot(x,n){return (x>>>n)|(x<<(32-n));}

  // 纯 JS 版 SHA-256：HTTPS/localhost 之外没有 crypto.subtle（非安全上下文），
  // 只依赖 crypto.subtle 会让纯 HTTP 部署的站点永远过不了校验，所以必须留兜底。
  function sha256hex(ascii){
    var K=[0x428a2f98,0x71374491,0xb5c0fbcf,0xe9b5dba5,0x3956c25b,0x59f111f1,0x923f82a4,0xab1c5ed5,
    0xd807aa98,0x12835b01,0x243185be,0x550c7dc3,0x72be5d74,0x80deb1fe,0x9bdc06a7,0xc19bf174,
    0xe49b69c1,0xefbe4786,0x0fc19dc6,0x240ca1cc,0x2de92c6f,0x4a7484aa,0x5cb0a9dc,0x76f988da,
    0x983e5152,0xa831c66d,0xb00327c8,0xbf597fc7,0xc6e00bf3,0xd5a79147,0x06ca6351,0x14292967,
    0x27b70a85,0x2e1b2138,0x4d2c6dfc,0x53380d13,0x650a7354,0x766a0abb,0x81c2c92e,0x92722c85,
    0xa2bfe8a1,0xa81a664b,0xc24b8b70,0xc76c51a3,0xd192e819,0xd6990624,0xf40e3585,0x106aa070,
    0x19a4c116,0x1e376c08,0x2748774c,0x34b0bcb5,0x391c0cb3,0x4ed8aa4a,0x5b9cca4f,0x682e6ff3,
    0x748f82ee,0x78a5636f,0x84c87814,0x8cc70208,0x90befffa,0xa4506ceb,0xbef9a3f7,0xc67178f2];
    var H=[0x6a09e667,0xbb67ae85,0x3c6ef372,0xa54ff53a,0x510e527f,0x9b05688c,0x1f83d9ab,0x5be0cd19];
    var bytes=[],i,c;
    for(i=0;i<ascii.length;i++){
      c=ascii.charCodeAt(i);
      if(c<128)bytes.push(c);
      else if(c<2048)bytes.push(192|(c>>6),128|(c&63));
      else bytes.push(224|(c>>12),128|((c>>6)&63),128|(c&63));
    }
    var bitLen=bytes.length*8;
    bytes.push(0x80);
    while(bytes.length%64!==56)bytes.push(0);
    bytes.push(0,0,0,0,(bitLen>>>24)&255,(bitLen>>>16)&255,(bitLen>>>8)&255,bitLen&255);
    var w=new Array(64);
    for(var off=0;off<bytes.length;off+=64){
      for(i=0;i<16;i++)w[i]=(bytes[off+i*4]<<24)|(bytes[off+i*4+1]<<16)|(bytes[off+i*4+2]<<8)|bytes[off+i*4+3];
      for(i=16;i<64;i++){
        var s0=rrot(w[i-15],7)^rrot(w[i-15],18)^(w[i-15]>>>3);
        var s1=rrot(w[i-2],17)^rrot(w[i-2],19)^(w[i-2]>>>10);
        w[i]=(w[i-16]+s0+w[i-7]+s1)|0;
      }
      var a=H[0],b=H[1],cc=H[2],d=H[3],e=H[4],f=H[5],g=H[6],hh=H[7];
      for(i=0;i<64;i++){
        var S1=rrot(e,6)^rrot(e,11)^rrot(e,25);
        var ch=(e&f)^((~e)&g);
        var t1=(hh+S1+ch+K[i]+w[i])|0;
        var S0=rrot(a,2)^rrot(a,13)^rrot(a,22);
        var mj=(a&b)^(a&cc)^(b&cc);
        var t2=(S0+mj)|0;
        hh=g;g=f;f=e;e=(d+t1)|0;d=cc;cc=b;b=a;a=(t1+t2)|0;
      }
      H[0]=(H[0]+a)|0;H[1]=(H[1]+b)|0;H[2]=(H[2]+cc)|0;H[3]=(H[3]+d)|0;
      H[4]=(H[4]+e)|0;H[5]=(H[5]+f)|0;H[6]=(H[6]+g)|0;H[7]=(H[7]+hh)|0;
    }
    var out="";
    for(i=0;i<8;i++)out+=("00000000"+(H[i]>>>0).toString(16)).slice(-8);
    return out;
  }

  function zerosOf(hex){
    var z=0;
    for(var i=0;i<hex.length;i+=2){
      var b=parseInt(hex.substr(i,2),16);
      if(b===0){z+=8;continue;}
      z+=Math.clz32(b)-24;
      break;
    }
    return z;
  }

  // 直接跑纯 JS 哈希：crypto.subtle 是异步 API，逐次调用有固定开销，
  // 16 位难度下反而比同步循环慢好几倍；纯 JS 在主流浏览器约 20 万次/秒，
  // 平均 6.5 万次（约 0.3 秒）就能出解，且不依赖 HTTPS 安全上下文。
  var counter=0, maxCounter=8000000;
  function work(){
    var end=Math.min(counter+2048,maxCounter);
    for(;counter<end;counter++){
      if(zerosOf(sha256hex(tok+"|"+counter))>=bits){ finish(counter); return; }
    }
    if(counter>=maxCounter){ tip.textContent="校验未通过，正在重试…"; setTimeout(function(){location.reload();},800); return; }
    tip.textContent="校验浏览器环境…  "+Math.min(99,Math.round(counter/65536*100))+"%";
    setTimeout(work,0);
  }

  // 算出答案后换取通行证：服务端校验令牌 + 答案，通过则 Set-Cookie 并可跳回原地址。
  function finish(c){
    tip.textContent="校验通过，正在跳转…";
    fetch(pass+"?t="+encodeURIComponent(tok)+"&c="+c+"&to="+encodeURIComponent(to),{
      headers:{"X-WAF-JS":"1"},credentials:"same-origin",cache:"no-store"
    }).then(function(r){
      if(!r.ok) throw new Error("pass failed");
      location.replace(to);
    }).catch(function(){
      setTimeout(function(){location.reload();},1200);
    });
  }

  work();
})();
</script></body></html>`)
}

// scriptString 把任意字符串编码成可安全嵌入 <script> 的 JSON 字面量。
// 参数 v：待编码字符串；返回 string：带引号的 JSON 文本，"</" 已转义为 "<\/"。
func scriptString(v string) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `""`
	}
	return strings.ReplaceAll(string(b), "</", "<\\/")
}

// issuePass 验完工作量证明后下发通行证并跳回原地址。
//
// 令牌 t 与答案 c 都来自挑战页：t 是服务端签名的挑战令牌，c 是客户端算出的计数器。
// 只带 X-WAF-JS 头、没有合法令牌与答案的请求一律拒绝——原实现正是在这里
// 无条件签发通行证，等于「加一行请求头就过校验」。
// 参数 w：响应写出器；参数 r：客户端请求；参数 ip：客户端 IP（签发时绑定，校验时比对）。
// 返回值无：校验通过则签发通行证 Cookie（HttpOnly，30 分钟有效），JS 请求返回 204；
// 普通请求 302 跳回 ?to= 指定的站内路径；校验失败返回 403 提示页。
func (h *Handler) issuePass(w http.ResponseWriter, r *http.Request, ip netip.Addr) {
	challenge := r.URL.Query().Get("t")
	counter, err := strconv.ParseUint(r.URL.Query().Get("c"), 10, 64)
	if challenge == "" || err != nil || !h.rt.Chall.VerifyPoW(challenge, ip.String(), counter, time.Now()) {
		noStore(w)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, blockPage(http.StatusForbidden,
			"人机校验未通过（校验令牌缺失、过期或答案不正确），请刷新页面重试。", newRequestID()))
		return
	}
	token := h.rt.Chall.Sign(ip.String(), time.Now())
	// 下发通行证的响应必须不缓存：缓存住了别人就能复用这张 Cookie。
	noStore(w)
	http.SetCookie(w, &http.Cookie{
		Name:     h.rt.Chall.CookieName(),
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   1800,
	})
	if r.Header.Get("X-WAF-JS") == "1" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	to := r.URL.Query().Get("to")
	if to == "" || !strings.HasPrefix(to, "/") {
		to = "/"
	}
	http.Redirect(w, r, to, http.StatusFound)
}
