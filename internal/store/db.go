// Package store 负责数据库连接、版本化迁移与数据访问。
//
// 分层边界（见方案 6.1）：
//   - 配置类表（用户/站点/规则/IP/审计/设置）走 GORM；
//   - events / stats_1m 这类高频写入走原生 SQL 批量事务。
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"gowaf/internal/store/model"
)

// migrationFS 内嵌的 SQL 迁移脚本集合（migrations/*.sql），编译期打入二进制。
//
//go:embed migrations/*.sql
var migrationFS embed.FS

// Store 聚合两个连接池：写池（GORM）与只读池。
type Store struct {
	path  string   // 数据库文件路径（即打开时传入的 dsn）
	gorm  *gorm.DB // GORM 句柄，配置类表（用户/站点/规则等）走这里
	write *sql.DB  // 写连接池：单连接，承载迁移与高频批量写入
	read  *sql.DB  // 只读连接池：多连接并发查询；未初始化时回退到写池
}

// Open 打开 SQLite 数据库、执行 PRAGMA 调优与版本化迁移，返回就绪的存储实例。
// 参数 dsn：数据库文件路径，其父目录不存在时会自动创建（权限 0750）。
// 返回 (*Store, error)：存储实例；建目录、打开数据库或迁移失败时返回错误。
func Open(dsn string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dsn), 0o750); err != nil {
		return nil, fmt.Errorf("创建数据目录: %w", err)
	}

	gdb, err := gorm.Open(sqlite.Open(buildDSN(dsn, false)), &gorm.Config{
		Logger:  logger.Default.LogMode(logger.Silent),
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, fmt.Errorf("打开数据库: %w", err)
	}

	write, err := gdb.DB()
	if err != nil {
		return nil, err
	}
	// 写池单连接：SQLite 只允许一个写者，多连接只会制造 database is locked
	write.SetMaxOpenConns(1)
	write.SetMaxIdleConns(1)
	write.SetConnMaxLifetime(0)

	s := &Store{path: dsn, gorm: gdb, write: write}
	if err := s.applyPragmas(); err != nil {
		return nil, err
	}
	if err := s.migrate(); err != nil {
		return nil, err
	}

	read, err := sql.Open("sqlite", buildDSN(dsn, true))
	if err != nil {
		return nil, fmt.Errorf("打开只读池: %w", err)
	}
	read.SetMaxOpenConns(8)
	read.SetMaxIdleConns(8)
	s.read = read

	return s, nil
}

// buildDSN 拼接带 PRAGMA 参数的 SQLite 连接串。
// 参数 path：数据库文件路径；readOnly：true 时追加只读模式（mode=ro 与 query_only=1）。
// 返回 string：形如 file:<path>?_pragma=... 的 DSN。
func buildDSN(path string, readOnly bool) string {
	params := []string{
		"_pragma=journal_mode(WAL)",
		"_pragma=busy_timeout(5000)",
		"_pragma=synchronous(NORMAL)",
		"_pragma=temp_store(MEMORY)",
		"_pragma=cache_size(-32000)",
	}
	if readOnly {
		params = append(params, "mode=ro", "_pragma=query_only(1)")
	}
	return "file:" + path + "?" + strings.Join(params, "&")
}

// applyPragmas 在写连接上逐条执行性能与并发相关的 PRAGMA（WAL、busy_timeout 等），
// 与 buildDSN 中的连接参数保持一致。
// 返回 error：任一 PRAGMA 执行失败时返回错误。
func (s *Store) applyPragmas() error {
	stmts := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA temp_store = MEMORY",
		"PRAGMA cache_size = -32000",
	}
	for _, q := range stmts {
		if _, err := s.write.Exec(q); err != nil {
			return fmt.Errorf("执行 %s: %w", q, err)
		}
	}
	return nil
}

