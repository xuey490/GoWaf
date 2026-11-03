-- 003: 站点级防护配置覆盖 + 计划任务中心
--
-- sites.protect_cfg：JSON 或空串。空串=完全跟随全局设置；
-- enabled=1 时按字段覆盖（未设置的字段仍跟随全局），限流计数独立（key 为站点内 IP，
-- 不与全局限流器共享桶）。
ALTER TABLE sites ADD COLUMN protect_cfg TEXT NOT NULL DEFAULT '';

-- tasks：计划任务。内置 clean_events（清理过期攻击事件）与 backup_rules（备份检测规则）。
CREATE TABLE IF NOT EXISTS tasks (
    name            TEXT PRIMARY KEY,
    enabled         INTEGER NOT NULL DEFAULT 1,
    interval_h      INTEGER NOT NULL DEFAULT 6,
    params          TEXT    NOT NULL DEFAULT '{}',
    last_run_ms     INTEGER NOT NULL DEFAULT 0,
    last_result     TEXT    NOT NULL DEFAULT '',
    last_duration_ms INTEGER NOT NULL DEFAULT 0,
    total_runs      INTEGER NOT NULL DEFAULT 0,
    updated_at      INTEGER NOT NULL DEFAULT 0
);
