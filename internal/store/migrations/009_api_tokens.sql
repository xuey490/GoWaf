-- 009: API 访问令牌（OpenAPI / MCP 客户端凭据）
--
-- 只存 SHA-256 摘要，明文令牌仅在创建接口返回一次。
-- scopes：read（默认，只读接口）/ read,write（含封禁等写接口）。
-- revoked_at：吊销时间戳，非 0 即失效；不做物理删除，保留审计线索。
CREATE TABLE IF NOT EXISTS api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  name         TEXT    NOT NULL DEFAULT '',
  token_hash   TEXT    NOT NULL DEFAULT '',
  scopes       TEXT    NOT NULL DEFAULT 'read',
  created_by   TEXT    NOT NULL DEFAULT '',
  created_at   INTEGER NOT NULL DEFAULT 0,
  last_used_at INTEGER NOT NULL DEFAULT 0,
  revoked_at   INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_api_tokens_hash ON api_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_api_tokens_revoked ON api_tokens(revoked_at);
