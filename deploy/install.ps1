# GoWAF Windows 部署脚本 —— 服务模式 + 小皮 / 宝塔 / 原生 Apache·Nginx / IIS
#
# 用法（管理员 PowerShell）：
#   .\install.ps1 doctor                 # 只读体检（探测引擎/站点/端口，不动任何东西）
#   .\install.ps1 install                # 备份 → 引擎让出 80/443 → gowaf svc → 防火墙
#   .\install.ps1 install -Engine nginx  # 同机多引擎时显式指定（iis|apache|nginx）
#   .\install.ps1 install -Panel bt      # 同机多面板时显式指定（phpstudy|bt|system）
#   .\install.ps1 install -SearchRoot D:\apps   # 非默认盘符/子目录：把安装父目录加进搜索
#   .\install.ps1 install -Engine nginx -VhostDir D:\stack\nginx\conf\vhost  # 完全自定义路径
#   .\install.ps1 install -HttpOnly      # 只接管 80，443 不动
#   .\install.ps1 rollback               # 从备份还原引擎配置，流量回到引擎直连
#   .\install.ps1 uninstall              # rollback + 删服务 + 删数据目录
#   .\install.ps1 status                 # 服务与端口状态
#
# 拓扑：gowaf 占宿主 80/443（TLS 终止）→ 引擎退守 127.0.0.1:8080（明文）
# 降级说明：IIS 没有 realip 模块，后端应用看到的客户端 IP 是 127.0.0.1；
#           WAF 在边缘看到的是真实 IP，限流/封禁不受影响。
#
# 适配器单测（非 Windows 也可）：$env:GOWAF_ADAPTER_TEST=1 后 dot-source 本文件。

[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('install', 'rollback', 'uninstall', 'doctor', 'status', 'start', 'stop')]
    [string]$Command = 'install',

    [ValidateSet('', 'iis', 'apache', 'nginx')]
    [string]$Engine = '',

    [ValidateSet('', 'phpstudy', 'bt', 'system')]
    [string]$Panel = '',

    # 非默认安装路径：把「phpstudy_pro / BtSoft / Apache24 / nginx 所在的父目录」加进来（可多次）
    [string[]]$SearchRoot = @(),

    # 完全自定义：直接指定站点配置目录（必须同时给 -Engine）。命令行仍用 -VhostDir
    [Alias('VhostDir')]
    [string]$CustomVhostDir = '',

    # 可选，配合 -VhostDir：httpd.conf / nginx.conf（改 Listen、include）
    [Alias('ConfFile')]
    [string]$CustomConfFile = '',

    [switch]$HttpOnly,
    [switch]$Force,
    [int]$UpstreamPort = 8080,
    [int]$AdminPort = 8899
)

$ErrorActionPreference = 'Stop'
$script:TestMode = ($env:GOWAF_ADAPTER_TEST -eq '1')

$script:BundleDir = $PSScriptRoot
$script:WindowsExe = Join-Path $PSScriptRoot 'gowaf-windows-amd64.exe'
if (-not $script:TestMode -and -not (Test-Path $script:WindowsExe)) {
    Write-Host "找不到 gowaf-windows-amd64.exe（部署包不完整）" -ForegroundColor Red
    exit 1
}

# ---------------------------------------------------------------- 常量

if ($env:GOWAF_HOME) {
    $GwHome = $env:GOWAF_HOME
} elseif ($env:ProgramData) {
    $GwHome = Join-Path $env:ProgramData 'gowaf'
} else {
    $GwHome = Join-Path ([System.IO.Path]::GetTempPath()) 'gowaf'
}
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
    if ($script:TestMode) { throw $msg }
    exit 1
}
function Confirm-Yes($msg) {
    if ($Force) { return $true }
    $ans = Read-Host "$msg [y/N]"
    return ($ans -match '^[yY]')
}

function Assert-Admin {
    if ($script:TestMode) { return }
    try {
        $id = [Security.Principal.WindowsIdentity]::GetCurrent()
        $p = New-Object Security.Principal.WindowsPrincipal($id)
        if (-not $p.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
            Die "请用管理员身份运行 PowerShell（右键 → 以管理员身份运行）"
        }
    } catch {
        Die "请用管理员身份运行 PowerShell（右键 → 以管理员身份运行）"
    }
}

# ---------------------------------------------------------------- 路径 / 候选引擎

$script:Engine = ''       # iis / apache / nginx
$script:PanelKind = ''    # phpstudy / bt / system / iis
$script:EngineLabel = ''
$script:VhostDir = ''     # 站点配置目录
$script:ConfFile = ''     # 主配置 httpd.conf / nginx.conf（Listen 改写）
$script:CertDir = ''
$script:Appcmd = if ($env:SystemRoot) { Join-Path $env:SystemRoot 'System32\inetsrv\appcmd.exe' } else { 'appcmd.exe' }
$script:SearchRoots = @()

function Join-Under {
    param([string]$Root)
    $p = $Root
    foreach ($x in $args) { $p = Join-Path $p $x }
    return $p
}

function Add-SearchRoot([string]$p) {
    if (-not $p) { return }
    try { $p = [System.IO.Path]::GetFullPath($p) } catch { return }
    if (-not (Test-Path $p)) { return }
    foreach ($e in $script:SearchRoots) {
        if ([string]::Equals($e, $p, [StringComparison]::OrdinalIgnoreCase)) { return }
    }
    $script:SearchRoots += $p
}

