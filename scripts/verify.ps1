$ErrorActionPreference = 'Stop'

$unformatted = gofmt -l cmd contracts internal migrations
if ($unformatted) {
    throw "Go files need formatting:`n$unformatted"
}

go test ./cmd/... ./contracts/... ./internal/... ./migrations/...
if ($LASTEXITCODE -ne 0) { throw "Go tests failed with exit code $LASTEXITCODE" }
go vet ./cmd/... ./contracts/... ./internal/... ./migrations/...
if ($LASTEXITCODE -ne 0) { throw "Go vet failed with exit code $LASTEXITCODE" }

$buildRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("ob-data-orch-verify-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $buildRoot | Out-Null
try {
    $targets = @(
        @{ OS = 'windows'; Arch = 'amd64'; Extension = '.exe' },
        @{ OS = 'linux'; Arch = 'amd64'; Extension = '' },
        @{ OS = 'linux'; Arch = 'arm64'; Extension = '' }
    )
    foreach ($target in $targets) {
        $env:CGO_ENABLED = '0'
        $env:GOOS = $target.OS
        $env:GOARCH = $target.Arch
        go build -trimpath -o (Join-Path $buildRoot ("control-plane-{0}-{1}{2}" -f $target.OS, $target.Arch, $target.Extension)) ./cmd/control-plane
        if ($LASTEXITCODE -ne 0) { throw "Control plane build failed for $($target.OS)/$($target.Arch)" }
        go build -trimpath -o (Join-Path $buildRoot ("agent-{0}-{1}{2}" -f $target.OS, $target.Arch, $target.Extension)) ./cmd/agent
        if ($LASTEXITCODE -ne 0) { throw "Agent build failed for $($target.OS)/$($target.Arch)" }
        go test -c -o (Join-Path $buildRoot ("migration-test-{0}-{1}{2}" -f $target.OS, $target.Arch, $target.Extension)) ./internal/migrate
        if ($LASTEXITCODE -ne 0) { throw "Migration test build failed for $($target.OS)/$($target.Arch)" }
    }
}
finally {
    Remove-Item Env:CGO_ENABLED, Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
    $resolvedBuildRoot = [System.IO.Path]::GetFullPath($buildRoot)
    $resolvedTempRoot = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath())
    if (-not $resolvedBuildRoot.StartsWith($resolvedTempRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to remove unexpected verification path: $resolvedBuildRoot"
    }
    Remove-Item -LiteralPath $resolvedBuildRoot -Recurse -Force
}

Push-Location web
try {
    npm ci
    if ($LASTEXITCODE -ne 0) { throw "npm ci failed with exit code $LASTEXITCODE" }
    npm run lint
    if ($LASTEXITCODE -ne 0) { throw "Frontend lint failed with exit code $LASTEXITCODE" }
    npm run audit:wizards
    if ($LASTEXITCODE -ne 0) { throw "Wizard architecture audit failed with exit code $LASTEXITCODE" }
    npm run audit:business
    if ($LASTEXITCODE -ne 0) { throw "Business architecture audit failed with exit code $LASTEXITCODE" }
    npm run typecheck
    if ($LASTEXITCODE -ne 0) { throw "Frontend typecheck failed with exit code $LASTEXITCODE" }
    npm run test
    if ($LASTEXITCODE -ne 0) { throw "Frontend tests failed with exit code $LASTEXITCODE" }
    npm run build
    if ($LASTEXITCODE -ne 0) { throw "Frontend build failed with exit code $LASTEXITCODE" }
}
finally {
    Pop-Location
}

& "$PSScriptRoot/check-secrets.ps1"
