-- waf_detect.lua — 把 GoWAF 检测引擎挂到 Nginx 的 access 阶段。
--
-- 用法（nginx.conf）：
--   lua_shared_dict waf_cache 10m;
--   server {
--     location / {
--       access_by_lua_file /etc/nginx/waf_detect.lua;
--       proxy_pass http://your_backend;
--     }
--   }
--
-- 行为：POST JSON 到 GoWAF 独立检测端口 /detect；action=block/ban → 403；
-- challenge → 403 带重试提示（网关侧也可改为跳转挑战页）。
-- 简单旁路缓存：同 path+query 60 秒内命中过「放行」直接复用结论，
-- 省掉对静态资源的重复调用。

local http = require "resty.http"   -- lua-resty-http（opm get pintsized/lua-resty-http）
local cjson = require "cjson.safe"

local WAF_URL = "http://127.0.0.1:8020"  -- server.detect_listen
local WAF_TOKEN = ""                      -- server.detect_token，空则不认证

local dict = ngx.shared.waf_cache
local cache_key = ngx.var.uri .. "?" .. (ngx.var.args or "")
local cached = dict and dict:get(cache_key)
if cached == "1" then
  return -- 60 秒内已放行
end

local headers = {}
for k, v in pairs(ngx.req.get_headers()) do
  if type(v) == "table" then headers[k] = table.concat(v, ", ") else headers[k] = v end
end

-- 读取请求体（≤1MB，与 Nginx client_body_buffer_size 对齐）
ngx.req.read_body()
local body = ngx.req.get_body_data() or ""

local payload = cjson.encode({
  method = ngx.req.get_method(),
  host = ngx.var.host,
  path = ngx.var.uri,
  query = ngx.var.args or "",
  body = body,
  headers = headers,
})

local httpc = http.new()
httpc:set_timeout(2000)  -- 检测引擎超时 2s，与 engine.timeout_ms 对齐
local res, err = httpc:request_uri(WAF_URL .. "/detect", {
  method = "POST",
  body = payload,
  headers = {
    ["Content-Type"] = "application/json",
    ["X-WAF-Token"] = WAF_TOKEN,
  },
  keepalive_timeout = 15000,
  keepalive_pool = 32,
})

-- 引擎不可达：fail-open（与 engine.fail_mode=open 语义一致，保业务优先）
if not res then
  ngx.log(ngx.ERR, "waf detect unreachable: ", err)
  return
end

local verdict = cjson.decode(res.body)
if not verdict then return end

if verdict.action == "block" or verdict.action == "ban" then
  ngx.status = 403
  ngx.header["Content-Type"] = "application/json; charset=utf-8"
  ngx.say(cjson.encode({ blocked = true, score = verdict.score, rules = verdict.rule_ids }))
  ngx.exit(403)
end

if dict and verdict.action == "pass" and (ngx.var.args or "") == "" then
  dict:set(cache_key, "1", 60)
end
