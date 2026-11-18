// Package detectapi 把 GoWAF 的检测能力以独立 HTTP 服务形式暴露，
// 供 Nginx / Kong / Traefik / Ingress 等外部网关调用——这是接入形态上
// 与雷池 T1K 对标的能力：WAF 不必接管流量，只做「检测引擎」。
//
// 两个端点：
//
//	POST /detect  完整 JSON 协议（lua-resty / SDK 调用），返回结构化判定；
//	GET  /check   网关鉴权式（nginx auth_request / Traefik forwardAuth），
//	              2xx 放行、403 拦截、401+X-WAF-Action: challenge 需人机校验。
//
// 认证：配置 server.detect_token 后要求 Authorization: Bearer 或
// X-WAF-Token 头；未配置时无认证，此时监听地址务必绑回环或内网。
package detectapi

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gowaf/internal/detect"
	"gowaf/internal/runtime"
)

// Server 是独立检测服务的 HTTP 处理器。
type Server struct {
	rt    *runtime.Runtime
	token string // Bearer 令牌；空串表示不认证
}

// New 创建检测 API 处理器。
// 参数 rt：运行时中枢（共享数据面的全部引擎与设置）；token：访问令牌（可空）。
// 返回 *Server。
func New(rt *runtime.Runtime, token string) *Server {
	return &Server{rt: rt, token: strings.TrimSpace(token)}
}

// MaxDetectBody 单次检测请求体的字节上限（10MB）。
const MaxDetectBody = 10 << 20

// ServeHTTP 路由分发：/detect、/check、/healthz。
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.authOK(r) {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	switch {
	case r.URL.Path == "/healthz":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, `{"ok":true}`)
	case r.URL.Path == "/detect" && r.Method == http.MethodPost:
		s.handleDetect(w, r)
	case r.URL.Path == "/check":
		s.handleCheck(w, r)
	default:
		writeErr(w, http.StatusNotFound, "not found（可用端点：POST /detect、GET|POST /check、GET /healthz）")
	}
}

// authOK 校验访问令牌：未配置令牌时放行（由监听地址控制暴露面）。
func (s *Server) authOK(r *http.Request) bool {
	if s.token == "" {
		return true
	}
	if r.Header.Get("X-WAF-Token") == s.token {
		return true
	}
	auth := r.Header.Get("Authorization")
	return strings.HasPrefix(auth, "Bearer ") && strings.TrimPrefix(auth, "Bearer ") == s.token
}

// DetectRequest 是 /detect 的请求体。
type DetectRequest struct {
	Method  string            `json:"method"`   // HTTP 方法，缺省 GET
	Host    string            `json:"host"`     // 站点域名（Host 头）
	Path    string            `json:"path"`     // 请求路径
	Query   string            `json:"query"`    // 查询串（不含 ?）
	Body    string            `json:"body"`     // 请求体文本
	BodyB64 string            `json:"body_b64"` // 请求体 base64（二进制场景，优先于 body）
	Headers map[string]string `json:"headers"`  // 请求头键值对
	Site    string            `json:"site"`     // 站点名（供例外规则匹配，可空）
}

// DetectResponse 是判定结果。
type DetectResponse struct {
	Action     string       `json:"action"`     // pass / log / challenge / block / ban
	Score      int          `json:"score"`      // 累计得分
	Hits       []detect.Hit `json:"hits"`       // 命中明细
	RuleIDs    []string     `json:"rule_ids"`   // 命中规则 ID
	Categories []string     `json:"categories"` // 命中分类
	ElapsedUS  int64        `json:"elapsed_us"` // 检测耗时（微秒）
	Versions   struct {
		Rules string `json:"rules"` // 自研规则版本
		CRS   string `json:"crs"`   // CRS 版本（off 表示未启用）
		Mode  string `json:"mode"`  // 引擎模式 observe/protect
	} `json:"versions"`
}

