#!/usr/bin/env bash
# hound Linux install script.
# -----------------------------------------------------------------------------
# What it does:
#   1. Detects the LAN IP + verifies port 53 is free
#   2. Downloads the latest hound release for the host architecture
#      (or uses a local binary via --binary)
#   3. Installs to /usr/local/bin/hound
#   4. Creates /var/lib/hound (SQLite db)
#   5. Writes a systemd unit at /etc/systemd/system/hound.service
#   6. Enables + starts the unit
#   7. Runs a self-test with dig
#   8. Prints the IP + link to ROUTER-SETUP.md
#
# Router config stays manual (every brand's UI is different).
# -----------------------------------------------------------------------------
set -euo pipefail

BINARY_PATH=""
DRY_RUN=false
SKIP_FIREWALL=false
SKIP_SERVICE=false

usage() {
    cat <<EOF
Usage: sudo bash install.sh [options]

Options:
  --binary PATH       Use an existing hound binary instead of downloading.
  --dry-run           Print every action without executing.
  --skip-firewall     Do not touch ufw/firewalld.
  --skip-service      Do not register/start the systemd unit.
  -h, --help          Show this help.
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --binary)          BINARY_PATH="$2"; shift 2 ;;
        --dry-run)         DRY_RUN=true; shift ;;
        --skip-firewall)   SKIP_FIREWALL=true; shift ;;
        --skip-service)    SKIP_SERVICE=true; shift ;;
        -h|--help)         usage; exit 0 ;;
        *) echo "unknown arg: $1"; usage; exit 1 ;;
    esac
done

# ---------- helpers ---------------------------------------------------------

step()  { echo; echo "==> $*"; }
ok()    { echo "    [ok]   $*"; }
info()  { echo "    [info] $*"; }
warn()  { echo "    [warn] $*" >&2; }
fail()  { echo "    [fail] $*" >&2; exit 1; }
do_or_dry() {
    if $DRY_RUN; then
        info "would: $1"
    else
        info "$1"
        shift
        "$@"
    fi
}

# ---------- 0. sanity ------------------------------------------------------

if [[ $EUID -ne 0 ]]; then
    fail "this script must be run as root (use sudo)"
fi

if ! command -v systemctl >/dev/null 2>&1; then
    fail "systemd required. Non-systemd distros aren't supported by this script."
fi

# ---------- 1. detect ------------------------------------------------------

step "detecting environment"

# Prefer the default route's source IP
LAN_IP=$(ip route get 1.1.1.1 2>/dev/null | awk '/src/ {for(i=1;i<=NF;i++) if($i=="src") print $(i+1)}' | head -n1)
if [[ -z "$LAN_IP" ]]; then
    LAN_IP=$(hostname -I 2>/dev/null | awk '{print $1}')
fi
if [[ -z "$LAN_IP" || "$LAN_IP" == 127.* ]]; then
    fail "no LAN IPv4 address found."
fi
ok "LAN IP: $LAN_IP"

# port 53 check
if ss -lnu 2>/dev/null | grep -qE ':53\s'; then
    fail "UDP :53 is already in use (systemd-resolved? dnsmasq? another DNS server?). Free it before installing."
fi
if ss -lnt 2>/dev/null | grep -qE ':53\s'; then
    fail "TCP :53 is already in use."
fi
ok "port 53 is free"

# arch
case "$(uname -m)" in
    x86_64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) fail "unsupported architecture: $(uname -m). Pre-built binaries: amd64, arm64." ;;
esac
ok "arch: $ARCH"

# ---------- 2. binary ------------------------------------------------------

step "acquiring hound binary"

if [[ -n "$BINARY_PATH" ]]; then
    [[ -x "$BINARY_PATH" ]] || fail "--binary '$BINARY_PATH' not found or not executable"
    SRC_BIN=$(readlink -f "$BINARY_PATH")
    ok "using local binary: $SRC_BIN"
else
    info "fetching latest release from github.com/AnteurAbderraouf/hound"
    LATEST_URL=$(curl -fsSL 'https://api.github.com/repos/AnteurAbderraouf/hound/releases/latest' \
        | grep -oE '"browser_download_url":\s*"[^"]*"' \
        | grep -oE 'https[^"]*' \
        | grep -E "linux_${ARCH}\.tar\.gz$" | head -n1)
    [[ -n "$LATEST_URL" ]] || fail "no linux_${ARCH} archive found in the latest release"

    TMP=$(mktemp -d)
    trap 'rm -rf "$TMP"' EXIT
    ok "downloading $LATEST_URL"
    if ! $DRY_RUN; then
        curl -fsSL "$LATEST_URL" -o "$TMP/hound.tar.gz"
        tar -xzf "$TMP/hound.tar.gz" -C "$TMP"
        SRC_BIN=$(find "$TMP" -type f -name 'hound' | head -n1)
        [[ -n "$SRC_BIN" ]] || fail "hound binary not found in downloaded archive"
    else
        SRC_BIN="<downloaded hound>"
    fi