function Init-SearchRoots {
    $script:SearchRoots = @()
    foreach ($p in @($SearchRoot)) { Add-SearchRoot $p }
    if ($env:GOWAF_SEARCH_ROOTS) {
        foreach ($p in ($env:GOWAF_SEARCH_ROOTS -split ';' | ForEach-Object { $_.Trim() })) {
            Add-SearchRoot $p
        }
    }
    # 用户没指定时扫所有盘符 + Program Files，避免只认 C/D/E
    if ($script:SearchRoots.Count -eq 0) {
        try {
            Get-PSDrive -PSProvider FileSystem -ErrorAction SilentlyContinue | ForEach-Object {
                if ($_.Root -match '^[A-Za-z]:\\$') { Add-SearchRoot $_.Root }
            }
        } catch { }
        if ($script:SearchRoots.Count -eq 0) {
            @('C:\', 'D:\', 'E:\', 'F:\', 'G:\') | ForEach-Object { Add-SearchRoot $_ }
        }
        foreach ($pf in @($env:ProgramFiles, ${env:ProgramFiles(x86)}, $env:ProgramW6432)) {
            Add-SearchRoot $pf
        }
    }
}

function Get-ChildDirs($root) {
    if (-not $root -or -not (Test-Path $root)) { return @() }
    return @(Get-ChildItem $root -Directory -ErrorAction SilentlyContinue)
}

function Infer-PanelKind([string]$path) {
    $n = ($path -replace '\\', '/')
    if ($n -match '/phpstudy_pro(/|$)') { return 'phpstudy' }
    if ($n -match '/BtSoft(/|$)' -or $n -match '/btsoft(/|$)') { return 'bt' }
    return 'system'
}

function Test-HasConfFiles($dir) {
    if (-not $dir -or -not (Test-Path $dir)) { return $false }
    return [bool](Get-ChildItem $dir -File -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -match '\.conf$|^httpd-vhosts' })
}

function Test-IsPanelTree($path) {
    if (-not $path) { return $false }
    $n = $path -replace '\\', '/'
    return ($n -match '/phpstudy_pro(/|$)' -or $n -match '/BtSoft(/|$)' -or $n -match '/btsoft(/|$)')
}

function New-Candidate {
    param($PanelKind, $EngineName, $Label, $VhostDir, $ConfFile, $CertDir)
    [pscustomobject]@{
        Panel    = $PanelKind
        Engine   = $EngineName
        Label    = $Label
        VhostDir = $VhostDir
        ConfFile = $ConfFile
        CertDir  = $CertDir
    }
}

function Get-PhpstudyHomeDirs {
    $homes = @()
    foreach ($root in $script:SearchRoots) {
        $homes += @(Get-ChildItem $root -Directory -Filter 'phpstudy_pro' -ErrorAction SilentlyContinue)
        foreach ($child in (Get-ChildDirs $root)) {
            $homes += @(Get-ChildItem $child.FullName -Directory -Filter 'phpstudy_pro' -ErrorAction SilentlyContinue)
        }
    }
    return @($homes | Where-Object { $_ } | Sort-Object FullName -Unique)
}

function Get-PhpstudyCandidates {
    $found = @()
    foreach ($pro in (Get-PhpstudyHomeDirs)) {
            $ext = Join-Path $pro.FullName 'Extensions'
            if (-not (Test-Path $ext)) { continue }
            foreach ($d in (Get-ChildItem $ext -Directory -ErrorAction SilentlyContinue)) {
                $vh = Join-Under $d.FullName 'conf' 'vhosts'
                if (-not (Test-HasConfFiles $vh)) { continue }
                $engineName = $null
                if ($d.Name -like 'Apache*') { $engineName = 'apache' }
                elseif ($d.Name -like 'Nginx*') { $engineName = 'nginx' }
                else { continue }
                $conf = $null
                if ($engineName -eq 'apache') {
                    $c = Join-Under $d.FullName 'conf' 'httpd.conf'
                    if (Test-Path $c) { $conf = $c }
                } else {
                    $c = Join-Under $d.FullName 'conf' 'nginx.conf'
                    if (Test-Path $c) { $conf = $c }
                }
                $cert = $null
                $ssl = Join-Under $d.FullName 'conf' 'ssl'
                if (Test-Path $ssl) { $cert = $ssl }
                $found += ,(New-Candidate 'phpstudy' $engineName "小皮 $($d.Name)" $vh $conf $cert)
            }
    }
    return $found
}

function Get-BtSoftHomeDirs {
    $homes = @()
    foreach ($root in $script:SearchRoots) {
        foreach ($name in @('BtSoft', 'btsoft')) {
            $p = Join-Path $root $name
            if (Test-Path $p) { $homes += $p }
            foreach ($child in (Get-ChildDirs $root)) {
                $n = Join-Path $child.FullName $name
                if (Test-Path $n) { $homes += $n }
            }
        }
    }
    return @($homes | Sort-Object -Unique)
}

