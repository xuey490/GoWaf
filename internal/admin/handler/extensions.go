package handler

// 本文件承载第一批到第三批扩展能力的控制面接口：
//   - 精准访问控制策略（access_policies）
//   - Bot 策略与蜜罐路径（bot_policies / bot_honeypots）
//   - 页面缓存规则、设置、清空与统计（cache_rules）
//   - 接入节点与 Agent 配置/心跳（nodes）
//   - 站点多上游（site_upstreams）
//
// 所有写操作完成后调用 reloadRuntime，让数据面秒级热生效；
// 列表接口统一返回 {list,total}，与规则例外等既有页面保持一致。

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"gowaf/internal/admin/middleware"
	"gowaf/internal/admin/response"
	"gowaf/internal/bot"
	"gowaf/internal/cache"
	"gowaf/internal/policy"
	"gowaf/internal/runtime"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

/* ====================================================== 通用 */

// createRow 创建记录，并强制把 enabled 字段按调用方意图落库。
//
// 背景：模型的 Enabled 字段带 `default:1` 标签，GORM 在插入时会把零值字段
// 直接省略、交给数据库默认值，于是「新增时选择停用」（enabled=0）会被悄悄
// 写成启用。对策略 / 规则类配置而言这是安全隐患——用户以为只是先存着不发，
// 实际已经对全网生效。这里在创建成功后补一次显式更新把意图纠正回来。
//
// 参数 row：已填充的模型指针（创建后 ID 会被回填）；参数 enabled：期望的启用状态。
// 返回 error：创建或更新失败时返回错误。
func (s *Server) createRow(row any, enabled int) error {
	if err := s.Store.Gorm().Create(row).Error; err != nil {
		return err
	}
	if enabled != 0 {
		return nil
	}
	return s.Store.Gorm().Model(row).Update("enabled", 0).Error
}

/* ====================================================== 精准访问控制 */

// policyInput 是策略新增/修改请求体。
type policyInput struct {
	Name       string `json:"name"`
	Site       string `json:"site"`
	Priority   *int   `json:"priority"`
	Enabled    *int   `json:"enabled"`
	Action     string `json:"action"`
	Conditions string `json:"conditions"`
	// Config 动作附加参数（JSON 对象）：auth（mode/users/title/realm/ttlHours/password）
	// 与 rate_limit（windowSec/threshold/onExceed）。auth 的密码以明文传入，
	// 服务端 bcrypt 哈希后落库；列表回显时哈希一律掩码。
	Config json.RawMessage `json:"config"`
	Note   string          `json:"note"`
}

// policyConfigHashPattern 匹配 config JSON 里的 bcrypt 哈希字段值（回显掩码用）。
var policyConfigHashPattern = regexp.MustCompile(`"(?:passwordHash|hash)":\s*"\$2[aby]\$[^"]*"`)

// maskConfigHashes 把 config JSON 中的 bcrypt 哈希替换为掩码。
// 参数 cfg：config 列原文。
// 返回 string：掩码后的 JSON；原文为空原样返回。
func maskConfigHashes(cfg string) string {
	if cfg == "" {
		return ""
	}
	return policyConfigHashPattern.ReplaceAllString(cfg, `"passwordHash":"******"`)
}

// PolicyList 查询精准访问控制策略。
// 路由：GET /api/policies（仅需登录）。
// config 列中的密码哈希以 ****** 掩码回显（bcrypt 虽难逆，仍不外露）。
func (s *Server) PolicyList(c *gin.Context) {
	q := s.Store.Gorm().Model(&model.AccessPolicy{})
	if site := strings.TrimSpace(c.Query("site")); site != "" {
		q = q.Where("site = ? OR site = ''", site)
	}
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		q = q.Where("name LIKE ? OR note LIKE ?", "%"+kw+"%", "%"+kw+"%")
	}
	var rows []model.AccessPolicy
	if err := q.Order("priority ASC, id ASC").Limit(500).Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	for i := range rows {
		rows[i].Config = maskConfigHashes(rows[i].Config)
	}
	response.OK(c, gin.H{"list": rows, "total": len(rows)})
}

// PolicyCreate 新增精准访问控制策略。
// 路由：POST /api/policies（policy:write）。
func (s *Server) PolicyCreate(c *gin.Context) {
	var in policyInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	row, msg := buildPolicy(nil, &in)
	if msg != "" {
		response.BadRequest(c, msg)
		return
	}
	if err := s.createRow(row, row.Enabled); err != nil {
		s.failDB(c, err)
		return
	}
	c.Set(middleware.CtxAuditObj, "policy")
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "策略已保存但热加载失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"id": row.ID})
}

// PolicyUpdate 修改精准访问控制策略。
// 路由：PUT /api/policies/:id（policy:write）。
func (s *Server) PolicyUpdate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的策略 ID")
		return
	}
	var row model.AccessPolicy
	if err := s.Store.Gorm().First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "策略不存在")
			return
		}
		s.failDB(c, err)
		return
	}
	var in policyInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	next, msg := buildPolicy(&row, &in)
	if msg != "" {
		response.BadRequest(c, msg)
		return
	}
	if err := s.Store.Gorm().Save(next).Error; err != nil {
		s.failDB(c, err)
		return
	}
	c.Set(middleware.CtxAuditObj, "policy")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(id, 10))
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "策略已保存但热加载失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"id": next.ID})
}

// PolicyToggle 启用/停用策略。
// 路由：PATCH /api/policies/:id/toggle（policy:write）。
func (s *Server) PolicyToggle(c *gin.Context) {
	s.toggleRow(c, &model.AccessPolicy{}, "策略")
}

// PolicyDelete 删除策略。
// 路由：DELETE /api/policies/:id（policy:write）。
func (s *Server) PolicyDelete(c *gin.Context) {
	s.deleteRow(c, &model.AccessPolicy{}, "策略", "policy")
}

