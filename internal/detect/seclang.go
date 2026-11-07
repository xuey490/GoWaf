package detect

import (
	"fmt"
	"io/fs"
	"strings"

	corazaconf "github.com/corazawaf/coraza-coreruleset/coraza/v3"
	crsv4 "github.com/corazawaf/coraza-coreruleset/crs/v4"
	"github.com/corazawaf/coraza/v3"
	"github.com/jcchavezs/mergefs"

	"gowaf/internal/detect/crspatch"
)

/* ============================================================ SecLang 规则包 */

// CRSPackage 一个待编译进 CRS 引擎的 SecLang 规则包。
type CRSPackage struct {
	ID      string // slug，仅用于日志与注释标记
	Content string // SecLang 指令文本（可含 SecRule/SecAction/SecMarker/Include ...）
}

// secLangRootFS 返回编译 SecLang 包用的根 FS：
//   - crspatch 补打包仓库缺失文件；
//   - crs/v4 + coraza/v3 让第三方包能 Include @owasp_crs/*（CRS 插件包场景）。
//
// 返回 fs.FS：三者合并而成的根文件系统。
func secLangRootFS() fs.FS {
	return mergefs.Merge(crspatch.FS, crsv4.FS, corazaconf.FS)
}

// ValidateSecLang 独立编译校验一段 SecLang 文本，返回可读错误。
// 用 DetectionOnly + 内嵌 CRS 根 FS 编译，只验语法/语义，不执行规则；
// 包引用 @owasp_crs/* 文件也能通过（CRS 插件包场景）。
// 参数 content：待校验的 SecLang 指令文本。
// 返回 (int, error)：校验通过时为 (SecRule/SecAction 指令条数, nil)，失败时为 (0, err)。
func ValidateSecLang(content string) (int, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return 0, fmt.Errorf("规则包内容为空")
	}
	// 防呆：整包不允许改引擎开关，避免一个包打瘫全局策略。
	for _, banned := range []string{"SecRuleEngine", "SecRequestBodyAccess", "SecResponseBodyAccess"} {
		for _, line := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, banned) {
				return 0, fmt.Errorf("规则包不允许包含 %s（引擎全局开关由 WAF 管理）", banned)
			}
		}
	}
	directives := "SecRuleEngine DetectionOnly\n" + content
	waf, err := coraza.NewWAF(coraza.NewWAFConfig().
		WithRootFS(secLangRootFS()).
		WithDirectives(directives))
	if err != nil {
		return 0, err
	}
	// 编译成功即代表校验通过，WAF 实例用完即弃
	_ = waf
	// 统计 SecRule/SecAction 指令条数，作为规则数返回
	count := 0
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "SecRule ") || strings.HasPrefix(t, "SecAction ") {
			count++
		}
	}
	return count, nil
}