function Get-BtSoftCandidates {
    $found = @()
    foreach ($bt in (Get-BtSoftHomeDirs)) {
            $nginxVh = Join-Under $bt 'nginx' 'conf' 'vhost'
            $nginxConf = Join-Under $bt 'nginx' 'conf' 'nginx.conf'
            $apacheVh = Join-Under $bt 'apache' 'conf' 'vhost'
            $apacheConf = Join-Under $bt 'apache' 'conf' 'httpd.conf'
            $cert = Join-Under $bt 'panel' 'vhost' 'ssl'
            if (-not (Test-Path $cert)) {
                $alt = Join-Under $bt 'nginx' 'conf' 'vhost' 'cert'
                if (Test-Path $alt) { $cert = $alt } else { $cert = $null }
            }
            if ((Test-HasConfFiles $nginxVh) -or (Test-Path $nginxConf)) {
                $dir = if (Test-HasConfFiles $nginxVh) { $nginxVh } else { Split-Path $nginxConf }
                $found += ,(New-Candidate 'bt' 'nginx' "宝塔 Nginx（$bt）" $dir $nginxConf $cert)
            }
            if ((Test-HasConfFiles $apacheVh) -or (Test-Path $apacheConf)) {
                $dir = if (Test-HasConfFiles $apacheVh) { $apacheVh } else { Split-Path $apacheConf }
                $found += ,(New-Candidate 'bt' 'apache' "宝塔 Apache（$bt）" $dir $apacheConf $cert)
            }
    }
    return $found
}

function Get-NativeApacheRoots {
    $roots = @()
    foreach ($root in $script:SearchRoots) {
        foreach ($name in @('Apache24', 'Apache2.4', 'Apache')) {
            $p = Join-Path $root $name
            if (Test-Path (Join-Under $p 'conf' 'httpd.conf')) { $roots += $p }
        }
        $xampp = Join-Under $root 'xampp' 'apache'
        if (Test-Path (Join-Under $xampp 'conf' 'httpd.conf')) { $roots += $xampp }
        Get-ChildItem $root -Directory -ErrorAction SilentlyContinue | ForEach-Object {
            $x2 = Join-Under $_.FullName 'xampp' 'apache'
            if (Test-Path (Join-Under $x2 'conf' 'httpd.conf')) { $roots += $x2 }
            foreach ($name in @('Apache24', 'Apache2.4')) {
                $p = Join-Path $_.FullName $name
                if (Test-Path (Join-Under $p 'conf' 'httpd.conf')) { $roots += $p }
            }
            if ($_.Name -like 'wamp*') {
                $binApache = Join-Under $_.FullName 'bin' 'apache'
                if (Test-Path $binApache) {
                    Get-ChildItem $binApache -Directory -ErrorAction SilentlyContinue | ForEach-Object {
                        if (Test-Path (Join-Under $_.FullName 'conf' 'httpd.conf')) { $roots += $_.FullName }
                    }
                }
            }
        }
    }
    return @($roots | Where-Object { $_ -and -not (Test-IsPanelTree $_) } | Select-Object -Unique)
}

function Get-NativeNginxRoots {
    $roots = @()
    foreach ($root in $script:SearchRoots) {
        $candidates = @()
        foreach ($name in @('nginx')) {
            $candidates += (Join-Path $root $name)
        }
        $candidates += @(Get-ChildItem $root -Directory -Filter 'nginx-*' -ErrorAction SilentlyContinue | ForEach-Object { $_.FullName })
        foreach ($child in (Get-ChildDirs $root)) {
            $candidates += (Join-Path $child.FullName 'nginx')
            $candidates += @(Get-ChildItem $child.FullName -Directory -Filter 'nginx-*' -ErrorAction SilentlyContinue | ForEach-Object { $_.FullName })
        }
        foreach ($p in $candidates) {
            if ($p -and (Test-Path (Join-Under $p 'conf' 'nginx.conf'))) { $roots += $p }
        }
    }
    return @($roots | Where-Object { $_ -and -not (Test-IsPanelTree $_) } | Select-Object -Unique)
}

function Get-NativeCandidates {
    $found = @()
    foreach ($p in (Get-NativeApacheRoots)) {
        $conf = Join-Under $p 'conf' 'httpd.conf'
        $extra = Join-Under $p 'conf' 'extra'
        $dir = if (Test-HasConfFiles $extra) { $extra } else { Join-Path $p 'conf' }
        $found += ,(New-Candidate 'system' 'apache' "原生 Apache（$p）" $dir $conf $null)
    }
    foreach ($p in (Get-NativeNginxRoots)) {
        $conf = Join-Under $p 'conf' 'nginx.conf'
        $confd = Join-Under $p 'conf' 'conf.d'
        $sites = Join-Under $p 'conf' 'sites-enabled'
        $dir = if (Test-HasConfFiles $confd) { $confd } elseif (Test-HasConfFiles $sites) { $sites } else { Join-Path $p 'conf' }
        $found += ,(New-Candidate 'system' 'nginx' "原生 Nginx（$p）" $dir $conf $null)
    }
    return $found
}

function Test-IisPresent {
    if ($script:TestMode) { return $false }
    if (-not $env:SystemRoot) { return $false }
    return ((Get-Service W3SVC -ErrorAction SilentlyContinue) -and (Test-Path $script:Appcmd))
}

