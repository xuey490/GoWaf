-- 001_init.sql —— 全量建表，必须幂等。
-- 时间统一 Unix 毫秒（INTEGER）。

CREATE TABLE IF NOT EXISTS schema_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- 管理用户
CREATE TABLE IF NOT EXISTS sys_users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    user_name     TEXT    NOT NULL UNIQUE,
    password_hash TEXT    NOT NULL,
    nick_name     TEXT    DEFAULT '',
    email         TEXT    DEFAULT '',
    avatar        TEXT    DEFAULT '',
    role          TEXT    NOT NULL DEFAULT 'admin',
    buttons       TEXT    DEFAULT '[]',
    status        INTEGER NOT NULL DEFAULT 1,
    last_login_at INTEGER NOT NULL DEFAULT 0,
    last_login_ip TEXT    DEFAULT '',
    created_at    INTEGER NOT NULL DEFAULT 0,
    updated_at    INTEGER NOT NULL DEFAULT 0,
    deleted_at    INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sys_users_name ON sys_users(user_name);
CREATE INDEX IF NOT EXISTS idx_sys_users_deleted ON sys_users(deleted_at);

-- 刷新令牌
CREATE TABLE IF NOT EXISTS sys_refresh_tokens (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL,
    jti        TEXT    NOT NULL UNIQUE,
    expires_at INTEGER NOT NULL,
    revoked_at INTEGER NOT NULL DEFAULT 0,
    client_ip  TEXT    DEFAULT '',
    user_agent TEXT    DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_refresh_user ON sys_refresh_tokens(user_id, expires_at);

-- 站点
CREATE TABLE IF NOT EXISTS sites (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT    NOT NULL,
    hosts           TEXT    NOT NULL,
    upstream        TEXT    NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1,
    detect_body     INTEGER NOT NULL DEFAULT 1,
    max_body_bytes  INTEGER NOT NULL DEFAULT 262144,
    whitelist_paths TEXT    DEFAULT '[]',
    rate_limit      TEXT    DEFAULT '{}',
    created_at      INTEGER NOT NULL DEFAULT 0,
    updated_at      INTEGER NOT NULL DEFAULT 0
);

-- 规则
CREATE TABLE IF NOT EXISTS rules (
    id         TEXT    PRIMARY KEY,
    name       TEXT    NOT NULL,
    category   TEXT    NOT NULL,
    score      INTEGER NOT NULL DEFAULT 30,
    targets    TEXT    NOT NULL DEFAULT 'path,query,body',
    keywords   TEXT    DEFAULT '[]',
    pattern    TEXT    DEFAULT '',
    action     TEXT    NOT NULL DEFAULT 'block',
    enabled    INTEGER NOT NULL DEFAULT 1,
    builtin    INTEGER NOT NULL DEFAULT 0,
    priority   INTEGER NOT NULL DEFAULT 100,
    remark     TEXT    DEFAULT '',
    updated_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_rules_category ON rules(category);
CREATE INDEX IF NOT EXISTS idx_rules_enabled ON rules(enabled);

-- IP 名单
CREATE TABLE IF NOT EXISTS ip_rules (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    ip         TEXT    NOT NULL,
    kind       TEXT    NOT NULL,
    scope      TEXT    NOT NULL DEFAULT 'global',
    reason     TEXT    DEFAULT '',
    expires_at INTEGER NOT NULL DEFAULT 0,
    created_by TEXT    DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_ip_rules ON ip_rules(ip, kind, scope);

-- 封禁
CREATE TABLE IF NOT EXISTS bans (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    ip          TEXT    NOT NULL,
    reason      TEXT    DEFAULT '',
    source      TEXT    NOT NULL DEFAULT 'manual',
    hit_count   INTEGER NOT NULL DEFAULT 1,
    created_at  INTEGER NOT NULL DEFAULT 0,
    expires_at  INTEGER NOT NULL DEFAULT 0,
    released_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_bans_ip ON bans(ip, expires_at);
CREATE INDEX IF NOT EXISTS idx_bans_active ON bans(expires_at) WHERE released_at = 0;

-- 事件（热数据，走原生 SQL 批量写）
CREATE TABLE IF NOT EXISTS events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    ts         INTEGER NOT NULL,
    site       TEXT    DEFAULT '',
    client_ip  TEXT    NOT NULL,
    method     TEXT    DEFAULT '',
    host       TEXT    DEFAULT '',
    path       TEXT    DEFAULT '',
    query      TEXT    DEFAULT '',
    status     INTEGER NOT NULL DEFAULT 0,
    action     TEXT    NOT NULL DEFAULT 'pass',
    score      INTEGER NOT NULL DEFAULT 0,
    categories TEXT    DEFAULT '[]',
    rule_ids   TEXT    DEFAULT '[]',
    ua         TEXT    DEFAULT '',
    referer    TEXT    DEFAULT '',
    req_id     TEXT    DEFAULT '',
    latency_us INTEGER NOT NULL DEFAULT 0,
    upstream   TEXT    DEFAULT '',
    country    TEXT    DEFAULT '',
    bytes      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_events_ts       ON events(ts DESC);
CREATE INDEX IF NOT EXISTS idx_events_ip_ts    ON events(client_ip, ts DESC);
CREATE INDEX IF NOT EXISTS idx_events_action   ON events(action, ts DESC);
CREATE INDEX IF NOT EXISTS idx_events_site_ts  ON events(site, ts DESC);

-- 分钟级统计
CREATE TABLE IF NOT EXISTS stats_1m (
    minute     INTEGER NOT NULL,
    site       TEXT    NOT NULL,
    total      INTEGER NOT NULL DEFAULT 0,
    passed     INTEGER NOT NULL DEFAULT 0,
    blocked    INTEGER NOT NULL DEFAULT 0,
    challenged INTEGER NOT NULL DEFAULT 0,
    status_5xx INTEGER NOT NULL DEFAULT 0,
    categories TEXT    DEFAULT '{}',
    top_ips    TEXT    DEFAULT '{}',
    top_paths  TEXT    DEFAULT '{}',
    PRIMARY KEY (minute, site)
);

-- 审计
CREATE TABLE IF NOT EXISTS audit_logs (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    ts        INTEGER NOT NULL,
    operator  TEXT    NOT NULL,
    client_ip TEXT    DEFAULT '',
    action    TEXT    NOT NULL,
    object    TEXT    NOT NULL,
    object_id TEXT    DEFAULT '',
    before    TEXT    DEFAULT '',
    after     TEXT    DEFAULT '',
    result    TEXT    NOT NULL DEFAULT 'ok'
);
CREATE INDEX IF NOT EXISTS idx_audit_ts ON audit_logs(ts DESC);

-- 配置键值与版本
CREATE TABLE IF NOT EXISTS settings (
    key        TEXT    PRIMARY KEY,
    value      TEXT    DEFAULT '',
    version    INTEGER NOT NULL DEFAULT 1,
    updated_at INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS config_versions (
    version    INTEGER PRIMARY KEY AUTOINCREMENT,
    snapshot   TEXT    NOT NULL,
    operator   TEXT    DEFAULT '',
    remark     TEXT    DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT 0
);
