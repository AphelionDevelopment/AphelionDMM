[CmdletBinding()]
param(
	[Parameter(Mandatory = $true)]
	[string] $AphelionRoot,
	[string] $MeridianMcpRoot,
	[string] $ContentToolsRoot,
	[string] $MeridianRiftRoot,
	[string] $InstalledMcp,
	[string] $EvidencePath,
	[int] $TimeoutMinutes = 30,
	[switch] $AllowNetwork,
	[switch] $SkipLauncher,
	[switch] $PlanOnly
)

$ErrorActionPreference = "Stop"
$script:GateResults = [System.Collections.Generic.List[object]]::new()
$script:RunStarted = [DateTimeOffset]::UtcNow
$script:RunID = $script:RunStarted.ToString("yyyyMMddTHHmmssZ") + "-" + [guid]::NewGuid().ToString("N")

function Resolve-RequiredRoot {
	param([string] $Path, [string] $Name)
	if ([string]::IsNullOrWhiteSpace($Path)) { throw "$Name is required unless -PlanOnly is used." }
	if (-not (Test-Path -LiteralPath $Path -PathType Container)) { throw "$Name does not exist: $Path" }
	return (Resolve-Path -LiteralPath $Path).Path
}

function ConvertTo-ProcessArgument {
	param([string] $Value)
	if ($Value -notmatch '[\s"]') { return $Value }
	return '"' + ($Value -replace '(\\*)"', '$1$1\"' -replace '(\\+)$', '$1$1') + '"'
}

function Stop-ProcessTree {
	param([System.Diagnostics.Process] $Process)
	if (-not $Process -or $Process.HasExited) { return }
	& taskkill.exe /PID $Process.Id /T /F 2>$null | Out-Null
	try { $Process.Kill() } catch { }
}

function Add-GateResult {
	param(
		[string] $Name,
		[string] $Repository,
		[string] $Status,
		[int] $ExitCode,
		[double] $DurationSeconds,
		[string] $Log,
	[string] $Detail = ""
	)
	$stderrLog = ""
	if ($Log -like "*.stdout.log") { $stderrLog = $Log.Substring(0, $Log.Length - ".stdout.log".Length) + ".stderr.log" }
	[void]$script:GateResults.Add([ordered]@{
		name = $Name
		repository = $Repository
		status = $Status
		exit_code = $ExitCode
		duration_seconds = [Math]::Round($DurationSeconds, 3)
		stdout_log = $Log
		stderr_log = $stderrLog
		detail = $Detail
	})
}

