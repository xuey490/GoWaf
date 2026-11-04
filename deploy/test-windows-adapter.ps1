# Windows 六种环境适配器单测。非 Windows 也可跑（PowerShell 7）。
# 覆盖：小皮 Apache/Nginx、宝塔 Apache/Nginx、原生 Apache/Nginx。
#
#   pwsh -File deploy/test-windows-adapter.ps1
#
$ErrorActionPreference = 'Stop'
$env:GOWAF_ADAPTER_TEST = '1'
$Root = Split-Path -Parent $PSScriptRoot
$Install = Join-Path $PSScriptRoot 'install.ps1'
. $Install

$script:Fails = 0
$script:Passes = 0
$Work = Join-Path ([System.IO.Path]::GetTempPath()) ("gowaf-winlab-" + [guid]::NewGuid().ToString('n'))
New-Item -ItemType Directory -Path $Work -Force | Out-Null

function Pass($msg) { $script:Passes++; Write-Host "  OK  $msg" -ForegroundColor Green }
function Fail($msg) { $script:Fails++; Write-Host "FAIL: $msg" -ForegroundColor Red }

function Reset-DetectState {
    $script:Engine = ''
    $script:PanelKind = ''
    $script:EngineLabel = ''
    $script:VhostDir = ''
    $script:ConfFile = ''
    $script:CertDir = ''
    $script:SearchRoots = @()
    $Engine = ''
    $Panel = ''
    $HttpOnly = $false
    Set-Variable -Name Engine -Value '' -Scope Script -ErrorAction SilentlyContinue
}

function New-LabRoot($name) {
    $p = Join-Path $Work $name
    New-Item -ItemType Directory -Path $p -Force | Out-Null
    return $p
}

function Write-Utf8($path, $text) {
    $dir = Split-Path $path
    if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
    Set-Content -Path $path -Value $text -Encoding utf8
}

# ---------- 搭六套目录 ----------

