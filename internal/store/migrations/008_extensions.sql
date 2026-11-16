-- 008: 精准访问控制、Bot 管理、页面加速、双链路节点
--
-- 本迁移一次性建立第一批到第三批功能所需的配置表，全部由 GORM 读取，
-- 数据面在 runtime.Reload 时编译成不可变快照，请求路径不查库。

-- 精准访问控制策略：conditions 为 JSON 条件树（logic + items/groups）。
CREATE TABLE IF NOT EXISTS access_policies (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT    NOT NULL,
  site        TEXT    NOT NULL DEFAULT '',
  priority    INTEGER NOT NULL DEFAULT 100,
  enabled     INTEGER NOT NULL DEFAULT 1,
  action      TEXT    NOT NULL DEFAULT 'block',
  conditions  TEXT    NOT NULL DEFAULT '{"logic":"and","items":[]}',
  note        TEXT    NOT NULL DEFAULT '',
  hit_count   INTEGER NOT NULL DEFAULT 0,
  last_hit_at INTEGER NOT NULL DEFAULT 0,
  created_by  TEXT    NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL DEFAULT 0,
  updated_at  INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_access_policies_site ON access_policies(site);
CREATE INDEX IF NOT EXISTS idx_access_policies_enabled ON access_policies(enabled);

-- Bot 策略：match_type 支持 ua / path / ip / rdns，pattern 为正则。
CREATE TABLE IF NOT EXISTS bot_policies (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT    NOT NULL,
  site       TEXT    NOT NULL DEFAULT '',
  category   TEXT    NOT NULL DEFAULT 'unknown',
  action     TEXT    NOT NULL DEFAULT 'challenge',
  match_type TEXT    NOT NULL DEFAULT 'ua',
  pattern    TEXT    NOT NULL,
  priority   INTEGER NOT NULL DEFAULT 100,
  enabled    INTEGER NOT NULL DEFAULT 1,
  note       TEXT    NOT NULL DEFAULT '',
  created_by TEXT    NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_bot_policies_site ON bot_policies(site);

-- 蜜罐路径：命中即视为恶意 Bot，按站点配置。
CREATE TABLE IF NOT EXISTS bot_honeypots (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  site       TEXT    NOT NULL DEFAULT '',
  path       TEXT    NOT NULL,
  note       TEXT    NOT NULL DEFAULT '',
  enabled    INTEGER NOT NULL DEFAULT 1,
  created_by TEXT    NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_bot_honeypots_site ON bot_honeypots(site);

-- 页面缓存规则：kind 支持 suffix / exact / prefix / regex / directory / guest。
CREATE TABLE IF NOT EXISTS cache_rules (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT    NOT NULL,
  site        TEXT    NOT NULL DEFAULT '',
  kind        TEXT    NOT NULL DEFAULT 'suffix',
  pattern     TEXT    NOT NULL,
  ttl_seconds INTEGER NOT NULL DEFAULT 300,
  priority    INTEGER NOT NULL DEFAULT 100,
  enabled     INTEGER NOT NULL DEFAULT 1,
  note        TEXT    NOT NULL DEFAULT '',
  created_by  TEXT    NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL DEFAULT 0,
  updated_at  INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_cache_rules_site ON cache_rules(site);

-- 双链路回源：一个站点可配置多个上游，按线路标签与权重选择。
CREATE TABLE IF NOT EXISTS site_upstreams (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  site_id    INTEGER NOT NULL,
  addr       TEXT    NOT NULL,
  line       TEXT    NOT NULL DEFAULT 'bgp',
  weight     INTEGER NOT NULL DEFAULT 1,
  priority   INTEGER NOT NULL DEFAULT 100,
  enabled    INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_site_upstreams_site ON site_upstreams(site_id);

-- 接入节点：控制面登记，数据面 Agent 通过 token 上报心跳与拉取配置。
CREATE TABLE IF NOT EXISTS nodes (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  name           TEXT    NOT NULL,
  address        TEXT    NOT NULL DEFAULT '',
  line           TEXT    NOT NULL DEFAULT 'bgp',
  region         TEXT    NOT NULL DEFAULT '',
  weight         INTEGER NOT NULL DEFAULT 1,
  status         TEXT    NOT NULL DEFAULT 'offline',
  last_heartbeat INTEGER NOT NULL DEFAULT 0,
  version        TEXT    NOT NULL DEFAULT '',
  note           TEXT    NOT NULL DEFAULT '',
  created_at     INTEGER NOT NULL DEFAULT 0,
  updated_at     INTEGER NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_nodes_name ON nodes(name);
