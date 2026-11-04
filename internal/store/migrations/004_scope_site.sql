-- 004: 黑白名单 / 封禁支持「按站点」作用域
--
-- ip_rules.scope 早已存在（global/site），但"按站点"没有任何关联列，
-- 选完站点作用域后实际仍是全局生效。这里补上 site 列存站点名。
--
-- bans 原来只有全局封禁，补 scope + site：scope='global' 时 site 为空，
-- scope='site' 时只对该站点生效。自动封禁（auto / rule_ban）恒定全局。
ALTER TABLE ip_rules ADD COLUMN site TEXT NOT NULL DEFAULT '';

ALTER TABLE bans ADD COLUMN scope TEXT NOT NULL DEFAULT 'global';
ALTER TABLE bans ADD COLUMN site  TEXT NOT NULL DEFAULT '';

-- 旧唯一索引是 (ip, kind, scope)，两个站点加同一个 IP 会互相冲突，必须带 site
DROP INDEX IF EXISTS uk_ip_rules;
CREATE UNIQUE INDEX IF NOT EXISTS uk_ip_rules ON ip_rules(ip, kind, scope, site);

CREATE INDEX IF NOT EXISTS idx_ip_rules_site ON ip_rules(site);
CREATE INDEX IF NOT EXISTS idx_bans_site ON bans(site);
