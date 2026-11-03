-- 站点是否位于 CDN / 反向代理回源之后。
--
-- 取值：''（直连，灰云）｜'cloudflare'（橙云，对端是 CF 节点）。
-- 这一列不改变数据面的取 IP 逻辑（那是由 trusted_proxies 里是否有回源网段决定的），
-- 它只用于启动自检：标记了 CDN 却没信任回源网段时，真实 IP 会退化成 CF 节点 IP，
-- 限流与封禁会误伤该节点后面的全部访客。
ALTER TABLE sites ADD COLUMN behind_cdn TEXT NOT NULL DEFAULT '';