function Invoke-LauncherGate {
	param([string] $RepositoryRoot, [int] $TimeoutSeconds = 45)
	$name = "Content Tools real launcher"
	$gateRoot = Join-Path $script:EvidenceRoot "logs"
	New-Item -ItemType Directory -Force -Path $gateRoot | Out-Null
	$stdout = Join-Path $gateRoot "content-tools-real-launcher.stdout.log"
	$stderr = Join-Path $gateRoot "content-tools-real-launcher.stderr.log"
	$started = [DateTimeOffset]::UtcNow
	$process = $null
	try {
		Set-Content -LiteralPath $stdout -Value "" -Encoding UTF8
		Set-Content -LiteralPath $stderr -Value "" -Encoding UTF8
		$startInfo = [System.Diagnostics.ProcessStartInfo]::new()
		$startInfo.FileName = "cmd.exe"
		$startInfo.Arguments = '/d /c "Launch Aphelion Content Tools.cmd"'
		$startInfo.WorkingDirectory = $RepositoryRoot
		$startInfo.UseShellExecute = $false
		$startInfo.CreateNoWindow = $true
		$startInfo.RedirectStandardOutput = $true
		$startInfo.RedirectStandardError = $true
		if (-not [string]::IsNullOrWhiteSpace($script:WindowsPowerShellModulePath)) {
			$startInfo.EnvironmentVariables["PSModulePath"] = $script:WindowsPowerShellModulePath
		}
		$process = [System.Diagnostics.Process]::new()
		$process.StartInfo = $startInfo
		[void]$process.Start()
		$outputTask = $process.StandardOutput.ReadLineAsync()
		$deadline = [DateTimeOffset]::UtcNow.AddSeconds($TimeoutSeconds)
		$url = $null
		while ([DateTimeOffset]::UtcNow -lt $deadline) {
			$process.Refresh()
			if ($outputTask.IsCompleted) {
				$line = $outputTask.Result
				if ($null -ne $line) {
					Add-Content -LiteralPath $stdout -Value $line -Encoding UTF8
					if ($line -match 'Aphelion Content Tools is running at (https?://\S+)\. Close') {
						$url = $Matches[1]
					}
					$outputTask = $process.StandardOutput.ReadLineAsync()
				}
				if ($url) {
					break
				}
			}
			if ($process.HasExited) { break }
			Start-Sleep -Milliseconds 200
		}
		if (-not $url) {
			$status = if ($process.HasExited) { "failed" } else { "timeout" }
			Add-GateResult $name "aphelion-content-tools" $status $(if ($process.HasExited) { $process.ExitCode } else { -1 }) (([DateTimeOffset]::UtcNow - $started).TotalSeconds) $stdout "Launcher did not emit its readiness marker."
			return $false
		}
		$response = Invoke-WebRequest -UseBasicParsing -Uri ($url.TrimEnd('/') + "/api/health") -TimeoutSec 10
		if ($response.StatusCode -ne 200) { throw "Launcher health endpoint returned HTTP $($response.StatusCode)." }
		Add-GateResult $name "aphelion-content-tools" "passed" 0 (([DateTimeOffset]::UtcNow - $started).TotalSeconds) $stdout "Readiness marker and /api/health returned successfully; launcher process tree was then stopped."
		return $true
	}
	catch {
		Add-GateResult $name "aphelion-content-tools" "failed" -1 (([DateTimeOffset]::UtcNow - $started).TotalSeconds) $stdout $_.Exception.Message
		return $false
	}
	finally {
		Stop-ProcessTree $process
		if ($process -and $process.HasExited) {
			$errorText = $process.StandardError.ReadToEnd()
			if (-not [string]::IsNullOrWhiteSpace($errorText)) { Add-Content -LiteralPath $stderr -Value $errorText -Encoding UTF8 }
		}
	}
}

function Invoke-Gate {
	param(
		[string] $Name,
		[string] $Repository,
		[string] $Executable,
		[string[]] $Arguments,
		[string] $WorkingDirectory,
		[hashtable] $Environment = @{},
		[int] $TimeoutSeconds = ($TimeoutMinutes * 60)
	)
	$gateRoot = Join-Path $script:EvidenceRoot "logs"
	New-Item -ItemType Directory -Force -Path $gateRoot | Out-Null
	$slug = ($Name -replace '[^A-Za-z0-9._-]', '-').ToLowerInvariant()
	$stdout = Join-Path $gateRoot "$slug.stdout.log"
	$stderr = Join-Path $gateRoot "$slug.stderr.log"
	$started = [DateTimeOffset]::UtcNow
	$previous = @{}
	try {
		foreach ($key in $Environment.Keys) {
			$previous[$key] = [Environment]::GetEnvironmentVariable($key, "Process")
			[Environment]::SetEnvironmentVariable($key, [string]$Environment[$key], "Process")
		}
		$argumentLine = ($Arguments | ForEach-Object { ConvertTo-ProcessArgument ([string]$_) }) -join ' '
		$process = Start-Process -FilePath $Executable -ArgumentList $argumentLine -WorkingDirectory $WorkingDirectory -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
		if (-not $process.WaitForExit($TimeoutSeconds * 1000)) {
			Stop-ProcessTree $process
			Add-GateResult $Name $Repository "timeout" -1 (([DateTimeOffset]::UtcNow - $started).TotalSeconds) $stdout "Exceeded ${TimeoutSeconds}s; process tree terminated."
			return $false
		}
		$process.WaitForExit()
		$combinedBytes = 0
		foreach ($path in @($stdout, $stderr)) {
			if (Test-Path -LiteralPath $path) { $combinedBytes += (Get-Item -LiteralPath $path).Length }
		}
		if ($combinedBytes -gt 16MB) {
			Add-GateResult $Name $Repository "failed" -1 (([DateTimeOffset]::UtcNow - $started).TotalSeconds) $stdout "Combined output exceeded 16 MiB."
			return $false
		}
		$status = if ($process.ExitCode -eq 0) { "passed" } else { "failed" }
		Add-GateResult $Name $Repository $status $process.ExitCode (([DateTimeOffset]::UtcNow - $started).TotalSeconds) $stdout
		return $process.ExitCode -eq 0
	}
	catch {
		Add-GateResult $Name $Repository "failed" -1 (([DateTimeOffset]::UtcNow - $started).TotalSeconds) $stdout $_.Exception.Message
		return $false
	}
	finally {
		foreach ($key in $Environment.Keys) {
			[Environment]::SetEnvironmentVariable($key, $previous[$key], "Process")
		}
	}
}

