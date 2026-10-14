[CmdletBinding()]
param([ValidateSet('Start','Stop','Upgrade','Status','Register')][string]$Action)

$ErrorActionPreference = 'Stop'
function Test-ServiceExecutable([string]$CommandLine, [string]$ExpectedPath) {
    # 只比较完整的首个参数；无空格路径可不带引号，拒绝相同前缀的其他程序。
    $match = [regex]::Match($CommandLine, '^\s*(?:"([^"\r\n]+)"|([^\s"]+))(?=\s|$)')
    if (-not $match.Success) { return $false }
    $path = if ($match.Groups[1].Success) { $match.Groups[1].Value } else { $match.Groups[2].Value }
    return [string]::Equals($path, $ExpectedPath, [StringComparison]::OrdinalIgnoreCase)
}

function Resolve-ServiceAccountSID([string]$AccountName) {
    # 服务使用 .\ 记录本机账户，先补全机器名，再由系统解析实际身份。
    if ($AccountName.StartsWith('.\', [StringComparison]::Ordinal)) {
        if ([string]::IsNullOrWhiteSpace($env:COMPUTERNAME)) { throw '无法解析本机服务账户。' }
        $AccountName = $env:COMPUTERNAME + $AccountName.Substring(1)
    }
    return ([Security.Principal.NTAccount]::new($AccountName)).Translate([Security.Principal.SecurityIdentifier])
}

function Test-ServiceAccount([string]$AccountName, [Security.Principal.SecurityIdentifier]$ExpectedSID) {
    # 名称格式、大小写或账户别名不代表身份变化；仅相同 SID 可维护已有身份。
    if ([string]::IsNullOrWhiteSpace($AccountName) -or $null -eq $ExpectedSID) { return $false }
    try {
        $accountSID = Resolve-ServiceAccountSID $AccountName
        return $null -ne $accountSID -and $accountSID.Equals($ExpectedSID)
    } catch {
        # 无法解析账户时保持拒绝，不回退到名称或仅用户名比较。
        return $false
    }
}

function Initialize-Agent([string]$ExecutablePath, [string]$ServiceStatus, [switch]$Register) {
    # 已安装但停止的服务也须先在前台核验身份；身份缺失时在此读取注册码，不能交给后台等待输入。
    if ($ServiceStatus -and $ServiceStatus -ne 'Stopped') {
        if ($Register) { throw 'Agent 服务尚未停止，未替换身份；请检查服务状态后重试。' }
        return
    }
    if ($Register) { & $ExecutablePath -register -initialize } else { & $ExecutablePath -initialize }
    if ($Register -and $LASTEXITCODE -ne 0) { throw '重新注册未完成，服务保持停止；已保存的待关联材料可选择“1 启动”继续重试。请查看上方错误或 service.log，不要删除身份目录。' }
    if ($LASTEXITCODE -ne 0) { throw 'Agent 关联或身份核验未完成，未启动服务；请检查注册提示后再次启动。' }
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
    if ($isAgent) { Write-Host '5 重新注册（关联到新节点）' }
    $choice = Read-Host '选择操作，直接回车启动'
    $Action = switch ($choice) { '2' {'Stop'} '3' {'Upgrade'} '4' {'Status'} '5' {'Register'} default {'Start'} }
}
if ($Action -eq 'Register' -and -not $isAgent) { throw '重新注册仅适用于 Agent，请使用 Agent 原安装目录的启动入口。' }
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
    $accountSID = [Security.Principal.WindowsIdentity]::GetCurrent().User
    if (-not (Test-ServiceAccount $registration.StartName $accountSID)) {
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
if ($Action -eq 'Register') {
    if (-not $service) { throw 'Agent 尚未安装，请选择“1 启动”完成首次注册与安装。' }
    Write-Host '请先在平台删除旧节点，再新建节点并生成未使用的注册码。此操作会停止服务并替换本机节点身份，保留安装目录、配置、信任和日志。'
    Stop-App
}
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
        foreach ($required in @('web\index.html','agents\windows-x86_64\agent.exe','agents\linux-x86_64\agent','agents\linux-aarch64\agent')) {
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
if ($isAgent) {
    $agentServiceStatus = if ($service) { (Get-Service -Name $name).Status.ToString() } else { '' }
    Initialize-Agent $executable $agentServiceStatus -Register:($Action -eq 'Register')
}
if (-not $service) {
    if (-not $isAgent) {
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
try {
    Start-Service -Name $name
    (Get-Service $name).WaitForStatus('Running',[TimeSpan]::FromSeconds(30))
    Start-Sleep -Seconds 3
    if ((Get-Service $name).Status -ne 'Running') { throw '服务启动后退出。' }
} catch {
    $serviceLogHint = if ($isAgent) { 'Agent 身份目录中的 service.log（默认 data\agent-security\service.log）' } else { 'data\service.log' }
    throw [InvalidOperationException]::new("后台服务未成功启动，请检查 $serviceLogHint。", $_.Exception)
}
if (-not $isAgent) {
    & $executable -health-check
    if ($LASTEXITCODE -ne 0) { throw '控制面健康检查未通过，不能视为启动成功。' }
    $caPath = Join-Path $root 'data\control-plane-ca.pem'
    Import-Certificate -FilePath $caPath -CertStoreLocation Cert:\LocalMachine\Root | Out-Null
    $settings = Get-Content -Raw -LiteralPath (Join-Path $root 'data\settings.json') | ConvertFrom-Json
    Write-Host "已启动：$($settings.publicUrl)。本机已安装服务信任；其他浏览器首次需信任 data\control-plane-ca.pem，Agent 包已自动带入。"
} elseif ($Action -eq 'Register') {
    Write-Host '重新注册完成，Agent 服务已启动；请在新节点页面确认心跳、完成环境检查并启用节点。'
} else { Write-Host 'Agent 服务已启动；请在节点页面确认在线状态。正常重启与升级无需注册码。' }