// migrate 按文件名版本号顺序执行内嵌的 SQL 迁移脚本。
// 每个脚本的全部语句与版本号写入放在同一事务中，失败即回滚并中止启动，
// 保证不会出现只执行了一半的 schema。
// 返回 error：建版本表或任一迁移失败时返回错误。
func (s *Store) migrate() error {
	if _, err := s.write.Exec(`CREATE TABLE IF NOT EXISTS schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		return err
	}
	// 已应用的最新版本；全新库为 0，因此所有脚本都会依次执行
	current := s.schemaVersion()

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		ver, err := parseVersion(name)
		if err != nil {
			return err
		}
		if ver <= current {
			continue
		}
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := s.write.Begin()
		if err != nil {
			return err
		}
		for _, stmt := range splitStatements(string(body)) {
			if _, err := tx.Exec(stmt); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("迁移 %s 失败: %w", name, err)
			}
		}
		// 版本号与脚本语句同事务写入（upsert），回滚时一并撤销
		if _, err := tx.Exec(`INSERT INTO schema_meta(key, value) VALUES('version', ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, strconv.Itoa(ver)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		log.Printf("[store] 迁移完成: %s (version=%d)", name, ver)
	}
	return nil
}

// schemaVersion 读取 schema_meta 表中已应用的迁移版本号。
// 返回 int：版本号；表不存在、无记录或值非法时返回 0（按全新库处理）。
func (s *Store) schemaVersion() int {
	var v string
	err := s.write.QueryRow(`SELECT value FROM schema_meta WHERE key = 'version'`).Scan(&v)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(v)
	return n
}

// parseVersion 从迁移文件名解析版本号：取去掉 ".sql" 后缀后第一个 "_" 之前的数字前缀。
// 参数 name：迁移文件名，如 "0002_create_events.sql"。
// 返回 (int, error)：版本号；前缀不是整数时返回错误。
func parseVersion(name string) (int, error) {
	base := strings.TrimSuffix(name, ".sql")
	prefix := strings.SplitN(base, "_", 2)[0]
	return strconv.Atoi(prefix)
}

// splitStatements 把整段 SQL 脚本按分号切分为单条语句。
// 跳过空行和以 -- 开头的注释行；末尾没有分号的剩余片段也会作为一条语句返回。
// 参数 body：完整脚本文本。
// 返回 []string：去掉行尾分号后的语句列表。
func splitStatements(body string) []string {
	var out []string
	var cur strings.Builder
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		cur.WriteString(line)
		cur.WriteString("\n")
		if strings.HasSuffix(trimmed, ";") {
			stmt := strings.TrimSpace(cur.String())
			stmt = strings.TrimSuffix(stmt, ";")
			if stmt != "" {
				out = append(out, stmt)
			}
			cur.Reset()
		}
	}
	if rest := strings.TrimSpace(cur.String()); rest != "" {
		out = append(out, rest)
	}
	return out
}

// Gorm 返回 ORM 句柄，供上层对配置类表做读写。
// 返回 *gorm.DB：与写池同源的 GORM 连接。
func (s *Store) Gorm() *gorm.DB { return s.gorm }

// Write 返回原生 SQL 写连接（单连接）。
// 返回 *sql.DB：写池句柄，用于批量插入、迁移等场景。
func (s *Store) Write() *sql.DB { return s.write }

// Read 返回只读连接池，用于查询类访问。
// 返回 *sql.DB：只读池；尚未初始化时回退返回写池。
func (s *Store) Read() *sql.DB {
	if s.read != nil {
		return s.read
	}
	return s.write
}

// Path 返回数据库文件路径。
// 返回 string：打开时传入的 dsn。
func (s *Store) Path() string { return s.path }

// Close 关闭只读池与写池。
// 返回 error：写池关闭失败时返回错误（只读池的关闭错误被忽略）。
func (s *Store) Close() error {
	if s.read != nil {
		_ = s.read.Close()
	}
	return s.write.Close()
}

// NowMS 返回当前时间的毫秒时间戳，全库时间字段统一使用该精度。
// 返回 int64：毫秒时间戳。
func NowMS() int64 { return time.Now().UnixMilli() }

// 确保 model 包被引用（迁移依赖其表名常量）。
var _ = model.SchemaMeta{}