function Get-Listen80Engine {
    if ($script:TestMode) { return '' }
    try {
        $conn = Get-NetTCPConnection -LocalPort 80 -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
        if (-not $conn) { return '' }
        $procName = (Get-Process -Id $conn.OwningProcess -ErrorAction SilentlyContinue).ProcessName
        switch -Regex ($procName) {
            '^(System|w3wp|svchost)$' { if (Test-IisPresent) { return 'iis' } }
            '^(httpd|apache)$'        { return 'apache' }
            '^nginx$'                 { return 'nginx' }
            '^gowaf'                  { return 'gowaf' }
        }
    } catch { }
    return ''
}

function Find-NearbyConf([string]$start, [string]$engineName) {
    $dir = $start
    for ($i = 0; $i -lt 6 -and $dir; $i++) {
        if ($engineName -eq 'apache') {
            $httpd = Join-Under $dir 'conf' 'httpd.conf'
            if (Test-Path $httpd) { return $httpd }
            $httpd = Join-Path $dir 'httpd.conf'
            if (Test-Path $httpd) { return $httpd }
        } else {
            $ngx = Join-Under $dir 'conf' 'nginx.conf'
            if (Test-Path $ngx) { return $ngx }
            $ngx = Join-Path $dir 'nginx.conf'
            if (Test-Path $ngx) { return $ngx }
        }
        $parent = Split-Path $dir -Parent
        if (-not $parent -or $parent -eq $dir) { break }
        $dir = $parent
    }
    return $null
}

function Get-CandidateFromInstallDir([string]$dir, [string]$hintEngine) {
    if (-not $dir -or -not (Test-Path $dir)) { return $null }
    $engineName = $hintEngine
    $panelKind = Infer-PanelKind $dir
    $vhost = $null
    $conf = $null
    $walk = $dir
    for ($i = 0; $i -lt 6 -and $walk; $i++) {
        foreach ($rel in @(
            (@('conf', 'vhosts') -join [IO.Path]::DirectorySeparatorChar),
            (@('conf', 'vhost') -join [IO.Path]::DirectorySeparatorChar),
            (@('conf', 'extra') -join [IO.Path]::DirectorySeparatorChar),
            (@('conf', 'conf.d') -join [IO.Path]::DirectorySeparatorChar),
            (@('conf', 'sites-enabled') -join [IO.Path]::DirectorySeparatorChar)
        )) {
            $p = Join-Path $walk $rel
            if (Test-HasConfFiles $p) {
                $vhost = $p
                if (-not $engineName) {
                    if ($rel -match 'extra') { $engineName = 'apache' }
                    elseif ($rel -match 'vhosts' -and (Split-Path $walk -Leaf) -like 'Apache*') { $engineName = 'apache' }
                    elseif ($rel -match 'vhosts' -and (Split-Path $walk -Leaf) -like 'Nginx*') { $engineName = 'nginx' }
                    elseif ($rel -match 'vhost' -and $walk -match 'apache') { $engineName = 'apache' }
                    elseif ($rel -match 'vhost' -and $walk -match 'nginx') { $engineName = 'nginx' }
                    elseif ($rel -match 'conf.d|sites-enabled') { $engineName = 'nginx' }
                }
                break
            }
        }
        if ($vhost) { break }
        $parent = Split-Path $walk -Parent
        if (-not $parent -or $parent -eq $walk) { break }
        $walk = $parent
    }
    if (-not $engineName) {
        if (Find-NearbyConf $dir 'apache') { $engineName = 'apache' }
        elseif (Find-NearbyConf $dir 'nginx') { $engineName = 'nginx' }
    }
    if (-not $engineName) { return $null }
    $conf = Find-NearbyConf $(if ($vhost) { $vhost } else { $dir }) $engineName
    if (-not $vhost) {
        if ($conf) { $vhost = Split-Path $conf }
        else { return $null }
    }
    $label = switch ($panelKind) {
        'phpstudy' { "小皮 $engineName（自定义路径）" }
        'bt'       { "宝塔 $engineName（自定义路径）" }
        default    { "原生 $engineName（$dir）" }
    }
    return New-Candidate $panelKind $engineName $label $vhost $conf $null
}

function Get-ExePathFromCommandLine([string]$cmd) {
    if (-not $cmd) { return $null }
    if ($cmd -match '^"([^"]+\.exe)"') { return $Matches[1] }
    if ($cmd -match '^(\S+\.exe)') { return $Matches[1] }
    return $null
}

function Get-RunningEngineCandidates {
    if ($script:TestMode) { return @() }
    $found = @()
    $exes = @()
    foreach ($name in @('nginx', 'httpd', 'apache')) {
        try {
            Get-Process -Name $name -ErrorAction SilentlyContinue | ForEach-Object {
                if ($_.Path) { $exes += $_.Path }
            }
        } catch { }
    }
    try {
        Get-CimInstance Win32_Service -ErrorAction SilentlyContinue | Where-Object {
            $_.PathName -match 'httpd\.exe|nginx\.exe|apache\.exe'
        } | ForEach-Object {
            $p = Get-ExePathFromCommandLine $_.PathName
            if ($p) { $exes += $p }
        }
    } catch { }
    foreach ($exe in ($exes | Select-Object -Unique)) {
        $hint = if ($exe -match 'nginx') { 'nginx' } else { 'apache' }
        $c = Get-CandidateFromInstallDir (Split-Path $exe) $hint
        if ($c) { $found += ,$c }
    }
    return $found
}

