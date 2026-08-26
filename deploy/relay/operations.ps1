[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [ValidateSet('Setup', 'Validate', 'Build', 'StartCloudflare', 'StartLoopback', 'Status', 'Logs', 'Update', 'Stop')]
    [string]$Action,

    [ValidateSet('Cloudflare', 'Loopback')]
    [string]$Edge = 'Cloudflare'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$deploymentRoot = [System.IO.Path]::GetFullPath($PSScriptRoot)
$repositoryRoot = [System.IO.Path]::GetFullPath((Join-Path $deploymentRoot '..\..'))
$baseCompose = Join-Path $deploymentRoot 'compose.yaml'
$cloudflareCompose = Join-Path $deploymentRoot 'compose.cloudflare.yaml'
$loopbackCompose = Join-Path $deploymentRoot 'compose.loopback.yaml'
$environmentFile = Join-Path $deploymentRoot '.env'
$configFile = Join-Path $deploymentRoot 'relay.yaml'
$secretsRoot = Join-Path $deploymentRoot 'secrets'
$tunnelTokenFile = Join-Path $secretsRoot 'cloudflare_tunnel_token.txt'

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

function Assert-RegularFile {
    param([Parameter(Mandatory)][string]$Path)
    $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
    if ($item.PSIsContainer -or ($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint)) {
        throw "Required file is not a regular non-link file: $Path"
    }
}

function Assert-RegularDirectory {
    param([Parameter(Mandatory)][string]$Path)
    $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
    if (-not $item.PSIsContainer -or ($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint)) {
        throw "Required directory is not a regular non-link directory: $Path"
    }
}

function Get-ComposeArguments {
    param([Parameter(Mandatory)][string]$SelectedEdge)
    $overlay = if ($SelectedEdge -eq 'Cloudflare') { $cloudflareCompose } else { $loopbackCompose }
    return @('--env-file', $environmentFile, '-f', $baseCompose, '-f', $overlay)
}

function Invoke-Compose {
    param(
        [Parameter(Mandatory)][string]$SelectedEdge,
        [Parameter(Mandatory)][string[]]$Arguments
    )
    Invoke-NativeCommand -Executable 'docker' -Arguments (@('compose') + (Get-ComposeArguments -SelectedEdge $SelectedEdge) + $Arguments)
}

function Assert-Docker {
    Invoke-NativeCommand -Executable 'docker' -Arguments @('version')
    Invoke-NativeCommand -Executable 'docker' -Arguments @('compose', 'version')
}

function Assert-Inputs {
    param([Parameter(Mandatory)][string]$SelectedEdge)
    Assert-RegularFile -Path $environmentFile
    Assert-RegularFile -Path $configFile
    Assert-RegularDirectory -Path $secretsRoot
    if ($SelectedEdge -eq 'Cloudflare') {
        Assert-RegularFile -Path $tunnelTokenFile
    }
}

function Initialize-Relay {
    Assert-Docker
    if (Test-Path -LiteralPath $secretsRoot) {
        Assert-RegularDirectory -Path $secretsRoot
    } else {
        New-Item -ItemType Directory -Path $secretsRoot | Out-Null
    }
    if (Test-Path -LiteralPath $configFile) {
        Assert-RegularFile -Path $configFile
    } else {
        Copy-Item -LiteralPath (Join-Path $deploymentRoot 'relay.yaml.example') -Destination $configFile
    }
    if (Test-Path -LiteralPath $environmentFile) {
        Assert-RegularFile -Path $environmentFile
    } else {
        Copy-Item -LiteralPath (Join-Path $deploymentRoot '.env.example') -Destination $environmentFile
    }
    Write-Output "Relay setup prepared at $deploymentRoot"
    Write-Output 'Public mapping: mapping.a13.info -> http://relay:8080'
    Write-Output 'For Cloudflare, install the dedicated tunnel token directly in deploy/relay/secrets/cloudflare_tunnel_token.txt.'
}

function Get-EnvironmentValue {
    param([Parameter(Mandatory)][string]$Name)
    $line = Get-Content -LiteralPath $environmentFile | Where-Object { $_ -match "^$([regex]::Escape($Name))=" } | Select-Object -First 1
    if (-not $line) {
        throw "Missing $Name in $environmentFile"
    }
    return ($line -split '=', 2)[1].Trim()
}

function Update-Relay {
    Assert-Inputs -SelectedEdge $Edge
    $imageTag = Get-EnvironmentValue -Name 'APHELIONDMM_RELAY_IMAGE_TAG'
    $image = "apheliondmm-relay:$imageTag"
    & docker image inspect $image *> $null
    if ($LASTEXITCODE -eq 0) {
        $rollbackTag = 'rollback-' + (Get-Date -Format 'yyyyMMdd-HHmmss')
        Invoke-NativeCommand -Executable 'docker' -Arguments @('tag', $image, "apheliondmm-relay:$rollbackTag")
        Write-Output "Retained prior image as apheliondmm-relay:$rollbackTag"
    }
    Invoke-Compose -SelectedEdge $Edge -Arguments @('build', '--pull', 'relay')
    Invoke-Compose -SelectedEdge $Edge -Arguments @('run', '--rm', '--no-deps', 'relay', '-check-config', '-config', '/etc/apheliondmm/relay.yaml')
    Invoke-Compose -SelectedEdge $Edge -Arguments @('up', '--detach', '--no-deps', '--force-recreate', '--wait', 'relay')
}

Push-Location $repositoryRoot
try {
    switch ($Action) {
        'Setup' { Initialize-Relay }
        'Validate' {
            Assert-Docker
            Assert-Inputs -SelectedEdge $Edge
            Invoke-Compose -SelectedEdge $Edge -Arguments @('config', '--quiet')
        }
        'Build' {
            Assert-Docker
            Assert-Inputs -SelectedEdge $Edge
            Invoke-Compose -SelectedEdge $Edge -Arguments @('build', '--pull', 'relay')
            Invoke-Compose -SelectedEdge $Edge -Arguments @('run', '--rm', '--no-deps', 'relay', '-check-config', '-config', '/etc/apheliondmm/relay.yaml')
        }
        'StartCloudflare' {
            Assert-Docker
            Assert-Inputs -SelectedEdge 'Cloudflare'
            Invoke-Compose -SelectedEdge 'Cloudflare' -Arguments @('up', '--detach', '--build', '--wait')
        }
        'StartLoopback' {
            Assert-Docker
            Assert-Inputs -SelectedEdge 'Loopback'
            Invoke-Compose -SelectedEdge 'Loopback' -Arguments @('up', '--detach', '--build', '--wait')
        }
        'Status' {
            Assert-Inputs -SelectedEdge $Edge
            Invoke-Compose -SelectedEdge $Edge -Arguments @('ps')
        }
        'Logs' {
            Assert-Inputs -SelectedEdge $Edge
            $services = if ($Edge -eq 'Cloudflare') { @('relay', 'cloudflared') } else { @('relay') }
            Invoke-Compose -SelectedEdge $Edge -Arguments (@('logs', '--no-color', '--tail', '200') + $services)
        }
        'Update' { Update-Relay }
        'Stop' {
            Assert-Inputs -SelectedEdge $Edge
            Invoke-Compose -SelectedEdge $Edge -Arguments @('stop')
        }
    }
} finally {
    Pop-Location
}
