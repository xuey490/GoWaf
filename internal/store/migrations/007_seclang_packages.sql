-- 007: SecLang 规则包（第三方 SecLang/CRS 插件包导入）
--
-- SecLang 规则与自研 YAML 规则引擎完全不同（Coraza 编译执行），
-- 单独建表管理：内容落盘 data/seclang/<slug>.conf，这里只存元信息。
CREATE TABLE IF NOT EXISTS seclang_packages (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT    NOT NULL DEFAULT '',
  slug        TEXT    NOT NULL UNIQUE,
  vendor      TEXT    NOT NULL DEFAULT '',
  version     TEXT    NOT NULL DEFAULT '1',
  rule_count  INTEGER NOT NULL DEFAULT 0,
  enabled     INTEGER NOT NULL DEFAULT 1,
  last_error  TEXT    NOT NULL DEFAULT '',
  note        TEXT    NOT NULL DEFAULT '',
  created_by  TEXT    NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL DEFAULT 0,
  updated_at  INTEGER NOT NULL DEFAULT 0
);
