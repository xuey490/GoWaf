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
	ID       string   `yaml:"id" json:"id"`
	Name     string   `yaml:"name" json:"name"`
	Category string   `yaml:"category" json:"category"`
	Score    int      `yaml:"score" json:"score"`
	Targets  []string `yaml:"targets" json:"targets"`
	Keywords []string `yaml:"keywords" json:"keywords"`
	Pattern  string   `yaml:"pattern" json:"pattern"`
	Action   string   `yaml:"action" json:"action"`
	Priority int      `yaml:"priority" json:"priority"`
	Remark   string   `yaml:"remark" json:"remark"`
	Group    string   `yaml:"group" json:"group"`
	Mode     string   `yaml:"mode" json:"mode"`
	Source   string   `yaml:"source" json:"source"`
	Vendor   string   `yaml:"vendor" json:"vendor"`
	Version  string   `yaml:"version" json:"version"`
	Enabled  *int     `yaml:"enabled" json:"enabled"`
}

// RuleGroups 按规则组聚合并返回统计，供面板做整组启停。
func (s *Server) RuleGroups(c *gin.Context) {
	type bucket struct {
		Group   string `json:"group"`
		Source  string `json:"source"`
		Vendor  string `json:"vendor"`
		Total   int    `json:"total"`
		Enabled int    `json:"enabled"`
		Mode    string `json:"mode"`
	}
	var rows []model.Rule
	if err := s.Store.Gorm().Select("rule_group, source, vendor, enabled, mode").Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
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
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source == out[j].Source {
			return out[i].Group < out[j].Group
		}
		return out[i].Source < out[j].Source
	})
	response.OK(c, gin.H{"groups": out, "total": len(rows)})
}

// RuleBatch 批量操作：整组/勾选启停、改分、改动作、切观察模式、删除。
func (s *Server) RuleBatch(c *gin.Context) {
	var in struct {
		IDs    []string `json:"ids"`
		Group  string   `json:"group"`
		Source string   `json:"source"`
		Op     string   `json:"op"`
		Value  string   `json:"value"`
		Number int      `json:"number"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "参数格式错误")
		return
	}
	if in.Op == "" {
		response.BadRequest(c, "op 不能为空")
		return
	}
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
	switch in.Op {
	case "enable":
		q = q.Update("enabled", 1)
	case "disable":
		q = q.Update("enabled", 0)
	case "observe":
		q = q.Update("mode", "log")
	case "protect":
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
				ids = append(ids, r.ID)
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
	s.reloadAfterWrite(c)
	response.OK(c, gin.H{"affected": q.RowsAffected})
}

// reloadAfterWrite 写后热加载，失败只提示不影响已落库结果。
func (s *Server) reloadAfterWrite(c *gin.Context) {
	if err := s.reloadRuntime(c); err != nil {
		c.Set("reloadError", err.Error())
	}
}

// RuleExport 导出 YAML 规则包。
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
		enabled := r.Enabled
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
func (s *Server) RuleImport(c *gin.Context) {
	var in struct {
		Text   string `json:"text"`
		Mode   string `json:"mode"`
		DryRun bool   `json:"dryRun"`
		Vendor string `json:"vendor"`
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

	for _, it := range items {
		it.ID = strings.TrimSpace(it.ID)
		if it.ID == "" || it.Name == "" || it.Category == "" {
			skipped++
			errs = append(errs, "缺少 id/name/category")
			continue
		}
		if it.Pattern != "" {
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
			updates := map[string]any{
				"name": it.Name, "category": it.Category, "score": it.Score,
				"targets": targets, "keywords": string(kws), "pattern": it.Pattern,
				"action": action, "priority": priority, "remark": it.Remark,
				"rule_group": group, "mode": mode, "vendor": vendor,
				"source": source, "version": it.Version, "updated_at": now,
			}
			if it.Score <= 0 {
				delete(updates, "score")
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
			enabled = *it.Enabled
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
		c.Set(middleware.CtxAuditObj, "rule_import")
	}
	if len(errs) > 10 {
		errs = errs[:10]
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
func (s *Server) RuleStats(c *gin.Context) {
	hours := 24
	if v := c.Query("hours"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 720 {
			hours = n
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
			dead = append(dead, item)
		}
	}
	sort.Slice(top, func(i, j int) bool { return top[i]["hits"].(int64) > top[j]["hits"].(int64) })
	if len(top) > 50 {
		top = top[:50]
	}

	// 近 N 小时的真实命中（从事件表里数，比累计值更能反映现状）
	since := store.NowMS() - int64(hours)*3600*1000
	var events []model.Event
	_ = s.Store.Gorm().Select("rule_ids").Where("ts >= ? AND rule_ids != ''", since).
		Limit(20000).Find(&events).Error
	recent := map[string]int{}
	for _, e := range events {
		var ids []string
		if err := json.Unmarshal([]byte(e.RuleIDs), &ids); err != nil {
			continue
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
		recentList = recentList[:50]
	}
	response.OK(c, gin.H{
		"hours": hours, "top": top, "dead": dead, "recent": recentList,
		"total": len(rows),
	})
}

// RuleSamples 返回命中某条规则的最近事件，用于判断是不是误报。
func (s *Server) RuleSamples(c *gin.Context) {
	id := c.Param("id")
	limit := 10
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}
	var rows []model.Event
	if err := s.Store.Gorm().
		Where("rule_ids LIKE ?", "%"+id+"%").
		Order("ts DESC").Limit(limit).Find(&rows).Error; err != nil {
		s.failDB(c, err)
		return
	}
	response.OK(c, gin.H{"list": rows})
}
