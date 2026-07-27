#Requires -Version 5.1
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

    Works in both invocation modes:
      - script file:   .\install.ps1
      - iwr | iex:     iwr -useb <url> | iex
    Errors NEVER close the parent PowerShell window: the body is wrapped
    in a function whose exceptions are caught and printed by a top-level
    try/catch. That's on purpose -- earlier versions used `exit 1`, which
    silently closed the iex'ing session before the user could read why.

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

.PARAMETER ForcePort53
    When port 53 is already in use, terminate whatever is holding it
    without prompting. Safe DNS-server processes are always
    auto-terminated regardless of this flag; the flag additionally
    accepts WSL2 shutdown and third-party DNS server stops without
    interactive confirmation. Does not touch svchost / VPN / unknown
    processes even with this flag on -- those still get a hard refuse.

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
    [switch]$SkipService,
    [switch]$ForcePort53
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

function Test-IsAdministrator {
    $identity  = [System.Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object System.Security.Principal.WindowsPrincipal($identity)
    return $principal.IsInRole([System.Security.Principal.WindowsBuiltInRole]::Administrator)
}

# Get the list of processes currently holding UDP or TCP :53. Each entry
# carries the process name, PID, path, and which protocol it grabbed.
function Get-Port53Holders {
    $holders = @{}

    foreach ($e in (Get-NetUDPEndpoint -LocalPort 53 -ErrorAction SilentlyContinue)) {
        $holders[[int]$e.OwningProcess] = @{ pid = [int]$e.OwningProcess; protocols = @('UDP') }
    }
    foreach ($e in (Get-NetTCPConnection -LocalPort 53 -State Listen -ErrorAction SilentlyContinue)) {
        $procId = [int]$e.OwningProcess
        if ($holders.ContainsKey($procId)) {
            $holders[$procId].protocols += 'TCP'
        } else {
            $holders[$procId] = @{ pid = $procId; protocols = @('TCP') }
        }
    }

    $out = @()
    foreach ($h in $holders.Values) {
        $proc = Get-Process -Id $h.pid -ErrorAction SilentlyContinue
        $out += [PSCustomObject]@{
            Pid         = $h.pid
            ProcessName = if ($proc) { $proc.ProcessName } else { '<gone>' }
            Path        = if ($proc) { $proc.Path } else { $null }
            Protocols   = ($h.protocols | Sort-Object -Unique) -join '+'
        }
    }
    return $out
}

# Classify a port-53 holder so Resolve-Port53Conflict knows what to do
# with it. Returns one of:
#   'hound'       -- leftover hound.exe, safe to auto-kill
#   'wsl'         -- WSL2 host / distro, prompt to run `wsl --shutdown`
#   'dns-server'  -- known DNS server (AdGuard/PiHole/dnsmasq/...), prompt
#                    to stop
#   'system'      -- svchost / Dnscache / system service -- refuse
#   'unknown'     -- something else -- refuse
function Get-Port53HolderKind {
    param($Holder)
    $name = $Holder.ProcessName
    if (-not $name) { return 'unknown' }
    $name = $name.ToLower()
    switch -Wildcard ($name) {
        'hound'          { return 'hound' }
        'wsl'            { return 'wsl' }
        'wslservice'     { return 'wsl' }
        'wslhost'        { return 'wsl' }
        'vmmemwsl'       { return 'wsl' }
        'vmmem'          { return 'wsl' }
        'adguardhome'    { return 'dns-server' }
        'pihole-ftl'     { return 'dns-server' }
        'dnsmasq'        { return 'dns-server' }
        'dnscrypt-proxy' { return 'dns-server' }
        'nextdns'        { return 'dns-server' }
        'unbound'        { return 'dns-server' }
        'coredns'        { return 'dns-server' }
        'svchost'        { return 'system' }
        default          { return 'unknown' }
    }
}

# Ask a yes/no question and return $true on y/Y. In non-interactive
# contexts (Read-Host would block forever) callers should pass -Force to
# skip.
function Confirm-Action {
    param([string]$Question)
    $answer = Read-Host "$Question [y/N]"
    return ($answer -match '^[yY]')
}

# Attempt to free UDP+TCP :53. Handles safe cases automatically, prompts
# for risky-but-recoverable ones, refuses for system/unknown. Returns
# quietly when :53 ends up free; throws otherwise.
function Resolve-Port53Conflict {
    param(
        [switch]$Force,
        [switch]$DryRun
    )

    $holders = Get-Port53Holders
    if ($holders.Count -eq 0) { return }

    Write-Warn2 "port 53 is currently held by:"
    $holders | ForEach-Object {
        $shownPath = if ($_.Path) { $_.Path } else { '<no path>' }
        Write-Host ("        {0,-18} PID {1,-6} [{2}]  {3}" -f `
            $_.ProcessName, $_.Pid, $_.Protocols, $shownPath)
    }

    foreach ($h in $holders) {
        $kind = Get-Port53HolderKind $h
        switch ($kind) {
            'hound' {
                Write-Info "leftover hound.exe -- auto-terminating (PID $($h.Pid))"
                if (-not $DryRun) {
                    Stop-Process -Id $h.Pid -Force -ErrorAction SilentlyContinue
                }
            }
            'wsl' {
                $doIt = $Force -or (Confirm-Action "WSL2 is holding :53. Run 'wsl --shutdown' to free it? (will terminate all running WSL distros)")
                if ($doIt) {
                    Write-Info "shutting WSL down"
                    if (-not $DryRun) {
                        & wsl.exe --shutdown 2>$null | Out-Null
                    }
                } else {
                    throw "port 53 held by WSL2 and user declined shutdown"
                }
            }
            'dns-server' {
                $doIt = $Force -or (Confirm-Action "$($h.ProcessName) (a DNS server) is holding :53. Stop it?")
                if ($doIt) {
                    Write-Info "stopping $($h.ProcessName) (PID $($h.Pid))"
                    if (-not $DryRun) {
                        Stop-Process -Id $h.Pid -Force -ErrorAction SilentlyContinue
                    }
                } else {
                    throw "port 53 held by $($h.ProcessName) and user declined to stop it"
                }
            }
            'system' {
                Write-Fail "$($h.ProcessName) (PID $($h.Pid)) is a Windows system service."
                Write-Fail "install.ps1 refuses to terminate system processes."
                Write-Fail "If it's the DNS Client service (Dnscache), disabling it will break Windows"
                Write-Fail "DNS resolution -- don't do that on your main PC. Try installing hound on a"
                Write-Fail "dedicated always-on machine (mini PC, Raspberry Pi, Docker on NAS) instead."
                throw "port 53 held by system service $($h.ProcessName)"
            }
            default {
                Write-Fail "$($h.ProcessName) (PID $($h.Pid)) at $($h.Path) is holding :53."
                Write-Fail "install.ps1 does not know this process and will not terminate it blindly."
                Write-Fail "Manually stop it (Task Manager or Stop-Process -Id $($h.Pid) -Force) then re-run,"
                Write-Fail "or open an issue at https://github.com/AnteurAbderraouf/hound/issues if"
                Write-Fail "you think this process should be auto-handled."
                throw "port 53 held by unknown process $($h.ProcessName)"
            }
        }
    }

    if ($DryRun) { return }

    # Give the OS a moment to actually release the socket.
    Start-Sleep -Seconds 2
    $stillBusy = Get-Port53Holders
    if ($stillBusy.Count -gt 0) {
        Write-Fail "port 53 is still busy after cleanup attempts:"
        $stillBusy | Format-Table -AutoSize | Out-String | Write-Host
        throw "port 53 could not be freed automatically -- manual intervention needed"
    }
}

# ---------- core install (wrapped so `throw` never kills the parent shell) --

function Invoke-HoundInstall {
    [CmdletBinding()]
    param(
        [string]$BinaryPath,
        [string]$InstallDir,
        [string]$DataDir,
        [switch]$DryRun,
        [switch]$SkipFirewall,
        [switch]$SkipService,
        [switch]$ForcePort53
    )

    function Invoke-OrDry {
        param([string]$Description, [scriptblock]$Action)
        if ($DryRun) {
            Write-Info "would: $Description"
        } else {
            Write-Info $Description
            & $Action
        }
    }

    # ---- admin check --------------------------------------------------------
    if (-not (Test-IsAdministrator)) {
        throw "install.ps1 requires Administrator privileges. Right-click Windows PowerShell -> 'Run as administrator', then retry."
    }

    # ---- 1. environment detection ------------------------------------------
    Write-Step "detecting environment"

    $lanIP = $null
    $iface = ""
    $candidates = Get-NetIPAddress -AddressFamily IPv4 -PrefixOrigin Dhcp -ErrorAction SilentlyContinue |
        Where-Object { $_.IPAddress -notlike '127.*' -and $_.IPAddress -notlike '169.254.*' } |
        Sort-Object -Property InterfaceMetric
    if ($candidates) {
        $lanIP = $candidates[0].IPAddress
        $iface = $candidates[0].InterfaceAlias
        Write-Ok "LAN IP: $lanIP (via $iface)"
    } else {
        $any = Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
            Where-Object { $_.IPAddress -notlike '127.*' -and $_.IPAddress -notlike '169.254.*' }
        if ($any) {
            $lanIP = $any[0].IPAddress
            Write-Ok "LAN IP: $lanIP (fallback)"
        } else {
            throw "no LAN IPv4 address found. Are you connected to a network?"
        }
    }

    # Port 53 with auto-recovery for the common culprits (WSL2, leftover
    # hound processes, other DNS servers). Anything unknown -- especially
    # svchost -- gets a hard refuse so we never break the host system.
    Resolve-Port53Conflict -Force:$ForcePort53 -DryRun:$DryRun
    Write-Ok "port 53 is free"

    $tcp8080 = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue
    if ($tcp8080) {
        Write-Warn2 "port 8080 already in use -- the UI will conflict. Consider stopping the other listener."
    } else {
        Write-Ok "port 8080 is free"
    }

    # ---- 2. binary acquisition --------------------------------------------
    Write-Step "acquiring hound binary"

    $targetExe = Join-Path $InstallDir 'hound.exe'
    $sourceExe = $null

    if ($BinaryPath) {
        if (-not (Test-Path $BinaryPath)) {
            throw "-BinaryPath '$BinaryPath' does not exist"
        }
        Write-Ok "using local binary: $BinaryPath"
        $sourceExe = (Resolve-Path $BinaryPath).Path
    } else {
        Write-Info "fetching latest release info from github.com"
        $releaseUrl = "https://api.github.com/repos/AnteurAbderraouf/hound/releases/latest"
        try {
            $release = Invoke-RestMethod -Uri $releaseUrl -Headers @{ 'Accept' = 'application/vnd.github+json' } -TimeoutSec 30
        } catch {
            throw "could not reach GitHub releases API: $($_.Exception.Message)"
        }
        $asset = $release.assets | Where-Object { $_.name -like 'hound_*_windows_amd64.zip' } | Select-Object -First 1
        if (-not $asset) {
            throw "no windows_amd64.zip in release $($release.tag_name)"
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
                throw "hound.exe not found in downloaded archive"
            }
            $sourceExe = $found.FullName
            Write-Ok "extracted hound.exe (v$($release.tag_name))"
        } else {
            $sourceExe = "<downloaded hound.exe>"
        }
    }

    # ---- 3. filesystem layout ---------------------------------------------
    Write-Step "preparing filesystem"

    Invoke-OrDry "create $InstallDir" { New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null }
    Invoke-OrDry "create $DataDir"    { New-Item -ItemType Directory -Path $DataDir    -Force | Out-Null }

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

    # ---- 4. firewall ------------------------------------------------------
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

    # ---- 5. scheduled task (service) --------------------------------------
    if ($SkipService) {
        Write-Step "skipping scheduled task (via -SkipService)"
    } else {
        Write-Step "registering hound scheduled task (auto-start on boot as SYSTEM)"

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

    # ---- 6. self-test -----------------------------------------------------
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

    # ---- 7. next steps ----------------------------------------------------
    Write-Step "next steps -- LAST manual bit (router)"

    $macAddress = "<unknown -- check via: Get-NetAdapter>"
    try {
        $netEntry = Get-NetIPAddress -IPAddress $lanIP -AddressFamily IPv4 -ErrorAction Stop
        $adapter  = Get-NetAdapter -InterfaceIndex $netEntry.InterfaceIndex -ErrorAction Stop
        if ($adapter.MacAddress) {
            $macAddress = ($adapter.MacAddress -replace '-', ':').ToLower()
        }
    } catch {
        Write-Warn2 "could not resolve MAC for $lanIP -- run `Get-NetAdapter` manually to find it"
    }

    $subnet      = ($lanIP -split '\.')[0..2] -join '.'
    $suggestedIP = "$subnet.50"

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
}

# ---------- entry point: catch throws so we never close the parent shell ----

try {
    Invoke-HoundInstall `
        -BinaryPath   $BinaryPath `
        -InstallDir   $InstallDir `
        -DataDir      $DataDir `
        -DryRun:      $DryRun `
        -SkipFirewall:$SkipFirewall `
        -SkipService: $SkipService `
        -ForcePort53: $ForcePort53

    Write-Host ""
    Write-Host "==> done." -ForegroundColor Green
} catch {
    Write-Host ""
    Write-Host "==> installation aborted." -ForegroundColor Red
    Write-Host "    reason: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host ""
    Write-Host "    See the output above for details. Re-run with -DryRun to preview" -ForegroundColor DarkGray
    Write-Host "    without touching the system, or open an issue at:" -ForegroundColor DarkGray
    Write-Host "      https://github.com/AnteurAbderraouf/hound/issues" -ForegroundColor DarkGray
    Write-Host ""
    # Deliberately no `exit` here: when this script is invoked via
    # `iwr | iex`, the body runs in the caller's scope and `exit` would
    # close the user's PowerShell window before they can read the error.
}
