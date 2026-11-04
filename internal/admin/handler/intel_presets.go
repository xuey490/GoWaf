package handler

import (
	"strings"

	"github.com/gin-gonic/gin"

	"gowaf/internal/admin/middleware"
	"gowaf/internal/admin/response"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

// intelPreset 是内置的公共情报源模板。
//
// Approx 是 2026-11 实测的压缩后区间数，仅作面板展示参考，源方随时会变；
// Risk 是误杀风险评级，用来提示「全局开还是按站点开」。
type intelPreset struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	Kind       string `json:"kind"`
	Risk       string `json:"risk"`
	Approx     int    `json:"approx"`
	Note       string `json:"note"`
	NeedsKey   bool   `json:"needsKey"`
	HeaderName string `json:"headerName"`
	// Manual=true 表示地址必须由用户自备（如 CrowdSec Console 生成的 blocklist 地址），
	// 选中后不直接创建，而是把模板填进表单让用户补地址。
	Manual bool `json:"manual"`
}

var intelPresets = []intelPreset{
	{
		ID: "spamhaus-drop", Name: "Spamhaus DROP",
		URL:  "https://www.spamhaus.org/drop/drop.txt",
		Kind: "ip", Risk: "low", Approx: 1472,
		Note: "被劫持或租给犯罪团伙的整段网段，几乎没有正常访客，误杀极低，建议全局常开",
	},
	{
		ID: "firehol-level1", Name: "FireHOL level1",
		URL:  "https://raw.githubusercontent.com/firehol/blocklist-ipsets/master/firehol_level1.netset",
		Kind: "ip", Risk: "low", Approx: 3946,
		Note: "聚合多个低误报清单后的结果，覆盖僵尸网络与扫描器出口",
	},
	{
		ID: "et-compromised", Name: "ET 失陷主机",
		URL:  "https://rules.emergingthreats.net/blockrules/compromised-ips.txt",
		Kind: "ip", Risk: "low", Approx: 480,
		Note: "Emerging Threats 确认已被入侵的主机出口 IP，量小但很准",
	},
	{
		ID: "binarydefense", Name: "Binary Defense",
		URL:  "https://www.binarydefense.com/banlist.txt",
		Kind: "ip", Risk: "low", Approx: 563,
		Note: "商业蜜罐网络捕获的攻击源，更新及时",
	},
	{
		ID: "greensnow", Name: "GreenSnow",
		URL:  "https://blocklist.greensnow.co/greensnow.txt",
		Kind: "ip", Risk: "medium", Approx: 2192,
		Note: "以 SSH/Web 爆破为主，偶有共享出口 IP，建议先观察再全局",
	},
	{
		ID: "cins", Name: "CINS Army",
		URL:  "http://cinsscore.com/list/ci-badguys.txt",
		Kind: "ip", Risk: "medium", Approx: 5981,
		Note: "综合恶意 IP，更新频率一般，误杀中等",
	},
	{
		ID: "ipsum3", Name: "ipsum level3",
		URL:  "https://raw.githubusercontent.com/stamparm/ipsum/master/levels/3.txt",
		Kind: "ip", Risk: "medium", Approx: 8615,
		Note: "被 3 个以上黑名单同时收录的 IP，共识度高但仍有误杀",
	},
	{
		ID: "blocklistde", Name: "blocklist.de 全量",
		URL:  "https://lists.blocklist.de/lists/all.txt",
		Kind: "ip", Risk: "high", Approx: 13607,
		Note: "体量大，含大量家用动态宽带 IP（国内电信/联通段很多），强烈建议按站点使用",
	},
	{
		ID: "abuseipdb", Name: "AbuseIPDB 黑名单",
		URL:       "https://api.abuseipdb.com/api/v2/blacklist?plaintext=true&confidenceMinimum=90&limit=10000",
		Kind:      "ip", Risk: "medium", Approx: 10000,
		NeedsKey:  true,
		HeaderName: "Key",
		Note:      "需要免费账号的 API Key（写入请求头 Key）。免费额度：1 万条/次、每天 5 次拉取，后台 3 小时同步一次刚好够",
	},
	{
		ID: "crowdsec-cti", Name: "CrowdSec CTI",
		URL: "", Kind: "json", Risk: "medium", NeedsKey: true, HeaderName: "X-Api-Key", Manual: true,
		Note: "CrowdSec 的 blocklist 走它自己的引擎/控制台订阅，没有公开的纯列表接口；" +
			"请把控制台里生成的拉取地址与 Key 填进来（返回 JSON，本系统会自动提取其中的 IP）",
	},
}

