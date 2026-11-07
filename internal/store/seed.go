package store

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"

	"gowaf/internal/config"
	"gowaf/internal/store/model"
)

// builtinRule 是 rules/*.yaml 内置规则文件中单条规则的结构。
type builtinRule struct {
	ID       string   `yaml:"id"`       // 规则唯一标识（主键），如 sqli-union-select
	Name     string   `yaml:"name"`     // 规则名称（展示用）
	Category string   `yaml:"category"` // 攻击类别，如 sqli / xss / scanner
	Score    int      `yaml:"score"`    // 命中后累加的风险分
	Targets  []string `yaml:"targets"`  // 检测目标（path/query/body 等），空则默认 path,query,body
	Keywords []string `yaml:"keywords"` // 关键字列表（JSON 落库，供 AC 自动机预筛），空表示不预筛
	Pattern  string   `yaml:"pattern"`  // 正则表达式主体
	Action   string   `yaml:"action"`   // 处置动作（block/challenge/log/ban 等），空则默认 block
	Priority int      `yaml:"priority"` // 优先级，空则默认 100
	Remark   string   `yaml:"remark"`   // 备注说明
	Group    string   `yaml:"group"`    // 规则组，空则取 Category
	Mode     string   `yaml:"mode"`     // 匹配模式（inherit/log/block），空则默认 inherit
	Version  string   `yaml:"version"`  // 规则版本号，空则默认 "2"
}

// AllButtons 是权限按钮全集，管理员默认全给，viewer/demo 只读。
var AllButtons = []string{
	"ip:ban", "rule:write", "rule:delete", "site:write", "settings:write", "audit:read",
}

// HashPassword 用 bcrypt 生成口令哈希，cost 固定为 12。
// 参数 pwd：明文口令。
// 返回 (string, error)：bcrypt 哈希文本；生成失败时返回错误。
func HashPassword(pwd string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pwd), 12)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// RandomPassword 生成 16 字符的随机口令。
// 实现：读取 12 字节密码学安全随机数并做 base64url 编码。
// 返回 string：随机口令；系统熵源不可用时返回固定兜底口令。
func RandomPassword() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "ChangeMe_12345678"
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

