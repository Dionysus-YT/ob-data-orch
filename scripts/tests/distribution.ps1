$ErrorActionPreference = 'Stop'
$path = Join-Path $PSScriptRoot '../distribution/manage.ps1'
$tokens = $null
$errors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile($path, [ref]$tokens, [ref]$errors)
if ($errors.Count) { throw $errors[0] }
# 仅载入纯路径校验和文件更新函数，不运行管理员检查或系统服务操作。
foreach ($name in @('Test-ServiceExecutable','Update-Launchers')) {
    $definition = $ast.Find({ param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $name }, $true)
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
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('obdo-launchers-' + [guid]::NewGuid().ToString('N'))
try {
    $source = Join-Path $testRoot 'source'
    $target = Join-Path $testRoot 'target'
    New-Item -ItemType Directory -Path $source,$target | Out-Null
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
    Write-Output 'PASS: 10 service path cases; launcher update; upgrade success and missing-launcher rejection; configuration preserved'
} finally {
    $resolved = [IO.Path]::GetFullPath($testRoot)
    $temp = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\','/') + [IO.Path]::DirectorySeparatorChar
    if (-not $resolved.StartsWith($temp,[StringComparison]::OrdinalIgnoreCase)) { throw '测试清理路径越界。' }
    if (Test-Path -LiteralPath $resolved) { Remove-Item -LiteralPath $resolved -Recurse -Force }
}
