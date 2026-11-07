package handler

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"

	"gowaf/internal/admin/middleware"
	"gowaf/internal/admin/response"
	"gowaf/internal/store"
	"gowaf/internal/store/model"
)

/* ============================================================ P0 规则包 */

// rulePackItem 是导入/导出 YAML 的元素，字段与 rules/builtin.yaml 一致，
// 额外带上规则包元信息，方便第三方规则包直接投喂。
type rulePackItem struct {
	ID       string   `yaml:"id" json:"id"`             // 规则唯一标识（导入必填）
	Name     string   `yaml:"name" json:"name"`         // 规则名称（导入必填）
	Category string   `yaml:"category" json:"category"` // 分类（导入必填）
	Score    int      `yaml:"score" json:"score"`       // 命中分值；导入时 ≤0 的新建规则回落为 40
	Targets  []string `yaml:"targets" json:"targets"`   // 检测目标列表；导入时为空默认 path,query,body
	Keywords []string `yaml:"keywords" json:"keywords"` // 关键词列表
	Pattern  string   `yaml:"pattern" json:"pattern"`   // 正则表达式
	Action   string   `yaml:"action" json:"action"`     // 处置动作；导入时为空默认 block
	Priority int      `yaml:"priority" json:"priority"` // 优先级；导入时为 0 默认 100
	Remark   string   `yaml:"remark" json:"remark"`     // 备注说明
	Group    string   `yaml:"group" json:"group"`       // 分组；导入时为空归入 imported
	Mode     string   `yaml:"mode" json:"mode"`         // 检测模式；导入时为空默认 inherit
	Source   string   `yaml:"source" json:"source"`     // 来源；导入时为空标记为 imported
	Vendor   string   `yaml:"vendor" json:"vendor"`     // 维护方；条目缺省时取请求参数 vendor
	Version  string   `yaml:"version" json:"version"`   // 规则包版本号
	Enabled  *int     `yaml:"enabled" json:"enabled"`   // 启用状态指针：nil 视为启用；仅新建规则时生效，更新不改动
}

// RuleGroups 按规则组聚合并返回统计，供面板做整组启停。
// 路由：GET /api/rules/groups（需登录）。
// 参数 c：Gin 上下文。
// 返回：无（200 返回 {"groups":分组统计列表,"total":规则总数}；数据库错误返回 500）。
func (s *Server) RuleGroups(c *gin.Context) {
	// bucket 单个规则组的聚合结果。
	type bucket struct {
		Group   string `json:"group"`   // 分组名
		Source  string `json:"source"`  // 来源，与分组名共同构成唯一键
		Vendor  string `json:"vendor"`  // 维护方
		Total   int    `json:"total"`   // 组内规则总数
		Enabled int    `json:"enabled"` // 组内处于启用状态的条数
		Mode    string `json:"mode"`    // 组模式：任一成员为 log 则整组标记 log，否则 inherit
	}
	var rows []model.Rule
	if err := s.Store.Gorm().Select("rule_group, source, vendor, enabled, mode").Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	// 以 source/group 为键聚合：同分组不同来源视为两个组，避免内置包与导入包互相干扰
	idx := map[string]*bucket{}
	for _, r := range rows {
		key := r.Source + "/" + r.Group
		b, ok := idx[key]
		if !ok {
			b = &bucket{Group: r.Group, Source: r.Source, Vendor: r.Vendor, Mode: "inherit"}
			idx[key] = b
		}
		b.Total++
		if r.Enabled == 1 {
			b.Enabled++
		}
		if r.Mode == "log" {
			b.Mode = "log"
		}
	}
	out := make([]bucket, 0, len(idx))
	for _, b := range idx {
		out = append(out, *b)
	}
	// 先按来源、再按分组名排序，保证前端面板展示顺序稳定
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source == out[j].Source {
			return out[i].Group < out[j].Group
		}
		return out[i].Source < out[j].Source
	})
	response.OK(c, gin.H{"groups": out, "total": len(rows)})
}