// policyAuthCfgIn 是 auth 动作 config 的输入形态：密码为明文，服务端哈希。
type policyAuthCfgIn struct {
	Mode  string `json:"mode"`            // basic / page
	Realm string `json:"realm,omitempty"` // Basic realm
	Title string `json:"title,omitempty"` // 密码页标题
	Users []struct {
		User     string `json:"user"`     // 用户名
		Password string `json:"password"` // 明文密码；修改时留空 = 沿用原哈希
	} `json:"users,omitempty"`
	Password string `json:"password,omitempty"` // page 模式明文密码；修改时留空 = 沿用原哈希
	TTLHours int    `json:"ttlHours,omitempty"` // 通行证有效期（小时），默认 8
}

// normalizePolicyConfig 把前端传来的动作参数归一化成落库 JSON。
// 参数 action：策略动作；参数 old：原记录（修改时沿用旧哈希）；参数 raw：config 原文。
// 返回 (string, string)：落库 JSON 与错误提示（空串表示合法）。
// 非 auth / rate_limit 动作一律返回空串（清空无参动作的残留 config）。
func normalizePolicyConfig(action string, old *model.AccessPolicy, raw json.RawMessage) (string, string) {
	switch action {
	case "auth":
		if len(raw) == 0 {
			// 修改时未传 config：沿用原配置（原记录也是 auth 才成立）
			return "", "auth 策略缺少 config（认证方式与密码）"
		}
		var in policyAuthCfgIn
		if err := json.Unmarshal(raw, &in); err != nil {
			return "", "auth config 解析失败: " + err.Error()
		}
		in.Mode = strings.ToLower(strings.TrimSpace(in.Mode))
		if in.Mode != "basic" && in.Mode != "page" {
			return "", "auth mode 只能是 basic 或 page"
		}
		// 修改时从原 config 提取旧哈希，明文留空的账号沿用
		var oldCfg policy.AuthConfig
		if old != nil && old.Config != "" {
			_ = json.Unmarshal([]byte(old.Config), &oldCfg)
		}
		out := map[string]any{
			"mode":   in.Mode,
			"ttlSec": 0,
		}
		switch in.Mode {
		case "basic":
			if len(in.Users) == 0 {
				return "", "basic 认证至少需要一个账号"
			}
			users := make([]policy.AuthUser, 0, len(in.Users))
			oldUsers := map[string]string{}
			for _, u := range oldCfg.Users {
				oldUsers[u.User] = u.PasswordHash
			}
			for _, u := range in.Users {
				user := strings.TrimSpace(u.User)
				if user == "" {
					return "", "账号用户名不能为空"
				}
				hash := ""
				if u.Password == "" || u.Password == "******" {
					// 明文留空：沿用同名账号的旧哈希
					hash = oldUsers[user]
					if hash == "" {
						return "", "账号 " + user + " 未填写密码（新账号必须设置密码）"
					}
				} else {
					h, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
					if err != nil {
						return "", "密码哈希失败: " + err.Error()
					}
					hash = string(h)
				}
				users = append(users, policy.AuthUser{User: user, PasswordHash: hash})
			}
			out["users"] = users
			if strings.TrimSpace(in.Realm) != "" {
				out["realm"] = strings.TrimSpace(in.Realm)
			}
		case "page":
			hash := ""
			switch {
			case in.Password != "" && in.Password != "******":
				h, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
				if err != nil {
					return "", "密码哈希失败: " + err.Error()
				}
				hash = string(h)
			case oldCfg.PasswordHash != "":
				hash = oldCfg.PasswordHash // 留空 = 沿用旧哈希
			default:
				return "", "page 认证必须设置访问密码"
			}
			out["passwordHash"] = hash
			if strings.TrimSpace(in.Title) != "" {
				out["title"] = strings.TrimSpace(in.Title)
			}
		}
		if in.TTLHours > 0 {
			out["ttlSec"] = in.TTLHours * 3600
		} else if oldCfg.TTLSec > 0 {
			out["ttlSec"] = oldCfg.TTLSec
		}
		b, err := json.Marshal(out)
		if err != nil {
			return "", "auth config 序列化失败"
		}
		return string(b), ""

	case "rate_limit":
		if len(raw) == 0 {
			return "", "rate_limit 策略缺少 config（窗口与阈值）"
		}
		// 原样透传给 policy.Compile 校验（窗口/阈值/动作的边界在那里）
		var v map[string]any
		if err := json.Unmarshal(raw, &v); err != nil {
			return "", "rate_limit config 解析失败: " + err.Error()
		}
		b, err := json.Marshal(v)
		if err != nil {
			return "", "rate_limit config 序列化失败"
		}
		return string(b), ""

	default:
		// 无参动作：清空残留 config
		return "", ""
	}
}

// buildPolicy 把请求体合并进模型并做完整校验。
// 参数 old：修改时的原记录，新增传 nil；in：请求体。
// 返回 (*model.AccessPolicy, string)：模型与错误提示（空串表示合法）。
//
// auth / rate_limit 两个动作的 config 在这里归一化：
//   - auth：前端传明文密码（users[].password / password），服务端 bcrypt 哈希；
//     修改时密码留空 = 沿用原哈希（掩码 ****** 表示未改动）；
//   - rate_limit：窗口/阈值/超限动作原样落库，合法性由 policy.Compile 校验。
func buildPolicy(old *model.AccessPolicy, in *policyInput) (*model.AccessPolicy, string) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, "策略名称不能为空"
	}
	action := strings.ToLower(strings.TrimSpace(in.Action))
	switch action {
	case "", "allow", "block", "challenge", "log", "ban", "auth", "rate_limit":
	default:
		return nil, "动作只能是 allow/block/challenge/log/ban/auth/rate_limit"
	}
	if action == "" {
		action = "block"
	}
	raw := strings.TrimSpace(in.Conditions)
	if raw == "" {
		return nil, "条件不能为空（空条件会匹配所有请求）"
	}
	group, err := policy.DecodeGroup(raw)
	if err != nil {
		return nil, "条件 JSON 解析失败: " + err.Error()
	}
	// 门禁类动作的 config 归一化：明文密码换 bcrypt 哈希后再做编译校验。
	configJSON, cfgMsg := normalizePolicyConfig(action, old, in.Config)
	if cfgMsg != "" {
		return nil, cfgMsg
	}
	if _, err := policy.Compile([]policy.Definition{{
		ID: 1, Name: name, Site: strings.TrimSpace(in.Site), Priority: 100,
		Enabled: true, Action: policy.Action(action), Conditions: group, Config: configJSON,
	}}); err != nil {
		return nil, err.Error()
	}

	row := &model.AccessPolicy{}
	if old != nil {
		*row = *old
	}
	row.Name = name
	row.Site = strings.TrimSpace(in.Site)
	row.Action = action
	row.Conditions = raw
	row.Config = configJSON
	row.Note = strings.TrimSpace(in.Note)
	if in.Priority != nil {
		row.Priority = *in.Priority
	} else if old == nil {
		row.Priority = 100
	}
	if in.Enabled != nil {
		row.Enabled = *in.Enabled
	} else if old == nil {
		row.Enabled = 1
	}
	if old == nil {
		row.CreatedAt = store.NowMS()
	}
	row.UpdatedAt = store.NowMS()
	return row, ""
}