// IntelPresets 返回内置公共源模板，并标出哪些已经添加过。
func (s *Server) IntelPresets(c *gin.Context) {
	var rows []model.IntelSource
	_ = s.Store.Gorm().Select("id", "url", "name").Find(&rows).Error
	installed := map[string]bool{}
	for _, r := range rows {
		installed[strings.TrimSpace(r.URL)] = true
	}
	list := make([]gin.H, 0, len(intelPresets))
	for _, p := range intelPresets {
		list = append(list, gin.H{
			"id": p.ID, "name": p.Name, "url": p.URL, "kind": p.Kind,
			"risk": p.Risk, "approx": p.Approx, "note": p.Note,
			"needsKey": p.NeedsKey, "headerName": p.HeaderName, "manual": p.Manual,
			"installed": p.URL != "" && installed[p.URL],
		})
	}
	response.OK(c, gin.H{"list": list})
}

type presetAddInput struct {
	IDs   []string `json:"ids"`
	Token string   `json:"token"`
	Scope string   `json:"scope"`
	Site  string   `json:"site"`
}

// IntelPresetAdd 批量添加预设源。已存在同 URL 的源会被跳过，可重复点击。
func (s *Server) IntelPresetAdd(c *gin.Context) {
	var in presetAddInput
	if err := c.ShouldBindJSON(&in); err != nil || len(in.IDs) == 0 {
		response.BadRequest(c, "请至少选择一个预设源")
		return
	}
	byID := map[string]intelPreset{}
	for _, p := range intelPresets {
		byID[p.ID] = p
	}
	scope := strings.TrimSpace(in.Scope)
	if scope == "" {
		scope = "global"
	}
	if scope == "site" && strings.TrimSpace(in.Site) == "" {
		response.BadRequest(c, "作用域为站点时必须指定站点")
		return
	}

	var existing []model.IntelSource
	_ = s.Store.Gorm().Select("url").Find(&existing).Error
	seen := map[string]bool{}
	for _, r := range existing {
		seen[strings.TrimSpace(r.URL)] = true
	}

	created, skipped := 0, 0
	for _, id := range in.IDs {
		p, ok := byID[id]
		if !ok {
			continue
		}
		// 需要自备地址的模板、以及没给 Key 的鉴权源，都不该被静默创建成空壳。
		if p.URL == "" {
			response.BadRequest(c, p.Name+" 需要先填写拉取地址，请在表单里补全")
			return
		}
		if p.NeedsKey && strings.TrimSpace(in.Token) == "" {
			response.BadRequest(c, p.Name+" 需要 API Key")
			return
		}
		if seen[p.URL] {
			skipped++
			continue
		}
		headers := ""
		if p.NeedsKey {
			headers = p.HeaderName + ": " + strings.TrimSpace(in.Token)
		}
		row := model.IntelSource{
			Name: p.Name, URL: p.URL, Kind: p.Kind, Headers: headers,
			Scope: scope, Site: strings.TrimSpace(in.Site),
			TTLSeconds: 3600, Enabled: 1, CreatedAt: store.NowMS(),
		}
		if err := s.Store.Gorm().Create(&row).Error; err != nil {
			s.failDB(c, err)
			return
		}
		seen[p.URL] = true
		created++
	}
	c.Set(middleware.CtxAuditObj, "intel_source")
	response.OK(c, gin.H{"created": created, "skipped": skipped})
}
