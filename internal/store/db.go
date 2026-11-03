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

//go:embed migrations/*.sql
var migrationFS embed.FS

// Store 聚合两个连接池：写池（GORM）与只读池。
type Store struct {
	path  string
	gorm  *gorm.DB
	write *sql.DB
	read  *sql.DB
}

// Open 打开数据库、执行迁移、写入种子数据。
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

// buildDSN 拼接带 PRAGMA 的 DSN。
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

// migrate 按 schema_meta.version 顺序执行内嵌 SQL。
func (s *Store) migrate() error {
	if _, err := s.write.Exec(`CREATE TABLE IF NOT EXISTS schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		return err
	}
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

func (s *Store) schemaVersion() int {
	var v string
	err := s.write.QueryRow(`SELECT value FROM schema_meta WHERE key = 'version'`).Scan(&v)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(v)
	return n
}

func parseVersion(name string) (int, error) {
	base := strings.TrimSuffix(name, ".sql")
	prefix := strings.SplitN(base, "_", 2)[0]
	return strconv.Atoi(prefix)
}

// splitStatements 按分号切分 SQL（忽略注释行）。
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

// Gorm 返回 ORM 句柄。
func (s *Store) Gorm() *gorm.DB { return s.gorm }

// Write 返回写连接（原生 SQL）。
func (s *Store) Write() *sql.DB { return s.write }

// Read 返回只读连接池。
func (s *Store) Read() *sql.DB {
	if s.read != nil {
		return s.read
	}
	return s.write
}

// Path 返回数据库文件路径。
func (s *Store) Path() string { return s.path }

// Close 关闭连接。
func (s *Store) Close() error {
	if s.read != nil {
		_ = s.read.Close()
	}
	return s.write.Close()
}

// NowMS 统一毫秒时间戳。
func NowMS() int64 { return time.Now().UnixMilli() }

// 确保 model 包被引用（迁移依赖其表名常量）。
var _ = model.SchemaMeta{}