/* ====================================================== Bot 管理 */

// botInput 是 Bot 策略新增/修改请求体。
type botInput struct {
	Name      string `json:"name"`
	Site      string `json:"site"`
	Category  string `json:"category"`
	Action    string `json:"action"`
	MatchType string `json:"matchType"`
	Pattern   string `json:"pattern"`
	Priority  *int   `json:"priority"`
	Enabled   *int   `json:"enabled"`
	Note      string `json:"note"`
}

// BotList 查询 Bot 策略。
// 路由：GET /api/bots（仅需登录）。
func (s *Server) BotList(c *gin.Context) {
	q := s.Store.Gorm().Model(&model.BotPolicy{})
	if site := strings.TrimSpace(c.Query("site")); site != "" {
		q = q.Where("site = ? OR site = ''", site)
	}
	var rows []model.BotPolicy
	if err := q.Order("priority ASC, id ASC").Limit(500).Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	response.OK(c, gin.H{"list": rows, "total": len(rows)})
}

// BotCreate 新增 Bot 策略。
// 路由：POST /api/bots（bot:write）。
func (s *Server) BotCreate(c *gin.Context) {
	var in botInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	row, msg := buildBot(nil, &in)
	if msg != "" {
		response.BadRequest(c, msg)
		return
	}
	if err := s.createRow(row, row.Enabled); err != nil {
		s.failDB(c, err)
		return
	}
	c.Set(middleware.CtxAuditObj, "bot")
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "Bot 策略已保存但热加载失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"id": row.ID})
}

// BotUpdate 修改 Bot 策略。
// 路由：PUT /api/bots/:id（bot:write）。
func (s *Server) BotUpdate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 Bot 策略 ID")
		return
	}
	var row model.BotPolicy
	if err := s.Store.Gorm().First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "Bot 策略不存在")
			return
		}
		s.failDB(c, err)
		return
	}
	var in botInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	next, msg := buildBot(&row, &in)
	if msg != "" {
		response.BadRequest(c, msg)
		return
	}
	if err := s.Store.Gorm().Save(next).Error; err != nil {
		s.failDB(c, err)
		return
	}
	c.Set(middleware.CtxAuditObj, "bot")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(id, 10))
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "Bot 策略已保存但热加载失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"id": next.ID})
}

// BotToggle 启用/停用 Bot 策略。
// 路由：PATCH /api/bots/:id/toggle（bot:write）。
func (s *Server) BotToggle(c *gin.Context) {
	s.toggleRow(c, &model.BotPolicy{}, "Bot 策略")
}

// BotDelete 删除 Bot 策略。
// 路由：DELETE /api/bots/:id（bot:write）。
func (s *Server) BotDelete(c *gin.Context) {
	s.deleteRow(c, &model.BotPolicy{}, "Bot 策略", "bot")
}

// buildBot 把请求体合并进模型并做完整校验。
func buildBot(old *model.BotPolicy, in *botInput) (*model.BotPolicy, string) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, "策略名称不能为空"
	}
	pattern := strings.TrimSpace(in.Pattern)
	if pattern == "" {
		return nil, "匹配内容不能为空"
	}
	matchType := strings.ToLower(strings.TrimSpace(in.MatchType))
	switch matchType {
	case "ua", "path", "rdns":
		if _, err := regexp.Compile("(?i)" + pattern); err != nil {
			return nil, "正则非法: " + err.Error()
		}
	case "ip":
		if _, err := parseAddrOrPrefix(pattern); err != nil {
			return nil, "IP/CIDR 非法: " + err.Error()
		}
	default:
		return nil, "matchType 只能是 ua/path/ip/rdns"
	}
	action := strings.ToLower(strings.TrimSpace(in.Action))
	switch action {
	case "":
		action = string(bot.ActionChallenge)
	case "allow", "log", "challenge", "block", "ban":
	default:
		return nil, "动作只能是 allow/log/challenge/block/ban"
	}
	row := &model.BotPolicy{}
	if old != nil {
		*row = *old
	}
	row.Name = name
	row.Site = strings.TrimSpace(in.Site)
	row.Category = strings.TrimSpace(in.Category)
	if row.Category == "" {
		row.Category = "unknown"
	}
	row.Action = action
	row.MatchType = matchType
	row.Pattern = pattern
	row.Note = strings.TrimSpace(in.Note)
	if in.Priority != nil {
		row.Priority = *in.Priority
	} else if old == nil {
		row.Priority = 100
	}
	if in.Enabled != nil {
		row.Enabled = *in.Enabled
	} else if old == nil {
		row.Enabled = 1
	}
	if old == nil {
		row.CreatedAt = store.NowMS()
	}
	row.UpdatedAt = store.NowMS()
	return row, ""
}

