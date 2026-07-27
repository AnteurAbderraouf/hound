#Requires -Version 5.1
#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Install hound as a background service on Windows.

.DESCRIPTION
    Automates the "get hound running on port 53 for the whole LAN" flow:
      1. Detects the LAN IP
      2. Verifies port 53 is free
      3. Downloads the latest hound release (or reuses -BinaryPath if given)
      4. Installs to Program Files, creates a data folder in ProgramData
      5. Opens Windows Firewall for UDP/TCP :53 and TCP :8080
      6. Registers a Scheduled Task that starts hound at boot as SYSTEM
      7. Runs a self-test: sends a DNS query and confirms hound answered
      8. Prints the IP to configure in the router + links to setup docs

    Router configuration itself stays manual because every router brand
    exposes its DHCP/DNS settings differently. The script prints the
    exact IP and links to the ROUTER-SETUP.md section for the popular
    boxes so the last step takes 60 seconds.

.PARAMETER BinaryPath
    Path to an existing hound.exe. If omitted, the latest release is
    downloaded from GitHub.

.PARAMETER InstallDir
    Where to place hound.exe. Defaults to %ProgramFiles%\hound.

.PARAMETER DataDir
    Where hound will store its SQLite database. Defaults to
    %ProgramData%\hound.

.PARAMETER DryRun
    Print every action without executing. Safe to run for previewing.

.PARAMETER SkipFirewall
    Do not touch Windows Firewall (advanced, only if you already
    configured rules yourself).

.PARAMETER SkipService
    Do not register the scheduled task (advanced, only if you plan to
    run hound manually).

.EXAMPLE
    # Fresh install using the latest published release:
    .\install.ps1

.EXAMPLE
    # Preview what would happen without doing anything:
    .\install.ps1 -DryRun

.EXAMPLE
    # Install using a binary you already downloaded:
    .\install.ps1 -BinaryPath C:\Downloads\hound.exe
#>
[CmdletBinding()]
param(
    [string]$BinaryPath = "",
    [string]$InstallDir = "$env:ProgramFiles\hound",
    [string]$DataDir    = "$env:ProgramData\hound",
    [switch]$DryRun,
    [switch]$SkipFirewall,
    [switch]$SkipService
)

$ErrorActionPreference = 'Stop'

# ---------- helpers ---------------------------------------------------------

function Write-Step {
    param([string]$Msg)
    Write-Host ""
    Write-Host "==> $Msg" -ForegroundColor Cyan
}

function Write-Ok    { param($m) Write-Host "    [ok]   $m" -ForegroundColor Green }
function Write-Info  { param($m) Write-Host "    [info] $m" }
function Write-Warn2 { param($m) Write-Host "    [warn] $m" -ForegroundColor Yellow }
function Write-Fail  { param($m) Write-Host "    [fail] $m" -ForegroundColor Red }

function Invoke-OrDry {
    param(
        [string]$Description,
        [scriptblock]$Action
    )
    if ($DryRun) {
        Write-Info "would: $Description"
    } else {
        Write-Info $Description
        & $Action
    }
}

# ---------- 1. environment detection ---------------------------------------

Write-Step "detecting environment"

# LAN IP: prefer a non-loopback, non-link-local IPv4 on an "Up" interface
$lanIP = $null
$candidates = Get-NetIPAddress -AddressFamily IPv4 -PrefixOrigin Dhcp -ErrorAction SilentlyContinue |
    Where-Object { $_.IPAddress -notlike '127.*' -and $_.IPAddress -notlike '169.254.*' } |
    Sort-Object -Property InterfaceMetric
if ($candidates) {
    $lanIP = $candidates[0].IPAddress
    $iface = $candidates[0].InterfaceAlias
    Write-Ok "LAN IP: $lanIP (via $iface)"
} else {
    # fallback: any non-loopback IPv4
    $any = Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
        Where-Object { $_.IPAddress -notlike '127.*' -and $_.IPAddress -notlike '169.254.*' }
    if ($any) {
        $lanIP = $any[0].IPAddress
        Write-Ok "LAN IP: $lanIP (fallback)"
    } else {
        Write-Fail "no LAN IPv4 address found. Are you connected to a network?"
        exit 1
    }
}