// inspect 执行检测：与 edge 数据面同一套引擎编排（自研规则 + CRS +
// libinjection + 语义通道），动作由全局阈值与模式决定。
// 调度闸门不在此生效——网关按次付费调用，旁路策略由网关侧自行实现
// （如 lua 层缓存「同 path+query 哈希 60 秒内放行」）。
// 参数 method/host/path/query/body：请求要素；headerText：已拼好的
// "K: v" 头串（与 detect.Input.Headers 同格式）；site：站点名（可空）。
// 返回 DetectResponse：结构化判定。
func (s *Server) inspect(method, host, path, query, body, headerText, site string) DetectResponse {
	start := time.Now()
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "GET"
	}
	headers, ua, cookies := headerText, "", ""
	if hs := headerFields(headerText); len(hs) > 0 {
		for k, v := range hs {
			switch strings.ToLower(k) {
			case "user-agent":
				ua = v
			case "cookie":
				cookies = v
			}
		}
	}
	in := &detect.Input{
		Method: method, Host: host, Path: path, Query: query, Body: body,
		UA: ua, Headers: headers, Cookies: cookies, Site: site,
	}
	// 多层解码变体与数据面同配置
	if dc := s.rt.DecodeCfg(); dc.Enabled {
		qv, bv := detect.DecodeVariants(in.Query, in.Body, *dc)
		in.DecodedQuery, in.DecodedBody = qv, bv
	}

	engine := s.rt.Engine
	res := engine.Inspect(in)
	if crs := s.rt.CRS(); crs != nil && res.Action != "ban" {
		cres := crs.Inspect(in)
		engine.FilterByException(&cres, site, in.Path, in.Params)
		mergeInto(&res, &cres)
	}
	if li := s.rt.Libi(); li.Enabled() && res.Action != "ban" {
		lres := li.Inspect(in)
		engine.FilterByException(&lres, site, in.Path, in.Params)
		mergeInto(&res, &lres)
	}
	if se := s.rt.Sem(); se.Enabled() && res.Action != "ban" {
		sres := se.Inspect(in)
		engine.FilterByException(&sres, site, in.Path, in.Params)
		mergeInto(&res, &sres)
	}
	block, challenge := engine.Scores()
	out := DetectResponse{
		Action:     detect.Decide(res.Score, block, challenge, engine.Mode()),
		Score:      res.Score,
		Hits:       res.Hits,
		RuleIDs:    res.RuleIDs,
		Categories: res.Categories,
		ElapsedUS:  time.Since(start).Microseconds(),
	}
	out.Versions.Rules = engine.Info().Version
	if c := s.rt.CRS(); c != nil {
		out.Versions.CRS = c.Version()
	} else {
		out.Versions.CRS = "off"
	}
	out.Versions.Mode = engine.Mode()
	return out
}

// mergeInto 把 src 的命中并入 dst（分数累加、明细拼接）。
func mergeInto(dst, src *detect.Result) {
	dst.Score += src.Score
	dst.Hits = append(dst.Hits, src.Hits...)
	dst.RuleIDs = append(dst.RuleIDs, src.RuleIDs...)
	dst.Categories = append(dst.Categories, src.Categories...)
}

// writeErr 输出错误 JSON。
func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, `{"error":"`+msg+`"}`)
}

// writeJSON 输出判定结果与网关头。
func (s *Server) writeJSON(w http.ResponseWriter, code int, out DetectResponse) {
	w.Header().Set("X-WAF-Action", out.Action)
	w.Header().Set("X-WAF-Score", strconv.Itoa(out.Score))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(out)
}

// headerFields 把 "K1: v1 K2: v2" 头串还原成键值表（供 UA/Cookie 提取）。
// 与 detect.splitHeaderLines 相同的拼接语义：不含冒号的段拼回上一条。
func headerFields(s string) map[string]string {
	out := map[string]string{}
	if s == "" {
		return out
	}
	var lastK string
	for _, part := range strings.Split(s, " ") {
		if i := strings.Index(part, ":"); i > 0 {
			lastK = part[:i]
			out[lastK] = strings.TrimSpace(part[i+1:])
		} else if lastK != "" {
			out[lastK] += " " + part
		}
	}
	return out
}

// handleDetect 处理 POST /detect：完整 JSON 协议。
func (s *Server) handleDetect(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, MaxDetectBody+1))
	if err != nil || len(raw) > MaxDetectBody {
		writeErr(w, http.StatusRequestEntityTooLarge, "body too large")
		return
	}
	var dr DetectRequest
	if err := json.Unmarshal(raw, &dr); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	body := dr.Body
	if dr.BodyB64 != "" {
		if dec, err := base64.StdEncoding.DecodeString(dr.BodyB64); err == nil {
			body = string(dec)
		}
	}
	headerText := ""
	for k, v := range dr.Headers {
		headerText += k + ": " + v + " "
	}
	s.writeJSON(w, http.StatusOK, s.inspect(dr.Method, dr.Host, dr.Path, dr.Query, body, headerText, dr.Site))
}

// handleCheck 处理网关鉴权式调用（nginx auth_request / Traefik forwardAuth）。
//
// 请求信息取自 X-Forwarded-* 头（网关注入）：X-Forwarded-Method、
// X-Forwarded-Uri、X-Forwarded-Host；其余头原样参与检测。
// 响应约定：2xx 放行；403 拦截（block/ban）；401 且 X-WAF-Action: challenge
// 表示需人机校验（网关可自行决定是否透传挑战页）。响应头与 JSON 体均含判定详情。
func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	method := r.Header.Get("X-Forwarded-Method")
	if method == "" {
		method = r.Method
	}
	uri := r.Header.Get("X-Forwarded-Uri")
	if uri == "" {
		uri = r.URL.RequestURI()
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	path, query := uri, ""
	if i := strings.IndexByte(uri, '?'); i >= 0 {
		path, query = uri[:i], uri[i+1:]
	}
	headerText := ""
	for k, vs := range r.Header {
		if k == "Authorization" { // 令牌头不参与检测
			continue
		}
		for _, v := range vs {
			headerText += k + ": " + v + " "
		}
	}
	out := s.inspect(method, host, path, query, "", headerText, "")
	s.writeJSON(w, statusFor(out), out)
}

// statusFor 把动作映射为网关鉴权响应码。
func statusFor(out DetectResponse) int {
	switch out.Action {
	case "block", "ban":
		return http.StatusForbidden
	case "challenge":
		return http.StatusUnauthorized
	default:
		return http.StatusOK
	}
}