function Get-EngineCandidates {
    Init-SearchRoots
    $all = @()
    $all += Get-PhpstudyCandidates
    $all += Get-BtSoftCandidates
    $all += Get-NativeCandidates
    $all += Get-RunningEngineCandidates
    $uniq = @()
    $seen = @{}
    foreach ($c in $all) {
        if (-not $c) { continue }
        $key = "{0}|{1}|{2}" -f $c.Panel, $c.Engine, $c.VhostDir
        if ($seen.ContainsKey($key)) { continue }
        $seen[$key] = $true
        $uniq += ,$c
    }
    return $uniq
}

function Apply-Candidate($c) {
    $script:Engine = $c.Engine
    $script:PanelKind = $c.Panel
    $script:EngineLabel = $c.Label
    $script:VhostDir = $c.VhostDir
    $script:ConfFile = $c.ConfFile
    $script:CertDir = $c.CertDir
}

function Detect-Engine {
    Write-Step '探测引擎'
    Init-SearchRoots

    if ($CustomVhostDir) {
        if ($Engine -eq 'iis') { Die "-VhostDir 不能和 -Engine iis 一起用" }
        if ($Engine -ne 'apache' -and $Engine -ne 'nginx') {
            Die "指定了 -VhostDir 时必须同时给 -Engine apache 或 -Engine nginx"
        }
        if (-not (Test-Path $CustomVhostDir)) { Die "-VhostDir 不存在：$CustomVhostDir" }
        $conf = $CustomConfFile
        if ($conf -and -not (Test-Path $conf)) { Die "-ConfFile 不存在：$conf" }
        if (-not $conf) { $conf = Find-NearbyConf $CustomVhostDir $Engine }
        $panelKind = if ($Panel) { $Panel } else { Infer-PanelKind $CustomVhostDir }
        $label = "自定义 $Engine（$CustomVhostDir）"
        Apply-Candidate (New-Candidate $panelKind $Engine $label $CustomVhostDir $conf $null)
        Write-Ok "引擎：$script:EngineLabel"
        if ($script:ConfFile) { Write-Ok "主配置：$script:ConfFile" }
        return
    }

    if ($Engine -eq 'iis') {
        if (-not (Test-IisPresent)) { Die "指定了 iis，但机器上没有 IIS（W3SVC 或 appcmd 缺失）" }
        $script:Engine = 'iis'
        $script:PanelKind = 'iis'
        $script:EngineLabel = 'IIS'
        Write-Ok "引擎：$script:EngineLabel"
        return
    }

    $cands = @(Get-EngineCandidates)
    if ($Panel) { $cands = @($cands | Where-Object { $_.Panel -eq $Panel }) }
    if ($Engine) { $cands = @($cands | Where-Object { $_.Engine -eq $Engine }) }

    $holder = Get-Listen80Engine
    if ($holder -eq 'iis' -and -not $Engine -and -not $Panel -and (Test-IisPresent)) {
        $script:Engine = 'iis'
        $script:PanelKind = 'iis'
        $script:EngineLabel = 'IIS'
        Write-Ok "引擎：IIS（正在监听 80）"
        return
    }
    if ($holder -in @('apache', 'nginx') -and -not $Engine) {
        $byPort = @($cands | Where-Object { $_.Engine -eq $holder })
        if ($byPort.Count -gt 0) { $cands = $byPort }
    }

    if ($cands.Count -eq 0) {
        if (-not $Engine -and -not $Panel -and (Test-IisPresent)) {
            $script:Engine = 'iis'
            $script:PanelKind = 'iis'
            $script:EngineLabel = 'IIS'
            Write-Ok "引擎：IIS"
            return
        }
        Die @"
没探测到可接管的站点。默认会扫所有盘符下的：
    小皮   phpstudy_pro\Extensions\Apache*|Nginx*\conf\vhosts（含下一级子目录）
    宝塔   BtSoft\{nginx,apache}\conf\vhost
    原生   Apache24\conf  与  nginx\conf
    IIS    W3SVC + appcmd
    运行中的 httpd.exe / nginx.exe（沿可执行文件往上找 conf）
非默认路径请加：
    -SearchRoot D:\apps
    -Engine nginx -VhostDir D:\stack\nginx\conf\vhost [-ConfFile D:\stack\nginx\conf\nginx.conf]
"@
    }

    if ($cands.Count -gt 1) {
        $engines = @($cands.Engine | Select-Object -Unique)
        $panels = @($cands.Panel | Select-Object -Unique)
        if ($engines.Count -gt 1) {
            Die "同时检测到 $($cands.Label -join '、')。请加 -Engine apache 或 -Engine nginx"
        }
        if ($panels.Count -gt 1) {
            Die "同时检测到 $($cands.Label -join '、')。请加 -Panel phpstudy 或 -Panel bt 或 -Panel system"
        }
    }

    Apply-Candidate @($cands)[0]
    Write-Ok "引擎：$script:EngineLabel"
    if ($script:VhostDir) { Write-Ok "站点目录：$script:VhostDir" }
    if ($script:ConfFile) { Write-Ok "主配置：$script:ConfFile" }
}