function Add-UnavailableGate {
	param([string] $Name, [string] $Repository, [string] $Reason)
	Add-GateResult $Name $Repository "unavailable" -1 0 "" $Reason
}

function Prepare-MeridianOfflineDependencies {
	param([string] $SourceRoot, [string] $AcceptanceRoot)
	$name = "Meridian offline dependency staging"
	$started = [DateTimeOffset]::UtcNow
	try {
		$bootstrapCache = Join-Path $SourceRoot "tools\bootstrap\.cache"
		$cutterCache = Join-Path $SourceRoot "tools\icon_cutter\cache"
		$dependencyText = Get-Content -LiteralPath (Join-Path $SourceRoot "dependencies.sh") -Raw
		$pins = @{}
		foreach ($pinName in @("BUN_VERSION", "PYTHON_VERSION", "CUTTER_VERSION")) {
			if ($dependencyText -notmatch "(?m)^export $pinName=([A-Za-z0-9._-]+)$") { throw "Could not read $pinName from dependencies.sh." }
			$pins[$pinName] = $Matches[1]
		}
		$cutterName = "hypnagogic$($pins.CUTTER_VERSION.Replace('.', '-')).exe"
		foreach ($required in @(
			(Join-Path $bootstrapCache "bun-v$($pins.BUN_VERSION)-x64\bun.exe"),
			(Join-Path $bootstrapCache "python-$($pins.PYTHON_VERSION)\python.exe"),
			(Join-Path $bootstrapCache "python-$($pins.PYTHON_VERSION)\Scripts\pip.exe"),
			(Join-Path $bootstrapCache "python-$($pins.PYTHON_VERSION)\requirements.txt"),
			(Join-Path $cutterCache $cutterName)
		)) {
			if (-not (Test-Path -LiteralPath $required -PathType Leaf)) { throw "Missing local offline prerequisite: $required" }
		}
		$destination = Join-Path $AcceptanceRoot "tools\icon_cutter\cache"
		New-Item -ItemType Directory -Force -Path $destination | Out-Null
		Copy-Item -LiteralPath (Join-Path $cutterCache $cutterName) -Destination $destination -Force
		Add-GateResult $name "Meridian-Rift" "passed" 0 (([DateTimeOffset]::UtcNow - $started).TotalSeconds) "" "Reused the source checkout's ignored bootstrap cache read-only and copied the pinned icon cutter into the disposable worktree."
		return $bootstrapCache
	}
	catch {
		Add-GateResult $name "Meridian-Rift" "failed" -1 (([DateTimeOffset]::UtcNow - $started).TotalSeconds) "" $_.Exception.Message
		return $null
	}
}

function Write-Evidence {
	$passed = @($script:GateResults | Where-Object { $_.status -eq "passed" }).Count
	$complete = $script:GateResults.Count -gt 0 -and $passed -eq $script:GateResults.Count
	$evidence = [ordered]@{
		schema_version = 1
		started_at = $script:RunStarted.ToString("o")
		completed_at = [DateTimeOffset]::UtcNow.ToString("o")
		network_policy = if ($AllowNetwork) { "allowed" } else { "offline" }
		stack_accepted = $complete
		gates = $script:GateResults
	}
	$parent = Split-Path -Parent $EvidencePath
	New-Item -ItemType Directory -Force -Path $parent | Out-Null
	$temporary = "$EvidencePath.tmp"
	$evidence | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $temporary -Encoding UTF8
	Move-Item -LiteralPath $temporary -Destination $EvidencePath -Force
	Write-Host "Aphelion stack evidence: $EvidencePath"
	return $complete
}

$AphelionRoot = Resolve-RequiredRoot $AphelionRoot "AphelionRoot"
if ([string]::IsNullOrWhiteSpace($EvidencePath)) {
	$EvidencePath = Join-Path $AphelionRoot ".artifacts\stack\evidence.json"
}
$EvidencePath = [System.IO.Path]::GetFullPath($EvidencePath)
$script:EvidenceRoot = Split-Path -Parent $EvidencePath

