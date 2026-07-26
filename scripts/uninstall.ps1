#Requires -Version 5.1
#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Cleanly remove hound from Windows.

.DESCRIPTION
    Reverses everything scripts\install.ps1 did:
      - Stops and unregisters the 'hound' scheduled task
      - Removes the three Windows Firewall rules (DNS UDP/TCP, UI HTTP)
      - Deletes %ProgramFiles%\hound (binary + wrapper)
      - Optionally deletes %ProgramData%\hound (the SQLite database)

.PARAMETER InstallDir
    Where hound was installed. Defaults to %ProgramFiles%\hound.

.PARAMETER DataDir
    Where hound stored its data. Defaults to %ProgramData%\hound.

.PARAMETER KeepData
    Preserve the data folder (SQLite db, WAL sidecars). Handy if you
    plan to reinstall later and want to keep history.

.PARAMETER DryRun
    Print every action without executing.
#>
[CmdletBinding()]
param(
    [string]$InstallDir = "$env:ProgramFiles\hound",
    [string]$DataDir    = "$env:ProgramData\hound",
    [switch]$KeepData,
    [switch]$DryRun
)

$ErrorActionPreference = 'Continue'

function Write-Step { param($m) Write-Host ""; Write-Host "==> $m" -ForegroundColor Cyan }
function Write-Ok   { param($m) Write-Host "    [ok]   $m" -ForegroundColor Green }
function Write-Info { param($m) Write-Host "    [info] $m" }
function Write-Skip { param($m) Write-Host "    [skip] $m" -ForegroundColor DarkGray }

function Invoke-OrDry {
    param([string]$Description, [scriptblock]$Action)
    if ($DryRun) { Write-Info "would: $Description" } else { Write-Info $Description; & $Action }
}

# ---------- task -----------------------------------------------------------
Write-Step "removing scheduled task"

$task = Get-ScheduledTask -TaskName 'hound' -ErrorAction SilentlyContinue
if ($task) {
    Invoke-OrDry "stop scheduled task 'hound'" {
        Stop-ScheduledTask -TaskName 'hound' -ErrorAction SilentlyContinue
    }
    Invoke-OrDry "unregister scheduled task 'hound'" {
        Unregister-ScheduledTask -TaskName 'hound' -Confirm:$false -ErrorAction SilentlyContinue
    }
    Write-Ok "task removed"
} else {
    Write-Skip "no 'hound' task registered"
}

# ---------- firewall -------------------------------------------------------
Write-Step "removing firewall rules"

foreach ($name in @('hound DNS UDP', 'hound DNS TCP', 'hound UI HTTP')) {
    $rule = Get-NetFirewallRule -DisplayName $name -ErrorAction SilentlyContinue
    if ($rule) {
        Invoke-OrDry "remove rule '$name'" {
            Remove-NetFirewallRule -DisplayName $name -ErrorAction SilentlyContinue
        }
    } else {
        Write-Skip "'$name' not present"
    }
}

# ---------- binaries -------------------------------------------------------
Write-Step "removing install directory"

if (Test-Path $InstallDir) {
    Invoke-OrDry "delete $InstallDir" {
        Remove-Item -Path $InstallDir -Recurse -Force -ErrorAction SilentlyContinue
    }
} else {
    Write-Skip "$InstallDir does not exist"
}

# ---------- data -----------------------------------------------------------
Write-Step "data directory"

if (Test-Path $DataDir) {
    if ($KeepData) {
        Write-Skip "keeping $DataDir (via -KeepData)"
    } else {
        Invoke-OrDry "delete $DataDir (SQLite db + WAL)" {
            Remove-Item -Path $DataDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
} else {
    Write-Skip "$DataDir does not exist"
}

Write-Host ""
Write-Host "==> hound removed. Router DNS settings on the LAN side are unchanged -- revert those manually if needed." -ForegroundColor Green
