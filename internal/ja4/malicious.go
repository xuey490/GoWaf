package ja4

import (
	_ "embed"
	"encoding/json"
	"sync"
)

// 恶意/脚本栈指纹清单。
//
// 使用方式分两层：
//  1. Builtin() 内置清单：常见脚本与工具栈的指纹（本仓库真实握手采集，
//     逐条附来源与采集方式）。默认仅记录，是否拦截由运行时开关控制
//     （guard.ja4.block_builtin）——这类栈同时被大量正常工具使用，
//     默认拦截会误伤自动化运维与合法采集。
//  2. 管理员配置的 blocklist（guard.ja4.blocklist）：明确的恶意指纹，
//     命中即拦。条目可从社区 JA4 数据库（FoxIO ja4DB、各类威胁情报）
//     持续补充，清单结构与本文件一致。
//
// 诚实声明：内置清单目前只含两条真实采集的种子，不覆盖「top 恶意指纹」；
// 拦截效果主要依赖管理员维护的 blocklist。种子条目随测试演进可持续扩充。

// builtinEntry 内置清单条目。
type builtinEntry struct {
	Fingerprint string `json:"fingerprint"` // JA4 指纹
	Label       string `json:"label"`       // 栈描述
	Source      string `json:"source"`      // 来源说明（采集方式/出处）
}

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