// parseAddrOrPrefix 校验 IP 或 CIDR。
func parseAddrOrPrefix(v string) (string, error) {
	if _, err := netip.ParsePrefix(v); err == nil {
		return v, nil
	}
	if _, err := netip.ParseAddr(v); err == nil {
		return v, nil
	}
	return "", fmt.Errorf("既不是 IP 也不是 CIDR")
}

// honeypotInput 是蜜罐路径新增/修改请求体。
type honeypotInput struct {
	Site    string `json:"site"`
	Path    string `json:"path"`
	Note    string `json:"note"`
	Enabled *int   `json:"enabled"`
}

// HoneypotList 查询蜜罐路径。
// 路由：GET /api/bot-honeypots（仅需登录）。
func (s *Server) HoneypotList(c *gin.Context) {
	var rows []model.BotHoneypot
	if err := s.Store.Gorm().Order("id DESC").Limit(500).Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	response.OK(c, gin.H{"list": rows, "total": len(rows)})
}

// HoneypotCreate 新增蜜罐路径。
// 路由：POST /api/bot-honeypots（bot:write）。
func (s *Server) HoneypotCreate(c *gin.Context) {
	var in honeypotInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	row, msg := buildHoneypot(nil, &in)
	if msg != "" {
		response.BadRequest(c, msg)
		return
	}
	if err := s.createRow(row, row.Enabled); err != nil {
		s.failDB(c, err)
		return
	}
	c.Set(middleware.CtxAuditObj, "bot-honeypot")
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "蜜罐已保存但热加载失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"id": row.ID})
}

// HoneypotUpdate 修改蜜罐路径。
// 路由：PUT /api/bot-honeypots/:id（bot:write）。
func (s *Server) HoneypotUpdate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的蜜罐 ID")
		return
	}
	var row model.BotHoneypot
	if err := s.Store.Gorm().First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "蜜罐不存在")
			return
		}
		s.failDB(c, err)
		return
	}
	var in honeypotInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	next, msg := buildHoneypot(&row, &in)
	if msg != "" {
		response.BadRequest(c, msg)
		return
	}
	if err := s.Store.Gorm().Save(next).Error; err != nil {
		s.failDB(c, err)
		return
	}
	c.Set(middleware.CtxAuditObj, "bot-honeypot")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(id, 10))
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "蜜罐已保存但热加载失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"id": next.ID})
}

// HoneypotToggle 启用/停用蜜罐路径。
// 路由：PATCH /api/bot-honeypots/:id/toggle（bot:write）。
func (s *Server) HoneypotToggle(c *gin.Context) {
	s.toggleRow(c, &model.BotHoneypot{}, "蜜罐")
}

// HoneypotDelete 删除蜜罐路径。
// 路由：DELETE /api/bot-honeypots/:id（bot:write）。
func (s *Server) HoneypotDelete(c *gin.Context) {
	s.deleteRow(c, &model.BotHoneypot{}, "蜜罐", "bot-honeypot")
}

// buildHoneypot 合并并校验蜜罐路径。
func buildHoneypot(old *model.BotHoneypot, in *honeypotInput) (*model.BotHoneypot, string) {
	p := strings.TrimSpace(in.Path)
	if p == "" {
		return nil, "蜜罐路径不能为空"
	}
	if !strings.HasPrefix(p, "/") {
		return nil, "蜜罐路径必须以 / 开头"
	}
	row := &model.BotHoneypot{}
	if old != nil {
		*row = *old
	}
	row.Site = strings.TrimSpace(in.Site)
	row.Path = p
	row.Note = strings.TrimSpace(in.Note)
	if in.Enabled != nil {
		row.Enabled = *in.Enabled
	} else if old == nil {
		row.Enabled = 1
	}
	if old == nil {
		row.CreatedAt = store.NowMS()
	}
	return row, ""
}

/* ====================================================== 页面缓存 */

// cacheRuleInput 是缓存规则新增/修改请求体。
type cacheRuleInput struct {
	Name       string `json:"name"`
	Site       string `json:"site"`
	Kind       string `json:"kind"`
	Pattern    string `json:"pattern"`
	TTLSeconds *int   `json:"ttlSeconds"`
	Priority   *int   `json:"priority"`
	Enabled    *int   `json:"enabled"`
	Note       string `json:"note"`
}

// CacheRuleList 查询缓存规则。
// 路由：GET /api/cache/rules（仅需登录）。
func (s *Server) CacheRuleList(c *gin.Context) {
	q := s.Store.Gorm().Model(&model.CacheRule{})
	if site := strings.TrimSpace(c.Query("site")); site != "" {
		q = q.Where("site = ? OR site = ''", site)
	}
	var rows []model.CacheRule
	if err := q.Order("priority ASC, id ASC").Limit(500).Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	response.OK(c, gin.H{"list": rows, "total": len(rows)})
}

// CacheRuleCreate 新增缓存规则。
// 路由：POST /api/cache/rules（cache:write）。
func (s *Server) CacheRuleCreate(c *gin.Context) {
	var in cacheRuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	row, msg := buildCacheRule(nil, &in)
	if msg != "" {
		response.BadRequest(c, msg)
		return
	}
	if err := s.createRow(row, row.Enabled); err != nil {
		s.failDB(c, err)
		return
	}
	c.Set(middleware.CtxAuditObj, "cache-rule")
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "缓存规则已保存但热加载失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"id": row.ID})
}

// CacheRuleUpdate 修改缓存规则。
// 路由：PUT /api/cache/rules/:id（cache:write）。
func (s *Server) CacheRuleUpdate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的规则 ID")
		return
	}
	var row model.CacheRule
	if err := s.Store.Gorm().First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "缓存规则不存在")
			return
		}
		s.failDB(c, err)
		return
	}
	var in cacheRuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	next, msg := buildCacheRule(&row, &in)
	if msg != "" {
		response.BadRequest(c, msg)
		return
	}
	if err := s.Store.Gorm().Save(next).Error; err != nil {
		s.failDB(c, err)
		return
	}
	c.Set(middleware.CtxAuditObj, "cache-rule")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(id, 10))
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "缓存规则已保存但热加载失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"id": next.ID})
}

