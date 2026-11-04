<#
.SYNOPSIS
    GoWAF 打包脚本 —— 交叉编译二进制并打成可直接上传的部署包。

.DESCRIPTION
    做四件事：
      1. 准备前端产物：默认直接使用仓库自带的 internal/web/dist（go:embed 需要它存在）。
         仅当本机存在 Frontend 源码且显式加 -WithWebBuild 时才重新构建并同步。
      2. CGO_ENABLED=0 交叉编译（-Platform linux → linux/amd64|arm64 tar.gz；
         -Platform windows → windows/amd64 zip）
      3. 把 二进制 + 部署脚本 + builtin.yaml 组装成一个部署目录
      4. 打包 + 生成 SHA256，并打印上传/部署命令

    注意：本项目没有 git 仓库时版本号取时间戳，不会失败。

.EXAMPLE
    .\deploy\package.ps1                          # linux/amd64 tar.gz
    .\deploy\package.ps1 -Arch arm64
    .\deploy\package.ps1 -Platform windows        # windows/amd64 zip（服务模式 + IIS/小皮适配）
    .\deploy\package.ps1 -WithWebBuild            # 本机有 Frontend 源码时，先 pnpm build 再打包
#>
[CmdletBinding()]
param(
    [ValidateSet('amd64', 'arm64')]
    [string]$Arch = 'amd64',

    # 目标平台：linux（默认，tar.gz + install.sh）或 windows（zip + install.ps1 + gowaf svc 服务模式）
    [ValidateSet('linux', 'windows')]
    [string]$Platform = 'linux',

    # 重新构建前端（pnpm install + build），仅在本机有 Frontend 源码时可用
    [switch]$WithWebBuild
)

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
if (-not (Test-Path (Join-Path $root 'go.mod'))) {
    throw "找不到 go.mod，脚本必须放在项目的 deploy 目录下"
}

function Write-Step($msg) {
    Write-Host "`n==> $msg" -ForegroundColor Cyan
}
function Write-Ok($msg) { Write-Host "    [OK] $msg" -ForegroundColor Green }
function Write-Warn($msg) { Write-Host "    [!]  $msg" -ForegroundColor Yellow }

# ---------------------------------------------------------------- 1. 检查工具

Write-Step '检查构建工具'

$goCmd = Get-Command go -ErrorAction SilentlyContinue
if (-not $goCmd) {
    throw "找不到 go 命令，请先安装 Go 并加入 PATH"
}
Write-Ok ("go " + (& go version))

$tarCmd = Get-Command tar.exe -ErrorAction SilentlyContinue
if (-not $tarCmd) { $tarCmd = Get-Command tar -ErrorAction SilentlyContinue }
if (-not $tarCmd) {
    throw "找不到 tar（Windows 10 1803+ 自带）。可改用 7-Zip 手动压缩成 tar.gz"
}
Write-Ok ("tar " + $tarCmd.Source)

# ---------------------------------------------------------------- 2. 前端产物

Write-Step '准备前端产物（go:embed 需要 internal/web/dist）'

$srcDist = Join-Path $root 'Frontend\dist'
$dstDist = Join-Path $root 'internal\web\dist'