$phpApacheRoot = New-LabRoot 'phpstudy-apache'
Write-Utf8 (Join-Path $phpApacheRoot 'phpstudy_pro/Extensions/Apache2.4.39/conf/httpd.conf') @'
Listen 80
Listen 443
ServerRoot "C:/phpstudy_pro/Extensions/Apache2.4.39"
Include conf/vhosts/*.conf
'@
Write-Utf8 (Join-Path $phpApacheRoot 'phpstudy_pro/Extensions/Apache2.4.39/conf/vhosts/demo.conf') @'
<VirtualHost *:80>
    ServerName demo.local
    ServerAlias www.demo.local
    DocumentRoot "C:/phpstudy_pro/WWW/demo"
    RewriteEngine On
    RewriteCond %{SERVER_PORT} !^443$
    RewriteRule ^(.*)$ https://%{SERVER_NAME}$1 [L,R=301]
</VirtualHost>
<VirtualHost *:443>
    ServerName demo.local
    SSLEngine on
    SSLCertificateFile "C:/phpstudy_pro/Extensions/Apache2.4.39/conf/ssl/demo.crt"
</VirtualHost>
'@

$phpNginxRoot = New-LabRoot 'phpstudy-nginx'
Write-Utf8 (Join-Path $phpNginxRoot 'phpstudy_pro/Extensions/Nginx1.15.11/conf/nginx.conf') @'
worker_processes 1;
events { worker_connections 1024; }
http {
    include mime.types;
    include vhosts/*.conf;
}
'@
Write-Utf8 (Join-Path $phpNginxRoot 'phpstudy_pro/Extensions/Nginx1.15.11/conf/vhosts/demo.conf') @'
server {
    listen 80;
    listen 443 ssl;
    server_name demo.local www.demo.local;
    root html/demo;
    if ($server_port !~ ^443$){
        rewrite ^(/.*)$ https://$host$1 permanent;
    }
}
'@

$btApacheRoot = New-LabRoot 'bt-apache'
Write-Utf8 (Join-Path $btApacheRoot 'BtSoft/panel/BT-Panel') 'bt'
Write-Utf8 (Join-Path $btApacheRoot 'BtSoft/apache/conf/httpd.conf') @'
Listen 80
Listen 443
ServerRoot "C:/BtSoft/apache"
Include conf/vhost/*.conf
'@
Write-Utf8 (Join-Path $btApacheRoot 'BtSoft/apache/conf/vhost/demo.local.conf') @'
<VirtualHost *:80>
    ServerName demo.local
    DocumentRoot "C:/wwwroot/demo.local"
    #HTTP_TO_HTTPS_START
    RewriteEngine On
    RewriteCond %{SERVER_PORT} !^443$
    RewriteRule ^(.*)$ https://%{SERVER_NAME}$1 [L,R=301]
    #HTTP_TO_HTTPS_END
</VirtualHost>
<VirtualHost *:443>
    ServerName demo.local
    SSLEngine On
</VirtualHost>
'@

$btNginxRoot = New-LabRoot 'bt-nginx'
Write-Utf8 (Join-Path $btNginxRoot 'BtSoft/panel/BT-Panel') 'bt'
Write-Utf8 (Join-Path $btNginxRoot 'BtSoft/nginx/conf/nginx.conf') @'
worker_processes 1;
events { worker_connections 1024; }
http {
    include mime.types;
    include vhost/*.conf;
}
'@
Write-Utf8 (Join-Path $btNginxRoot 'BtSoft/nginx/conf/vhost/demo.local.conf') @'
server
{
    listen 80;
    listen [::]:80;
    listen 443 ssl http2;
    server_name demo.local www.demo.local;
    root C:/wwwroot/demo.local;
    if ($server_port !~ 443){
        rewrite ^(/.*)$ https://$host$1 permanent;
    }
    location / { try_files $uri $uri/ =404; }
}
'@

$sysApacheRoot = New-LabRoot 'native-apache'
Write-Utf8 (Join-Path $sysApacheRoot 'Apache24/conf/httpd.conf') @'
Listen 80
Listen 443
Define SRVROOT "C:/Apache24"
ServerRoot "${SRVROOT}"
#Include conf/extra/httpd-vhosts.conf
Include conf/extra/httpd-vhosts.conf
'@
Write-Utf8 (Join-Path $sysApacheRoot 'Apache24/conf/extra/httpd-vhosts.conf') @'
<VirtualHost *:80>
    ServerName demo.local
    DocumentRoot "C:/Apache24/htdocs/demo"
</VirtualHost>
<VirtualHost *:80>
    ServerName shop.local
    DocumentRoot "C:/Apache24/htdocs/shop"
</VirtualHost>
<VirtualHost *:443>
    ServerName demo.local
    SSLEngine on
</VirtualHost>
'@

$sysNginxRoot = New-LabRoot 'native-nginx'
Write-Utf8 (Join-Path $sysNginxRoot 'nginx/conf/nginx.conf') @'
worker_processes 1;
events { worker_connections 1024; }
http {
    include mime.types;
    include conf.d/*.conf;
    server {
        listen 80 default_server;
        server_name localhost;
        root html;
    }
}
'@
Write-Utf8 (Join-Path $sysNginxRoot 'nginx/conf/conf.d/shop.conf') @'
server {
    listen 80;
    server_name shop.local;
    root html/shop;
}
'@

function Invoke-Detect($labRoot, $expectPanel, $expectEngine) {
    Reset-DetectState
    $env:GOWAF_SEARCH_ROOTS = $labRoot
    $script:SearchRoots = @($labRoot)
    Detect-Engine
    if ($script:PanelKind -ne $expectPanel) {
        Fail "panel 期望 $expectPanel 实际 $($script:PanelKind)（$($script:EngineLabel)）"
        return $false
    }
    if ($script:Engine -ne $expectEngine) {
        Fail "engine 期望 $expectEngine 实际 $($script:Engine)（$($script:EngineLabel)）"
        return $false
    }
    return $true
}

function Assert-File($path, $pattern, $msg) {
    if (-not (Test-Path $path)) { Fail "$msg（文件不存在 $path）"; return }
    $c = Get-Content $path -Raw -Encoding utf8
    if ($c -notmatch $pattern) { Fail "$msg`n----`n$c`n----" } else { Pass $msg }
}

function Assert-NotFile($path, $pattern, $msg) {
    $c = Get-Content $path -Raw -Encoding utf8
    if ($c -match $pattern) { Fail "$msg`n----`n$c`n----" } else { Pass $msg }
}

function Test-NginxSyntax($confPath) {
    $nginx = Get-Command nginx -ErrorAction SilentlyContinue
    if (-not $nginx) { Write-Host "    (本机无 nginx，跳过 -t)"; return }
    $prefix = Join-Path $Work 'nginx-prefix'
    New-Item -ItemType Directory -Path (Join-Path $prefix 'logs') -Force | Out-Null
    $wrap = Join-Path $Work 'nginx-wrap.conf'
    $raw = Get-Content $confPath -Raw -Encoding utf8
    if ($raw -match '^\s*events\b' -or $raw -match '(?m)^\s*http\b') {
        Copy-Item $confPath $wrap -Force
        # 相对 include 改成绝对，避免 -c 到 wrap 后找不到 vhost
        $dir = Split-Path $confPath
        $txt = Get-Content $wrap -Raw
        $txt = $txt -replace 'include\s+vhosts/\*\.conf;', "include $dir/vhosts/*.conf;"
        $txt = $txt -replace 'include\s+vhost/\*\.conf;', "include $dir/vhost/*.conf;"
        $txt = $txt -replace 'include\s+conf\.d/\*\.conf;', "include $dir/conf.d/*.conf;"
        $txt = $txt -replace 'include\s+mime\.types;', 'include /etc/nginx/mime.types;'
        $txt = $txt -replace 'pid\s+\S+;', ''
        $errLog = (Join-Path $prefix 'logs/error.log') -replace '\\', '/'
        if ($txt -notmatch 'pid\s') { $txt = "pid $((Join-Path $prefix 'nginx.pid') -replace '\\','/');`nerror_log $errLog;`n" + $txt }
        $txt = [regex]::Replace($txt, '(?m)^(\s*http\s*\{)', "`$1`n    access_log off;")
        Set-Content $wrap $txt -Encoding utf8
        $err = & nginx -t -c $wrap -p $prefix 2>&1 | Out-String
        if ($LASTEXITCODE -ne 0) { Fail "nginx -t 失败：$err" } else { Pass "nginx -t $(Split-Path $confPath -Leaf)" }
    } else {
        $txt = @"
pid $(Join-Path $prefix 'nginx.pid');
events { worker_connections 64; }
http {
    include $confPath;
}
"@
        Set-Content $wrap $txt -Encoding utf8
        $err = & nginx -t -c $wrap -p $prefix 2>&1 | Out-String
        if ($LASTEXITCODE -ne 0) { Fail "nginx -t 失败：$err" } else { Pass "nginx -t $(Split-Path $confPath -Leaf)" }
    }
}

Write-Host "`n== 1. 小皮 Windows + Apache =="
if (Invoke-Detect $phpApacheRoot 'phpstudy' 'apache') {
    Pass "探测到 $($script:EngineLabel)"
    Patch-ApacheVhosts
    $vh = Join-Path $phpApacheRoot 'phpstudy_pro/Extensions/Apache2.4.39/conf/vhosts/demo.conf'
    $httpd = Join-Path $phpApacheRoot 'phpstudy_pro/Extensions/Apache2.4.39/conf/httpd.conf'
    Assert-File $vh '<VirtualHost 127\.0\.0\.1:8080>' 'VirtualHost *:80 → 127.0.0.1:8080'
    Assert-NotFile $vh '(?m)^\s*<VirtualHost\s+\*:80>' '不再保留 *:80'
    Assert-File $vh '# \[GOWAF\].*<VirtualHost \*:443>' '*:443 整段注释'
    Assert-File $vh 'X-Forwarded-Proto' '强制 HTTPS 补 X-Forwarded-Proto'
    Assert-File $httpd '# \[GOWAF\] Listen 80' 'httpd.conf Listen 80 已注释'
    Assert-File $httpd 'Listen 127\.0\.0\.1:8080' '注入 Listen 8080'
    $orig = Get-Content $vh -Raw
    Patch-ApacheVhosts
    if ((Get-Content $vh -Raw) -eq $orig) { Pass 'Apache 改写幂等' } else { Fail 'Apache 第二次改写改变了文件' }
}

Write-Host "`n== 2. 小皮 Windows + Nginx =="
if (Invoke-Detect $phpNginxRoot 'phpstudy' 'nginx') {
    Pass "探测到 $($script:EngineLabel)"
    Patch-NginxVhosts
    $vh = Join-Path $phpNginxRoot 'phpstudy_pro/Extensions/Nginx1.15.11/conf/vhosts/demo.conf'
    Assert-File $vh 'listen 127\.0\.0\.1:8080;' 'listen 80 → 127.0.0.1:8080'
    Assert-NotFile $vh '(?m)^\s*listen\s+80;' '不再保留 listen 80'
    Assert-File $vh '# \[GOWAF\].*listen 443 ssl' '同块 443 已注释'
    Assert-File $vh 'http_x_forwarded_proto' '强制 HTTPS 改 X-Forwarded-Proto'
    Test-NginxSyntax (Join-Path $phpNginxRoot 'phpstudy_pro/Extensions/Nginx1.15.11/conf/nginx.conf')
}

Write-Host "`n== 3. 宝塔 Windows + Apache =="
if (Invoke-Detect $btApacheRoot 'bt' 'apache') {
    Pass "探测到 $($script:EngineLabel)"
    Patch-ApacheVhosts
    $vh = Join-Path $btApacheRoot 'BtSoft/apache/conf/vhost/demo.local.conf'
    $httpd = Join-Path $btApacheRoot 'BtSoft/apache/conf/httpd.conf'
    Assert-File $vh '<VirtualHost 127\.0\.0\.1:8080>' '宝塔 Apache *:80 改写'
    Assert-File $vh '# \[GOWAF\].*<VirtualHost \*:443>' '宝塔 Apache *:443 注释'
    Assert-File $vh 'X-Forwarded-Proto' '宝塔强制 HTTPS 修复'
    Assert-File $httpd '# \[GOWAF\] Listen 80' '宝塔 httpd.conf Listen 80'
}

Write-Host "`n== 4. 宝塔 Windows + Nginx =="
if (Invoke-Detect $btNginxRoot 'bt' 'nginx') {
    Pass "探测到 $($script:EngineLabel)"
    Patch-NginxVhosts
    $vh = Join-Path $btNginxRoot 'BtSoft/nginx/conf/vhost/demo.local.conf'
    Assert-File $vh 'listen 127\.0\.0\.1:8080;' '宝塔 listen 80 改写（无 IP 前缀）'
    Assert-NotFile $vh '(?m)^\s*listen\s+80;' '宝塔 listen 80 已去掉'
    Assert-File $vh '# \[GOWAF\].*listen \[::\]:80' 'IPv6 listen 80 注释'
    Assert-File $vh '# \[GOWAF\].*listen 443 ssl' '宝塔 443 ssl 注释'
    Assert-File $vh 'http_x_forwarded_proto' '宝塔 `$server_port !~ 443` 修复'
    Test-NginxSyntax (Join-Path $btNginxRoot 'BtSoft/nginx/conf/nginx.conf')
}

Write-Host "`n== 5. 原生 Windows Apache =="
if (Invoke-Detect $sysApacheRoot 'system' 'apache') {
    Pass "探测到 $($script:EngineLabel)"
    Patch-ApacheVhosts
    $vh = Join-Path $sysApacheRoot 'Apache24/conf/extra/httpd-vhosts.conf'
    $httpd = Join-Path $sysApacheRoot 'Apache24/conf/httpd.conf'
    Assert-File $vh '<VirtualHost 127\.0\.0\.1:8080>' '原生 extra/httpd-vhosts.conf *:80'
    Assert-File $vh 'ServerName shop.local' '第二个 vhost 还在'
    Assert-File $vh '# \[GOWAF\].*<VirtualHost \*:443>' '原生 *:443 注释'
    Assert-File $httpd '# \[GOWAF\] Listen 80' '原生 Listen 80 注释'
    Assert-File $httpd '# \[GOWAF\] Listen 443' '原生 Listen 443 注释'
    Assert-File $httpd 'Listen 127\.0\.0\.1:8080' '原生注入 Listen 8080'
}

Write-Host "`n== 6. 原生 Windows Nginx =="
if (Invoke-Detect $sysNginxRoot 'system' 'nginx') {
    Pass "探测到 $($script:EngineLabel)"
    Patch-NginxVhosts
    $main = Join-Path $sysNginxRoot 'nginx/conf/nginx.conf'
    $site = Join-Path $sysNginxRoot 'nginx/conf/conf.d/shop.conf'
    Assert-File $main 'listen 127\.0\.0\.1:8080;' '原生 nginx.conf default_server 改写'
    Assert-NotFile $main 'default_server' '摘掉 default_server（多站点 8080 冲突）'
    Assert-File $site 'listen 127\.0\.0\.1:8080;' 'conf.d/shop.conf listen 80 改写'
    Assert-File $site 'server_name shop.local' 'shop.local 仍在'
    Test-NginxSyntax $main
}

Write-Host "`n== 7. 同机双引擎必须显式指定 =="
$dual = New-LabRoot 'dual'
Copy-Item (Join-Path $phpApacheRoot 'phpstudy_pro') (Join-Path $dual 'phpstudy_pro') -Recurse
# 把 nginx 也拷进同一棵 phpstudy_pro
$dstExt = Join-Path $dual 'phpstudy_pro/Extensions/Nginx1.15.11'
Copy-Item (Join-Path $phpNginxRoot 'phpstudy_pro/Extensions/Nginx1.15.11') $dstExt -Recurse
Reset-DetectState
$env:GOWAF_SEARCH_ROOTS = $dual
$script:SearchRoots = @($dual)
try {
    Detect-Engine
    Fail '双引擎没有拒绝自动猜测'
} catch {
    if ("$($_.Exception.Message)" -match '-Engine') { Pass '双引擎拒绝自动猜测，要求 -Engine' }
    else { Fail "双引擎报错不符合预期：$($_.Exception.Message)" }
}

Write-Host ""
Remove-Item $Work -Recurse -Force -ErrorAction SilentlyContinue
if ($script:Fails -gt 0) {
    Write-Host "失败 $($script:Fails) 项，通过 $($script:Passes) 项" -ForegroundColor Red
    exit 1
}
Write-Host "Windows 六种环境适配器全部通过（$($script:Passes) 项）" -ForegroundColor Green
exit 0
