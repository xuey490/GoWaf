package ja4

import (
	_ "embed"
	"encoding/json"
	"sync"
)

// 客户端指纹参考清单（内置清单）。
//
// 清单按 category 分层，语义与拦截策略绑定：
//   - script（27 条）：脚本/自动化工具栈（Go/curl/python/node/.NET 及配置
//     变体）——双重用途，默认仅记录进事件库；guard.ja4.block_builtin 开启
//     时也**不会**拦截，避免误伤自动化运维与合法采集；
//   - browser（12 条）：Chromium/Firefox/Safari 基线（FoxIO 公开样例）——
//     用于「UA 伪装浏览器但指纹是脚本栈」的对照标注，永不拦截；
//   - malware（5 条）：公开报告的恶意软件 C2 栈（Cobalt Strike beacon ×4、
//     IcedID ×1，FoxIO 公开样例）——**block_builtin 唯一会拦截的类别**；
//   - vpn / iot / system（6 条）：VPN 客户端、IoT 固件、Windows 系统组件
//     ——参考数据，不拦截。
//
// 来源两类，逐条标注：
//  1. tools/ja4probe 本机真实握手采集（经验证）；
//  2. FoxIO-LLC/ja4 仓库的 ja4plus-mapping.csv（FoxIO License 1.1，其中
//     JA4 部分按 BSD-3 发布）。JA4DB 已转闭源（2026-05），其第三方镜像
//     许可状态不明，按许可纪律不直接入库。
//
// 管理员自定义拦截走 guard.ja4.blocklist（逗号分隔指纹，命中即拦），
// 结构与本清单无关。

// builtinEntry 内置清单条目。
type builtinEntry struct {
	Fingerprint string `json:"fingerprint"` // JA4 指纹
	Category    string `json:"category"`    // script/browser/malware/vpn/iot/system
	Label       string `json:"label"`       // 栈描述
	Source      string `json:"source"`      // 来源说明（采集方式/出处）
}

// CatMalware 恶意软件类目：block_builtin 开启时唯一会拦截的内置类别。
const CatMalware = "malware"

//go:embed malicious_ja4.json
var builtinJSON []byte

// builtinCache 进程内解析缓存（首次 Builtin 调用时解析一次）。
var (
	builtinOnce  sync.Once
	builtinItems []builtinEntry
)

// Builtin 返回内置指纹清单（解析失败时返回空清单，不影响主链路）。
// 返回 map[string]builtinEntry：指纹 -> 条目。
func Builtin() map[string]builtinEntry {
	builtinOnce.Do(func() {
		if err := json.Unmarshal(builtinJSON, &builtinItems); err != nil {
			builtinItems = nil
		}
	})
	out := make(map[string]builtinEntry, len(builtinItems))
	for _, it := range builtinItems {
		out[it.Fingerprint] = it
	}
	return out
}

// MatchBuiltin 判断指纹是否命中内置清单。
// 参数 fp：JA4 指纹。返回 (builtinEntry, bool)：命中返回条目与 true。
func MatchBuiltin(fp string) (builtinEntry, bool) {
	if fp == "" {
		return builtinEntry{}, false
	}
	e, ok := Builtin()[fp]
	return e, ok
}