# Port 53 availability
$udp53 = Get-NetUDPEndpoint -LocalPort 53 -ErrorAction SilentlyContinue
$tcp53 = Get-NetTCPConnection -LocalPort 53 -State Listen -ErrorAction SilentlyContinue
if ($udp53 -or $tcp53) {
    Write-Fail "port 53 is already in use. Something (WSL2, another DNS server, ...) is listening. Free it up and re-run."
    if ($udp53) { $udp53 | Format-Table -AutoSize | Out-String | Write-Host }
    if ($tcp53) { $tcp53 | Format-Table -AutoSize | Out-String | Write-Host }
    exit 1
}
Write-Ok "port 53 is free"

# Port 8080 (informational only)
$tcp8080 = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue
if ($tcp8080) {
    Write-Warn2 "port 8080 already in use -- the UI will conflict. Consider stopping the other listener."
} else {
    Write-Ok "port 8080 is free"
}

# ---------- 2. binary acquisition -------------------------------------------

Write-Step "acquiring hound binary"

$targetExe = Join-Path $InstallDir 'hound.exe'

if ($BinaryPath) {
    if (-not (Test-Path $BinaryPath)) {
        Write-Fail "-BinaryPath '$BinaryPath' does not exist"
        exit 1
    }
    Write-Ok "using local binary: $BinaryPath"
    $sourceExe = (Resolve-Path $BinaryPath).Path
} else {
    Write-Info "fetching latest release info from github.com"
    $releaseUrl = "https://api.github.com/repos/AnteurAbderraouf/hound/releases/latest"
    try {
        $release = Invoke-RestMethod -Uri $releaseUrl -Headers @{ 'Accept' = 'application/vnd.github+json' } -TimeoutSec 30
    } catch {
        Write-Fail "could not reach GitHub releases API: $($_.Exception.Message)"
        exit 1
    }
    $asset = $release.assets | Where-Object { $_.name -like 'hound_*_windows_amd64.zip' } | Select-Object -First 1
    if (-not $asset) {
        Write-Fail "no windows_amd64.zip in release $($release.tag_name)"
        exit 1
    }
    Write-Ok "latest release: $($release.tag_name) -- $($asset.name)"

    $tmp = New-Item -ItemType Directory -Path (Join-Path $env:TEMP "hound-install-$(Get-Random)") -Force
    $zipPath = Join-Path $tmp $asset.name

    Invoke-OrDry "download $($asset.browser_download_url)" {
        Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $zipPath -UseBasicParsing
    }
    if (-not $DryRun) {
        Expand-Archive -Path $zipPath -DestinationPath $tmp -Force
        $found = Get-ChildItem -Path $tmp -Filter 'hound.exe' -Recurse | Select-Object -First 1
        if (-not $found) {
            Write-Fail "hound.exe not found in downloaded archive"
            exit 1
        }
        $sourceExe = $found.FullName
        Write-Ok "extracted hound.exe (v$($release.tag_name))"
    } else {
        $sourceExe = "<downloaded hound.exe>"
    }
}

# ---------- 3. filesystem layout -------------------------------------------

Write-Step "preparing filesystem"

Invoke-OrDry "create $InstallDir" { New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null }
Invoke-OrDry "create $DataDir"    { New-Item -ItemType Directory -Path $DataDir    -Force | Out-Null }

# Stop the task if it already exists so the exe isn't locked while we copy.
$existing = Get-ScheduledTask -TaskName 'hound' -ErrorAction SilentlyContinue
if ($existing) {
    Invoke-OrDry "stop existing hound task before overwriting binary" {
        Stop-ScheduledTask -TaskName 'hound' -ErrorAction SilentlyContinue
        Start-Sleep -Seconds 1
    }
}

Invoke-OrDry "copy $sourceExe -> $targetExe" {
    Copy-Item -Path $sourceExe -Destination $targetExe -Force
}

# ---------- 4. firewall -----------------------------------------------------

