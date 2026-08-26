[CmdletBinding()]
param(
	[Parameter(Mandatory)]
	[ValidateSet('Setup', 'Validate', 'Package', 'Install', 'Start', 'Status', 'Logs', 'Update', 'Stop', 'Uninstall')]
	[string]$Action,

	[string]$PackageRoot = $PSScriptRoot,
	[string]$TunnelTokenFile,
	[string]$CloudflaredPath,
	[switch]$SkipCloudflare,
	[switch]$PurgeData,
	[switch]$Force,
	[string]$InstallRoot = (Join-Path $env:ProgramFiles 'AphelionDMM Relay'),
	[string]$DataRoot = (Join-Path $env:ProgramData 'AphelionDMM\Relay')
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$packageRootPath = [System.IO.Path]::GetFullPath($PackageRoot)
$exampleConfig = Join-Path $PSScriptRoot 'relay.yaml.example'
$packageConfig = Join-Path $packageRootPath 'relay.yaml'
$packageName = 'AphelionDMM-Relay-Windows-x64'
$cloudflaredVersion = '2026.7.3'
$cloudflaredURL = "https://github.com/cloudflare/cloudflared/releases/download/$cloudflaredVersion/cloudflared-windows-amd64.exe"
$cloudflaredSHA256 = '8635da433b6df8194746e88ed9d2589566c20e38bfc2a80e431a348b7c765841'
$serviceName = 'AphelionDMMRelay'
$serviceDisplayName = 'AphelionDMM Collaboration Relay'
$installRootPath = [System.IO.Path]::GetFullPath($InstallRoot)
$dataRootPath = [System.IO.Path]::GetFullPath($DataRoot)
$installedRelay = Join-Path $installRootPath 'apheliondmm-relay.exe'
$installedHealthcheck = Join-Path $installRootPath 'apheliondmm-healthcheck.exe'
$installedCloudflared = Join-Path $installRootPath 'cloudflared.exe'
$installedConfig = Join-Path $dataRootPath 'relay.yaml'
$installedToken = Join-Path $dataRootPath 'cloudflare_tunnel_token.txt'

function Invoke-NativeCommand {
	param(
		[Parameter(Mandatory)][string]$Executable,
		[Parameter(Mandatory)][string[]]$Arguments
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

function Assert-Administrator {
	$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
	$principal = [Security.Principal.WindowsPrincipal]::new($identity)
	if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
		throw 'This action requires an elevated Administrator PowerShell session'
	}
}

function Get-TunnelToken {
	param([Parameter(Mandatory)][string]$Path)
	Assert-RegularFile -Path $Path
	$token = (Get-Content -LiteralPath $Path -Raw).Trim()
	if ($token -notmatch '^[A-Za-z0-9._-]{100,}$') {
		throw 'Cloudflare tunnel token is invalid'
	}
	return $token
}

function Set-RestrictedDirectoryACL {
	param([Parameter(Mandatory)][string]$Path)
	Invoke-NativeCommand -Executable 'icacls.exe' -Arguments @($Path, '/inheritance:r', '/grant:r', '*S-1-5-18:(OI)(CI)F', '*S-1-5-32-544:(OI)(CI)F', '*S-1-5-19:(OI)(CI)RX')
}

function Set-RestrictedFileACL {
	param([Parameter(Mandatory)][string]$Path)
	Invoke-NativeCommand -Executable 'icacls.exe' -Arguments @($Path, '/inheritance:r', '/grant:r', '*S-1-5-18:F', '*S-1-5-32-544:F', '*S-1-5-19:R')
}

function Initialize-Package {
	Assert-RegularFile -Path $exampleConfig
	if (Test-Path -LiteralPath $packageRootPath) {
		Assert-RegularDirectory -Path $packageRootPath
	} else {
		New-Item -ItemType Directory -Path $packageRootPath | Out-Null
	}
	if (Test-Path -LiteralPath $packageConfig) {
		Assert-RegularFile -Path $packageConfig
	} else {
		Copy-Item -LiteralPath $exampleConfig -Destination $packageConfig
	}
	Write-Output "Relay setup prepared at $packageRootPath"
	Write-Output 'Public mapping: mapping.a13.info -> http://127.0.0.1:8080'
}

function New-ServicePackage {
	$repositoryRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
	Assert-RegularFile -Path (Join-Path $repositoryRoot 'go.mod')
	if (Test-Path -LiteralPath $packageRootPath) {
		Assert-RegularDirectory -Path $packageRootPath
	} else {
		New-Item -ItemType Directory -Path $packageRootPath | Out-Null
	}
	$stage = Join-Path $packageRootPath $packageName
	$archive = Join-Path $packageRootPath "$packageName.zip"
	foreach ($existing in @($stage, $archive)) {
		if (Test-Path -LiteralPath $existing) {
			if (-not $Force) {
				throw "Package output already exists: $existing"
			}
			$resolved = [System.IO.Path]::GetFullPath($existing)
			if (-not $resolved.StartsWith($packageRootPath + [System.IO.Path]::DirectorySeparatorChar, [System.StringComparison]::OrdinalIgnoreCase)) {
				throw "Refusing to replace package output outside $packageRootPath"
			}
			Remove-Item -LiteralPath $resolved -Recurse -Force
		}
	}
	$binaryRoot = Join-Path $stage 'bin'
	New-Item -ItemType Directory -Path $binaryRoot | Out-Null
	$relayBinary = Join-Path $binaryRoot 'apheliondmm-relay.exe'
	$healthcheckBinary = Join-Path $binaryRoot 'apheliondmm-healthcheck.exe'
	Push-Location $repositoryRoot
	try {
		$revision = (& git rev-parse --short=8 HEAD).Trim()
		if ($LASTEXITCODE -ne 0 -or -not $revision) {
			throw 'Unable to determine repository revision'
		}
		Invoke-NativeCommand -Executable 'go' -Arguments @('build', '-trimpath', "-ldflags=-s -w -X main.buildRevision=$revision", '-o', $relayBinary, './cmd/apheliondmm-relay')
		Invoke-NativeCommand -Executable 'go' -Arguments @('build', '-trimpath', '-ldflags=-s -w', '-o', $healthcheckBinary, './cmd/apheliondmm-healthcheck')
	} finally {
		Pop-Location
	}
	Copy-Item -LiteralPath $PSCommandPath -Destination (Join-Path $stage 'service.ps1')
	Copy-Item -LiteralPath $exampleConfig -Destination (Join-Path $stage 'relay.yaml.example')
	Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'README.md') -Destination (Join-Path $stage 'README.md')
	$manifest = [ordered]@{
		format_version = 1
		build_revision = $revision
		relay_sha256 = (Get-FileHash -LiteralPath $relayBinary -Algorithm SHA256).Hash
		healthcheck_sha256 = (Get-FileHash -LiteralPath $healthcheckBinary -Algorithm SHA256).Hash
		cloudflared = [ordered]@{
			version = $cloudflaredVersion
			url = $cloudflaredURL
			sha256 = $cloudflaredSHA256
		}
	}
	$manifest | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $stage 'package-manifest.json') -Encoding utf8NoBOM
	Compress-Archive -LiteralPath $stage -DestinationPath $archive -CompressionLevel Optimal
	Write-Output "Windows relay package: $archive"
}

