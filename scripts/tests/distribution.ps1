$ErrorActionPreference = 'Stop'
$path = Join-Path $PSScriptRoot '../distribution/manage.ps1'
$tokens = $null
$errors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile($path, [ref]$tokens, [ref]$errors)
if ($errors.Count) { throw $errors[0] }
# 仅载入校验和文件更新函数，不运行管理员检查或系统服务操作。
foreach ($name in @('Test-ServiceExecutable','Resolve-ServiceAccountSID','Test-ServiceAccount','Initialize-Agent','Update-Launchers','Invoke-InitialAuditAccess')) {
    $definition = $ast.Find({ param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $name }, $true)
    if (-not $definition) { throw "找不到测试所需函数 $name。" }
    . ([scriptblock]::Create($definition.Extent.Text))
}
$cases = @(
    @('E:\Apps\control-plane.exe -data-dir E:\Apps\data','E:\Apps\control-plane.exe',$true),
    @('"E:\Apps\control-plane.exe" -data-dir E:\Apps\data','E:\Apps\control-plane.exe',$true),
    @('"E:\My Apps\agent.exe"','E:\My Apps\agent.exe',$true),
    @('e:\apps\agent.exe','E:\Apps\agent.exe',$true),
    @('E:\Apps\agent.exe.old','E:\Apps\agent.exe',$false),
    @('"E:\Apps\agent.exe"suffix','E:\Apps\agent.exe',$false),
    @('E:\Other\agent.exe','E:\Apps\agent.exe',$false),
    @('E:\My Apps\agent.exe','E:\My Apps\agent.exe',$false),
    @('"E:\Apps\agent.exe','E:\Apps\agent.exe',$false),
    @('','E:\Apps\agent.exe',$false)
)
foreach ($case in $cases) {
    if ((Test-ServiceExecutable $case[0] $case[1]) -ne $case[2]) { throw '服务路径匹配回归失败。' }
}

# 只替换系统账户解析端口，以合成 SID 验证别名与失败关闭，不创建账户或访问域服务。
$nativeAccountResolver = (Get-Command Resolve-ServiceAccountSID).ScriptBlock
$fixtureAccountSID = [Security.Principal.SecurityIdentifier]::new('S-1-5-21-111-222-333-1001')
$fixtureOtherSID = [Security.Principal.SecurityIdentifier]::new('S-1-5-21-111-222-333-1002')
function Resolve-ServiceAccountSID([string]$AccountName) {
    switch ($AccountName.ToLowerInvariant()) {
        '.\synthetic-operator' { return $fixtureAccountSID }
        'fixture-host\synthetic-operator' { return $fixtureAccountSID }
        'fixture-domain\synthetic-operator' { return $fixtureAccountSID }
        'synthetic-operator@fixture.invalid' { return $fixtureAccountSID }
        'other-domain\synthetic-operator' { return $fixtureOtherSID }
        'fixture-host\synthetic-other' { return $fixtureOtherSID }
        'fixture-host\lookup-failed' { throw '合成账户解析失败。' }
        default { return $null }
    }
}
$accountCases = @(
    @('.\synthetic-operator',$fixtureAccountSID,$true),
    @('fixture-host\synthetic-operator',$fixtureAccountSID,$true),
    @('FIXTURE-HOST\SYNTHETIC-OPERATOR',$fixtureAccountSID,$true),
    @('fixture-domain\synthetic-operator',$fixtureAccountSID,$true),
    @('synthetic-operator@fixture.invalid',$fixtureAccountSID,$true),
    @('other-domain\synthetic-operator',$fixtureAccountSID,$false),
    @('fixture-host\synthetic-other',$fixtureAccountSID,$false),
    @('fixture-host\synthetic-operator',$fixtureOtherSID,$false),
    @('fixture-host\unresolved',$fixtureAccountSID,$false),
    @('fixture-host\lookup-failed',$fixtureAccountSID,$false),
    @('',$fixtureAccountSID,$false),
    @('  ',$fixtureAccountSID,$false),
    @('fixture-host\synthetic-operator',$null,$false)
)
try {
    foreach ($case in $accountCases) {
        if ((Test-ServiceAccount $case[0] $case[1]) -ne $case[2]) { throw '服务账户 SID 校验回归失败。' }
    }
} finally {
    Set-Item -Path Function:\Resolve-ServiceAccountSID -Value $nativeAccountResolver
}

