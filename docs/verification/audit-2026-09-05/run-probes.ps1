param([string]$Filter = '^TestAudit', [int]$Count = 1, [switch]$Editor, [switch]$Unknown)
$ErrorActionPreference = 'Stop'
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../../..'))
$scratch = Join-Path ([IO.Path]::GetTempPath()) ('admm-audit-' + [guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($scratch) | Out-Null
$mapping = @{}
foreach ($package in @('server', 'client')) {
    $virtual = Join-Path $repo ('internal/aphelion/collab/' + $package + '/audit_20260905_test.go')
    $mapping[$virtual] = Join-Path $PSScriptRoot ($package + '_test.go.txt')
}
$packages = @('./internal/aphelion/collab/server', './internal/aphelion/collab/client')
if ($Editor) {
    $mapping[(Join-Path $repo 'internal/app/ui/cpwsarea/wsmap/pmap/editor/audit_20260905_test.go')] = Join-Path $PSScriptRoot 'editor_test.go.txt'
    $packages = @('./internal/app/ui/cpwsarea/wsmap/pmap/editor')
}
if ($Unknown) {
    $mapping[(Join-Path $repo 'internal/app/ui/cpvareditor/audit_20260905_test.go')] = Join-Path $PSScriptRoot 'unknown_test.go.txt'
    $packages = @('./internal/app/ui/cpvareditor')
}
$overlay = Join-Path $scratch 'overlay.json'
@{ Replace = $mapping } | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $overlay -Encoding utf8
Push-Location $repo
try {
    & go test ('-overlay=' + $overlay) @packages -run $Filter -count $Count -timeout 60s -v
    $probeExit = $LASTEXITCODE
} finally {
    Pop-Location
    Remove-Item -LiteralPath $overlay
    Remove-Item -LiteralPath $scratch
}
exit $probeExit