function Test-ServicePackage {
	Assert-RegularDirectory -Path $packageRootPath
	$config = Join-Path $packageRootPath 'relay.yaml'
	$relayBinary = Join-Path $packageRootPath 'bin\apheliondmm-relay.exe'
	$healthcheckBinary = Join-Path $packageRootPath 'bin\apheliondmm-healthcheck.exe'
	$manifestPath = Join-Path $packageRootPath 'package-manifest.json'
	foreach ($path in @($config, $relayBinary, $healthcheckBinary, $manifestPath)) {
		Assert-RegularFile -Path $path
	}
	$manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
	if ($manifest.format_version -ne 1) {
		throw "Unsupported package manifest version: $($manifest.format_version)"
	}
	$relayHash = (Get-FileHash -LiteralPath $relayBinary -Algorithm SHA256).Hash
	$healthcheckHash = (Get-FileHash -LiteralPath $healthcheckBinary -Algorithm SHA256).Hash
	if ($relayHash -ne $manifest.relay_sha256 -or $healthcheckHash -ne $manifest.healthcheck_sha256) {
		throw 'Package binary hash verification failed'
	}
	if ($manifest.cloudflared.version -ne $cloudflaredVersion -or $manifest.cloudflared.url -ne $cloudflaredURL -or $manifest.cloudflared.sha256 -ne $cloudflaredSHA256) {
		throw 'Package Cloudflare pin does not match the reviewed installer pin'
	}
	Invoke-NativeCommand -Executable $relayBinary -Arguments @('-check-config', '-config', $config)
	if (-not $SkipCloudflare) {
		if (-not $TunnelTokenFile) {
			throw '-TunnelTokenFile is required unless -SkipCloudflare is supplied'
		}
		$null = Get-TunnelToken -Path ([System.IO.Path]::GetFullPath($TunnelTokenFile))
	}
	if ($CloudflaredPath) {
		Test-CloudflaredBinary -Path ([System.IO.Path]::GetFullPath($CloudflaredPath))
	}
	Write-Output 'Windows relay package validation passed'
}