// RuleBatch 批量操作：整组/勾选启停、改分、改动作、切观察模式、删除。
// 路由：POST /api/rules/batch（写操作组，需 rule:write 权限）。
// 参数 c：Gin 上下文，JSON 正文携带操作范围（ids/group/source 三选一）与 op 操作类型。
// 返回：无（200 返回 {"affected":受影响条数}；参数非法 400，数据库错误 500）。
func (s *Server) RuleBatch(c *gin.Context) {
	var in struct {
		IDs    []string `json:"ids"`    // 勾选的规则 ID 列表，范围匹配优先级最高
		Group  string   `json:"group"`  // 按分组匹配，可与 source 叠加收窄范围
		Source string   `json:"source"` // 按来源匹配
		Op     string   `json:"op"`     // 操作类型：enable/disable/observe/protect/score/priority/action/group/delete
		Value  string   `json:"value"`  // op=action 时的目标动作 / op=group 时的目标分组名
		Number int      `json:"number"` // op=score/priority 时的目标数值
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	if in.Op == "" {
		response.BadRequest(c, "op 不能为空")
		return
	}
	// 先圈定操作范围：ids 优先，其次 group（可叠加 source），再次 source；
	// 三者都不传直接拒绝，防止误伤全表。
	q := s.Store.Gorm().Model(&model.Rule{})
	switch {
	case len(in.IDs) > 0:
		q = q.Where("id IN ?", in.IDs)
	case in.Group != "":
		q = q.Where("rule_group = ?", in.Group)
		if in.Source != "" {
			q = q.Where("source = ?", in.Source)
		}
	case in.Source != "":
		q = q.Where("source = ?", in.Source)
	default:
		response.BadRequest(c, "必须指定 ids / group / source 之一，避免误伤全表")
		return
	}

	now := store.NowMS()
	// 除 delete 外各操作都走同一条 UPDATE 语句，作用域即上面圈定的 q
	switch in.Op {
	case "enable": // 批量启用
		q = q.Update("enabled", 1)
	case "disable": // 批量停用
		q = q.Update("enabled", 0)
	case "observe": // 切观察模式：只记录不计分
		q = q.Update("mode", "log")
	case "protect": // 切回防护模式：跟随引擎全局模式
		q = q.Update("mode", "inherit")
	case "score":
		if in.Number <= 0 || in.Number > 1000 {
			response.BadRequest(c, "分值必须在 1~1000 之间")
			return
		}
		q = q.Update("score", in.Number)
	case "priority":
		if in.Number < 0 || in.Number > 10000 {
			response.BadRequest(c, "优先级必须在 0~10000 之间")
			return
		}
		q = q.Update("priority", in.Number)
	case "action":
		if in.Value != "block" && in.Value != "challenge" && in.Value != "log" && in.Value != "ban" {
			response.BadRequest(c, "动作只能是 block/challenge/log/ban")
			return
		}
		q = q.Update("action", in.Value)
	case "group":
		if strings.TrimSpace(in.Value) == "" {
			response.BadRequest(c, "分组名不能为空")
			return
		}
		q = q.Update("rule_group", strings.TrimSpace(in.Value))
	case "delete":
		// 删除是特例：内置规则不可删，先查出范围内全部规则再过滤，
		// 因此 affected/skipped 需要单独统计，不能复用下面的 q.RowsAffected。
		var builtin int64
		s.Store.Gorm().Model(&model.Rule{}).Where("builtin = 1").Count(&builtin)
		var rows []model.Rule
		if err := q.Select("id, builtin").Find(&rows).Error; err != nil {
			s.failDB(c, err)
			return
		}
		ids := make([]string, 0, len(rows))
		for _, r := range rows {
			if r.Builtin == 0 {
				ids = append(ids, r.ID) // 只删除自定义规则
			}
		}
		if len(ids) == 0 {
			response.BadRequest(c, "选中项都是内置规则，只能停用不能删除")
			return
		}
		if err := s.Store.Gorm().Delete(&model.Rule{}, "id IN ?", ids).Error; err != nil {
			s.failDB(c, err)
			return
		}
		s.reloadAfterWrite(c)
		response.OK(c, gin.H{"affected": len(ids), "skipped": len(rows) - len(ids)})
		return
	default:
		response.BadRequest(c, "不支持的 op: "+in.Op)
		return
	}
	if q.Error != nil {
		s.failDB(c, q.Error)
		return
	}
	_ = now
	s.reloadAfterWrite(c) // 热加载规则使变更立即生效
	response.OK(c, gin.H{"affected": q.RowsAffected})
}

// reloadAfterWrite 写后热加载规则，失败只提示不影响已落库结果。
// 参数 c：Gin 上下文，重载错误写入其 reloadError 键，由响应包装层附带返回。
// 返回：无。
func (s *Server) reloadAfterWrite(c *gin.Context) {
	if err := s.reloadRuntime(c); err != nil {
		c.Set("reloadError", err.Error())
	}
}

// RuleExport 导出 YAML 规则包，可按分组/来源过滤后下载。
// 路由：GET /api/rules/export（需登录）。
// 参数 c：Gin 上下文；查询参数 group（分组）、source（来源）可选。
// 返回：无（200 以附件返回 YAML 文本，文件名 gowaf-rules-<毫秒时间戳>.yaml；
// 数据库或序列化错误返回 500）。
func (s *Server) RuleExport(c *gin.Context) {
	q := s.Store.Gorm().Model(&model.Rule{})
	if g := c.Query("group"); g != "" {
		q = q.Where("rule_group = ?", g)
	}
	if src := c.Query("source"); src != "" {
		q = q.Where("source = ?", src)
	}
	var rows []model.Rule
	if err := q.Order("priority ASC, id ASC").Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	items := make([]rulePackItem, 0, len(rows))
	for _, r := range rows {
		var kws []string
		_ = json.Unmarshal([]byte(r.Keywords), &kws)
		enabled := r.Enabled // Enabled 字段是指针，先拷贝再取地址，避免悬引用
		items = append(items, rulePackItem{
			ID: r.ID, Name: r.Name, Category: r.Category, Score: r.Score,
			Targets: strings.Split(r.Targets, ","), Keywords: kws, Pattern: r.Pattern,
			Action: r.Action, Priority: r.Priority, Remark: r.Remark,
			Group: r.Group, Mode: r.Mode, Source: r.Source, Vendor: r.Vendor,
			Version: r.Version, Enabled: &enabled,
		})
	}
	body, err := yaml.Marshal(items)
	if err != nil {
		response.Internal(c, "导出失败: "+err.Error())
		return
	}
	name := fmt.Sprintf("gowaf-rules-%d.yaml", store.NowMS())
	c.Header("Content-Disposition", "attachment; filename="+name)
	c.Data(200, "application/x-yaml; charset=utf-8", body)
}

// RuleImport 导入 YAML 规则包。
//
// mode=merge（默认）：按 id upsert，保留已有规则的启用状态；
// mode=replace：先删掉此前由导入产生、且本次文件中不存在的规则。
// dryRun=true 只做校验和差异统计，不落库。
// 路由：POST /api/rules/import（写操作组，需 rule:write 权限）。
// 参数 c：Gin 上下文，JSON 正文携带 text（YAML 文本）、mode、dryRun、vendor。
// 返回：无（200 返回 {"created","updated","skipped","pruned","errors","dryRun"} 统计；
// 文本为空或 YAML 解析失败 400）。
func (s *Server) RuleImport(c *gin.Context) {
	var in struct {
		Text   string `json:"text"`   // YAML 规则包全文
		Mode   string `json:"mode"`   // 导入模式：merge（默认）/ replace
		DryRun bool   `json:"dryRun"` // true 只校验并统计差异，不写库
		Vendor string `json:"vendor"` // 条目未声明 vendor 时使用的默认维护方
	}
	if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Text) == "" {
		response.BadRequest(c, "text 不能为空")
		return
	}
	var items []rulePackItem
	if err := yaml.Unmarshal([]byte(in.Text), &items); err != nil {
		response.BadRequest(c, "YAML 解析失败: "+err.Error())
		return
	}
	if len(items) == 0 {
		response.BadRequest(c, "文件里没有规则")
		return
	}

	now := store.NowMS()
	operator := c.GetString(middleware.CtxUserName)
	created, updated, skipped := 0, 0, 0
	var errs []string

	// 逐条处理：单条校验失败或写库失败只记入 skipped/errors，不中断整包导入
	for _, it := range items {
		it.ID = strings.TrimSpace(it.ID)
		if it.ID == "" || it.Name == "" || it.Category == "" {
			skipped++ // 缺少必填字段，跳过并记录原因
			errs = append(errs, "缺少 id/name/category")
			continue
		}
		if it.Pattern != "" {
			// 带正则的条目先过编译校验，避免脏规则入库后拖垮全量重载
			if msg := validateRule(&ruleInput{
				Name: it.Name, Category: it.Category, Pattern: it.Pattern,
				Keywords: it.Keywords, Targets: it.Targets, Action: it.Action, Mode: it.Mode,
			}); msg != "" {
				skipped++
				errs = append(errs, it.ID+": "+msg)
				continue
			}
		}
		targets := strings.Join(it.Targets, ",")
		if targets == "" {
			targets = "path,query,body"
		}
		kws, _ := json.Marshal(it.Keywords)
		action := it.Action
		if action == "" {
			action = "block"
		}
		priority := it.Priority
		if priority == 0 {
			priority = 100
		}
		group := it.Group
		if group == "" {
			group = "imported"
		}
		mode := it.Mode
		if mode == "" {
			mode = "inherit"
		}
		vendor := it.Vendor
		if vendor == "" {
			vendor = in.Vendor
		}
		if vendor == "" {
			vendor = "imported"
		}
		source := it.Source
		if source == "" {
			source = "imported"
		}

		var old model.Rule
		// 按库中是否已有同 ID 规则决定走更新还是新建；DryRun 时只计数不写库
		found := s.Store.Gorm().Where("id = ?", it.ID).First(&old).Error == nil
		if in.DryRun {
			if found {
				updated++
			} else {
				created++
			}
			continue
		}
		if found {
			// 更新分支：不改动 enabled，保留已有规则的启用状态（merge 语义）
			updates := map[string]any{
				"name": it.Name, "category": it.Category, "score": it.Score,
				"targets": targets, "keywords": string(kws), "pattern": it.Pattern,
				"action": action, "priority": priority, "remark": it.Remark,
				"rule_group": group, "mode": mode, "vendor": vendor,
				"source": source, "version": it.Version, "updated_at": now,
			}
			if it.Score <= 0 {
				delete(updates, "score") // 文件未给有效分值时保留库中原分值
			}
			if err := s.Store.Gorm().Model(&model.Rule{}).Where("id = ?", it.ID).Updates(updates).Error; err != nil {
				errs = append(errs, it.ID+": "+err.Error())
				skipped++
				continue
			}
			updated++
			continue
		}
		enabled := 1
		if it.Enabled != nil {
			enabled = *it.Enabled // 仅新建规则时尊重文件里的启用状态
		}
		row := model.Rule{
			ID: it.ID, Name: it.Name, Category: it.Category, Score: it.Score,
			Targets: targets, Keywords: string(kws), Pattern: it.Pattern,
			Action: action, Enabled: enabled, Builtin: 0, Priority: priority,
			Remark: it.Remark, UpdatedAt: now, Source: source, Vendor: vendor,
			Version: it.Version, Group: group, Mode: mode,
		}
		if row.Score <= 0 {
			row.Score = 40
		}
		if err := s.Store.Gorm().Create(&row).Error; err != nil {
			errs = append(errs, it.ID+": "+err.Error())
			skipped++
			continue
		}
		created++
	}

	// replace 模式：清理库中此前由导入产生、但本次文件里不存在的规则，防止旧规则残留
	pruned := 0
	if in.Mode == "replace" && !in.DryRun {
		keep := make([]string, 0, len(items))
		for _, it := range items {
			keep = append(keep, it.ID)
		}
		res := s.Store.Gorm().Where("source = ? AND id NOT IN ?", "imported", keep).
			Delete(&model.Rule{})
		pruned = int(res.RowsAffected)
	}

	if !in.DryRun {
		s.reloadAfterWrite(c)
		c.Set(middleware.CtxAuditObj, "rule_import") // 审计对象为导入操作本身
	}
	if len(errs) > 10 {
		errs = errs[:10] // 错误明细最多返回 10 条，避免响应过大
	}
	_ = operator
	response.OK(c, gin.H{
		"created": created, "updated": updated, "skipped": skipped,
		"pruned": pruned, "errors": errs, "dryRun": in.DryRun,
	})
}

