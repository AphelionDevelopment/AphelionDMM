[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [ValidateSet('Initialize', 'Validate', 'Build', 'StartCloudflare', 'StartLoopback', 'Status', 'Logs', 'Backup', 'Restore', 'Upgrade', 'Rollback', 'Stop')]
    [string]$Action,

    [ValidateSet('Cloudflare', 'Loopback')]
    [string]$Edge = 'Cloudflare',

    [string]$BackupFile,
    [string]$ImageTag,
    [switch]$ConfirmRestore
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$deploymentRoot = [System.IO.Path]::GetFullPath($PSScriptRoot)
$repositoryRoot = [System.IO.Path]::GetFullPath((Join-Path $deploymentRoot '..\..'))
$baseCompose = Join-Path $deploymentRoot 'compose.yaml'
$cloudflareCompose = Join-Path $deploymentRoot 'compose.cloudflare.yaml'
$loopbackCompose = Join-Path $deploymentRoot 'compose.loopback.yaml'
$secretsRoot = Join-Path $deploymentRoot 'secrets'
$backupsRoot = Join-Path $deploymentRoot 'backups'

function Invoke-NativeCommand {
    param(
        [Parameter(Mandatory)]
        [string]$Executable,
        [Parameter(Mandatory)]
        [string[]]$Arguments
    )
    & $Executable @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$Executable exited with code $LASTEXITCODE"
    }
}

function Get-ComposeFiles {
    param([string]$SelectedEdge)
    $overlay = if ($SelectedEdge -eq 'Cloudflare') { $cloudflareCompose } else { $loopbackCompose }
    return @('-f', $baseCompose, '-f', $overlay)
}

function Invoke-Compose {
    param(
        [string]$SelectedEdge,
        [string[]]$Arguments
    )
    $composeFiles = Get-ComposeFiles -SelectedEdge $SelectedEdge
    Invoke-NativeCommand -Executable 'docker' -Arguments (@('compose') + $composeFiles + $Arguments)
}

function Assert-RegularFile {
    param([string]$Path)
    $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
    if (-not $item.PSIsContainer -and -not ($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint)) {
        return
    }
    throw "Required file is not a regular non-link file: $Path"
}

function Assert-DeploymentInputs {
    param([string]$SelectedEdge)
    Assert-RegularFile -Path (Join-Path $deploymentRoot 'config.yaml')
    foreach ($name in @('postgres_password.txt', 'database_dsn.txt', 'oidc_client_secret.txt')) {
        Assert-RegularFile -Path (Join-Path $secretsRoot $name)
    }
    if ($SelectedEdge -eq 'Cloudflare') {
        Assert-RegularFile -Path (Join-Path $secretsRoot 'cloudflare_tunnel_token.txt')
    }
}

function Initialize-Deployment {
    New-Item -ItemType Directory -Path $secretsRoot -Force | Out-Null
    New-Item -ItemType Directory -Path $backupsRoot -Force | Out-Null
    $configPath = Join-Path $deploymentRoot 'config.yaml'
    if (-not (Test-Path -LiteralPath $configPath)) {
        Copy-Item -LiteralPath (Join-Path $deploymentRoot 'config.yaml.example') -Destination $configPath
    }
    $environmentPath = Join-Path $deploymentRoot '.env'
    if (-not (Test-Path -LiteralPath $environmentPath)) {
        Copy-Item -LiteralPath (Join-Path $deploymentRoot '.env.example') -Destination $environmentPath
    }
    Write-Output "Initialized $deploymentRoot"
    Write-Output 'Create the required files under deploy/production/secrets, then restrict their ACLs before starting.'
}

function New-Backup {
    Assert-DeploymentInputs -SelectedEdge $Edge
    New-Item -ItemType Directory -Path $backupsRoot -Force | Out-Null
    $target = if ($BackupFile) { [System.IO.Path]::GetFullPath($BackupFile) } else { Join-Path $backupsRoot ("apheliondmm-{0}.dump" -f (Get-Date -Format 'yyyyMMdd-HHmmss')) }
    if (Test-Path -LiteralPath $target) {
        throw "Backup target already exists: $target"
    }
    $remoteBackup = '/tmp/apheliondmm-backup.dump'
    Invoke-Compose -SelectedEdge $Edge -Arguments @('exec', '-T', 'postgres', 'pg_dump', '--username', 'apheliondmm', '--dbname', 'apheliondmm', '--format', 'custom', '--file', $remoteBackup)
    try {
        Invoke-Compose -SelectedEdge $Edge -Arguments @('cp', "postgres:$remoteBackup", $target)
    } finally {
        Invoke-Compose -SelectedEdge $Edge -Arguments @('exec', '-T', 'postgres', 'rm', '-f', $remoteBackup)
    }
    Write-Output "Backup written to $target"
}