// CacheRuleToggle 启用/停用缓存规则。
// 路由：PATCH /api/cache/rules/:id/toggle（cache:write）。
func (s *Server) CacheRuleToggle(c *gin.Context) {
	s.toggleRow(c, &model.CacheRule{}, "缓存规则")
}

// CacheRuleDelete 删除缓存规则。
// 路由：DELETE /api/cache/rules/:id（cache:write）。
func (s *Server) CacheRuleDelete(c *gin.Context) {
	s.deleteRow(c, &model.CacheRule{}, "缓存规则", "cache-rule")
}

// buildCacheRule 合并并校验缓存规则。
func buildCacheRule(old *model.CacheRule, in *cacheRuleInput) (*model.CacheRule, string) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, "规则名称不能为空"
	}
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	switch kind {
	case "":
		kind = cache.KindSuffix
	case cache.KindSuffix, cache.KindExact, cache.KindPrefix, cache.KindDirectory, cache.KindGuest:
	case cache.KindRegex:
	default:
		return nil, "匹配类型只能是 suffix/exact/prefix/regex/directory/guest"
	}
	pattern := strings.TrimSpace(in.Pattern)
	if pattern == "" {
		return nil, "匹配内容不能为空"
	}
	if kind == cache.KindRegex {
		if _, err := regexp.Compile(pattern); err != nil {
			return nil, "正则非法: " + err.Error()
		}
	}
	ttl := 300
	if in.TTLSeconds != nil && *in.TTLSeconds > 0 {
		ttl = *in.TTLSeconds
	}
	row := &model.CacheRule{}
	if old != nil {
		*row = *old
	}
	row.Name = name
	row.Site = strings.TrimSpace(in.Site)
	row.Kind = kind
	row.Pattern = pattern
	row.TTLSeconds = ttl
	row.Note = strings.TrimSpace(in.Note)
	if in.Priority != nil {
		row.Priority = *in.Priority
	} else if old == nil {
		row.Priority = 100
	}
	if in.Enabled != nil {
		row.Enabled = *in.Enabled
	} else if old == nil {
		row.Enabled = 1
	}
	if old == nil {
		row.CreatedAt = store.NowMS()
	}
	row.UpdatedAt = store.NowMS()
	return row, ""
}

// cacheSettingsDTO 是页面加速设置的返回结构（时间为秒，方便前端编辑）。
type cacheSettingsDTO struct {
	Enabled          bool  `json:"enabled"`
	MaxBytes         int64 `json:"maxBytes"`
	MaxObjectBytes   int64 `json:"maxObjectBytes"`
	DefaultTTL       int   `json:"defaultTtl"`
	Compress         bool  `json:"compress"`
	Brotli           bool  `json:"brotli"`
	WebP             bool  `json:"webp"`
	CompressMinBytes int   `json:"compressMinBytes"`
	Entries          int   `json:"entries"`
	UsedBytes        int64 `json:"usedBytes"`
	RuleCount        int   `json:"ruleCount"`
}

// CacheSettingsGet 返回页面加速设置与当前占用。
// 路由：GET /api/cache/settings（仅需登录）。
func (s *Server) CacheSettingsGet(c *gin.Context) {
	st := s.RT.Cache.Settings()
	stats := s.RT.Cache.Stats()
	response.OK(c, cacheSettingsDTO{
		Enabled: st.Enabled, MaxBytes: st.MaxBytes, MaxObjectBytes: st.MaxObjectBytes,
		DefaultTTL: int(st.DefaultTTL.Seconds()), Compress: st.Compress, Brotli: st.Brotli,
		WebP: st.WebP, CompressMinBytes: st.CompressMinBytes,
		Entries: stats.Entries, UsedBytes: stats.Bytes, RuleCount: s.RT.Cache.RuleCount(),
	})
}

// cacheSettingsInput 是设置保存请求体；指针字段允许部分更新。
type cacheSettingsInput struct {
	Enabled          *bool  `json:"enabled"`
	MaxBytes         *int64 `json:"maxBytes"`
	MaxObjectBytes   *int64 `json:"maxObjectBytes"`
	DefaultTTL       *int   `json:"defaultTtl"`
	Compress         *bool  `json:"compress"`
	Brotli           *bool  `json:"brotli"`
	WebP             *bool  `json:"webp"`
	CompressMinBytes *int   `json:"compressMinBytes"`
}

// CacheSettingsUpdate 保存页面加速设置并热加载。
// 路由：PUT /api/cache/settings（cache:write）。
func (s *Server) CacheSettingsUpdate(c *gin.Context) {
	var in cacheSettingsInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	if in.MaxBytes != nil && (*in.MaxBytes < 1<<20 || *in.MaxBytes > 1<<34) {
		response.BadRequest(c, "缓存总容量需在 1MiB ~ 16GiB 之间")
		return
	}
	if in.MaxObjectBytes != nil && (*in.MaxObjectBytes < 1024 || *in.MaxObjectBytes > 1<<30) {
		response.BadRequest(c, "单对象上限需在 1KiB ~ 1GiB 之间")
		return
	}
	if in.DefaultTTL != nil && (*in.DefaultTTL < 1 || *in.DefaultTTL > 30*24*3600) {
		response.BadRequest(c, "默认缓存时长需在 1 秒 ~ 30 天之间")
		return
	}
	if in.CompressMinBytes != nil && (*in.CompressMinBytes < 0 || *in.CompressMinBytes > 10<<20) {
		response.BadRequest(c, "压缩阈值需在 0 ~ 10MiB 之间")
		return
	}
	set := func(key, val string) bool {
		if err := s.touchSettings(key, val, c.GetString(middleware.CtxUserName)); err != nil {
			s.failDB(c, err)
			return false
		}
		return true
	}
	if in.Enabled != nil && !set(runtime.KeyCacheEnabled, strconv.FormatBool(*in.Enabled)) {
		return
	}
	if in.MaxBytes != nil && !set(runtime.KeyCacheMaxBytes, strconv.FormatInt(*in.MaxBytes, 10)) {
		return
	}
	if in.MaxObjectBytes != nil && !set(runtime.KeyCacheMaxObject, strconv.FormatInt(*in.MaxObjectBytes, 10)) {
		return
	}
	if in.DefaultTTL != nil && !set(runtime.KeyCacheDefaultTTL, strconv.Itoa(*in.DefaultTTL)) {
		return
	}
	if in.Compress != nil && !set(runtime.KeyCacheCompress, strconv.FormatBool(*in.Compress)) {
		return
	}
	if in.Brotli != nil && !set(runtime.KeyCacheBrotli, strconv.FormatBool(*in.Brotli)) {
		return
	}
	if in.WebP != nil && !set(runtime.KeyCacheWebP, strconv.FormatBool(*in.WebP)) {
		return
	}
	if in.CompressMinBytes != nil && !set(runtime.KeyCacheCompressMin, strconv.Itoa(*in.CompressMinBytes)) {
		return
	}
	c.Set(middleware.CtxAuditObj, "cache-settings")
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "设置已保存但热加载失败: "+err.Error())
		return
	}
	s.CacheSettingsGet(c)
}