function Test-CloudflaredBinary {
	param([Parameter(Mandatory)][string]$Path)
	Assert-RegularFile -Path $Path
	$hash = (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
	if ($hash -ne $cloudflaredSHA256) {
		throw "cloudflared SHA-256 mismatch: $hash"
	}
	$signature = Get-AuthenticodeSignature -LiteralPath $Path
	if ($signature.Status -ne [System.Management.Automation.SignatureStatus]::Valid -or $signature.SignerCertificate.Subject -notmatch 'Cloudflare') {
		throw "cloudflared Authenticode verification failed: $($signature.Status)"
	}
}

function Install-CloudflaredBinary {
	$tempDownload = Join-Path ([System.IO.Path]::GetTempPath()) "apheliondmm-cloudflared-$PID.exe"
	try {
		Invoke-WebRequest -Uri $cloudflaredURL -OutFile $tempDownload
		Test-CloudflaredBinary -Path $tempDownload
		Copy-Item -LiteralPath $tempDownload -Destination $installedCloudflared -Force
	} finally {
		if (Test-Path -LiteralPath $tempDownload -PathType Leaf) {
			Remove-Item -LiteralPath $tempDownload -Force
		}
	}
}

function Get-RelayService {
	return Get-CimInstance -ClassName Win32_Service -Filter "Name='$serviceName'" -ErrorAction SilentlyContinue
}

function Assert-OwnedRelayService {
	$service = Get-RelayService
	if (-not $service) {
		throw "Windows service $serviceName is not installed"
	}
	$expected = [System.IO.Path]::GetFullPath($installedRelay)
	$binaryPath = [string]$service.PathName
	if (-not $binaryPath.StartsWith('"' + $expected + '"', [System.StringComparison]::OrdinalIgnoreCase)) {
		throw "Refusing to operate on $serviceName because its executable is outside $installRootPath"
	}
	return $service
}

function Wait-RelayReady {
	$deadline = [DateTime]::UtcNow.AddSeconds(45)
	do {
		& $installedHealthcheck *> $null
		if ($LASTEXITCODE -eq 0) {
			return
		}
		Start-Sleep -Milliseconds 250
	} while ([DateTime]::UtcNow -lt $deadline)
	throw 'Relay service did not become ready within 45 seconds'
}

function Start-RelayService {
	Assert-Administrator
	$null = Assert-OwnedRelayService
	Start-Service -Name $serviceName
	(Get-Service -Name $serviceName).WaitForStatus('Running', [TimeSpan]::FromSeconds(30))
	Wait-RelayReady
	Write-Output "$serviceName is running and ready"
}

function Stop-RelayService {
	Assert-Administrator
	$null = Assert-OwnedRelayService
	$service = Get-Service -Name $serviceName
	if ($service.Status -ne 'Stopped') {
		Stop-Service -Name $serviceName
		$service.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(30))
	}
	Write-Output "$serviceName is stopped"
}

