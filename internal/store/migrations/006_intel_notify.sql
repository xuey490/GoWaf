-- 006: 情报源鉴权 + 邮件告警通知
--
-- headers：订阅源的自定义请求头，支持两种写法：
--   1) JSON 对象   {"Authorization":"Bearer xxx","Key":"yyy"}
--   2) 行文本      Authorization: Bearer xxx
-- 有了它才能接 AbuseIPDB / CrowdSec CTI 这类需要 API Key 的源。
ALTER TABLE intel_sources ADD COLUMN headers TEXT NOT NULL DEFAULT '';

-- 告警通知发送记录：面板上能看到最近发了什么、失败原因是什么，
-- 排障不用去翻 journalctl。channel 预留 email/feishu/sms/wecom。
CREATE TABLE IF NOT EXISTS notify_logs (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  channel    TEXT    NOT NULL DEFAULT 'email',
  target     TEXT    NOT NULL DEFAULT '',
  subject    TEXT    NOT NULL DEFAULT '',
  summary    TEXT    NOT NULL DEFAULT '',
  count      INTEGER NOT NULL DEFAULT 0,
  status     TEXT    NOT NULL DEFAULT 'ok',
  error      TEXT    NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_notify_logs_created ON notify_logs(created_at);
