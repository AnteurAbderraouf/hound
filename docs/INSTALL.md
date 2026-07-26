# Installation

hound ships as a one-line installer, as a single binary, and as a
Docker image. Pick the option that best matches how you already run
stuff at home.

- [0. One-line installer (Windows / Linux)](#0-one-line-installer)
- [1. Docker (any OS; best for NAS / homelab)](#1-docker)
- [2. Native binary (manual)](#2-native-binary)
- [3. Build from source](#3-build-from-source)
- [Configuration reference (env vars)](#configuration-reference)
- [Verifying it works](#verifying-it-works)

---

## 0. One-line installer

The fastest path. Handles LAN IP detection, port-53 availability
check, firewall rules, service registration, and a self-test. Router
configuration is the only remaining manual step (impossible to
automate cross-brand).

### Windows (PowerShell as Administrator)

```powershell
iwr -useb https://raw.githubusercontent.com/AnteurAbderraouf/hound/main/scripts/install.ps1 | iex
```

What it does:
- Downloads the latest hound release, extracts to `C:\Program Files\hound\`
- Creates a data folder at `C:\ProgramData\hound\` (the SQLite db lives there)
- Opens Windows Firewall for UDP+TCP :53 and TCP :8080
- Registers a Scheduled Task `hound` that runs as `SYSTEM` at boot
- Starts the task and runs a DNS self-test
- Prints your LAN IP + links to per-brand router setup docs

Preview without changes (safe):

```powershell
iwr -useb https://raw.githubusercontent.com/AnteurAbderraouf/hound/main/scripts/install.ps1 -OutFile install.ps1
.\install.ps1 -DryRun
```

Uninstall:

```powershell
iwr -useb https://raw.githubusercontent.com/AnteurAbderraouf/hound/main/scripts/uninstall.ps1 | iex
```

### Linux (systemd distros)

```bash
curl -fsSL https://raw.githubusercontent.com/AnteurAbderraouf/hound/main/scripts/install.sh | sudo bash
```

What it does:
- Downloads the arch-appropriate hound binary to `/usr/local/bin/hound`
- Creates system user `hound` and data folder `/var/lib/hound/`
- Writes `/etc/systemd/system/hound.service` with `CAP_NET_BIND_SERVICE`
  so hound can bind :53 without running as root
- Opens ufw / firewalld ports if either is present
- Enables and starts the service, runs a `dig` self-test
- Prints your LAN IP + router setup pointers

Preview:

```bash
curl -fsSL https://raw.githubusercontent.com/AnteurAbderraouf/hound/main/scripts/install.sh -o install.sh
sudo bash install.sh --dry-run
```

Uninstall:

```bash
curl -fsSL https://raw.githubusercontent.com/AnteurAbderraouf/hound/main/scripts/uninstall.sh | sudo bash
```

---

---

## 1. Docker

The published multi-arch image (`amd64` + `arm64`) lives on GHCR:

```bash
docker run -d \
  --name hound \
  --network host \
  -v hound-data:/data \
  ghcr.io/anteurabderraouf/hound:latest
```

Or, if you prefer the checked-in `docker-compose.yml`:

```bash
git clone https://github.com/AnteurAbderraouf/hound.git
cd hound
docker compose up -d
```

Both use `network_mode: host` so hound can see the real client IPs on
your LAN. Without host mode, every query would appear to come from the
Docker bridge, breaking per-device tracking.

That's it. hound is now listening on:

- **UDP/TCP :53** — the DNS server
- **HTTP :8080** — the web UI (open `http://<your-host-ip>:8080`)

To see logs: `docker compose logs -f hound`

To stop: `docker compose down`

The SQLite database persists in `./data/hound.db` on the host.

> **Note.** `network_mode: host` only works on Linux hosts. If you insist
> on running Docker on Windows or macOS, use `docker run -p 53:53/udp
> -p 53:53/tcp -p 8080:8080` instead — but expect every query to look
> like it came from the Docker gateway, which defeats per-device
> tracking. Prefer the native binary on Windows/macOS.

---

## 2. Native binary

Pre-built binaries for every release live on the
[releases page](https://github.com/AnteurAbderraouf/hound/releases).
Each archive contains `hound` (the server) and `hound-query` (the demo
tool) plus this documentation.

### Windows

Download `hound_<version>_windows_amd64.zip`, unzip, and double-click
`hound.exe`. A chromeless Edge/Chrome window opens with the UI.

Because DNS port 53 is a privileged port on Windows, you have two
choices:

- **Run as Administrator** so hound can bind :53 (easiest for real use)
- **Use a non-privileged port** for testing:
  ```powershell
  $env:HOUND_DNS_ADDR = ":5300"
  .\hound.exe
  ```
  and then point a device at `192.168.X.X:5300` for tests.

### macOS

Grab `hound_<version>_darwin_arm64.tar.gz` (Apple Silicon) or
`_darwin_amd64.tar.gz` (Intel). Same story: `sudo ./hound` to bind
:53, or use a high port for testing.

### Linux

`hound_<version>_linux_amd64.tar.gz` or `_linux_arm64.tar.gz`. Extract
and run. For a systemd service, see the sample unit in the
[releases discussion](https://github.com/AnteurAbderraouf/hound/discussions).

---

## 3. Build from source

You need Go 1.25 or newer.

```bash
git clone https://github.com/AnteurAbderraouf/hound.git
cd hound

# using make
make build       # -> ./bin/hound(.exe)

# or plain go
go build -o bin/hound ./cmd/hound
```

Run it:

```bash
# Linux/macOS
sudo ./bin/hound          # :53 needs root

# Windows
.\bin\hound.exe           # as Administrator

# Or on a non-privileged port (any OS, any user)
HOUND_DNS_ADDR=:5300 HOUND_HTTP_ADDR=:8085 ./bin/hound
```

---

## Configuration reference

hound is configured through environment variables (all optional).

| Variable            | Default                | Meaning                                                                 |
|---------------------|------------------------|-------------------------------------------------------------------------|
| `HOUND_DB_PATH`     | `hound.db`             | SQLite database file. Directory must be writable.                       |
| `HOUND_DNS_ADDR`    | `:53`                  | Address the DNS server listens on. Use `:5300` for testing without root.|
| `HOUND_HTTP_ADDR`   | `:8080`                | Address the web UI listens on.                                          |
| `HOUND_UPSTREAM`    | `1.1.1.1:53,8.8.8.8:53`| Comma-separated list of upstream DNS resolvers. Round-robin.            |
| `HOUND_HEADLESS`    | `` (unset)             | If set to any non-empty value, don't auto-open the UI window on launch. |

---

## Verifying it works

Once hound is running:

1. Open the UI at `http://<your-host-ip>:8080` (or the auto-opened window
   if running natively). You should see the retro-terminal UI, empty of
   queries.
2. From another machine on the LAN, send a DNS query to your host:
   ```bash
   dig @<your-host-ip> example.com          # linux/mac
   nslookup example.com <your-host-ip>       # windows
   ```
3. The query should appear in the "live" panel within 2 seconds, tagged
   with a category (or "other" for unknown domains).
4. Only after this smoke test works, [configure your router](ROUTER-SETUP.md)
   to use your host as the primary DNS.