fi

# ---------- 3. install -----------------------------------------------------

step "installing to /usr/local/bin + /var/lib/hound"

do_or_dry "install $SRC_BIN -> /usr/local/bin/hound" \
    install -m 0755 "$SRC_BIN" /usr/local/bin/hound

do_or_dry "mkdir /var/lib/hound" \
    mkdir -p /var/lib/hound

# create dedicated system user so hound doesn't run as root beyond binding :53
if ! id hound >/dev/null 2>&1; then
    do_or_dry "create system user 'hound'" \
        useradd --system --no-create-home --shell /usr/sbin/nologin hound
fi

do_or_dry "chown /var/lib/hound to hound" \
    chown -R hound:hound /var/lib/hound

# ---------- 4. systemd unit ------------------------------------------------

if $SKIP_SERVICE; then
    step "skipping systemd unit (--skip-service)"
else
    step "writing systemd unit"

    UNIT_PATH=/etc/systemd/system/hound.service
    UNIT_CONTENT=$(cat <<EOF
[Unit]
Description=hound — retro-terminal DNS monitor for home networks
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=hound
Group=hound
# Grant CAP_NET_BIND_SERVICE so an unprivileged user can bind :53
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=/var/lib/hound
ProtectHome=true

Environment=HOUND_HEADLESS=1
Environment=HOUND_DB_PATH=/var/lib/hound/hound.db
Environment=HOUND_DNS_ADDR=:53
Environment=HOUND_HTTP_ADDR=:8080
Environment=HOUND_UPSTREAM=1.1.1.1:53,8.8.8.8:53

ExecStart=/usr/local/bin/hound
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
)
    if $DRY_RUN; then
        info "would: write $UNIT_PATH"
    else
        echo "$UNIT_CONTENT" > "$UNIT_PATH"
        chmod 0644 "$UNIT_PATH"
        info "wrote $UNIT_PATH"
    fi

    do_or_dry "systemctl daemon-reload" systemctl daemon-reload
    do_or_dry "enable hound.service"    systemctl enable hound.service
    do_or_dry "start hound.service"     systemctl start hound.service
fi

# ---------- 5. firewall ----------------------------------------------------

if $SKIP_FIREWALL; then
    step "skipping firewall (--skip-firewall)"
else
    step "firewall"
    if command -v ufw >/dev/null 2>&1; then
        do_or_dry "ufw allow 53/udp" ufw allow 53/udp
        do_or_dry "ufw allow 53/tcp" ufw allow 53/tcp
        do_or_dry "ufw allow 8080/tcp" ufw allow 8080/tcp
    elif command -v firewall-cmd >/dev/null 2>&1; then
        do_or_dry "firewall-cmd --add-port=53/udp"    firewall-cmd --permanent --add-port=53/udp
        do_or_dry "firewall-cmd --add-port=53/tcp"    firewall-cmd --permanent --add-port=53/tcp
        do_or_dry "firewall-cmd --add-port=8080/tcp"  firewall-cmd --permanent --add-port=8080/tcp
        do_or_dry "firewall-cmd --reload"             firewall-cmd --reload
    else
        warn "no ufw/firewall-cmd detected; open UDP+TCP :53 and TCP :8080 manually if you have iptables/nftables rules"
    fi
fi

# ---------- 6. self-test ---------------------------------------------------

step "self-test"

if $DRY_RUN; then
    info "would: dig @127.0.0.1 example.com"
elif command -v dig >/dev/null 2>&1; then
    sleep 2
    if dig +short @127.0.0.1 example.com >/dev/null; then
        ok "hound answered example.com via 127.0.0.1"
    else
        warn "self-test failed. Check: systemctl status hound"
    fi
else
    warn "dig not installed — skipping self-test. Install bind-utils or dnsutils to enable it."
fi

# ---------- 7. next steps --------------------------------------------------

step "next steps — LAST manual bit (router)"

cat <<EOF

hound is installed and running as a systemd service.

  UI                : http://$LAN_IP:8080
  data folder       : /var/lib/hound
  binary            : /usr/local/bin/hound
  service unit      : /etc/systemd/system/hound.service

Service management:
  systemctl status hound
  systemctl restart hound
  journalctl -u hound -f

Router (last manual step) — set:
  PRIMARY DNS    : $LAN_IP
  SECONDARY DNS  : 1.1.1.1

Per-brand walkthroughs (Freebox, Livebox, Bbox, SFR, TP-Link, Netgear,
ASUS, generic):
  https://github.com/AnteurAbderraouf/hound/blob/main/docs/ROUTER-SETUP.md

To uninstall later:
  bash scripts/uninstall.sh

EOF

echo "==> done."