if ($PlanOnly) {
	Add-GateResult "Aphelion integration contracts" "AphelionDMM" "planned" 0 0 "" "Available in this checkout."
	foreach ($candidate in @(
		@("Installed Meridian-MCP conformance", "Meridian-MCP", $InstalledMcp),
		@("Content Tools repository gates", "aphelion-content-tools", $ContentToolsRoot),
		@("Staged Meridian inspection", "Meridian-Rift", $MeridianRiftRoot),
		@("Authoritative Meridian build", "Meridian-Rift", $MeridianRiftRoot)
	)) {
		if ([string]::IsNullOrWhiteSpace($candidate[2]) -or -not (Test-Path -LiteralPath $candidate[2])) {
			Add-UnavailableGate $candidate[0] $candidate[1] "Dependency is intentionally unavailable in the single-repository CI checkout."
		}
		else {
			Add-GateResult $candidate[0] $candidate[1] "planned" 0 0 "" "Dependency path is available."
		}
	}
	Write-Evidence | Out-Null
	exit 0
}

$MeridianMcpRoot = Resolve-RequiredRoot $MeridianMcpRoot "MeridianMcpRoot"
$ContentToolsRoot = Resolve-RequiredRoot $ContentToolsRoot "ContentToolsRoot"
$MeridianRiftRoot = Resolve-RequiredRoot $MeridianRiftRoot "MeridianRiftRoot"
if (-not (Test-Path -LiteralPath $InstalledMcp -PathType Leaf)) { throw "InstalledMcp does not exist: $InstalledMcp" }
$InstalledMcp = (Resolve-Path -LiteralPath $InstalledMcp).Path

$toolchainText = Get-Content -LiteralPath (Join-Path $MeridianMcpRoot "rust-toolchain.toml") -Raw
if ($toolchainText -notmatch 'channel\s*=\s*"([^"]+)"') { throw "Meridian-MCP rust-toolchain.toml has no channel." }
$meridianToolchain = $Matches[1]
$python = Join-Path $ContentToolsRoot ".venv\Scripts\python.exe"
if (-not (Test-Path -LiteralPath $python -PathType Leaf)) { $python = (Get-Command python.exe -ErrorAction Stop).Source }
$script:WindowsPowerShellModulePath = (& powershell.exe -NoProfile -Command '[Environment]::GetEnvironmentVariable("PSModulePath", "Process")').Trim()
if ([string]::IsNullOrWhiteSpace($script:WindowsPowerShellModulePath)) { throw "Windows PowerShell returned an empty PSModulePath." }
$goBin = (& go.exe env GOBIN).Trim()
if ([string]::IsNullOrWhiteSpace($goBin)) {
	$goPath = (& go.exe env GOPATH).Trim().Split([System.IO.Path]::PathSeparator)[0]
	$goBin = Join-Path $goPath "bin"
}
$goToolEnvironment = @{ PATH = $goBin + [System.IO.Path]::PathSeparator + $env:PATH }

[void](Invoke-Gate "Aphelion Go contracts" "AphelionDMM" "go.exe" @("test", "./...", "-count=1") $AphelionRoot)
$rustTarget = "1.82-x86_64-pc-windows-gnu"
$goToolEnvironment.RUST_TARGET = $rustTarget
[void](Invoke-Gate "Aphelion Windows resources" "AphelionDMM" "task.exe" @("task_win:gen_syso") $AphelionRoot $goToolEnvironment)
[void](Invoke-Gate "Aphelion cross-stack build" "AphelionDMM" "task.exe" @("build") $AphelionRoot $goToolEnvironment)

[void](Invoke-Gate "Meridian-MCP pinned tests" "Meridian-MCP" "rustup.exe" @("run", $meridianToolchain, "cargo", "test", "--locked", "--all-targets") $MeridianMcpRoot)