function Get-ConfFilesToPatch {
    $files = @()
    if ($script:VhostDir -and (Test-Path $script:VhostDir)) {
        $files += @(Get-ChildItem $script:VhostDir -File -ErrorAction SilentlyContinue |
            Where-Object { $_.Name -notmatch '\.bak$|\.disabled$|~$|\.dpkg-|\.ucf-' -and $_.Name -notlike 'README*' })
    }
    if ($script:ConfFile -and (Test-Path $script:ConfFile)) {
        $already = $false
        foreach ($f in $files) {
            if ($f.FullName -eq $script:ConfFile) { $already = $true; break }
        }
        if (-not $already) {
            $files += ,(Get-Item $script:ConfFile)
        }
    }
    return $files
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
        Copy-Item (Join-Path $script:VhostDir '*') (Join-Path $bk 'vhosts') -Force
    }
    if ($script:ConfFile -and (Test-Path $script:ConfFile)) {
        Copy-Item $script:ConfFile (Join-Path $bk 'main.conf') -Force
    }
    @{
        Engine       = $script:Engine
        Panel        = $script:PanelKind
        VhostDir     = $script:VhostDir
        ConfFile     = $script:ConfFile
        UpstreamPort = $UpstreamPort
        Take443      = [int](-not $HttpOnly)
        Time         = $ts
    } | ConvertTo-Json | Set-Content (Join-Path $bk 'meta.json') -Encoding utf8

    $latest = Join-Path $GwBackup 'latest'
    try {
        New-Item -ItemType SymbolicLink -Path $latest -Target $bk -Force | Out-Null
    } catch {
        if (Test-Path $latest) { Remove-Item $latest -Force -ErrorAction SilentlyContinue }
        New-Item -ItemType Directory -Path $latest -Force | Out-Null
        Set-Content (Join-Path $latest 'TARGET') $bk
    }
    Get-ChildItem $GwBackup -Directory | Where-Object { $_.Name -match '^\d{8}-' } |
        Sort-Object Name -Descending | Select-Object -Skip 10 | Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
    Write-Ok "引擎配置已备份到 $bk"
    return $bk
}

function Get-LatestBackup {
    $link = Join-Path $GwBackup 'latest'
    if (-not (Test-Path $link)) { return $null }
    $item = Get-Item $link
    if ($item.Target) { return $item.Target }
    $marker = Join-Path $link 'TARGET'
    if (Test-Path $marker) { return (Get-Content $marker -Raw).Trim() }
    return $null
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
    }
    $mainBk = Join-Path $bk 'main.conf'
    if ($script:ConfFile -and (Test-Path $mainBk)) {
        Copy-Item $mainBk $script:ConfFile -Force
    }
    Restart-Engine
    Write-Ok "引擎配置已还原"
}

# ---------------------------------------------------------------- Apache / Nginx 改写

function Find-EngineExe {
    $exeName = if ($script:Engine -eq 'apache') { 'httpd.exe' } else { 'nginx.exe' }
    $starts = @()
    if ($script:VhostDir) { $starts += $script:VhostDir }
    if ($script:ConfFile) { $starts += (Split-Path $script:ConfFile) }
    foreach ($start in $starts) {
        $dir = $start
        for ($i = 0; $i -lt 6 -and $dir; $i++) {
            $direct = Join-Path $dir $exeName
            if (Test-Path $direct) { return $direct }
            $inBin = Join-Under $dir 'bin' $exeName
            if (Test-Path $inBin) { return $inBin }
            $dir = Split-Path $dir -Parent
        }
    }
    return $null
}

function Restart-Engine {
    if ($script:TestMode) { return }
    switch ($script:Engine) {
        'apache' {
            $httpd = Find-EngineExe
            $svcName = $null
            try {
                $svcName = (Get-Service -ErrorAction SilentlyContinue |
                    Where-Object { $_.Name -match 'Apache' -and $_.Status -eq 'Running' } |
                    Select-Object -First 1).Name
            } catch { }
            if ($svcName) { Restart-Service $svcName -Force; return }
            if ($httpd) {
                $binDir = Split-Path $httpd
                & $httpd -k restart 2>$null
                if ($LASTEXITCODE -ne 0) {
                    Get-Process httpd -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
                    Start-Sleep 1
                    Start-Process $httpd -WorkingDirectory $binDir -WindowStyle Hidden
                }
            }
        }
        'nginx' {
            $ng = Find-EngineExe
            if ($ng) {
                Get-Process nginx -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
                Start-Sleep 1
                Start-Process $ng -WorkingDirectory (Split-Path $ng) -WindowStyle Hidden
            }
        }
    }
}

function Disable-SslVhostsInText([string]$c) {
    $nl = if ($c -match "`r`n") { "`r`n" } else { "`n" }
    $lines = $c -split '\r?\n'
    $out = New-Object System.Collections.Generic.List[string]
    $in443 = $false
    foreach ($l in $lines) {
        if (-not $in443 -and $l -match '^\s*<VirtualHost[^>]*:443[^>]*>' -and $l -notmatch '\[GOWAF\]') {
            $in443 = $true
        }
        if ($in443) {
            if ($l -match '^\s*# \[GOWAF\]') { $out.Add($l) } else { $out.Add("# [GOWAF] $l") }
            if ($l -match '</VirtualHost>') { $in443 = $false }
        } else {
            $out.Add($l)
        }
    }
    return ($out -join $nl)
}

