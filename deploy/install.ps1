# GoWAF Windows 部署脚本 —— 服务模式 + IIS / 小皮 Apache / 小皮 Nginx 适配
#
# 用法（管理员 PowerShell）：
#   .\install.ps1 doctor                 # 只读体检（探测引擎/站点/端口，不动任何东西）
#   .\install.ps1 install                # 备份 → 引擎让出 80/443 → gowaf svc → 防火墙
#   .\install.ps1 install -Engine iis    # 同机多引擎时显式指定（iis|apache|nginx）
#   .\install.ps1 install -HttpOnly      # 只接管 80，443 不动
#   .\install.ps1 rollback               # 从备份还原引擎配置，流量回到引擎直连
#   .\install.ps1 uninstall              # rollback + 删服务 + 删数据目录
#   .\install.ps1 status                 # 服务与端口状态
#
# 拓扑：gowaf 占宿主 80/443（TLS 终止）→ 引擎退守 127.0.0.1:8080（明文）
# 降级说明：IIS 没有 realip 模块，后端应用看到的客户端 IP 是 127.0.0.1；
#           WAF 在边缘看到的是真实 IP，限流/封禁不受影响。

[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('install', 'rollback', 'uninstall', 'doctor', 'status', 'start', 'stop')]
    [string]$Command = 'install',

    # 同机多引擎时显式指定接管谁
    [ValidateSet('', 'iis', 'apache', 'nginx')]
    [string]$Engine = '',

    [switch]$HttpOnly,
    [switch]$Force,
    [int]$UpstreamPort = 8080,
    [int]$AdminPort = 8899
)

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
if (-not (Test-Path (Join-Path $PSScriptRoot 'gowaf-windows-amd64.exe'))) {
    Write-Host "找不到 gowaf-windows-amd64.exe（部署包不完整）" -ForegroundColor Red
    exit 1
}

# ---------------------------------------------------------------- 常量

$GwHome = Join-Path $env:ProgramData 'gowaf'
$GwBin = Join-Path $GwHome 'bin\gowaf.exe'
$GwConf = Join-Path $GwHome 'configs\config.yaml'
$GwRules = Join-Path $GwHome 'rules\builtin.yaml'
$GwBackup = Join-Path $GwHome 'backup'
$GwData = Join-Path $GwHome 'data'

function Write-Step($msg) { Write-Host "`n==> $msg" -ForegroundColor Cyan }
function Write-Ok($msg)   { Write-Host "    [OK] $msg" -ForegroundColor Green }
function Write-Warn2($msg) { Write-Host "    [!]  $msg" -ForegroundColor Yellow }
function Die($msg) {
    Write-Host "`n[X] $msg" -ForegroundColor Red
    exit 1
}
function Confirm-Yes($msg) {
    if ($Force) { return $true }
    $ans = Read-Host "$msg [y/N]"
    return ($ans -match '^[yY]')
}

function Assert-Admin {
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    $p = New-Object Security.Principal.WindowsPrincipal($id)
    if (-not $p.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        Die "请用管理员身份运行 PowerShell（右键 → 以管理员身份运行）"
    }
}

# ---------------------------------------------------------------- 引擎探测

$script:Engine = ''       # iis / apache / nginx
$script:EngineLabel = ''
$script:VhostDir = ''     # 站点配置目录（phpStudy 的 vhosts；IIS 无需）
$script:Appcmd = "$env:SystemRoot\System32\inetsrv\appcmd.exe"

function Test-IisPresent {
    return ((Get-Service W3SVC -ErrorAction SilentlyContinue) -and (Test-Path $script:Appcmd))
}