[void](Invoke-Gate "Content Tools Ruff" "aphelion-content-tools" $python @("-m", "ruff", "check", ".") $ContentToolsRoot)
[void](Invoke-Gate "Content Tools Pyright" "aphelion-content-tools" $python @("-m", "pyright") $ContentToolsRoot)
[void](Invoke-Gate "Content Tools Python tests" "aphelion-content-tools" $python @("-m", "unittest", "discover") $ContentToolsRoot)
[void](Invoke-Gate "Content Tools API contract" "aphelion-content-tools" "npm.cmd" @("--prefix", "webapp/frontend", "run", "gen:api") $ContentToolsRoot)
[void](Invoke-Gate "Content Tools frontend tests" "aphelion-content-tools" "npm.cmd" @("--prefix", "webapp/frontend", "test", "--", "--run") $ContentToolsRoot)
[void](Invoke-Gate "Content Tools frontend types" "aphelion-content-tools" "npm.cmd" @("--prefix", "webapp/frontend", "run", "typecheck") $ContentToolsRoot)
[void](Invoke-Gate "Content Tools production SPA" "aphelion-content-tools" "npm.cmd" @("--prefix", "webapp/frontend", "run", "build") $ContentToolsRoot)
if ($SkipLauncher) {
	Add-UnavailableGate "Content Tools real launcher" "aphelion-content-tools" "Skipped explicitly; prior launcher evidence does not make this run complete."
}
else {
	[void](Invoke-LauncherGate $ContentToolsRoot)
}

$stageRoot = Join-Path $script:EvidenceRoot ("stages\" + $script:RunID)
$stageEnvironment = @{
	APHELION_MERIDIAN_MCP_REAL = $InstalledMcp
	APHELION_MERIDIAN_RIFT_ROOT = $MeridianRiftRoot
	APHELION_MERIDIAN_STAGE_ROOT = $stageRoot
}
$stagePassed = Invoke-Gate "Installed MCP staged-map conformance" "AphelionDMM + Meridian-MCP + Meridian-Rift" "go.exe" @("test", "./internal/aphelion/integration/meridian", "-run", "TestRealMeridianStagingOnly", "-count=1", "-v") $AphelionRoot $stageEnvironment

$stageEvidence = Get-ChildItem -LiteralPath $stageRoot -Filter "*.mcp-evidence.json" -ErrorAction SilentlyContinue | Sort-Object LastWriteTimeUtc -Descending | Select-Object -First 1
if (-not $stagePassed -or -not $stageEvidence) {
	Add-UnavailableGate "Authoritative Meridian build" "Meridian-Rift" "Staged-map conformance did not produce evidence."
}
else {
	$stage = Get-Content -LiteralPath $stageEvidence.FullName -Raw | ConvertFrom-Json
	$acceptanceRoot = Join-Path $script:EvidenceRoot "meridian-acceptance"
	$resolvedEvidenceRoot = [System.IO.Path]::GetFullPath($script:EvidenceRoot)
	$resolvedAcceptanceRoot = [System.IO.Path]::GetFullPath($acceptanceRoot)
	if (-not $resolvedAcceptanceRoot.StartsWith($resolvedEvidenceRoot + [System.IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
		throw "Acceptance worktree escaped the evidence root."
	}
	$worktreeAdded = $false
	try {
		$worktreeAdded = Invoke-Gate "Meridian clean acceptance checkout" "Meridian-Rift" "git.exe" @("-C", $MeridianRiftRoot, "worktree", "add", "--detach", $acceptanceRoot, $stage.repository_revision) $AphelionRoot
		if ($worktreeAdded) {
			$target = Join-Path $acceptanceRoot "_maps\virtual_domains\test_only.dmm"
			Copy-Item -LiteralPath $stage.stage.map_file -Destination $target -Force
			$bootstrapCache = Prepare-MeridianOfflineDependencies $MeridianRiftRoot $acceptanceRoot
			if ($bootstrapCache) {
				$network = if ($AllowNetwork) { "allow" } else { "offline" }
				[void](Invoke-Gate "Authoritative Meridian build" "Meridian-Rift" "cmd.exe" @("/d", "/c", "RIFT_BUILD.cmd") $acceptanceRoot @{ MERIDIAN_RIFT_BUILD_NETWORK = $network; TG_BOOTSTRAP_CACHE = $bootstrapCache; PSModulePath = $script:WindowsPowerShellModulePath } ($TimeoutMinutes * 60))
			}
			else {
				Add-UnavailableGate "Authoritative Meridian build" "Meridian-Rift" "Local offline prerequisites were unavailable."
			}
		}
		else {
			Add-UnavailableGate "Authoritative Meridian build" "Meridian-Rift" "The clean detached acceptance checkout could not be created."
		}
	}
	finally {
		if ($worktreeAdded) {
			[void](Invoke-Gate "Meridian acceptance cleanup" "Meridian-Rift" "git.exe" @("-C", $MeridianRiftRoot, "worktree", "remove", "--force", $acceptanceRoot) $AphelionRoot @{} 120)
		}
	}
}

$complete = Write-Evidence
if (-not $complete) { exit 1 }