function Install-RelayService {
	Assert-Administrator
	Test-ServicePackage
	if (Get-RelayService) {
		throw "$serviceName is already installed; use -Action Update"
	}
	foreach ($directory in @($installRootPath, $dataRootPath)) {
		if (Test-Path -LiteralPath $directory) {
			Assert-RegularDirectory -Path $directory
		} else {
			New-Item -ItemType Directory -Path $directory | Out-Null
		}
	}
	Set-RestrictedDirectoryACL -Path $installRootPath
	Set-RestrictedDirectoryACL -Path $dataRootPath
	Copy-Item -LiteralPath (Join-Path $packageRootPath 'bin\apheliondmm-relay.exe') -Destination $installedRelay -Force
	Copy-Item -LiteralPath (Join-Path $packageRootPath 'bin\apheliondmm-healthcheck.exe') -Destination $installedHealthcheck -Force
	if (-not (Test-Path -LiteralPath $installedConfig)) {
		Copy-Item -LiteralPath $packageConfig -Destination $installedConfig
	}
	Set-RestrictedFileACL -Path $installedConfig
	$arguments = @('-config', $installedConfig)
	if (-not $SkipCloudflare) {
		$sourceToken = [System.IO.Path]::GetFullPath($TunnelTokenFile)
		$null = Get-TunnelToken -Path $sourceToken
		Copy-Item -LiteralPath $sourceToken -Destination $installedToken -Force
		Set-RestrictedFileACL -Path $installedToken
		Install-CloudflaredBinary
		$arguments += @('-cloudflared', $installedCloudflared, '-tunnel-token-file', $installedToken)
	}
	foreach ($path in @($installedRelay, $installedHealthcheck)) {
		Set-RestrictedFileACL -Path $path
	}
	if (-not $SkipCloudflare) {
		Set-RestrictedFileACL -Path $installedCloudflared
	}
	& $installedRelay -check-config -config $installedConfig
	if ($LASTEXITCODE -ne 0) {
		throw 'Installed relay configuration validation failed'
	}
	if (-not [System.Diagnostics.EventLog]::SourceExists($serviceName)) {
		[System.Diagnostics.EventLog]::CreateEventSource($serviceName, 'Application')
	}
	$quotedArguments = $arguments | ForEach-Object { '"' + ($_ -replace '"', '\"') + '"' }
	$binaryCommand = '"' + $installedRelay + '" ' + ($quotedArguments -join ' ')
	Invoke-NativeCommand -Executable 'sc.exe' -Arguments @('create', $serviceName, 'binPath=', $binaryCommand, 'start=', 'delayed-auto', 'obj=', 'NT AUTHORITY\LocalService', 'DisplayName=', $serviceDisplayName)
	Invoke-NativeCommand -Executable 'sc.exe' -Arguments @('description', $serviceName, 'Stateless end-to-end encrypted AphelionDMM collaboration relay')
	Invoke-NativeCommand -Executable 'sc.exe' -Arguments @('failure', $serviceName, 'reset=', '86400', 'actions=', 'restart/20000/restart/20000/restart/20000')
	Invoke-NativeCommand -Executable 'sc.exe' -Arguments @('failureflag', $serviceName, '1')
	Start-RelayService
}

function Show-RelayStatus {
	$service = Assert-OwnedRelayService
	& $installedHealthcheck *> $null
	$relayReady = $LASTEXITCODE -eq 0
	[pscustomobject]@{
		Name = $service.Name
		State = $service.State
		StartMode = $service.StartMode
		ServiceAccount = $service.StartName
		RelayReady = $relayReady
		CloudflareEnabled = Test-Path -LiteralPath $installedCloudflared -PathType Leaf
	}
}

function Show-RelayLogs {
	$null = Assert-OwnedRelayService
	Get-WinEvent -FilterHashtable @{ LogName = 'Application'; ProviderName = $serviceName } -MaxEvents 200 -ErrorAction SilentlyContinue |
		Select-Object TimeCreated, LevelDisplayName, Message
}