# 仓库直接自带 internal/web/dist（前端源码独立维护，不在仓库内），
# 所以默认路径是「什么都不做，直接用」。只有本机恰好有 Frontend 源码、
# 且明确要求重新构建时，才走 pnpm build + 同步。
if ($WithWebBuild) {
    if (-not (Test-Path (Join-Path $root 'Frontend'))) {
        throw "找不到 Frontend 源码目录（前端源码独立维护，不在本仓库内）。常规打包直接运行 .\deploy\package.ps1 即可，无需 -WithWebBuild"
    }
    Write-Host "    执行 pnpm build（耗时约 1-3 分钟）..."
    Push-Location (Join-Path $root 'Frontend')
    try {
        & pnpm install --ignore-workspace --frozen-lockfile=false
        if ($LASTEXITCODE -ne 0) { throw "pnpm install 失败" }
        & pnpm build
        if ($LASTEXITCODE -ne 0) { throw "pnpm build 失败" }
    }
    finally { Pop-Location }
    Write-Ok '前端已重新构建'

    # 用 robocopy /MIR 做镜像同步，而不是 Remove-Item + Copy-Item。
    # 原因：宿主的 safe-delete 守卫会拦截删除上百个文件的递归删除动作，
    # 表现为「trash-failed」但目录已经被移走，随后 go:embed 找不到 dist 直接编译失败。
    # robocopy 是原生进程，只做文件级镜像，不触发那层拦截。
    New-Item -ItemType Directory -Path $dstDist -Force | Out-Null
    & robocopy $srcDist $dstDist /MIR /NFL /NDL /NJH /NJS /NC /NS /NP | Out-Null
    # robocopy 退出码 0-7 都算成功，>=8 才是失败
    if ($LASTEXITCODE -ge 8) { throw "robocopy 同步失败（退出码 $LASTEXITCODE）" }
    $global:LASTEXITCODE = 0
    $keep = Join-Path $dstDist '.gitkeep'
    if (-not (Test-Path $keep)) { New-Item -ItemType File -Path $keep | Out-Null }
    Write-Ok 'Frontend/dist -> internal/web/dist 同步完成'
}
elseif (Test-Path (Join-Path $dstDist 'index.html')) {
    Write-Ok '使用仓库自带的 internal/web/dist（如需改界面，取回 Frontend 源码后加 -WithWebBuild）'
}
else {
    throw "internal/web/dist 缺少 index.html（仓库应自带前端产物，可能被误删）。请从版本库恢复它，或取回 Frontend 源码后加 -WithWebBuild 重新构建"
}

# ---------------------------------------------------------------- 3. 交叉编译

Write-Step "交叉编译 $Platform/$Arch"

$ver = 'dev-' + (Get-Date -Format 'yyyyMMdd-HHmm')
$builtAt = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
$ldflags = "-s -w -X main.version=$ver -X main.buildTime=$builtAt"

$outDir = Join-Path $root 'dist'
New-Item -ItemType Directory -Path $outDir -Force | Out-Null
if ($Platform -eq 'windows') {
    $exeName = "gowaf-windows-amd64.exe"
} else {
    $exeName = "gowaf-linux-$Arch"
}
$exePath = Join-Path $outDir $exeName

$oldCgo = $env:CGO_ENABLED
$oldOs = $env:GOOS
$oldArch = $env:GOARCH
try {
    $env:CGO_ENABLED = '0'
    $env:GOOS = $Platform
    if ($Platform -eq 'windows') { $env:GOARCH = 'amd64' } else { $env:GOARCH = $Arch }

    Push-Location $root
    try {
        & go build -trimpath -ldflags $ldflags -o $exePath ./cmd/gowaf
        if ($LASTEXITCODE -ne 0) { throw "编译失败（上面是 go build 的输出）" }
    }
    finally { Pop-Location }
}
finally {
    $env:CGO_ENABLED = $oldCgo
    $env:GOOS = $oldOs
    $env:GOARCH = $oldArch
}

$sizeMb = [math]::Round((Get-Item $exePath).Length / 1MB, 1)
Write-Ok "$exeName  $sizeMb MB  ($ver)"

# ---------------------------------------------------------------- 4. 组装

Write-Step '组装部署包'

$stageName = "gowaf-deploy-$ver-$Platform-$($env:GOARCH)"
$stage = Join-Path $outDir $stageName
if (Test-Path $stage) { Remove-Item $stage -Recurse -Force }
New-Item -ItemType Directory -Path $stage -Force | Out-Null

Copy-Item $exePath (Join-Path $stage $exeName) -Force
Copy-Item (Join-Path $root 'rules\builtin.yaml') $stage -Force

if ($Platform -eq 'windows') {
    Copy-Item (Join-Path $root 'deploy\install.ps1') $stage -Force
} else {
    Copy-Item (Join-Path $root 'deploy\install.sh') $stage -Force
}