// CacheStats 返回缓存命中统计。
// 路由：GET /api/cache/stats（仅需登录）。
func (s *Server) CacheStats(c *gin.Context) {
	response.OK(c, s.RT.Cache.Stats())
}

// CacheEntryList 列出当前内存中的缓存对象。
// 路由：GET /api/cache/entries（仅需登录）。
//
// 参数：site 站点过滤、keyword 路径/文件名/域名模糊匹配、sort 排序字段、
// order 升/降序、page/size 分页。返回 list 只包含当前页，另附过滤后的总数。
func (s *Server) CacheEntryList(c *gin.Context) {
	all := s.RT.Cache.Entries(time.Now())

	site := strings.TrimSpace(c.Query("site"))
	kw := strings.ToLower(strings.TrimSpace(c.Query("keyword")))
	matched := make([]cache.EntryInfo, 0, len(all))
	var matchedBytes int64
	for _, e := range all {
		if site != "" && !strings.EqualFold(e.Site, site) {
			continue
		}
		if kw != "" &&
			!strings.Contains(strings.ToLower(e.Path), kw) &&
			!strings.Contains(strings.ToLower(e.Filename), kw) &&
			!strings.Contains(strings.ToLower(e.Host), kw) {
			continue
		}
		matched = append(matched, e)
		matchedBytes += e.Size
	}
	sortCacheEntries(matched, c.DefaultQuery("sort", "storedAt"), c.DefaultQuery("order", "desc"))

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	start := (page - 1) * size
	if start > len(matched) {
		start = len(matched)
	}
	end := start + size
	if end > len(matched) {
		end = len(matched)
	}

	response.OK(c, gin.H{
		"list": matched[start:end], "total": len(matched),
		"entries": len(all), "bytes": matchedBytes,
		"page": page, "size": size, "now": time.Now().UnixMilli(),
	})
}

// sortCacheEntries 按指定字段排序；order=asc 时反转比较结果。
// 未知字段回落到写入时间倒序，避免前端传入脏值导致顺序不可预期。
func sortCacheEntries(list []cache.EntryInfo, field, order string) {
	var less func(i, j int) bool
	switch field {
	case "size":
		less = func(i, j int) bool { return list[i].Size > list[j].Size }
	case "hits":
		less = func(i, j int) bool { return list[i].Hits > list[j].Hits }
	case "expiresAt":
		less = func(i, j int) bool { return list[i].ExpiresAt < list[j].ExpiresAt }
	case "remain":
		less = func(i, j int) bool { return list[i].RemainSeconds < list[j].RemainSeconds }
	default:
		less = func(i, j int) bool { return list[i].StoredAt > list[j].StoredAt }
	}
	if strings.EqualFold(order, "asc") {
		base := less
		less = func(i, j int) bool { return base(j, i) }
	}
	sort.SliceStable(list, less)
}

// CachePurge 清空缓存对象。
// 路由：POST /api/cache/purge（cache:write）。
func (s *Server) CachePurge(c *gin.Context) {
	var in struct {
		Site   string `json:"site"`
		Prefix string `json:"prefix"`
	}
	if err := c.ShouldBindJSON(&in); err != nil && err.Error() != "EOF" {
		response.BadRequest(c, "参数格式错误")
		return
	}
	n := s.RT.Cache.Purge(strings.TrimSpace(in.Site), strings.TrimSpace(in.Prefix))
	c.Set(middleware.CtxAuditObj, "cache")
	response.OK(c, gin.H{"purged": n})
}

/* ====================================================== 站点多上游 */

// upstreamInput 是站点回源目标请求体。
type upstreamInput struct {
	Addr     string `json:"addr"`
	Line     string `json:"line"`
	Weight   *int   `json:"weight"`
	Priority *int   `json:"priority"`
	Enabled  *int   `json:"enabled"`
}

// SiteUpstreamList 查询站点的额外回源目标。
// 路由：GET /api/sites/:id/upstreams（仅需登录）。
func (s *Server) SiteUpstreamList(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的站点 ID")
		return
	}
	var rows []model.SiteUpstream
	if err := s.Store.Gorm().Where("site_id = ?", id).
		Order("priority ASC, weight DESC, id ASC").Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	response.OK(c, gin.H{"list": rows, "total": len(rows)})
}

