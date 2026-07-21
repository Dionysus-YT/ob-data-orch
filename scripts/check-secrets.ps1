$ErrorActionPreference = 'Stop'

$pattern = '(mysql|obclient)\s+.*-[pP]\S+|BEGIN\s+(RSA\s+|EC\s+|OPENSSH\s+)?PRIVATE\s+KEY|AKIA[0-9A-Z]{16}'
$files = git ls-files --cached --others --exclude-standard -- cmd contracts internal migrations web .github
if ($LASTEXITCODE -ne 0) {
    throw "git ls-files failed with exit code $LASTEXITCODE"
}
$matches = foreach ($file in $files) {
    Select-String -LiteralPath $file -Pattern $pattern
}
if ($matches) {
    $details = $matches | ForEach-Object { "{0}:{1}:{2}" -f $_.Path, $_.LineNumber, $_.Line }
    Write-Error "Potential secret material found:`n$($details -join "`n")"
}
Write-Output 'Secret scan passed.'