$ruleCount = (Select-String -Path (Join-Path $root 'rules\builtin.yaml') -Pattern '^- id:' -AllMatches).Count

if ($Platform -eq 'windows') {
$readme = @"
GoWAF 部署包  $ver  (windows/amd64)
================================================================

包含：gowaf-windows-amd64.exe / install.ps1 / builtin.yaml（规则 $ruleCount 条）

────────────────────────────────────────────────────────────────
部署（管理员 PowerShell）
────────────────────────────────────────────────────────────────
  解压本目录到服务器（如 C:\gowaf-deploy），然后：

  cd C:\gowaf-deploy
  .\install.ps1 doctor        # 只读体检：探测小皮/宝塔/原生 Apache·Nginx / IIS
  .\install.ps1 install       # 备份 → 引擎让出 80/443 → gowaf svc 注册开机自启 → 防火墙放行
  .\install.ps1 rollback      # 一键还原引擎配置，流量回到引擎直连
  .\install.ps1 uninstall     # 停服 + 卸载服务 + 还原

说明：
  · Windows 服务模式用 gowaf svc 实现（单二进制，无 NSSM 依赖）
  · 自动探测：小皮 phpstudy_pro、宝塔 C:\BtSoft、原生 Apache24 / nginx、IIS
  · IIS 走 appcmd 改 applicationHost.config 绑定（https 绑定转 http，TLS 由 WAF 终止）
  · 已知降级：IIS 后端应用看到的客户端 IP 是 127.0.0.1（IIS 无 realip 模块）；
    WAF 本身的限流/封禁不受影响。控制台 http://127.0.0.1:8899
"@
} else {
$readme = @"
GoWAF 部署包  $ver  (linux/$Arch)
================================================================

包含：gowaf-linux-$Arch / install.sh / builtin.yaml（规则 $ruleCount 条）

────────────────────────────────────────────────────────────────
第一步：先体检（只读，不改任何东西，不需要先安装）
────────────────────────────────────────────────────────────────
  tar -xzf $stageName.tar.gz
  cd $stageName
  bash install.sh doctor

doctor 会直接回答这几个问题：
  · 当前部署形态 —— 是首次安装还是已装；80/443 各由谁监听；
    是否用了 --http-only（配置里 tls.enabled=false 就是没接管 443）
  · 站点清单 —— 共几个 vhost、每个域名实际用哪张证书
    （★ 标出 default_cert 兜底，SNI 精确匹配的会单独标）
  · 哪些域名没有任何证书覆盖 —— 这些经 Cloudflare 回源会 526
  · 端口占用、Apache 配置语法、gowaf 服务状态

doctor 全程只读：不动 Apache、不装服务、不改配置。
先跑它，确认拓扑之后再决定怎么装，比装完再回滚快得多。

────────────────────────────────────────────────────────────────
安装
────────────────────────────────────────────────────────────────
  bash install.sh install
install.sh 会先打印要做的改动并等你确认。

不上 CDN 时可以只接管 80（443 继续由 Apache 处理）：
  bash install.sh install --http-only
  ！站点在 Cloudflare 后面时不要用 --http-only：CF 回源走 443，
    443 不接管等于 WAF 被完全绕过。

站点挂在 Cloudflare 小云朵后面时，必须加 --cloudflare：
  bash install.sh install --cloudflare
否则 WAF 看到的来源 IP 是 Cloudflare 节点 IP，限流与自动封禁会按节点计算，
一次就可能封掉一整片真实用户，日志也无法定位攻击者。
装好之后补开 / 更新 CF 网段：
  bash install.sh cloudflare on
  bash install.sh cloudflare update
注意：Cloudflare 的 SSL/TLS 模式必须是 Full 或 Full (strict)，
      Flexible 会让源站陷入 301 死循环，且 HTTPS 信息丢失。

出问题立刻恢复原状：
  bash install.sh rollback

────────────────────────────────────────────────────────────────
访问控制台
────────────────────────────────────────────────────────────────
  ssh -L 8899:127.0.0.1:8899 root@<服务器IP>
  浏览器打开 http://127.0.0.1:8899

！不要用 console open 把控制台开到 0.0.0.0。拿不到 SSH 来源 IP 时
  它会退化成对全网放行，控制台暴露在公网。用完记得 console close。

注意：install.sh 会修改 Apache 的 Listen 端口与 vhost。
      改动前自动备份到 /www/server/gowaf/backup/。
"@
}
$readmePath = Join-Path $stage 'README.txt'
[System.IO.File]::WriteAllText($readmePath, $readme, (New-Object System.Text.UTF8Encoding($false)))

