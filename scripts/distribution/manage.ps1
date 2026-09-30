[CmdletBinding()]
param([ValidateSet('Start','Stop','Upgrade','Status')][string]$Action)

$ErrorActionPreference = 'Stop'
function Test-ServiceExecutable([string]$CommandLine, [string]$ExpectedPath) {
    # 只比较完整的首个参数；无空格路径可不带引号，拒绝相同前缀的其他程序。
    $match = [regex]::Match($CommandLine, '^\s*(?:"([^"\r\n]+)"|([^\s"]+))(?=\s|$)')
    if (-not $match.Success) { return $false }
    $path = if ($match.Groups[1].Success) { $match.Groups[1].Value } else { $match.Groups[2].Value }
    return [string]::Equals($path, $ExpectedPath, [StringComparison]::OrdinalIgnoreCase)
}

function Update-Launchers([string]$SourceDirectory, [string]$TargetDirectory) {
    # 当前 PowerShell 已解析完整脚本；更新入口后，下次菜单使用新版本。
    foreach ($launcher in @('启动.cmd','manage.ps1')) {
        $inputPath = Join-Path $SourceDirectory $launcher
        $targetPath = Join-Path $TargetDirectory $launcher
        Copy-Item -LiteralPath $inputPath -Destination $targetPath -Force
        if ((Get-FileHash -LiteralPath $inputPath).Hash -ne (Get-FileHash -LiteralPath $targetPath).Hash) {
            throw '启动入口安装校验失败，未声明升级成功。'
        }
    }
}
$OutputEncoding = [Text.UTF8Encoding]::new($false)
[Console]::InputEncoding = [Text.UTF8Encoding]::new($false)
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$root = [IO.Path]::GetFullPath($PSScriptRoot)
$isAgent = Test-Path -LiteralPath (Join-Path $root 'agent-config.json')
$name = if ($isAgent) { 'OBDataOrchAgent' } else { 'OBDataOrch' }
$binary = if ($isAgent) { 'agent.exe' } else { 'control-plane.exe' }
$executable = Join-Path $root $binary
if (-not $Action) {
    Write-Host '1 启动（首次自动安装）  2 停止  3 升级  4 查看状态'
    $choice = Read-Host '选择操作，直接回车启动'
    $Action = switch ($choice) { '2' {'Stop'} '3' {'Upgrade'} '4' {'Status'} default {'Start'} }
}
$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw '请右键“启动.cmd”，选择“以管理员身份运行”；安装与服务操作需要本机管理员权限。'
}
$service = Get-Service -Name $name -ErrorAction SilentlyContinue
if ($service) {
    $registration = Get-CimInstance Win32_Service -Filter "Name='$name'"
    if (-not (Test-ServiceExecutable $registration.PathName $executable)) {
        throw '已有服务安装在其他目录。请使用原安装目录的启动入口，不能创建第二份身份。'
    }
    $account = [Security.Principal.WindowsIdentity]::GetCurrent().Name
    if (-not [string]::Equals($registration.StartName,$account,[StringComparison]::OrdinalIgnoreCase)) {
        throw '请使用原安装账户维护服务，不能更换身份保护账户。'
    }
}
function Stop-App {
    if ($service -and $service.Status -ne 'Stopped') {
        Stop-Service -Name $name
        (Get-Service $name).WaitForStatus('Stopped',[TimeSpan]::FromSeconds(60))
    }
    if (-not $isAgent) {
        $settingsPath = Join-Path $root 'data\settings.json'
        if (Test-Path -LiteralPath $settingsPath) {
            $settings = Get-Content -Raw -LiteralPath $settingsPath | ConvertFrom-Json
            $port = ([Uri]$settings.publicUrl).Port
            if (Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue) { throw '监听端口仍未关闭，已停止升级。' }
        }
    }
}
if ($Action -eq 'Status') { if ($service) { $service | Format-Table Name,Status } else { Write-Host '尚未安装，选择启动完成首次安装。' }; exit }
if ($Action -eq 'Stop') { Stop-App; Write-Host '服务已停止，配置与身份已保留。'; exit }
if ($Action -eq 'Upgrade') {
    if (-not $service) { throw '请先完成首次安装。' }
    $source = [IO.Path]::GetFullPath((Read-Host '输入新版安装包解压目录'))
    if ($source -eq $root) { throw '新版安装包须位于独立目录。' }
    $newBinary = Join-Path $source $binary
    if (-not (Test-Path -LiteralPath $newBinary -PathType Leaf)) { throw '新版安装包缺少程序。' }
    & $newBinary -version
    if ($LASTEXITCODE -ne 0) { throw '新版程序校验失败。' }
    foreach ($launcher in @('启动.cmd','manage.ps1')) {
        if (-not (Test-Path -LiteralPath (Join-Path $source $launcher) -PathType Leaf)) { throw '新版安装包缺少启动入口，未停止原服务。' }
    }
    if (-not $isAgent) {
        foreach ($required in @('web\index.html','agents\windows-amd64\agent.exe','agents\linux-amd64\agent','agents\linux-arm64\agent')) {
            if (-not (Test-Path -LiteralPath (Join-Path $source $required) -PathType Leaf)) { throw "新版安装包缺少 $required，未停止原服务。" }
        }
    }
    Stop-App
    # 精确替换当前安装目录的程序，保留全部 data、连接配置和信任材料。
    if ([IO.Path]::GetDirectoryName([IO.Path]::GetFullPath($executable)) -ne $root) { throw '程序目标越界。' }
    Remove-Item -LiteralPath $executable -Force
    Copy-Item -LiteralPath $newBinary -Destination $executable
    if ((Get-FileHash -LiteralPath $newBinary).Hash -ne (Get-FileHash -LiteralPath $executable).Hash) { throw '新版程序安装校验失败，未声明升级成功。' }
    if (-not $isAgent) {
        foreach ($directory in @('web','agents')) {
            $target = Join-Path $root $directory
            $inputDirectory = Join-Path $source $directory
            if (-not (Test-Path -LiteralPath $inputDirectory -PathType Container)) { throw "新版安装包缺少 $directory。" }
            New-Item -ItemType Directory -Path $target -Force | Out-Null
            Get-ChildItem -LiteralPath $inputDirectory | Copy-Item -Destination $target -Recurse -Force
        }
    }
    Update-Launchers $source $root
}
if (-not $service) {
    if ($isAgent) {
        # 相同入口在已有身份时直接复用，首次才提示输入注册码。
        & $executable -initialize
        if ($LASTEXITCODE -ne 0) { throw 'Agent 关联未完成，服务尚未安装；可再次启动重试。' }
    } else {
        if (-not (Test-Path -LiteralPath (Join-Path $root 'data\settings.json'))) {
            $address = Read-Host '控制面访问地址（例如 https://orch.example.internal:8080）'
            $secret = Read-Host '设置 admin 密码（至少 8 个字符）' -AsSecureString
            $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secret)
            try {
                $plain = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer)
                "$address`n$plain" | & $executable -initialize
                if ($LASTEXITCODE -ne 0) { throw '首次配置失败。' }
            } finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer); $plain = $null; $secret.Dispose() }
        }
    }
    Write-Host '服务固定使用当前 Windows 账户，以保留加密身份。只在首次安装时需要其 Windows 密码（不是 PIN）。'
    $secret = Read-Host '当前 Windows 账户密码' -AsSecureString
    $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secret)
    try {
        $plain = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer)
        $plain | & $executable -install-service
        if ($LASTEXITCODE -ne 0) { throw '服务安装失败，请检查账户与系统权限。已有身份已保留。' }
    } finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer); $plain=$null; $secret.Dispose() }
}
Start-Service -Name $name
(Get-Service $name).WaitForStatus('Running',[TimeSpan]::FromSeconds(30))
Start-Sleep -Seconds 3
if ((Get-Service $name).Status -ne 'Running') { throw '服务启动后退出，请检查 data\service.log。' }
if (-not $isAgent) {
    & $executable -health-check
    if ($LASTEXITCODE -ne 0) { throw '控制面健康检查未通过，不能视为启动成功。' }
    $caPath = Join-Path $root 'data\control-plane-ca.pem'
    Import-Certificate -FilePath $caPath -CertStoreLocation Cert:\LocalMachine\Root | Out-Null
    $settings = Get-Content -Raw -LiteralPath (Join-Path $root 'data\settings.json') | ConvertFrom-Json
    Write-Host "已启动：$($settings.publicUrl)。本机已安装服务信任；其他浏览器首次需信任 data\control-plane-ca.pem，Agent 包已自动带入。"
} else { Write-Host 'Agent 服务已启动；请在节点页面确认在线状态。正常重启与升级无需注册码。' }