function Repair-ApacheHttpsLoop([string]$c) {
    if ($c -match 'X-Forwarded-Proto') { return $c }
    $nl = if ($c -match "`r`n") { "`r`n" } else { "`n" }
    $lines = $c -split '\r?\n'
    $out = New-Object System.Collections.Generic.List[string]
    foreach ($l in $lines) {
        if ($l -match 'RewriteCond\s+%\{SERVER_PORT\}\s+!\^443\$') {
            $indent = ''
            if ($l -match '^(\s*)') { $indent = $Matches[1] }
            $out.Add("${indent}RewriteCond %{HTTP:X-Forwarded-Proto} !^https$")
        }
        $out.Add($l)
    }
    return ($out -join $nl)
}

function Patch-ApacheListen {
    if (-not $script:ConfFile -or -not (Test-Path $script:ConfFile)) { return }
    if ($script:Engine -ne 'apache') { return }
    $c = Get-Content $script:ConfFile -Raw -Encoding utf8
    if ($null -eq $c) { $c = '' }
    $orig = $c
    $c = [regex]::Replace($c, '(?m)^(\s*)Listen\s+(?:\[::\]:)?(?:0\.0\.0\.0:)?80\s*$', '# [GOWAF] Listen 80')
    if (-not $HttpOnly) {
        $c = [regex]::Replace($c, '(?m)^(\s*)Listen\s+(?:\[::\]:)?(?:0\.0\.0\.0:)?443\s*$', '# [GOWAF] Listen 443')
    }
    if ($c -notmatch "Listen 127\.0\.0\.1:$UpstreamPort") {
        if ($c -notmatch 'GOWAF-MANAGED') {
            $c = $c.TrimEnd() + "`r`n`r`n# ===== GOWAF-MANAGED BEGIN =====`r`nListen 127.0.0.1:$UpstreamPort`r`n# ===== GOWAF-MANAGED END =====`r`n"
        }
    }
    if ($c -ne $orig) {
        Set-Content $script:ConfFile $c -Encoding utf8 -NoNewline
        Write-Ok "已改写 Listen $(Split-Path $script:ConfFile -Leaf)"
    }
}

function Patch-ApacheVhosts {
    $takeLabel = if (-not $HttpOnly) { '/443' } else { '' }
    Write-Step "改写 Apache vhost（让出 80$takeLabel，退到 127.0.0.1:$UpstreamPort）"
    Patch-ApacheListen
    foreach ($f in (Get-ConfFilesToPatch)) {
        if ($script:ConfFile -and $f.FullName -eq $script:ConfFile -and (Get-Content $f.FullName -Raw -ErrorAction SilentlyContinue) -notmatch '<VirtualHost') {
            continue
        }
        $c = Get-Content $f.FullName -Raw -Encoding utf8
        if ($null -eq $c) { continue }
        $orig = $c
        $c = [regex]::Replace($c, '(?m)^(\s*)<VirtualHost\s+\*:80\s*>', "`$1<VirtualHost 127.0.0.1:$UpstreamPort>")
        $c = [regex]::Replace($c, '(?m)^(\s*)<VirtualHost\s+\*:8080\s*>', "`$1<VirtualHost 127.0.0.1:$UpstreamPort>")
        $c = Repair-ApacheHttpsLoop $c
        if (-not $HttpOnly) {
            $c = Disable-SslVhostsInText $c
        }
        if ($c -ne $orig) {
            Set-Content $f.FullName $c -Encoding utf8 -NoNewline
            Write-Ok "已改写 $($f.Name)"
        }
    }
}

function Test-NginxListen80([string]$line) {
    return [bool]($line -match '^\s*listen\s+(?:(?:\*|0\.0\.0\.0):)?80(?:\s+[^;]*)?;')
}

function Patch-NginxVhosts {
    $takeLabel = if (-not $HttpOnly) { '/443' } else { '' }
    Write-Step "改写 Nginx vhost（让出 80$takeLabel，退到 127.0.0.1:$UpstreamPort）"
    foreach ($f in (Get-ConfFilesToPatch)) {
        $lines = @(Get-Content $f.FullName -Encoding utf8)
        $has80 = $false
        foreach ($l in $lines) {
            if (Test-NginxListen80 $l) { $has80 = $true; break }
        }
        $out = New-Object System.Collections.Generic.List[string]
        foreach ($l in $lines) {
            $n = $l
            if ($l -match '^\s*#' -and $l -match '\[GOWAF\]') {
                $out.Add($n)
                continue
            }
            if ($l -match '^\s*listen\s+\[::\]:80\b') {
                $n = "# [GOWAF] $($l.TrimStart())"
            }
            elseif (Test-NginxListen80 $l) {
                $indent = ''
                if ($l -match '^(\s*)') { $indent = $Matches[1] }
                $n = "${indent}listen 127.0.0.1:$UpstreamPort;"
            }
            elseif ((-not $HttpOnly) -and $l -match '^\s*listen\s+(?:(?:\*|0\.0\.0\.0|\[::\]):)?443\b') {
                if ($has80) { $n = "# [GOWAF] $($l.TrimStart())" }
                else {
                    $indent = ''
                    if ($l -match '^(\s*)') { $indent = $Matches[1] }
                    $n = "${indent}listen 127.0.0.1:$UpstreamPort;"
                }
            }
            elseif ($l -match 'if\s*\(\s*\$server_port\s*!~\s*\^?443\$?\s*\)') {
                $n = $l -replace 'if\s*\(\s*\$server_port\s*!~\s*\^?443\$?\s*\)', 'if ($http_x_forwarded_proto != "https")'
            }
            $out.Add($n)
        }
        Set-Content $f.FullName $out -Encoding utf8
        Write-Ok "已改写 $($f.Name)"
    }
}