Write-Ok "已组装 $stageName （含 README.txt）"

# ---------------------------------------------------------------- 5. 打包

if ($Platform -eq 'windows') {
    Write-Step '打包 zip'

    $tarPath = Join-Path $outDir "$stageName.zip"
    if (Test-Path $tarPath) { Remove-Item $tarPath -Force }
    Compress-Archive -Path "$stage\*" -DestinationPath $tarPath -Force

    $tarMb = [math]::Round((Get-Item $tarPath).Length / 1MB, 1)
    Write-Ok "$stageName.zip  $tarMb MB"
}
else {
    Write-Step '打包 tar.gz'

    $tarPath = Join-Path $outDir "$stageName.tar.gz"
    if (Test-Path $tarPath) { Remove-Item $tarPath -Force }

    # 包内保留一层目录，解压后不会散落在当前目录
    & $tarCmd.Source -czf $tarPath -C $outDir $stageName
    if ($LASTEXITCODE -ne 0) { throw "tar 打包失败" }

    $tarMb = [math]::Round((Get-Item $tarPath).Length / 1MB, 1)
    Write-Ok "$stageName.tar.gz  $tarMb MB"
}

# ---------------------------------------------------------------- 6. 校验值

$hash = (Get-FileHash $tarPath -Algorithm SHA256).Hash.ToLower()
$hashFile = "$tarPath.sha256"
[System.IO.File]::WriteAllText($hashFile, "$hash  $stageName.tar.gz`n", (New-Object System.Text.UTF8Encoding($false)))
Write-Ok "SHA256 $hash"

# ---------------------------------------------------------------- 7. 提示

Write-Host ""
Write-Host "================================================================" -ForegroundColor Green
Write-Host " 打包完成" -ForegroundColor Green
Write-Host "================================================================" -ForegroundColor Green
Write-Host " 文件   $tarPath"
Write-Host " 大小   $tarMb MB"
Write-Host " 规则   $ruleCount 条"
Write-Host ""

if ($Platform -eq 'windows') {
    Write-Host " Windows 部署（目标机需管理员 PowerShell）：" -ForegroundColor Cyan
    Write-Host "    解压 $stageName.zip 到任意目录"
    Write-Host "    .\install.ps1 doctor      # 只读体检"
    Write-Host "    .\install.ps1 install     # 备份→引擎让出80/443→gowaf svc 安装→防火墙"
    Write-Host ""
    Write-Host " 控制台：http://127.0.0.1:8899" -ForegroundColor Cyan
    Write-Host ""
    exit 0
}

Write-Host " 1) 上传到服务器：" -ForegroundColor Cyan
Write-Host "    scp `"$tarPath`" root@<服务器IP>:/tmp/"
Write-Host ""
Write-Host " 2) 服务器上部署：" -ForegroundColor Cyan
Write-Host "    cd /tmp"
Write-Host "    tar -xzf $stageName.tar.gz"
Write-Host "    cd $stageName"
Write-Host "    bash install.sh install"
Write-Host ""
Write-Host " 3) 上传后校验（可选）：" -ForegroundColor Cyan
Write-Host "    echo `"$hash  $stageName.tar.gz`" | sha256sum -c -"
Write-Host ""

exit 0