// SiteUpstreamSave 覆盖保存站点的额外回源目标。
// 路由：PUT /api/sites/:id/upstreams（site:write）。
func (s *Server) SiteUpstreamSave(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的站点 ID")
		return
	}
	var site model.Site
	if err := s.Store.Gorm().First(&site, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "站点不存在")
			return
		}
		s.failDB(c, err)
		return
	}
	var in []upstreamInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	if len(in) > 16 {
		response.BadRequest(c, "单站点最多配置 16 条上游")
		return
	}
	now := store.NowMS()
	rows := make([]model.SiteUpstream, 0, len(in))
	for _, u := range in {
		addr := strings.TrimSpace(u.Addr)
		if addr == "" {
			response.BadRequest(c, "上游地址不能为空")
			return
		}
		parsed, err := url.Parse(addr)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			response.BadRequest(c, "上游地址非法: "+addr)
			return
		}
		line := strings.ToLower(strings.TrimSpace(u.Line))
		switch line {
		case "", "bgp":
			line = "bgp"
		case "telecom", "unicom", "mobile":
		default:
			response.BadRequest(c, "线路只能是 telecom/unicom/mobile/bgp")
			return
		}
		weight, priority, enabled := 1, 100, 1
		if u.Weight != nil && *u.Weight > 0 {
			weight = *u.Weight
		}
		if u.Priority != nil && *u.Priority > 0 {
			priority = *u.Priority
		}
		if u.Enabled != nil {
			enabled = *u.Enabled
		}
		rows = append(rows, model.SiteUpstream{
			SiteID: uint(id), Addr: addr, Line: line, Weight: weight,
			Priority: priority, Enabled: enabled, CreatedAt: now, UpdatedAt: now,
		})
	}
	err = s.Store.Gorm().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("site_id = ?", id).Delete(&model.SiteUpstream{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		s.failDB(c, err)
		return
	}
	c.Set(middleware.CtxAuditObj, "site-upstream")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(id, 10))
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "上游已保存但热加载失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"total": len(rows), "list": rows})
}

// UpstreamStatus 返回全部回源目标的健康状态。
// 路由：GET /api/upstreams/status（仅需登录）。
func (s *Server) UpstreamStatus(c *gin.Context) {
	response.OK(c, gin.H{"list": s.RT.Upstreams.TargetStatuses()})
}

/* ====================================================== 节点管理 */

// nodeInput 是节点新增/修改请求体。
type nodeInput struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Line    string `json:"line"`
	Region  string `json:"region"`
	Weight  *int   `json:"weight"`
	Status  string `json:"status"`
	Note    string `json:"note"`
}

// NodeList 查询接入节点。
// 路由：GET /api/nodes（仅需登录）。
func (s *Server) NodeList(c *gin.Context) {
	var rows []model.Node
	if err := s.Store.Gorm().Order("id ASC").Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	response.OK(c, gin.H{"list": rows, "total": len(rows)})
}

// NodeCreate 新增接入节点。
// 路由：POST /api/nodes（node:write）。
func (s *Server) NodeCreate(c *gin.Context) {
	var in nodeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	row, msg := buildNode(nil, &in)
	if msg != "" {
		response.BadRequest(c, msg)
		return
	}
	// 节点没有 enabled 字段，走统一的创建逻辑但不需要纠正开关
	if err := s.createRow(row, 1); err != nil {
		s.failDB(c, err)
		return
	}
	c.Set(middleware.CtxAuditObj, "node")
	response.OK(c, gin.H{"id": row.ID})
}

// NodeUpdate 修改接入节点。
// 路由：PUT /api/nodes/:id（node:write）。
func (s *Server) NodeUpdate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的节点 ID")
		return
	}
	var row model.Node
	if err := s.Store.Gorm().First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "节点不存在")
			return
		}
		s.failDB(c, err)
		return
	}
	var in nodeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	next, msg := buildNode(&row, &in)
	if msg != "" {
		response.BadRequest(c, msg)
		return
	}
	if err := s.Store.Gorm().Save(next).Error; err != nil {
		s.failDB(c, err)
		return
	}
	c.Set(middleware.CtxAuditObj, "node")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(id, 10))
	response.OK(c, gin.H{"id": next.ID})
}

// NodeDelete 删除接入节点。
// 路由：DELETE /api/nodes/:id（node:write）。
func (s *Server) NodeDelete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的节点 ID")
		return
	}
	if err := s.Store.Gorm().Delete(&model.Node{}, id).Error; err != nil {
		s.failDB(c, err)
		return
	}
	c.Set(middleware.CtxAuditObj, "node")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(id, 10))
	response.OK(c, gin.H{"id": id})
}

// NodeToken 生成/轮换节点 Agent 上报令牌。
// 路由：POST /api/nodes/token（node:write）。
func (s *Server) NodeToken(c *gin.Context) {
	token := store.RandomPassword()
	if err := s.touchSettings(runtime.KeyNodeAgentToken, token, c.GetString(middleware.CtxUserName)); err != nil {
		s.failDB(c, err)
		return
	}
	// 令牌保存后必须立即热加载，否则 Agent 拿新令牌请求 /api/agent/config
	// 时仍会撞上旧的运行时快照（首次生成时甚至还是空串）。
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, "令牌已生成但热加载失败: "+err.Error())
		return
	}
	c.Set(middleware.CtxAuditObj, "node")
	response.OK(c, gin.H{"token": token})
}

// buildNode 合并并校验节点。
func buildNode(old *model.Node, in *nodeInput) (*model.Node, string) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, "节点名称不能为空"
	}
	line := strings.ToLower(strings.TrimSpace(in.Line))
	switch line {
	case "", "bgp":
		line = "bgp"
	case "telecom", "unicom", "mobile":
	default:
		return nil, "线路只能是 telecom/unicom/mobile/bgp"
	}
	status := strings.ToLower(strings.TrimSpace(in.Status))
	switch status {
	case "", "online", "offline", "disabled":
	default:
		return nil, "状态只能是 online/offline/disabled"
	}
	row := &model.Node{}
	if old != nil {
		*row = *old
	} else {
		row.Status = "offline"
		row.CreatedAt = store.NowMS()
	}
	row.Name = name
	row.Address = strings.TrimSpace(in.Address)
	row.Line = line
	row.Region = strings.TrimSpace(in.Region)
	row.Note = strings.TrimSpace(in.Note)
	if status != "" {
		row.Status = status
	}
	if in.Weight != nil {
		row.Weight = *in.Weight
	} else if old == nil {
		row.Weight = 1
	}
	row.UpdatedAt = store.NowMS()
	return row, ""
}