function Restore-Backup {
    if (-not $ConfirmRestore) {
        throw 'Restore is destructive. Re-run with -ConfirmRestore after checking the selected backup.'
    }
    if (-not $BackupFile) {
        throw '-BackupFile is required for Restore.'
    }
    Assert-DeploymentInputs -SelectedEdge $Edge
    $source = (Resolve-Path -LiteralPath $BackupFile -ErrorAction Stop).Path
    Assert-RegularFile -Path $source
    $remoteBackup = '/tmp/apheliondmm-restore.dump'
    Invoke-Compose -SelectedEdge $Edge -Arguments @('stop', 'hosted')
    Invoke-Compose -SelectedEdge $Edge -Arguments @('cp', $source, "postgres:$remoteBackup")
    try {
        Invoke-Compose -SelectedEdge $Edge -Arguments @('exec', '-T', 'postgres', 'pg_restore', '--username', 'apheliondmm', '--dbname', 'apheliondmm', '--clean', '--if-exists', '--exit-on-error', $remoteBackup)
    } finally {
        Invoke-Compose -SelectedEdge $Edge -Arguments @('exec', '-T', 'postgres', 'rm', '-f', $remoteBackup)
    }
    Invoke-Compose -SelectedEdge $Edge -Arguments @('up', '--detach', '--no-deps', 'hosted')
}

Push-Location $repositoryRoot
try {
    switch ($Action) {
        'Initialize' { Initialize-Deployment }
        'Validate' {
            Assert-DeploymentInputs -SelectedEdge $Edge
            Invoke-Compose -SelectedEdge $Edge -Arguments @('config', '--quiet')
        }
        'Build' {
            Assert-DeploymentInputs -SelectedEdge $Edge
            Invoke-Compose -SelectedEdge $Edge -Arguments @('build', '--pull', 'hosted')
        }
        'StartCloudflare' {
            Assert-DeploymentInputs -SelectedEdge 'Cloudflare'
            Invoke-Compose -SelectedEdge 'Cloudflare' -Arguments @('up', '--detach', '--build', '--wait')
        }
        'StartLoopback' {
            Assert-DeploymentInputs -SelectedEdge 'Loopback'
            Invoke-Compose -SelectedEdge 'Loopback' -Arguments @('up', '--detach', '--build', '--wait')
        }
        'Status' { Invoke-Compose -SelectedEdge $Edge -Arguments @('ps') }
        'Logs' { Invoke-Compose -SelectedEdge $Edge -Arguments @('logs', '--no-color', '--tail', '200', 'hosted', 'postgres') }
        'Backup' { New-Backup }
        'Restore' { Restore-Backup }
        'Upgrade' {
            Assert-DeploymentInputs -SelectedEdge $Edge
            Invoke-Compose -SelectedEdge $Edge -Arguments @('build', '--pull', 'hosted')
            Invoke-Compose -SelectedEdge $Edge -Arguments @('stop', 'hosted')
            Invoke-Compose -SelectedEdge $Edge -Arguments @('up', '--detach', '--no-deps', '--wait', 'hosted')
        }
        'Rollback' {
            if (-not $ImageTag) {
                throw '-ImageTag is required for Rollback.'
            }
            $env:APHELIONDMM_IMAGE_TAG = $ImageTag
            Invoke-NativeCommand -Executable 'docker' -Arguments @('image', 'inspect', "apheliondmm-hosted:$ImageTag")
            Invoke-Compose -SelectedEdge $Edge -Arguments @('stop', 'hosted')
            Invoke-Compose -SelectedEdge $Edge -Arguments @('up', '--detach', '--no-build', '--no-deps', '--wait', 'hosted')
        }
        'Stop' { Invoke-Compose -SelectedEdge $Edge -Arguments @('stop') }
    }
} finally {
    Pop-Location
}

