-- P1 新特性（v1.7.0）：
-- 1. JA4 TLS 客户端指纹：事件库记录指纹，供 Bot/扫描器识别与恶意指纹拦截
-- 2. 控制台 TOTP 两步验证：管理员账号绑定 RFC6238 动态码
-- 3. 站点级自定义拦截页：模板/品牌化

ALTER TABLE events ADD COLUMN ja4 TEXT NOT NULL DEFAULT '';

ALTER TABLE sys_users ADD COLUMN totp_secret TEXT NOT NULL DEFAULT '';
ALTER TABLE sys_users ADD COLUMN totp_enabled INTEGER NOT NULL DEFAULT 0;

ALTER TABLE sites ADD COLUMN block_page TEXT NOT NULL DEFAULT '';