function Get-PhpstudyVhostDirs {
    # phpstudy_pro 安装在任意盘：Extensions\Apache*\conf\vhosts 与 Nginx*\conf\vhosts
    $found = @()
    foreach ($drive in (@('C', 'D', 'E') | Where-Object { Test-Path "$($_):\" })) {
        $pro = Get-ChildItem "$($drive):\" -Directory -Filter 'phpstudy_pro' -ErrorAction SilentlyContinue | Select-Object -First 1
        if (-not $pro) { continue }
        $ext = Join-Path $pro.FullName 'Extensions'
        if (-not (Test-Path $ext)) { continue }
        foreach ($d in (Get-ChildItem $ext -Directory -ErrorAction SilentlyContinue)) {
            $vh = Join-Path $d.FullName 'conf\vhosts'
            if ((Test-Path $vh) -and (Get-ChildItem $vh -Filter '*.conf' -ErrorAction SilentlyContinue)) {
                $found += ,@($d.Name, $vh)
            }
        }
    }
    return $found
}

function Detect-Engine {
    Write-Step '探测引擎'

    # 显式指定优先
    if ($Engine -ne '') {
        $script:Engine = $Engine
    }
    else {
        # 谁在监听 80？
        $conn = Get-NetTCPConnection -LocalPort 80 -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($conn) {
            $procName = (Get-Process -Id $conn.OwningProcess -ErrorAction SilentlyContinue).ProcessName
            switch -Regex ($procName) {
                '^(System|w3wp|svchost)$' { if (Test-IisPresent) { $script:Engine = 'iis' } }   # http.sys 由 System 持有
                '^httpd$'                 { $script:Engine = 'apache' }
                '^nginx$'                 { $script:Engine = 'nginx' }
            }
        }
        if ($script:Engine -eq '') {
            # 没在跑：按存在性判断
            if (Test-IisPresent) { $script:Engine = 'iis' }
            else {
                $php = Get-PhpstudyVhostDirs
                if ($php.Count -gt 0) {
                    $apacheDir = ($php | Where-Object { $_[0] -like 'Apache*' })
                    if ($apacheDir) { $script:Engine = 'apache' } else { $script:Engine = 'nginx' }
                } else {
                    Die "没探测到 IIS / 小皮 Apache / 小皮 Nginx。没有站点可接管的机器装 WAF 无意义"
                }
            }
        }
    }

    switch ($script:Engine) {
        'iis' {
            if (-not (Test-IisPresent)) { Die "指定了 iis，但机器上没有 IIS（W3SVC 或 appcmd 缺失）" }
            $script:EngineLabel = 'IIS'
        }
        'apache' {
            $php = Get-PhpstudyVhostDirs | Where-Object { $_[0] -like 'Apache*' }
            if (-not $php) { Die "指定了 apache，但没找到 phpstudy_pro\Extensions\Apache*\conf\vhosts" }
            $script:VhostDir = $php[0][1]
            $script:EngineLabel = "小皮 Apache（$($php[0][0])）"
        }
        'nginx' {
            $php = Get-PhpstudyVhostDirs | Where-Object { $_[0] -like 'Nginx*' }
            if (-not $php) { Die "指定了 nginx，但没找到 phpstudy_pro\Extensions\Nginx*\conf\vhosts" }
            $script:VhostDir = $php[0][1]
            $script:EngineLabel = "小皮 Nginx（$($php[0][0])）"
        }
    }
    Write-Ok "引擎：$script:EngineLabel"
    if ($script:VhostDir) { Write-Ok "站点目录：$script:VhostDir" }
}

# ---------------------------------------------------------------- 备份 / 还原

function Backup-Engine {
    $ts = Get-Date -Format 'yyyyMMdd-HHmmss'
    $bk = Join-Path $GwBackup $ts
    New-Item -ItemType Directory -Path $bk -Force | Out-Null

    if ($script:Engine -eq 'iis') {
        Copy-Item "$env:SystemRoot\System32\inetsrv\config\applicationHost.config" $bk -Force
        Copy-Item "$env:SystemRoot\System32\inetsrv\config\applicationHost.config.bak" $bk -ErrorAction SilentlyContinue -Force
    }
    if ($script:VhostDir -and (Test-Path $script:VhostDir)) {
        New-Item -ItemType Directory -Path (Join-Path $bk 'vhosts') -Force | Out-Null
        Copy-Item "$script:VhostDir\*" (Join-Path $bk 'vhosts') -Force
    }
    @{
        Engine       = $script:Engine
        VhostDir     = $script:VhostDir
        UpstreamPort = $UpstreamPort
        Take443      = [int](-not $HttpOnly)
        Time         = $ts
    } | ConvertTo-Json | Set-Content (Join-Path $bk 'meta.json') -Encoding utf8

    (New-Item -ItemType SymbolicLink -Path (Join-Path $GwBackup 'latest') -Target $bk -Force) | Out-Null
    # 只留最近 10 份
    Get-ChildItem $GwBackup -Directory | Where-Object { $_.Name -match '^\d{8}-' } |
        Sort-Object Name -Descending | Select-Object -Skip 10 | Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
    Write-Ok "引擎配置已备份到 $bk"
    return $bk
}

function Get-LatestBackup {
    $link = Join-Path $GwBackup 'latest'
    if (-not (Test-Path $link)) { return $null }
    return (Get-Item $link).Target
}

function Restore-Engine($bk) {
    if (-not $bk -or -not (Test-Path $bk)) { Die "备份目录不存在：$bk" }
    Write-Step "从备份还原引擎配置（$bk）"

    if ($script:Engine -eq 'iis') {
        Stop-Service W3SVC -Force -ErrorAction SilentlyContinue
        Stop-Service WAS -Force -ErrorAction SilentlyContinue
        Copy-Item (Join-Path $bk 'applicationHost.config') "$env:SystemRoot\System32\inetsrv\config\applicationHost.config" -Force
        Start-Service WAS -ErrorAction SilentlyContinue
        Start-Service W3SVC -ErrorAction SilentlyContinue
    }
    if ($script:VhostDir -and (Test-Path (Join-Path $bk 'vhosts'))) {
        Copy-Item (Join-Path $bk 'vhosts\*') $script:VhostDir -Force
        Restart-Engine
    }
    Write-Ok "引擎配置已还原"
}

# ---------------------------------------------------------------- Apache / Nginx 改写（小皮 Windows）

function Restart-Engine {
    switch ($script:Engine) {
        'apache' {
            $httpd = Get-ChildItem 'C:\','D:\','E:\' -Recurse -Filter 'httpd.exe' -Depth 4 -ErrorAction SilentlyContinue |
                Where-Object { $_.FullName -like '*phpstudy_pro*' } | Select-Object -First 1
            if ($httpd) {
                $svcName = (Get-Service | Where-Object { $_.Name -match 'Apache' -and $_.Status -eq 'Running' } | Select-Object -First 1).Name
                if ($svcName) { Restart-Service $svcName -Force }
            }
        }
        'nginx' {
            $ng = Get-ChildItem 'C:\','D:\','E:\' -Recurse -Filter 'nginx.exe' -Depth 4 -ErrorAction SilentlyContinue |
                Where-Object { $_.FullName -like '*phpstudy_pro*' } | Select-Object -First 1
            if ($ng) {
                Get-Process nginx -ErrorAction SilentlyContinue | Stop-Process -Force
                Start-Sleep 1
                Start-Process $ng.FullName -WorkingDirectory $ng.DirectoryName -WindowStyle Hidden
            }
        }
    }
}

# Apache：VirtualHost *:80 → 127.0.0.1:8080；443 处理；强制 HTTPS 修复
function Patch-ApacheVhosts {
    $takeLabel = if (-not $HttpOnly) { '/443' } else { '' }
    Write-Step "改写小皮 Apache vhost（让出 80$takeLabel，退到 127.0.0.1:$UpstreamPort）"
    foreach ($f in (Get-ChildItem "$script:VhostDir\*.conf")) {
        $c = Get-Content $f.FullName -Raw -Encoding utf8
        $orig = $c
        $c = $c -replace '(?m)^(\s*<VirtualHost\s+)\*:80(\s*>)', "`$1 127.0.0.1:$UpstreamPort`$2"
        if (-not $HttpOnly) {
            # TLS 已由 WAF 终止，443 vhost 整段注释（避免重复 ServerName）
            $c = $c -replace '(?ms)^(\s*<VirtualHost\s+\*):443(\s*>.*?)</VirtualHost>', "# [GOWAF] `$1:443`$2</VirtualHost>"
            # 强制 HTTPS 死循环修复（宝塔模板同款 RewriteCond）
            if ($c -notmatch 'X-Forwarded-Proto') {
                $c = $c -replace 'RewriteCond\s+%\{SERVER_PORT\}\s+!\^443\$', "RewriteCond `%{HTTP:X-Forwarded-Proto} !^https`$`n        RewriteCond `%{SERVER_PORT} !^443`$"
            }
        }
        if ($c -ne $orig) {
            Set-Content $f.FullName $c -Encoding utf8 -NoNewline
            Write-Ok "已改写 $($f.Name)"
        }
    }
}

# Nginx：listen 80 → 127.0.0.1:8080；IPv6 注释；纯 443 块转明文
function Patch-NginxVhosts {
    $takeLabel = if (-not $HttpOnly) { '/443' } else { '' }
    Write-Step "改写小皮 Nginx vhost（让出 80$takeLabel，退到 127.0.0.1:$UpstreamPort）"
    foreach ($f in (Get-ChildItem "$script:VhostDir\*.conf")) {
        $lines = Get-Content $f.FullName -Encoding utf8
        $has80 = $false
        foreach ($l in $lines) { if ($l -match '^\s*listen\s+(\*|0\.0\.0\.0):?80\s*;') { $has80 = $true; break } }
        $out = @()
        foreach ($l in $lines) {
            $n = $l
            if ($l -match '^\s*listen\s+\[::\]:80\s*;') {
                $n = "# [GOWAF] $($l.TrimStart())"
            }
            elseif ($l -match '^(\s*listen\s+)((\*|0\.0\.0\.0):)?80(\s+[^;]*)?;\s*$') {
                $n = "$($Matches[1])127.0.0.1:$UpstreamPort;"
            }
            elseif ((-not $HttpOnly) -and $l -match '^\s*listen\s+((\*|0\.0\.0\.0|\[::\]):)?443\s+ssl[^;]*;\s*$') {
                if ($has80) { $n = "# [GOWAF] $($l.TrimStart())" }
                else { $n = $l -replace '^\s*listen\s+.*;\s*$', "    listen 127.0.0.1:$UpstreamPort;" }
            }
            elseif ((-not $HttpOnly) -and $l -match 'if\s*\(\s*\$server_port\s*!~\s*\^443\$?\s*\)') {
                $n = 'if ($http_x_forwarded_proto != "https"){'
            }
            $out += $n
        }
        Set-Content $f.FullName $out -Encoding utf8
        Write-Ok "已改写 $($f.Name)"
    }
}

# ---------------------------------------------------------------- IIS 适配

function Get-IisSites {
    # 输出：site 名 | 绑定列表（原始）
    $raw = & $script:Appcmd list sites 2>$null
    $sites = @()
    foreach ($line in $raw) {
        if ($line -match '^SITE\s+"(.+?)"\s+\(id:(\d+),bindings:(.+),state:') {
            $sites += ,@($Matches[1], [int]$Matches[2], $Matches[3])
        }
    }
    return $sites
}

function Patch-IisBindings {
    Write-Step "改写 IIS 绑定（让出 80/443，退到 127.0.0.1:$UpstreamPort）"

    # 改 applicationHost.config 前先停 W3SVC（避免运行中写配置被覆盖回去）
    Stop-Service W3SVC -Force -ErrorAction SilentlyContinue

    foreach ($site in (Get-IisSites)) {
        $name = $site[0]; $bindings = $site[2]
        $newB = @()
        $changed = $false
        foreach ($b in ($bindings -split ',')) {
            if ($b -match '^http/(\*|[^:]*):80:(.*)$') {
                $host_ = $Matches[2]
                $newB += "http/127.0.0.1:$UpstreamPort`:$host_"
                $changed = $true
            }
            elseif (-not $HttpOnly -and $b -match '^https?/([^:]*):443:(.*)$') {
                # https 绑定转 http（TLS 由 WAF 终止；IIS 侧不再需要证书）
                $host_ = $Matches[2]
                if ($host_ -ne '') { $newB += "http/127.0.0.1:$UpstreamPort`:$host_" }
                $changed = $true
            }
            else {
                $newB += $b
            }
        }
        if (-not $changed) { Write-Warn2 "跳过 $($name)（没有 *:80 / *:443 绑定）"; continue }
        $joined = $newB -join ','
        & $script:Appcmd set site "$name" /bindings:"$joined" | Out-Null
        Write-Ok "$name → $joined"
    }

    Start-Service W3SVC -ErrorAction SilentlyContinue
    Write-Warn2 "提示：IIS 无 realip 模块，后端应用看到的客户端 IP 是 127.0.0.1（WAF 侧不受影响）"
}

# ---------------------------------------------------------------- gowaf 本体安装

# 站点发现：IIS 绑定域名 / phpStudy vhost 域名声明
function Get-SiteHostnames {
    $names = @()
    if ($script:Engine -eq 'iis') {
        foreach ($site in (Get-IisSites)) {
            foreach ($b in ($site[2] -split ',')) {
                if ($b -match '^https?/[^:]*:\d+:(.+)$') {
                    $h = $Matches[1]
                    if ($h -and $h -ne '*' -and $names -notcontains $h) { $names += $h }
                }
            }
        }
    }
    elseif ($script:VhostDir) {
        $pat = if ($script:Engine -eq 'apache') { '^\s*Server(Name|Alias)\s+(.+)$' } else { '^\s*server_name\s+(.+);' }
        foreach ($f in (Get-ChildItem "$script:VhostDir\*.conf" -ErrorAction SilentlyContinue)) {
            $hits = Select-String -Path $f.FullName -Pattern $pat -ErrorAction SilentlyContinue
            foreach ($h in $hits) {
                foreach ($tok in ($h.Matches[0].Groups[2].Value -split '\s+')) {
                    $tok = $tok.Trim().TrimEnd(';').ToLower()
                    if ($tok -and $tok -ne '*' -and $tok -ne '_' -and $tok -ne 'localhost' -and $names -notcontains $tok) { $names += $tok }
                }
            }
        }
    }
    return $names
}

# 与 install.sh render_config 同构的 Windows 配置渲染
function Render-WindowsConfig($mode) {
    Write-Step "生成配置（engine.mode=$mode）"

    $take443 = -not $HttpOnly
    $siteNames = @(Get-SiteHostnames)

    $sb = New-Object System.Text.StringBuilder
    [void]$sb.AppendLine("# GoWAF 生产配置（由 deploy/install.ps1 生成于 $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')）")
    [void]$sb.AppendLine("# 修改后需 gowaf svc stop + start 生效；控制台里改的设置优先于本文件的 sites。")
    [void]$sb.AppendLine("")
    [void]$sb.AppendLine("server:")
    [void]$sb.AppendLine('  listen: "0.0.0.0:80"')
    if ($take443) { [void]$sb.AppendLine('  tls_listen: "0.0.0.0:443"') } else { [void]$sb.AppendLine('  tls_listen: ""') }
    [void]$sb.AppendLine("  admin_listen: `"127.0.0.1:$AdminPort`"")
    [void]$sb.AppendLine('  read_header_timeout: 5s')
    [void]$sb.AppendLine('  read_timeout: 30s')
    [void]$sb.AppendLine('  write_timeout: 0s')
    [void]$sb.AppendLine('  idle_timeout: 90s')
    [void]$sb.AppendLine('  max_header_bytes: 32768')
    [void]$sb.AppendLine('  shutdown_timeout: 15s')
    [void]$sb.AppendLine('  trusted_proxies:')
    [void]$sb.AppendLine('    - "127.0.0.1/32"')
    [void]$sb.AppendLine('')
    if ($take443) {
        [void]$sb.AppendLine('tls:')
        [void]$sb.AppendLine('  enabled: true')
        [void]$sb.AppendLine("  cert_dir: `"$GwData\certs`"")
        if ($siteNames.Count -gt 0) { [void]$sb.AppendLine("  default_cert: `"$($siteNames[0])`"") }
        [void]$sb.AppendLine('  min_version: "1.2"')
        [void]$sb.AppendLine('  hsts: false          # 首次上线先别开 HSTS，避免证书问题被浏览器记住')
    } else {
        [void]$sb.AppendLine('tls:')
        [void]$sb.AppendLine('  enabled: false       # 未接管 443：HTTPS 仍直连引擎，WAF 不检测这部分流量')
    }
    [void]$sb.AppendLine('')
    [void]$sb.AppendLine('storage:')
    [void]$sb.AppendLine("  dsn: `"$GwData\waf.db`"")
    [void]$sb.AppendLine('  retain_days: 30')
    [void]$sb.AppendLine('  sample_rate: 0.01')
    [void]$sb.AppendLine('')
    [void]$sb.AppendLine('sites:')
    foreach ($n in $siteNames) {
        [void]$sb.AppendLine("  - name: `"$n`"")
        [void]$sb.AppendLine("    hosts: [`"$n`"]")
        [void]$sb.AppendLine("    upstream: `"http://127.0.0.1:$UpstreamPort`"")
    }
    [void]$sb.AppendLine('  - name: "兜底站点"')
    [void]$sb.AppendLine('    hosts: ["*"]')
    [void]$sb.AppendLine("    upstream: `"http://127.0.0.1:$UpstreamPort`"")
    [void]$sb.AppendLine('')
    [void]$sb.AppendLine('engine:')
    [void]$sb.AppendLine("  mode: `"$mode`"")
    [void]$sb.AppendLine('  block_score: 60')
    [void]$sb.AppendLine('  challenge_score: 30')
    [void]$sb.AppendLine('  max_body_inspect: 262144')
    [void]$sb.AppendLine('  max_targets: 64')
    [void]$sb.AppendLine('  regex_timeout_ms: 50')
    [void]$sb.AppendLine("  rules_file: `"$GwRules`"")
    [void]$sb.AppendLine('')
    [void]$sb.AppendLine('guard:')
    if ($mode -eq 'observe') {
        [void]$sb.AppendLine('  global_rps: 0')
        [void]$sb.AppendLine('  ip_rps: 500')
        [void]$sb.AppendLine('  ip_burst: 1000')
        [void]$sb.AppendLine('  ip_per_minute: 0')
        [void]$sb.AppendLine('  auto_ban: false')
    } else {
        [void]$sb.AppendLine('  global_rps: 0')
        [void]$sb.AppendLine('  ip_rps: 50')
        [void]$sb.AppendLine('  ip_burst: 100')
        [void]$sb.AppendLine('  ip_per_minute: 2000')
        [void]$sb.AppendLine('  auto_ban: true')
    }
    [void]$sb.AppendLine('  challenge_per_minute: 300')
    [void]$sb.AppendLine('  ban_per_minute: 600')
    [void]$sb.AppendLine('  ban_1h_per_minute: 1200')
    [void]$sb.AppendLine('  ban_window: 10m')
    [void]$sb.AppendLine('  ban_threshold: 5')
    [void]$sb.AppendLine('  ban_ttl: 1h')
    [void]$sb.AppendLine('  ban_max_ttl: 24h')
    [void]$sb.AppendLine('  ban_max_strikes: 3')
    [void]$sb.AppendLine('  ban_strike_reset: 24h')
    [void]$sb.AppendLine('  whitelist_static: true')
    [void]$sb.AppendLine('')
    [void]$sb.AppendLine('admin:')
    [void]$sb.AppendLine('  allowed_ips: ["127.0.0.1/32"]')

    Set-Content -Path $GwConf -Value $sb.ToString() -Encoding utf8
    Write-Ok "配置已生成：$GwConf（站点 $($siteNames.Count) 个 + 兜底）"
}

function Install-Gowaf {
    Write-Step '安装 gowaf 主程序'

    New-Item -ItemType Directory -Path (Join-Path $GwHome 'bin') -Force | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $GwHome 'rules') -Force | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $GwHome 'configs') -Force | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $GwHome 'logs') -Force | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $GwData 'certs') -Force | Out-Null

    # 服务在跑时不能覆盖 exe：先改名（老进程继续跑），最后 svc restart 切换
    $svc = Get-Service gowaf -ErrorAction SilentlyContinue
    if ($svc -and $svc.Status -eq 'Running') {
        Move-Item $GwBin "$GwBin.prev" -Force -ErrorAction SilentlyContinue
    }
    Copy-Item (Join-Path $PSScriptRoot 'gowaf-windows-amd64.exe') $GwBin -Force
    Copy-Item (Join-Path $PSScriptRoot 'builtin.yaml') $GwRules -Force
    Write-Ok "二进制与规则就位：$GwHome"

    # 重装/升级不偷偷改模式：已经是 protect 就保持 protect
    $mode = 'observe'
    if (Test-Path $GwConf) {
        $oldMode = (Select-String -Path $GwConf -Pattern '^\s*mode:\s*"?(\w+)"?' | Select-Object -First 1).Matches[0].Groups[1].Value
        if ($oldMode -eq 'protect') { $mode = 'protect' }
        # 模式记录下来给渲染函数
        $script:KeepMode = $mode
        # 现有配置不动（保留控制台侧修改）
        Write-Ok "已有配置，保持不变（模式 $mode）"
    }
}

function Install-Svc {
    Write-Step '注册 Windows 服务（gowaf svc install）'
    & $GwBin svc install $GwConf
    if ($LASTEXITCODE -ne 0) { Die "服务安装失败（需要管理员权限）" }
}

function Set-Firewall {
    Write-Step '防火墙放行 80/443/8899'
    foreach ($p in @(80, 443, $AdminPort)) {
        $rule = "GoWAF-TCP-$p"
        $exists = Get-NetFirewallRule -DisplayName $rule -ErrorAction SilentlyContinue
        if (-not $exists) {
            New-NetFirewallRule -DisplayName $rule -Direction Inbound -Protocol TCP -LocalPort $p -Action Allow -Profile Any | Out-Null
            Write-Ok "放行 TCP $p"
        } else {
            Write-Ok "TCP $p 规则已存在"
        }
    }
}

# ---------------------------------------------------------------- 命令

function Cmd-Doctor {
    Assert-Admin
    Detect-Engine
    Write-Step '端口占用'
    foreach ($p in @(80, 443, $UpstreamPort, $AdminPort)) {
        $c = Get-NetTCPConnection -LocalPort $p -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($c) {
            $proc = (Get-Process -Id $c.OwningProcess -ErrorAction SilentlyContinue).ProcessName
            Write-Host "    $p  监听中（$proc pid=$($c.OwningProcess)）"
        } else {
            Write-Host "    $p  空闲"
        }
    }
    if ($script:Engine -eq 'iis') {
        Write-Step 'IIS 站点清单'
        foreach ($s in (Get-IisSites)) { Write-Host "    $($s[0])  bindings: $($s[2])" }
    }
    elseif ($script:VhostDir) {
        Write-Step '站点清单（域名声明）'
        foreach ($f in (Get-ChildItem "$script:VhostDir\*.conf")) {
            $names = @()
            if ($script:Engine -eq 'apache') {
                $names = (Select-String -Path $f.FullName -Pattern '^\s*Server(Name|Alias)\s+(.+)$' | ForEach-Object { $_.Matches[0].Groups[2].Value })
            } else {
                $names = (Select-String -Path $f.FullName -Pattern '^\s*server_name\s+(.+);' | ForEach-Object { $_.Matches[0].Groups[1].Value })
            }
            if ($names) { Write-Host "    $($f.Name)  $($names -join ' ')" }
        }
    }
    $svc = Get-Service gowaf -ErrorAction SilentlyContinue
    if ($svc) { Write-Ok "gowaf 服务：$($svc.Status)" } else { Write-Ok "gowaf 服务：未安装" }
}

function Cmd-Install {
    Assert-Admin
    Detect-Engine
    Install-Gowaf

    $mode = if ($script:KeepMode) { $script:KeepMode } else { 'observe' }
    if (-not (Test-Path $GwConf)) {
        Render-WindowsConfig $mode
    }
    & $GwBin -t -c $GwConf
    if ($LASTEXITCODE -ne 0) { Die "配置校验失败：$GwConf" }
    Write-Ok "gowaf 配置校验通过"

    $bk = Backup-Engine

    try {
        switch ($script:Engine) {
            'iis'    { Patch-IisBindings }
            'apache' { Patch-ApacheVhosts; Restart-Engine }
            'nginx'  { Patch-NginxVhosts; Restart-Engine }
        }
        # 确认引擎真的让出了 80
        Start-Sleep 2
        $c = Get-NetTCPConnection -LocalPort 80 -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($c) {
            $proc = (Get-Process -Id $c.OwningProcess).ProcessName
            if ($proc -notmatch 'gowaf') {
                Write-Warn2 "80 仍被 $proc 占用（gowaf 尚未接管）"
            }
        }
        Install-Svc
        Set-Firewall
    }
    catch {
        Write-Warn2 "安装失败：$($_.Exception.Message)，正在回滚..."
        Restore-Engine $bk
        Die "已回滚，站点恢复原状"
    }

    Write-Host ""
    Write-Host "================================================" -ForegroundColor Green
    Write-Host " GoWAF 部署完成" -ForegroundColor Green
    Write-Host "================================================" -ForegroundColor Green
    Write-Host " 后端     127.0.0.1:$UpstreamPort（$script:EngineLabel）"
    Write-Host " 控制台   http://127.0.0.1:$AdminPort"
    Write-Host " 服务     Get-Service gowaf / gowaf svc start|stop"
    Write-Host " 回滚     .\install.ps1 rollback"
    Write-Host ""
    $pwFile = Join-Path $GwData 'initial-password.txt'
    if (Test-Path $pwFile) {
        Write-Host " 初始账号 admin / $(Get-Content $pwFile -ErrorAction SilentlyContinue)" -ForegroundColor Cyan
        Write-Host " （登录改密后请删除 $pwFile）"
    }
}

function Cmd-Rollback {
    Assert-Admin
    Detect-Engine
    $bk = Get-LatestBackup
    if (-not $bk) { Die "没有可用备份，无法回滚" }

    Write-Step '停止 gowaf 服务'
    $svc = Get-Service gowaf -ErrorAction SilentlyContinue
    if ($svc) { Stop-Service gowaf -Force -ErrorAction SilentlyContinue }

    Restore-Engine $bk
    Write-Ok "已还原到部署前状态（备份 $bk），80/443 交还引擎"
}

function Cmd-Uninstall {
    Assert-Admin
    Cmd-Rollback
    Write-Step '卸载服务与数据'
    & $GwBin svc uninstall 2>$null
    Start-Sleep 1
    Remove-Item $GwHome -Recurse -Force -ErrorAction SilentlyContinue
    foreach ($p in @(80, 443, $AdminPort)) {
        Remove-NetFirewallRule -DisplayName "GoWAF-TCP-$p" -ErrorAction SilentlyContinue
    }
    Write-Ok "已彻底卸载"
}

function Cmd-Status {
    $svc = Get-Service gowaf -ErrorAction SilentlyContinue
    if ($svc) { Write-Host "gowaf 服务：$($svc.Status)" } else { Write-Host "gowaf 服务：未安装" }
    foreach ($p in @(80, 443, $UpstreamPort, $AdminPort)) {
        $c = Get-NetTCPConnection -LocalPort $p -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($c) {
            Write-Host "    $p  $( (Get-Process -Id $c.OwningProcess -ErrorAction SilentlyContinue).ProcessName )"
        } else { Write-Host "    $p  空闲" }
    }
}

switch ($Command) {
    'install'   { Cmd-Install }
    'rollback'  { Cmd-Rollback }
    'uninstall' { Cmd-Uninstall }
    'doctor'    { Cmd-Doctor }
    'status'    { Cmd-Status }
    'start'     { Assert-Admin; Start-Service gowaf; Write-Ok 'gowaf 已启动' }
    'stop'      { Assert-Admin; Stop-Service gowaf -Force; Write-Warn2 'gowaf 已停止——80/443 无人监听，站点不可访问。恢复请 rollback 或 start' }
}
