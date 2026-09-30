[CmdletBinding()]
param([string]$OutputDirectory = (Join-Path (Split-Path -Parent $PSScriptRoot) ('artifacts\release-' + (Get-Date -Format 'yyyyMMdd-HHmmss'))))
$ErrorActionPreference = 'Stop'
$repository = Split-Path -Parent $PSScriptRoot
$output = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $output) { throw '构建输出必须是新的独立目录，不能覆盖安装目录。' }
New-Item -ItemType Directory -Path $output | Out-Null
$savedOS=$env:GOOS; $savedArch=$env:GOARCH; $savedCGO=$env:CGO_ENABLED
Push-Location $repository
try {
    Push-Location web
    try { npm run build; if ($LASTEXITCODE -ne 0) { throw '网页构建失败。' } } finally { Pop-Location }
    $env:CGO_ENABLED='0'
    foreach ($target in @(@('windows','amd64'),@('linux','amd64'),@('linux','arm64'))) {
        $env:GOOS=$target[0]; $env:GOARCH=$target[1]
        $platform=$target -join '-'
        $directory=Join-Path $output "ob-data-orch-$platform"
        New-Item -ItemType Directory -Path $directory | Out-Null
        $suffix=if ($target[0] -eq 'windows') { '.exe' } else { '' }
        go build -trimpath -o (Join-Path $directory "control-plane$suffix") ./cmd/control-plane
        if ($LASTEXITCODE -ne 0) { throw "控制面 $platform 构建失败。" }
        Copy-Item -LiteralPath (Join-Path $repository 'web\dist') -Destination (Join-Path $directory 'web') -Recurse
        foreach ($agentTarget in @(@('windows','amd64'),@('linux','amd64'),@('linux','arm64'))) {
            $env:GOOS=$agentTarget[0]; $env:GOARCH=$agentTarget[1]
            $agentPlatform=$agentTarget -join '-'
            $agentDirectory=Join-Path $directory "agents\$agentPlatform"
            New-Item -ItemType Directory -Path $agentDirectory -Force | Out-Null
            $agentSuffix=if ($agentTarget[0] -eq 'windows') { '.exe' } else { '' }
            go build -trimpath -o (Join-Path $agentDirectory "agent$agentSuffix") ./cmd/agent
            if ($LASTEXITCODE -ne 0) { throw "Agent $agentPlatform 构建失败。" }
            if ($agentTarget[0] -eq 'windows') { Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'distribution\启动.cmd'),(Join-Path $PSScriptRoot 'distribution\manage.ps1') -Destination $agentDirectory }
            else { Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'distribution\start.sh') -Destination $agentDirectory }
        }
        if ($target[0] -eq 'windows') { Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'distribution\启动.cmd'),(Join-Path $PSScriptRoot 'distribution\manage.ps1') -Destination $directory }
        else { Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'distribution\start.sh') -Destination $directory }
        Compress-Archive -LiteralPath $directory -DestinationPath "$directory.zip"
    }
    Write-Output "安装包已生成：$output"
} finally { $env:GOOS=$savedOS; $env:GOARCH=$savedArch; $env:CGO_ENABLED=$savedCGO; Pop-Location }