/* ============================================================ P4 命中统计 */

// RuleStats 返回规则命中情况：
//   - top：命中次数最多的规则（累计值来自后台回写，见 hits 列）
//   - dead：启用但从没命中过的规则，说明要么写错关键词，要么该删
//   - recent：近 N 小时事件里真实出现过的规则号
//
// 路由：GET /api/rules/stats（需登录）。
// 参数 c：Gin 上下文；查询参数 hours 为统计窗口小时数（1~720，默认 24）。
// 返回：无（200 返回 {"hours","top","dead","recent","total"}；数据库错误返回 500）。
func (s *Server) RuleStats(c *gin.Context) {
	hours := 24
	if v := c.Query("hours"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 720 {
			hours = n // 非法取值静默回落为默认 24 小时
		}
	}
	var rows []model.Rule
	if err := s.Store.Gorm().Where("enabled = 1").Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	top := make([]gin.H, 0, len(rows))
	dead := make([]gin.H, 0, 32)
	for _, r := range rows {
		item := gin.H{
			"id": r.ID, "name": r.Name, "category": r.Category,
			"group": r.Group, "hits": r.Hits, "lastHitAt": r.LastHitAt,
			"mode": r.Mode, "score": r.Score,
		}
		top = append(top, item)
		if r.Hits == 0 {
			dead = append(dead, item) // 从未命中的规则同时进入 dead 列表
		}
	}
	sort.Slice(top, func(i, j int) bool { return top[i]["hits"].(int64) > top[j]["hits"].(int64) })
	if len(top) > 50 {
		top = top[:50] // top 列表只保留命中最多的前 50 条
	}

	// 近 N 小时的真实命中（从事件表里数，比累计值更能反映现状）
	since := store.NowMS() - int64(hours)*3600*1000
	var events []model.Event
	// 统计失败不阻断响应，recent 退化为空列表；上限 2 万条防止大表拖慢接口
	_ = s.Store.Gorm().Select("rule_ids").Where("ts >= ? AND rule_ids != ''", since).
		Limit(20000).Find(&events).Error
	recent := map[string]int{}
	for _, e := range events {
		var ids []string
		if err := json.Unmarshal([]byte(e.RuleIDs), &ids); err != nil {
			continue // 单条事件解析失败直接跳过
		}
		for _, id := range ids {
			recent[id]++
		}
	}
	recentList := make([]gin.H, 0, len(recent))
	for id, n := range recent {
		recentList = append(recentList, gin.H{"id": id, "count": n})
	}
	sort.Slice(recentList, func(i, j int) bool {
		return recentList[i]["count"].(int) > recentList[j]["count"].(int)
	})
	if len(recentList) > 50 {
		recentList = recentList[:50] // 同样只返回前 50 条
	}
	response.OK(c, gin.H{
		"hours": hours, "top": top, "dead": dead, "recent": recentList,
		"total": len(rows),
	})
}

// RuleSamples 返回命中某条规则的最近事件，用于判断是不是误报。
// 路由：GET /api/rules/:id/samples（需登录）。
// 参数 c：Gin 上下文；路径参数 id 为规则 ID，查询参数 limit 为返回条数（1~50，默认 10）。
// 返回：无（200 返回 {"list":[事件记录]}，按时间倒序；数据库错误返回 500）。
func (s *Server) RuleSamples(c *gin.Context) {
	id := c.Param("id")
	limit := 10
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}
	var rows []model.Event
	// rule_ids 存的是 JSON 数组文本，这里用子串模糊匹配（ID 互为子串时可能带出邻近记录）
	if err := s.Store.Gorm().
		Where("rule_ids LIKE ?", "%"+id+"%").
		Order("ts DESC").Limit(limit).Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	response.OK(c, gin.H{"list": rows})
}