/* ====================================================== Agent 上报接口 */

// checkAgentToken 校验节点 Agent 请求头里的令牌。
// 参数 c：Gin 上下文。返回 bool：令牌有效返回 true；无效时已写出 401/403。
func (s *Server) checkAgentToken(c *gin.Context) bool {
	want := s.RT.NodeAgentToken()
	if want == "" {
		response.Forbidden(c, "节点接入未启用：请先在控制面生成 Agent 令牌")
		return false
	}
	got := strings.TrimSpace(c.GetHeader("X-Agent-Token"))
	if got == "" {
		got = strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		response.Unauthorized(c, "Agent 令牌无效")
		return false
	}
	return true
}

// AgentConfig 返回节点启动所需的配置快照。
// 路由：GET /api/agent/config（Agent 令牌）。
func (s *Server) AgentConfig(c *gin.Context) {
	if !s.checkAgentToken(c) {
		return
	}
	var sites []model.Site
	if err := s.Store.Gorm().Where("enabled = 1").Order("id ASC").Find(&sites).Error; err != nil {
		s.failDB(c, err)
		return
	}
	type siteView struct {
		ID       uint     `json:"id"`
		Name     string   `json:"name"`
		Hosts    []string `json:"hosts"`
		Upstream string   `json:"upstream"`
	}
	list := make([]siteView, 0, len(sites))
	for _, site := range sites {
		var hosts []string
		if site.Hosts != "" {
			_ = json.Unmarshal([]byte(site.Hosts), &hosts)
		}
		list = append(list, siteView{ID: site.ID, Name: site.Name, Hosts: hosts, Upstream: site.Upstream})
	}
	keys := []string{
		runtime.KeyMode, runtime.KeyBlockScore, runtime.KeyChallengeScore,
		runtime.KeyBotEnabled, runtime.KeyBotVerifyRDNS, runtime.KeyBotScoreThreshold, runtime.KeyBotAction,
		runtime.KeyCacheEnabled, runtime.KeyCacheMaxBytes, runtime.KeyCacheDefaultTTL,
		runtime.KeyUpstreamHealth, runtime.KeyNodeHeartbeatTTL,
	}
	settings := make(map[string]string, len(keys))
	for _, k := range keys {
		settings[k] = s.RT.Setting(k, "")
	}
	response.OK(c, gin.H{
		"version":  s.Version,
		"sites":    list,
		"settings": settings,
		"policies": s.RT.PolicySet().Count(),
	})
}

// AgentHeartbeat 接收节点心跳并更新在线状态。
// 路由：POST /api/agent/heartbeat（Agent 令牌）。
func (s *Server) AgentHeartbeat(c *gin.Context) {
	if !s.checkAgentToken(c) {
		return
	}
	var in struct {
		Name    string `json:"name"`
		Address string `json:"address"`
		Line    string `json:"line"`
		Region  string `json:"region"`
		Version string `json:"version"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		response.BadRequest(c, "节点名称不能为空")
		return
	}
	now := store.NowMS()
	updates := map[string]any{
		"address": strings.TrimSpace(in.Address), "line": strings.TrimSpace(in.Line),
		"region": strings.TrimSpace(in.Region), "version": strings.TrimSpace(in.Version),
		"status": "online", "last_heartbeat": now, "updated_at": now,
	}
	var node model.Node
	err := s.Store.Gorm().Where("name = ?", name).First(&node).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		node = model.Node{
			Name: name, Address: strings.TrimSpace(in.Address), Line: strings.TrimSpace(in.Line),
			Region: strings.TrimSpace(in.Region), Weight: 1, Status: "online",
			LastHeartbeat: now, Version: strings.TrimSpace(in.Version), CreatedAt: now, UpdatedAt: now,
		}
		if node.Line == "" {
			node.Line = "bgp"
		}
		if err := s.Store.Gorm().Create(&node).Error; err != nil {
			s.failDB(c, err)
			return
		}
	} else if err != nil {
		s.failDB(c, err)
		return
	} else {
		if err := s.Store.Gorm().Model(&model.Node{}).Where("id = ?", node.ID).Updates(updates).Error; err != nil {
			s.failDB(c, err)
			return
		}
	}
	response.OK(c, gin.H{"status": "online", "serverTime": now})
}

/* ====================================================== 通用操作 */

// toggleRow 通用启用/停用：按表主键切换 enabled。
// 参数 c：上下文；row：目标模型；label：错误文案里的对象名。
func (s *Server) toggleRow(c *gin.Context, row any, label string) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 ID")
		return
	}
	var enabled int
	if err := s.Store.Gorm().Model(row).Select("enabled").Where("id = ?", id).Scan(&enabled).Error; err != nil {
		s.failDB(c, err)
		return
	}
	next := 0
	if enabled == 0 {
		next = 1
	}
	if err := s.Store.Gorm().Model(row).Where("id = ?", id).Update("enabled", next).Error; err != nil {
		s.failDB(c, err)
		return
	}
	c.Set(middleware.CtxAuditObj, "toggle")
	c.Set(middleware.CtxAuditID, strconv.FormatUint(id, 10))
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, label+"状态已切换但热加载失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"id": id, "enabled": next})
}

// deleteRow 通用删除：按主键删除并热加载。
// 参数 c：上下文；row：目标模型；label：错误文案里的对象名；auditObj：审计对象类型。
func (s *Server) deleteRow(c *gin.Context, row any, label, auditObj string) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 ID")
		return
	}
	if err := s.Store.Gorm().Delete(row, id).Error; err != nil {
		s.failDB(c, err)
		return
	}
	c.Set(middleware.CtxAuditObj, auditObj)
	c.Set(middleware.CtxAuditID, strconv.FormatUint(id, 10))
	if err := s.reloadRuntime(c); err != nil {
		response.Internal(c, label+"已删除但热加载失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"id": id})
}

// 保留 http 包引用：Agent 接口的错误码全部经 response 包写出，
// 此处显式声明便于后续扩展直接使用状态码常量。
var _ = http.StatusOK
