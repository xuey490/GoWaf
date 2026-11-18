-- 010: 精准访问控制策略动作参数（页面密码 / 自定义 CC 规则）
--
-- auth 与 rate_limit 两个动作需要附加参数（认证用户/密码哈希、限流窗口/阈值），
-- 统一以 JSON 存放在 config 列；其余动作保持空串。
-- auth 的密码一律存 bcrypt 哈希，明文只在创建/修改时经过内存。
ALTER TABLE access_policies ADD COLUMN config TEXT NOT NULL DEFAULT '';
