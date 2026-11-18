package main

import (
	"fmt"
	"net/url"
	"strings"
)

// parsedReq 是样本原始 HTTP 请求的宽松解析结果。
// 不用 net/http.ReadRequest：走私类样本（TE/CL 并存、重复 CL）会被标准库
// 规范化或拒绝，跑分器必须原样保留头组合才能测到 sem-smugg。
type parsedReq struct {
	Method  string
	Path    string
	RawQuery string
	Host    string
	UA      string
	Referer string
	Cookies string
	Body    string
	others  []headerLine // 其余全部头（含重复头），按出现顺序
}

// headerLine 原样保留的一条头。
type headerLine struct {
	k, v string
}

// parseRaw 解析原始 HTTP 请求文本。格式：请求行 + CRLF 头区 + 空行 + 体。
// 宽松策略：接受 LF-only 行尾、容忍未知头、不校验方法合法性。
func parseRaw(data []byte) (*parsedReq, error) {
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	if i := strings.Index(s, "\n\n"); i >= 0 {
		return parseHeadBody(s[:i], s[i+2:], true)
	}
	return parseHeadBody(s, "", true)
}

// parseHeadBody 解析头区与请求体。
func parseHeadBody(head, body string, crlf bool) (*parsedReq, error) {
	lines := strings.Split(head, "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("空请求")
	}
	first := strings.TrimSpace(lines[0])
	parts := strings.Fields(first)
	if len(parts) < 2 {
		return nil, fmt.Errorf("请求行非法: %q", first)
	}
	r := &parsedReq{Method: strings.ToUpper(parts[0]), Body: body}
	target := parts[1]
	if i := strings.IndexByte(target, '?'); i >= 0 {
		r.Path = target[:i]
		r.RawQuery = target[i+1:]
	} else {
		r.Path = target
	}
	for _, ln := range lines[1:] {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		i := strings.IndexByte(ln, ':')
		if i <= 0 {
			continue
		}
		k, v := ln[:i], strings.TrimSpace(ln[i+1:])
		switch strings.ToLower(k) {
		case "host":
			r.Host = v
		case "user-agent":
			r.UA = v
		case "referer":
			r.Referer = v
		case "cookie":
			r.Cookies = v
		case "authorization":
			// 与 edge.buildInput 一致：authorization 不进头串
		default:
			r.others = append(r.others, headerLine{k: k, v: v})
		}
	}
	return r, nil
}

// minimalQueryEncode 保证 query 放置可解析的最小编码：空格/#/&/+/= 会破坏
// 请求行或参数结构，转成 %xx（Normalize 的多层解码会还原，等价于原文）。
func minimalQueryEncode(p string) string {
	r := strings.NewReplacer(
		" ", "%20", "#", "%23", "&", "%26", "+", "%2B", "=", "%3D", "?", "%3F",
	)
	return r.Replace(p)
}

// buildRaw 组装原始 HTTP 请求文本（\r\n 线格式，样本 = 真实线格式）。
// 参数 extra：追加的原始头行（走私样本用），每行一条 "K: v"。
func buildRaw(method, host, target, ua string, extra []string, body string) string {
	if method == "" {
		if body != "" {
			method = "POST"
		} else {
			method = "GET"
		}
	}
	var b strings.Builder
	b.WriteString(method)
	b.WriteString(" ")
	b.WriteString(target)
	b.WriteString(" HTTP/1.1\r\n")
	b.WriteString("Host: " + host + "\r\n")
	if ua != "" {
		b.WriteString("User-Agent: " + ua + "\r\n")
	}
	if body != "" {
		b.WriteString("Content-Type: application/x-www-form-urlencoded\r\n")
		b.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(body)))
	}
	for _, h := range extra {
		b.WriteString(h + "\r\n")
	}
	b.WriteString("Connection: close\r\n\r\n")
	b.WriteString(body)
	return b.String()
}

// uenc 供种子/生成器做 query 目标编码的小工具。
func uenc(p string) string { return url.QueryEscape(p) }