# ---------------------------------------------------------------- IIS 适配

function Get-IisSites {
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
        return $names
    }
    $pat = if ($script:Engine -eq 'apache') { '^\s*Server(Name|Alias)\s+(.+)$' } else { '^\s*server_name\s+(.+);' }
    foreach ($f in (Get-ConfFilesToPatch)) {
        $hits = Select-String -Path $f.FullName -Pattern $pat -ErrorAction SilentlyContinue
        foreach ($h in $hits) {
            foreach ($tok in ($h.Matches[0].Groups[2].Value -split '\s+')) {
                $tok = $tok.Trim().TrimEnd(';').ToLower()
                if ($tok -and $tok -ne '*' -and $tok -ne '_' -and $tok -ne 'localhost' -and $names -notcontains $tok) {
                    $names += $tok
                }
            }
        }
    }
    return $names
}

function Render-WindowsConfig($mode) {
    Write-Step "生成配置（engine.mode=$mode）"

    $take443 = -not $HttpOnly
    $siteNames = @(Get-SiteHostnames)
    $certDir = if ($script:CertDir) { $script:CertDir } else { Join-Path $GwData 'certs' }

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
        [void]$sb.AppendLine("  cert_dir: `"$certDir`"")
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

    $svc = Get-Service gowaf -ErrorAction SilentlyContinue
    if ($svc -and $svc.Status -eq 'Running') {
        Move-Item $GwBin "$GwBin.prev" -Force -ErrorAction SilentlyContinue
    }
    Copy-Item $script:WindowsExe $GwBin -Force
    Copy-Item (Join-Path $PSScriptRoot 'builtin.yaml') $GwRules -Force
    Write-Ok "二进制与规则就位：$GwHome"

    $mode = 'observe'
    if (Test-Path $GwConf) {
        $oldMode = (Select-String -Path $GwConf -Pattern '^\s*mode:\s*"?(\w+)"?' | Select-Object -First 1).Matches[0].Groups[1].Value
        if ($oldMode -eq 'protect') { $mode = 'protect' }
        $script:KeepMode = $mode
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
        try {
            $c = Get-NetTCPConnection -LocalPort $p -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
            if ($c) {
                $proc = (Get-Process -Id $c.OwningProcess -ErrorAction SilentlyContinue).ProcessName
                Write-Host "    $p  监听中（$proc pid=$($c.OwningProcess)）"
            } else {
                Write-Host "    $p  空闲"
            }
        } catch {
            Write-Host "    $p  （本机无法探测 TCP 占用）"
        }
    }
    if ($script:Engine -eq 'iis') {
        Write-Step 'IIS 站点清单'
        foreach ($s in (Get-IisSites)) { Write-Host "    $($s[0])  bindings: $($s[2])" }
    }
    elseif ($script:VhostDir -or $script:ConfFile) {
        Write-Step '站点清单（域名声明）'
        foreach ($n in (Get-SiteHostnames)) { Write-Host "    $n" }
        if (-not (Get-SiteHostnames)) {
            foreach ($f in (Get-ConfFilesToPatch)) { Write-Host "    $($f.FullName)" }
        }
    }
    try {
        $svc = Get-Service gowaf -ErrorAction SilentlyContinue
        if ($svc) { Write-Ok "gowaf 服务：$($svc.Status)" } else { Write-Ok "gowaf 服务：未安装" }
    } catch {
        Write-Ok "gowaf 服务：未安装"
    }
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
        Start-Sleep 2
        try {
            $c = Get-NetTCPConnection -LocalPort 80 -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
            if ($c) {
                $proc = (Get-Process -Id $c.OwningProcess).ProcessName
                if ($proc -notmatch 'gowaf') {
                    Write-Warn2 "80 仍被 $proc 占用（gowaf 尚未接管）"
                }
            }
        } catch { }
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

if ($script:TestMode) { return }

switch ($Command) {
    'install'   { Cmd-Install }
    'rollback'  { Cmd-Rollback }
    'uninstall' { Cmd-Uninstall }
    'doctor'    { Cmd-Doctor }
    'status'    { Cmd-Status }
    'start'     { Assert-Admin; Start-Service gowaf; Write-Ok 'gowaf 已启动' }
    'stop'      { Assert-Admin; Stop-Service gowaf -Force; Write-Warn2 'gowaf 已停止——80/443 无人监听，站点不可访问。恢复请 rollback 或 start' }
}