// SeedAdmin 用户表为空时创建初始管理员账号（role=admin，拥有全部按钮权限），保证首次部署可登录。
// 参数 userName：管理员用户名。
// 返回 (string, error)：随机明文口令（仅首次创建时返回，调用方需展示给用户；已存在用户时为空串）；查询或创建失败时返回错误。
func (s *Store) SeedAdmin(userName string) (string, error) {
	var count int64
	if err := s.gorm.Model(&model.User{}).Where("deleted_at = 0").Count(&count).Error; err != nil {
		return "", err
	}
	if count > 0 {
		return "", nil
	}
	pwd := RandomPassword()
	hash, err := HashPassword(pwd)
	if err != nil {
		return "", err
	}
	buttons, _ := json.Marshal(AllButtons)
	now := NowMS()
	u := model.User{
		UserName:     userName,
		PasswordHash: hash,
		NickName:     "管理员",
		Role:         "admin",
		Buttons:      string(buttons),
		Status:       1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.gorm.Create(&u).Error; err != nil {
		return "", err
	}
	return pwd, nil
}

// SeedDemoUser 创建演示账号（role=demo，无任何按钮权限），同名用户已存在则跳过。
// 参数 userName：用户名；password：明文口令。
// 返回 error：查询或创建失败时返回错误；跳过时返回 nil。
func (s *Store) SeedDemoUser(userName, password string) error {
	var count int64
	if err := s.gorm.Model(&model.User{}).Where("user_name = ?", userName).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	now := NowMS()
	return s.gorm.Create(&model.User{
		UserName:     userName,
		PasswordHash: hash,
		NickName:     "演示账号",
		Role:         "demo",
		Buttons:      "[]",
		Status:       1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}).Error
}

// SeedSettings 逐项写入默认设置，键已存在则跳过（幂等，不覆盖用户修改）。
// 参数 values：默认设置的键值对。
// 返回 error：任一键查询或写入失败时返回错误。
func (s *Store) SeedSettings(values map[string]string) error {
	now := NowMS()
	for k, v := range values {
		var count int64
		if err := s.gorm.Model(&model.Setting{}).Where("key = ?", k).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		if err := s.gorm.Create(&model.Setting{Key: k, Value: v, Version: 1, UpdatedAt: now}).Error; err != nil {
			return err
		}
	}
	return nil
}

// SeedSites 把配置中的站点种子写入库，同名站点已存在则跳过（幂等）。
// 参数 sites：待初始化的站点列表。
// 返回 error：任一站点查询或写入失败时返回错误。
func (s *Store) SeedSites(sites []config.SiteSeed) error {
	now := NowMS()
	for _, item := range sites {
		var count int64
		if err := s.gorm.Model(&model.Site{}).Where("name = ?", item.Name).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		hosts, _ := json.Marshal(item.Hosts)
		if err := s.gorm.Create(&model.Site{
			Name:          item.Name,
			Hosts:         string(hosts),
			Upstream:      item.Upstream,
			Enabled:       1,
			DetectBody:    1,
			MaxBodyBytes:  262144,
			WhitelistPath: "[]",
			RateLimit:     "{}",
			BehindCDN:     model.NormalizeBehindCDN(item.BehindCDN),
			CreatedAt:     now,
			UpdatedAt:     now,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

// ImportBuiltinRules 从 YAML 文件同步内置规则到 rules 表。
// 参数 path：内置规则 YAML 文件路径；文件不存在时静默返回（视为无内置规则）。
// 返回 error：读取、解析或写库失败时返回错误。
//
// 语义：新增缺失的规则；已存在的内置规则**更新规则内容但保留 enabled 状态**。
// 这样升级二进制后规则库会跟着更新，而用户手动停用的规则不会被强行打开。
// 用户自建的规则（builtin=0）完全不受影响。
func (s *Store) ImportBuiltinRules(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var list []builtinRule
	if err := yaml.Unmarshal(raw, &list); err != nil {
		return fmt.Errorf("解析内置规则 %s: %w", path, err)
	}

	var existing []model.Rule
	if err := s.gorm.Where("builtin = 1").Find(&existing).Error; err != nil {
		return err
	}
	byID := make(map[string]model.Rule, len(existing))
	for _, r := range existing {
		byID[r.ID] = r
	}

	now := NowMS()
	for _, r := range list {
		// 缺省值兜底：targets/action/priority/group/mode/version 为空时取内置默认
		targets := strings.Join(r.Targets, ",")
		if targets == "" {
			targets = "path,query,body"
		}
		kws, _ := json.Marshal(r.Keywords)
		action := r.Action
		if action == "" {
			action = "block"
		}
		priority := r.Priority
		if priority == 0 {
			priority = 100
		}
		group := r.Group
		if group == "" {
			group = r.Category
		}
		mode := r.Mode
		if mode == "" {
			mode = "inherit"
		}
		version := r.Version
		if version == "" {
			version = "2"
		}

		if old, ok := byID[r.ID]; ok {
			// 只在内容变化时写库，避免每次启动都刷 updated_at
			if old.Score == r.Score && old.Pattern == r.Pattern && old.Keywords == string(kws) &&
				old.Targets == targets && old.Name == r.Name && old.Category == r.Category &&
				old.Action == action && old.Priority == priority &&
				old.Group == group && old.Mode == mode {
				continue
			}
			err := s.gorm.Model(&model.Rule{}).Where("id = ?", r.ID).Updates(map[string]any{
				"name":       r.Name,
				"category":   r.Category,
				"score":      r.Score,
				"targets":    targets,
				"keywords":   string(kws),
				"pattern":    r.Pattern,
				"action":     action,
				"priority":   priority,
				"remark":     r.Remark,
				"rule_group": group,
				"mode":       mode,
				"version":    version,
				"updated_at": now,
			}).Error
			if err != nil {
				return err
			}
			continue
		}

		rule := model.Rule{
			ID:        r.ID,
			Name:      r.Name,
			Category:  r.Category,
			Score:     r.Score,
			Targets:   targets,
			Keywords:  string(kws),
			Pattern:   r.Pattern,
			Action:    action,
			Enabled:   1,
			Builtin:   1,
			Priority:  priority,
			Remark:    r.Remark,
			UpdatedAt: now,
			Source:    "builtin",
			Vendor:    "gowaf",
			Version:   version,
			Group:     group,
			Mode:      mode,
		}
		if err := s.gorm.Create(&rule).Error; err != nil {
			return err
		}
	}
	return nil
}

// SeedDemoData 为演示库写入演示站点与 2000 条模拟攻击事件；站点或事件已存在则跳过（幂等）。
// 返回 error：写站点或灌事件失败时返回错误。
func (s *Store) SeedDemoData() error {
	var siteCount int64
	if err := s.gorm.Model(&model.Site{}).Count(&siteCount).Error; err != nil {
		return err
	}
	now := NowMS()
	if siteCount == 0 {
		hosts, _ := json.Marshal([]string{"demo.example.com", "www.demo.example.com"})
		if err := s.gorm.Create(&model.Site{
			Name:          "演示站点",
			Hosts:         string(hosts),
			Upstream:      "http://127.0.0.1:8080",
			Enabled:       1,
			DetectBody:    1,
			MaxBodyBytes:  262144,
			WhitelistPath: "[]",
			RateLimit:     "{}",
			CreatedAt:     now,
			UpdatedAt:     now,
		}).Error; err != nil {
			return err
		}
	}

	var eventCount int64
	if err := s.gorm.Model(&model.Event{}).Count(&eventCount).Error; err != nil {
		return err
	}
	if eventCount > 0 {
		return nil
	}

	// 2000 条模拟事件，IP 全部取自 RFC 5737 文档段，不含任何真实地址
	paths := []string{"/.env", "/wp-login.php", "/api/user?id=1%27%20or%201=1", "/index.php?s=/Index/think/app/invokefunction",
		"/api/order", "/.git/config", "/phpmyadmin/index.php", "/api/pay/notify", "/search?q=hello", "/static/app.js"}
	cats := map[string]string{
		"/.env":                        `["scanner","sensitive_file"]`,
		"/wp-login.php":                `["scanner"]`,
		"/api/user?id=1%27%20or%201=1": `["sqli"]`,
		"/index.php?s=/Index/think/app/invokefunction": `["rce"]`,
		"/api/order":            `[]`,
		"/.git/config":          `["sensitive_file"]`,
		"/phpmyadmin/index.php": `["scanner"]`,
		"/api/pay/notify":       `[]`,
		"/search?q=hello":       `[]`,
		"/static/app.js":        `[]`,
	}
	uas := []string{"sqlmap/1.7.2#stable", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)", "nikto/2.5.0", "curl/8.4.0"}

	tx, err := s.write.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(eventInsertSQL)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	for i := 0; i < 2000; i++ {
		p := paths[i%len(paths)]
		category := cats[p]
		action := "pass"
		status := 200
		score := 0
		if category != `[]` {
			action = "block"
			status = 403
			score = 60
		}
		if i%17 == 0 {
			// 每 17 条混入一条 challenge，避免事件分布全是 block/pass 两类
			action = "challenge"
			status = 403
			score = 35
		}
		// 时间戳从当前时刻向前铺开约 20.5 小时（每条间隔 37 秒），让趋势图与统计有时序数据
		ts := now - int64((2000-i)*1000*37)
		ip := fmt.Sprintf("198.51.100.%d", (i%250)+1)
		_, err := stmt.Exec(ts, "演示站点", ip, "GET", "demo.example.com", p, "",
			status, action, score, category, `[]`, uas[i%len(uas)], "", fmt.Sprintf("demo-%06d", i),
			int64(300+i%900), "http://127.0.0.1:8080", "", int64(200+i%3000))
		if err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return err
		}
	}
	_ = stmt.Close()
	return tx.Commit()
}

// ResetDemo 清空事件、封禁与审计日志后重新灌入演示数据，保证每次演示从相同状态开始。
// 返回 error：清表或灌数失败时返回错误。
func (s *Store) ResetDemo() error {
	if _, err := s.write.Exec(`DELETE FROM events`); err != nil {
		return err
	}
	if _, err := s.write.Exec(`DELETE FROM bans`); err != nil {
		return err
	}
	if _, err := s.write.Exec(`DELETE FROM audit_logs`); err != nil {
		return err
	}
	return s.SeedDemoData()
}