$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('obdo-launchers-' + [guid]::NewGuid().ToString('N'))
try {
    $source = Join-Path $testRoot 'source'
    $target = Join-Path $testRoot 'target'
    New-Item -ItemType Directory -Path $source,$target | Out-Null
    # 首次审计授予只运行假程序，核验停止状态、显式确认、标准输入和失败传播。
    $auditCalls = Join-Path $testRoot 'audit-calls.txt'
    $auditInput = Join-Path $testRoot 'audit-input.txt'
    $auditProgram = Join-Path $testRoot 'audit-fixture.cmd'
    Set-Content -LiteralPath $auditProgram -Encoding ASCII -Value @"
@echo off
if not "%~1"=="-bootstrap-audit-access" exit /b 99
more > "$auditInput"
echo granted>>"$auditCalls"
exit /b 0
"@
    function Read-Host { param([string]$Prompt, [switch]$AsSecureString)
        if ($AsSecureString) { return ConvertTo-SecureString 'synthetic-audit-password' -AsPlainText -Force }
        return $script:auditConfirmation
    }
    $script:auditConfirmation = 'GRANT_AUDIT_LOG admin'
    foreach ($status in @('Running','StartPending','StopPending','')) {
        $rejected = $false
        try { Invoke-InitialAuditAccess $auditProgram $status } catch { $rejected = $true }
        if (-not $rejected -or (Test-Path -LiteralPath $auditCalls)) { throw '活动或未知服务状态仍执行首次授予。' }
    }
    $script:auditConfirmation = 'cancel'
    $rejected = $false
    try { Invoke-InitialAuditAccess $auditProgram 'Stopped' } catch { $rejected = $true }
    if (-not $rejected -or (Test-Path -LiteralPath $auditCalls)) { throw '未明确确认仍执行首次授予。' }
    $script:auditConfirmation = 'GRANT_AUDIT_LOG admin'
    Invoke-InitialAuditAccess $auditProgram 'Stopped'
    if (@(Get-Content -LiteralPath $auditCalls).Count -ne 1) { throw '首次授予没有通过标准输入调用固定入口。' }
    $auditLines = @(Get-Content -LiteralPath $auditInput | Where-Object { $_ -ne '' })
    if ($auditLines.Count -ne 2 -or $auditLines[0] -ne 'synthetic-audit-password' -or $auditLines[1] -cne 'GRANT_AUDIT_LOG admin') { throw '首次授予未通过标准输入完整传递原口令和确认；不得传入 argv。' }
    Set-Content -LiteralPath $auditProgram -Encoding ASCII -Value "@echo off`r`nexit /b 1"
    $rejected = $false
    try { Invoke-InitialAuditAccess $auditProgram 'Stopped' } catch { $rejected = $true }
    if (-not $rejected) { throw '首次授予失败仍宣称完成。' }
    $auditGuard = $ast.Find({ param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -eq '$Action -eq ''AuditAccess'' -and $isAgent' }, $true)
    $auditBranch = $ast.Find({ param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -eq '$Action -eq ''AuditAccess''' }, $true)
    if (-not $auditGuard -or -not $auditBranch) { throw '找不到首次审计授予的组件与安装限制。' }
    $Action = 'AuditAccess'; $isAgent = $true
    $rejected = $false
    try { . ([scriptblock]::Create($auditGuard.Extent.Text)) } catch { $rejected = $true }
    if (-not $rejected) { throw 'Agent 可以进入首次审计授予。' }
    $isAgent = $false; $service = $null
    $rejected = $false
    try { . ([scriptblock]::Create($auditBranch.Extent.Text)) } catch { $rejected = $true }
    if (-not $rejected) { throw '未安装的控制面仍进入首次授予。' }
    Remove-Item -Path Function:\Read-Host
    # 用假程序核验前台初始化，不读取真实注册码或启动系统服务。
    $initializationCalls = Join-Path $testRoot 'initialization-calls.txt'
    $initializationProgram = Join-Path $testRoot 'initialize-fixture.cmd'
    Set-Content -LiteralPath $initializationProgram -Encoding ASCII -Value @"
@echo off
if "%~1"=="-register" (
  if not "%~2"=="-initialize" exit /b 99
  echo registered>>"$initializationCalls"
) else (
  if not "%~1"=="-initialize" exit /b 99
  echo initialized>>"$initializationCalls"
)
exit /b 0
"@
    Initialize-Agent $initializationProgram ''
    Initialize-Agent $initializationProgram 'Stopped'
    Initialize-Agent $initializationProgram 'Running'
    Initialize-Agent $initializationProgram 'StartPending'
    Initialize-Agent $initializationProgram 'StopPending'
    if (@(Get-Content -LiteralPath $initializationCalls).Count -ne 2) { throw '首次或停止的 Agent 应初始化，活动服务不得重新初始化。' }
    # 执行真实装配分支，确认已安装但停止的 Agent 不会再被跳过。
    $initialization = $ast.Find({ param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -eq '$isAgent' -and $node.Extent.Text.Contains('Initialize-Agent') }, $true)
    if (-not $initialization) { throw '找不到 Agent 前台初始化分支。' }
    $initializationBlock = [scriptblock]::Create($initialization.Extent.Text)
    $service = $true
    $isAgent = $true
    $Action = 'Start'
    $name = 'fixture-service'
    $executable = $initializationProgram
    function Get-Service { return [pscustomobject]@{ Status='Stopped' } }
    . $initializationBlock
    if (@(Get-Content -LiteralPath $initializationCalls).Count -ne 3) { throw '已安装但停止的 Agent 未进入前台初始化。' }
    $isAgent = $false
    . $initializationBlock
    if (@(Get-Content -LiteralPath $initializationCalls).Count -ne 3) { throw '控制面错误进入了 Agent 初始化。' }
    # 重新注册必须先停止服务，只有显式操作才传入替换身份参数。
    $registration = $ast.Find({ param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -eq '$Action -eq ''Register''' }, $true)
    $registrationGuard = $ast.Find({ param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -eq '$Action -eq ''Register'' -and -not $isAgent' }, $true)
    if (-not $registration -or -not $registrationGuard) { throw '找不到重新注册流程或组件限制。' }
    $registrationBlock = [scriptblock]::Create($registration.Extent.Text)
    $registrationGuardBlock = [scriptblock]::Create($registrationGuard.Extent.Text)
    $Action = 'Register'
    $isAgent = $true
    $script:registrationStatus = 'Running'
    $script:registrationStopCalled = $false
    function Get-Service { return [pscustomobject]@{ Status=$script:registrationStatus } }
    function Stop-App { $script:registrationStopCalled = $true; $script:registrationStatus = 'Stopped' }
    . $registrationBlock
    . $initializationBlock
    if (-not $script:registrationStopCalled -or (Get-Content -LiteralPath $initializationCalls | Select-Object -Last 1) -ne 'registered') { throw '重新注册未先停服并调用显式替换入口。' }
    foreach ($status in @('Running','StartPending','StopPending')) {
        $rejected = $false
        try { Initialize-Agent $initializationProgram $status -Register } catch { $rejected = $true }
        if (-not $rejected) { throw '活动服务不应替换身份。' }
    }
    if (@(Get-Content -LiteralPath $initializationCalls).Count -ne 4) { throw '服务未停止时执行了注册。' }
    function Stop-App { throw '合成停服失败。' }
    $rejected = $false
    try { . $registrationBlock; . $initializationBlock } catch { $rejected = $true }
    if (-not $rejected -or @(Get-Content -LiteralPath $initializationCalls).Count -ne 4) { throw '停服失败仍替换了身份。' }
    $service = $null
    $rejected = $false
    try { . $registrationBlock } catch { $rejected = $true }
    if (-not $rejected) { throw '尚未安装的 Agent 不应进入重新注册。' }
    $isAgent = $false
    $rejected = $false
    try { . $registrationGuardBlock } catch { $rejected = $true }
    if (-not $rejected) { throw '控制面不应进入重新注册。' }
    $service = $true
    Set-Content -LiteralPath $initializationProgram -Encoding ASCII -Value "@echo off`r`nexit /b 1"
    $isAgent = $true
    $initializationRejected = $false
    $serviceStartReached = $false
    try { . $initializationBlock; $serviceStartReached = $true } catch { $initializationRejected = $true }
    if (-not $initializationRejected -or $serviceStartReached) { throw '初始化失败时不应继续启动服务。' }
    $Action = 'Start'
    $initializationRejected = $false
    try { . $initializationBlock } catch { $initializationRejected = $true }
    if (-not $initializationRejected) { throw '普通启动未阻断关联失败。' }
    foreach ($name in @('启动.cmd','manage.ps1','agent-config.json','control-plane-ca.pem')) {
        Set-Content -LiteralPath (Join-Path $source $name) -Value 'new-synthetic'
        Set-Content -LiteralPath (Join-Path $target $name) -Value 'original-synthetic'
    }
    Update-Launchers $source $target
    foreach ($name in @('启动.cmd','manage.ps1')) {
        if ((Get-Content -LiteralPath (Join-Path $target $name)) -ne 'new-synthetic') { throw '入口未更新。' }
    }
    foreach ($name in @('agent-config.json','control-plane-ca.pem')) {
        if ((Get-Content -LiteralPath (Join-Path $target $name)) -ne 'original-synthetic') { throw '配置或信任被覆盖。' }
    }
    # 执行实际升级分支，用假程序和假停服函数验证停服前校验及完整文件替换。
    $upgrade = $ast.Find({ param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Clauses[0].Item1.Extent.Text -eq '$Action -eq ''Upgrade''' }, $true)
    if (-not $upgrade) { throw '找不到升级分支。' }
    $upgradeBlock = [scriptblock]::Create($upgrade.Extent.Text)
    $root = $target
    $binary = 'fixture.cmd'
    $executable = Join-Path $root $binary
    $service = $true
    $isAgent = $true
    $Action = 'Upgrade'
    function Read-Host { return $source }
    function Stop-App { $script:stopCalled = $true }
    Set-Content -LiteralPath (Join-Path $source $binary) -Value "@echo off`r`nexit /b 0"
    Set-Content -LiteralPath $executable -Value 'old-binary'
    Remove-Item -LiteralPath (Join-Path $source 'manage.ps1')
    $script:stopCalled = $false
    $rejected = $false
    try { . $upgradeBlock } catch { $rejected = $true }
    if (-not $rejected -or $script:stopCalled -or (Get-Content -LiteralPath $executable) -ne 'old-binary') { throw '缺少入口时不应停止或替换旧服务。' }
    Set-Content -LiteralPath (Join-Path $source 'manage.ps1') -Value 'new-synthetic'
    . $upgradeBlock
    if (-not $script:stopCalled -or (Get-FileHash $executable).Hash -ne (Get-FileHash (Join-Path $source $binary)).Hash) { throw '升级未替换程序。' }
    if ((Get-Content -LiteralPath (Join-Path $target 'agent-config.json')) -ne 'original-synthetic') { throw '升级覆盖了身份配置。' }
    # 控制面升级必须先核对三种新命名资源；缺少任一目标时不能停止原服务。
    $isAgent = $false
    $required = @('web/index.html','agents/windows-x86_64/agent.exe','agents/linux-x86_64/agent','agents/linux-aarch64/agent')
    foreach ($name in $required) {
        $file = Join-Path $source $name
        New-Item -ItemType Directory -Path (Split-Path -Parent $file) -Force | Out-Null
        Set-Content -LiteralPath $file -Value 'synthetic-platform-resource'
    }
    Remove-Item -LiteralPath (Join-Path $source 'agents/linux-aarch64/agent')
    $script:stopCalled = $false
    $rejected = $false
    try { . $upgradeBlock } catch { $rejected = $true }
    if (-not $rejected -or $script:stopCalled) { throw '缺少新命名平台资源时不应停止旧服务。' }
    Set-Content -LiteralPath (Join-Path $source 'agents/linux-aarch64/agent') -Value 'synthetic-platform-resource'
    . $upgradeBlock
    if (-not $script:stopCalled) { throw '完整新命名资源未进入受控升级。' }
    foreach ($name in $required) {
        if ((Get-FileHash -LiteralPath (Join-Path $source $name)).Hash -ne (Get-FileHash -LiteralPath (Join-Path $root $name)).Hash) { throw '新命名平台资源未完整部署。' }
    }
    Write-Output 'PASS: 10 service path cases; 13 account SID cases; 9 initial audit access cases; Agent initialization and re-registration lifecycle; launcher update; Agent and control-plane upgrade; missing launcher/platform rejection; configuration preserved'
} finally {
    $resolved = [IO.Path]::GetFullPath($testRoot)
    $temp = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\','/') + [IO.Path]::DirectorySeparatorChar
    if (-not $resolved.StartsWith($temp,[StringComparison]::OrdinalIgnoreCase)) { throw '测试清理路径越界。' }
    if (Test-Path -LiteralPath $resolved) { Remove-Item -LiteralPath $resolved -Recurse -Force }
}
