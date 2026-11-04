-- 005: 规则包体系（P0）、规则例外（P4）、威胁情报源（P3）
--
-- rules 增加来源/版本/分组/观察模式/命中统计：
--   source  builtin（随发行版）/ crs（OWASP CRS）/ custom（手写）/ imported（导入）
--   vendor  规则维护方，便于按来源整体启停
--   version 规则包版本，升级后能看出哪些规则更新过
--   rule_group 规则组，面板按组批量操作
--   mode    inherit（跟随引擎）/ log（只记录，计 0 分）/ block
--   hits / last_hit_at 命中计数，由后台任务定期刷库，供「规则该不该留」的判断
ALTER TABLE rules ADD COLUMN source     TEXT NOT NULL DEFAULT 'builtin';
ALTER TABLE rules ADD COLUMN vendor     TEXT NOT NULL DEFAULT 'gowaf';
ALTER TABLE rules ADD COLUMN version    TEXT NOT NULL DEFAULT '1';
ALTER TABLE rules ADD COLUMN rule_group TEXT NOT NULL DEFAULT 'base';
ALTER TABLE rules ADD COLUMN mode       TEXT NOT NULL DEFAULT 'inherit';
ALTER TABLE rules ADD COLUMN hits       INTEGER NOT NULL DEFAULT 0;
ALTER TABLE rules ADD COLUMN last_hit_at INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_rules_group  ON rules(rule_group);
CREATE INDEX IF NOT EXISTS idx_rules_source ON rules(source);

-- 规则例外：误报治理的核心。空 rule_id 表示该路径/参数下所有规则都跳过。
CREATE TABLE IF NOT EXISTS rule_exceptions (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  site        TEXT    NOT NULL DEFAULT '',
  rule_id     TEXT    NOT NULL DEFAULT '',
  category    TEXT    NOT NULL DEFAULT '',
  path_prefix TEXT    NOT NULL DEFAULT '',
  param       TEXT    NOT NULL DEFAULT '',
  note        TEXT    NOT NULL DEFAULT '',
  enabled     INTEGER NOT NULL DEFAULT 1,
  created_by  TEXT    NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_rule_exc_site ON rule_exceptions(site);

-- 威胁情报源：定时拉取 IP/CIDR 列表，命中直接封禁（可按站点作用域）。
CREATE TABLE IF NOT EXISTS intel_sources (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  name         TEXT    NOT NULL,
  url          TEXT    NOT NULL,
  kind         TEXT    NOT NULL DEFAULT 'ip',
  scope        TEXT    NOT NULL DEFAULT 'global',
  site         TEXT    NOT NULL DEFAULT '',
  ttl_seconds  INTEGER NOT NULL DEFAULT 0,
  enabled      INTEGER NOT NULL DEFAULT 1,
  last_sync_at INTEGER NOT NULL DEFAULT 0,
  last_count   INTEGER NOT NULL DEFAULT 0,
  last_error   TEXT    NOT NULL DEFAULT '',
  created_at   INTEGER NOT NULL DEFAULT 0
);