if ($SkipFirewall) {
    Write-Step "skipping firewall (via -SkipFirewall)"
} else {
    Write-Step "opening Windows Firewall for DNS + UI"
    $rules = @(
        @{ Name = 'hound DNS UDP'; Protocol = 'UDP'; Port = 53 }
        @{ Name = 'hound DNS TCP'; Protocol = 'TCP'; Port = 53 }
        @{ Name = 'hound UI HTTP'; Protocol = 'TCP'; Port = 8080 }
    )
    foreach ($rule in $rules) {
        $existingRule = Get-NetFirewallRule -DisplayName $rule.Name -ErrorAction SilentlyContinue
        if ($existingRule) {
            Write-Info "rule '$($rule.Name)' already exists -- skipping"
            continue
        }
        $desc = "add firewall rule '$($rule.Name)' ($($rule.Protocol) port $($rule.Port))"
        Invoke-OrDry $desc {
            New-NetFirewallRule -DisplayName $rule.Name -Direction Inbound -Protocol $rule.Protocol -LocalPort $rule.Port -Action Allow -Profile Domain,Private | Out-Null
        }
    }
}

# ---------- 5. scheduled task (service) ------------------------------------

if ($SkipService) {
    Write-Step "skipping scheduled task (via -SkipService)"
} else {
    Write-Step "registering hound scheduled task (auto-start on boot as SYSTEM)"

    # A wrapper cmd file that sets env vars then runs hound. Cleaner than
    # stuffing env into the Task action, and easy to edit if the user
    # ever wants to tweak the config.
    $wrapper = Join-Path $InstallDir 'run-hound.cmd'
    $wrapperContent = @"
@echo off
REM Auto-generated by scripts\install.ps1 -- edit env values below if needed.
set HOUND_HEADLESS=1
set HOUND_DB_PATH=$DataDir\hound.db
set HOUND_DNS_ADDR=:53
set HOUND_HTTP_ADDR=:8080
set HOUND_UPSTREAM=1.1.1.1:53,8.8.8.8:53
"$targetExe"
"@
    Invoke-OrDry "write wrapper $wrapper" {
        Set-Content -Path $wrapper -Value $wrapperContent -Encoding ASCII
    }

    if ($existing) {
        Invoke-OrDry "unregister existing hound scheduled task" {
            Unregister-ScheduledTask -TaskName 'hound' -Confirm:$false -ErrorAction SilentlyContinue
        }
    }

    Invoke-OrDry "register scheduled task 'hound'" {
        $action    = New-ScheduledTaskAction -Execute $wrapper
        $trigger   = New-ScheduledTaskTrigger -AtStartup
        $principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
        $settings  = New-ScheduledTaskSettingsSet `
            -AllowStartIfOnBatteries `
            -DontStopIfGoingOnBatteries `
            -StartWhenAvailable `
            -RestartInterval (New-TimeSpan -Minutes 1) `
            -RestartCount 5 `
            -ExecutionTimeLimit (New-TimeSpan -Hours 0)
        Register-ScheduledTask -TaskName 'hound' -Description 'hound DNS monitor' -Action $action -Trigger $trigger -Principal $principal -Settings $settings | Out-Null
    }

    Invoke-OrDry "start hound scheduled task now" {
        Start-ScheduledTask -TaskName 'hound'
        Start-Sleep -Seconds 3
    }
}

# ---------- 6. self-test ----------------------------------------------------

Write-Step "self-test"

if ($DryRun) {
    Write-Info "would: send test DNS query to 127.0.0.1:53"
} else {
    try {
        $answer = Resolve-DnsName -Name 'example.com' -Server 127.0.0.1 -DnsOnly -Type A -ErrorAction Stop
        if ($answer -and $answer.IPAddress) {
            Write-Ok "hound answered example.com -> $($answer[0].IPAddress)"
        } else {
            Write-Warn2 "query returned no A record -- hound may still be starting"
        }
    } catch {
        Write-Warn2 "self-test failed: $($_.Exception.Message)"
        Write-Warn2 "hound may need a few more seconds to bind. Check the task in Task Scheduler."
    }
}

# ---------- 7. next steps ---------------------------------------------------

Write-Step "next steps -- LAST manual bit (router)"

# --- resolve the PC MAC for the DHCP reservation form -----------------------