function Update-RelayService {
	Assert-Administrator
	$null = Assert-OwnedRelayService
	Test-ServicePackage
	$rollbackRoot = Join-Path $installRootPath ('rollback-' + (Get-Date -Format 'yyyyMMdd-HHmmss'))
	$rollbackToken = Join-Path $dataRootPath 'cloudflare_tunnel_token.rollback'
	New-Item -ItemType Directory -Path $rollbackRoot | Out-Null
	foreach ($path in @($installedRelay, $installedHealthcheck, $installedCloudflared)) {
		if (Test-Path -LiteralPath $path -PathType Leaf) {
			Copy-Item -LiteralPath $path -Destination $rollbackRoot
		}
	}
	if (-not $SkipCloudflare -and (Test-Path -LiteralPath $installedToken -PathType Leaf)) {
		Copy-Item -LiteralPath $installedToken -Destination $rollbackToken -Force
		Set-RestrictedFileACL -Path $rollbackToken
	}
	Stop-RelayService
	try {
		Copy-Item -LiteralPath (Join-Path $packageRootPath 'bin\apheliondmm-relay.exe') -Destination $installedRelay -Force
		Copy-Item -LiteralPath (Join-Path $packageRootPath 'bin\apheliondmm-healthcheck.exe') -Destination $installedHealthcheck -Force
		if (-not $SkipCloudflare) {
			$sourceToken = [System.IO.Path]::GetFullPath($TunnelTokenFile)
			$null = Get-TunnelToken -Path $sourceToken
			Copy-Item -LiteralPath $sourceToken -Destination $installedToken -Force
			Set-RestrictedFileACL -Path $installedToken
			Install-CloudflaredBinary
		}
		Start-RelayService
		Write-Output "Rollback files retained at $rollbackRoot"
	} catch {
		Copy-Item -LiteralPath (Join-Path $rollbackRoot 'apheliondmm-relay.exe') -Destination $installedRelay -Force
		Copy-Item -LiteralPath (Join-Path $rollbackRoot 'apheliondmm-healthcheck.exe') -Destination $installedHealthcheck -Force
		$rollbackCloudflared = Join-Path $rollbackRoot 'cloudflared.exe'
		if (Test-Path -LiteralPath $rollbackCloudflared -PathType Leaf) {
			Copy-Item -LiteralPath $rollbackCloudflared -Destination $installedCloudflared -Force
		}
		if (Test-Path -LiteralPath $rollbackToken -PathType Leaf) {
			Copy-Item -LiteralPath $rollbackToken -Destination $installedToken -Force
			Set-RestrictedFileACL -Path $installedToken
		}
		Start-RelayService
		throw
	} finally {
		if (Test-Path -LiteralPath $rollbackToken -PathType Leaf) {
			Remove-Item -LiteralPath $rollbackToken -Force
		}
	}
}

function Uninstall-RelayService {
	Assert-Administrator
	$null = Assert-OwnedRelayService
	Stop-RelayService
	Invoke-NativeCommand -Executable 'sc.exe' -Arguments @('delete', $serviceName)
	$deadline = [DateTime]::UtcNow.AddSeconds(30)
	while ((Get-RelayService) -and [DateTime]::UtcNow -lt $deadline) {
		Start-Sleep -Milliseconds 250
	}
	if (Get-RelayService) {
		throw "$serviceName was not removed within 30 seconds"
	}
	if ([System.Diagnostics.EventLog]::SourceExists($serviceName)) {
		[System.Diagnostics.EventLog]::DeleteEventSource($serviceName)
	}
	if (Test-Path -LiteralPath $installRootPath) {
		Assert-RegularDirectory -Path $installRootPath
		Remove-Item -LiteralPath $installRootPath -Recurse -Force
	}
	if ($PurgeData -and (Test-Path -LiteralPath $dataRootPath)) {
		Assert-RegularDirectory -Path $dataRootPath
		Remove-Item -LiteralPath $dataRootPath -Recurse -Force
	}
	Write-Output "$serviceName was uninstalled"
}

switch ($Action) {
	'Setup' { Initialize-Package }
	'Validate' { Test-ServicePackage }
	'Package' { New-ServicePackage }
	'Install' { Install-RelayService }
	'Start' { Start-RelayService }
	'Status' { Show-RelayStatus }
	'Logs' { Show-RelayLogs }
	'Update' { Update-RelayService }
	'Stop' { Stop-RelayService }
	'Uninstall' { Uninstall-RelayService }
}