$macAddress = "<unknown -- check via: Get-NetAdapter>"
try {
    $netEntry = Get-NetIPAddress -IPAddress $lanIP -AddressFamily IPv4 -ErrorAction Stop
    $adapter  = Get-NetAdapter -InterfaceIndex $netEntry.InterfaceIndex -ErrorAction Stop
    if ($adapter.MacAddress) {
        # convert Windows dash format AA-BB-CC-DD-EE-FF to colon format
        $macAddress = ($adapter.MacAddress -replace '-', ':').ToLower()
    }
} catch {
    Write-Warn2 "could not resolve MAC for $lanIP -- run `Get-NetAdapter` manually to find it"
}

# --- suggest a reservation IP inside the subnet -----------------------------
# .50 is a commonly-free slot in typical home DHCP pools (2..254). If the
# router refuses ("IP already in use"), the user can pick .60, .70, .100...
$subnet       = ($lanIP -split '\.')[0..2] -join '.'
$suggestedIP  = "$subnet.50"

# ---------------------------------------------------------------------------

Write-Host ""
Write-Host "  hound is installed and running as a background task." -ForegroundColor White
Write-Host ""
Write-Host "    UI              : http://localhost:8080"
Write-Host "    data folder     : $DataDir"
Write-Host "    binary          : $targetExe"
Write-Host "    scheduled task  : 'hound' (runs at boot as SYSTEM)"
Write-Host ""
Write-Host "  Router configuration -- 2 steps in your router's admin panel:" -ForegroundColor White
Write-Host ""
Write-Host "  ---- Step 1: DHCP static IP reservation ----" -ForegroundColor Cyan
Write-Host "    Find the 'DHCP Static IP' / 'DHCP Reservation' section and add:"
Write-Host ""
Write-Host "      MAC address     : $macAddress" -ForegroundColor Yellow
Write-Host "      Reserved IP     : $suggestedIP" -ForegroundColor Yellow
Write-Host ""
Write-Host "    Why: your PC's IP must never change, otherwise the LAN's DNS"
Write-Host "    setting (Step 2) will point to a dead address after any reboot."
Write-Host ""
Write-Host "    If the router refuses with 'IP already in use', pick another"
Write-Host "    IP in the DHCP range (.60, .70, .100, ...) and use THAT in Step 2."
Write-Host ""
Write-Host "    After adding the reservation, force this PC to pick it up:"
Write-Host "      ipconfig /release"
Write-Host "      ipconfig /renew"
Write-Host ""
Write-Host "  ---- Step 2: DHCP DNS servers ----" -ForegroundColor Cyan
Write-Host "    In the same DHCP section, set the DNS handed out to devices:"
Write-Host ""
Write-Host "      PRIMARY DNS     : $suggestedIP   (your PC running hound)" -ForegroundColor Yellow
Write-Host "      SECONDARY DNS   : 1.1.1.1        (Cloudflare fallback)" -ForegroundColor Yellow
Write-Host ""
Write-Host "    Fallback matters: when your PC is off, the LAN falls back to"
Write-Host "    Cloudflare so no one loses internet. You just miss tracking"
Write-Host "    during that window."
Write-Host ""
Write-Host "  Per-brand walkthroughs (Freebox / Livebox / Bbox / SFR / TP-Link /" -ForegroundColor White
Write-Host "  Netgear / ASUS / FiberHome / generic):"
Write-Host "    https://github.com/AnteurAbderraouf/hound/blob/main/docs/ROUTER-SETUP.md"
Write-Host ""
Write-Host "  Recommended smoke test before touching the router: point ONE" -ForegroundColor White
Write-Host "  device (your phone) at the reserved IP manually:"
Write-Host "    Phone -> WiFi -> DNS -> Manual -> $suggestedIP + 1.1.1.1"
Write-Host "  If queries appear in the UI, roll it out to the router via Step 2."
Write-Host ""
Write-Host "  To stop or uninstall later:" -ForegroundColor White
Write-Host "    Stop-ScheduledTask -TaskName hound"
Write-Host "    .\uninstall.ps1                # full clean removal"
Write-Host ""

Write-Host "==> done." -ForegroundColor Green
