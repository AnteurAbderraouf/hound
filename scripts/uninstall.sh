#!/usr/bin/env bash
# hound Linux uninstall.
set -euo pipefail

KEEP_DATA=false
DRY_RUN=false

usage() {
    cat <<EOF
Usage: sudo bash uninstall.sh [options]

Options:
  --keep-data   Keep /var/lib/hound (SQLite db). Handy for reinstalls.
  --dry-run     Print every action without executing.
  -h, --help    Show this help.
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --keep-data) KEEP_DATA=true; shift ;;
        --dry-run)   DRY_RUN=true; shift ;;
        -h|--help)   usage; exit 0 ;;
        *) echo "unknown arg: $1"; usage; exit 1 ;;
    esac
done

if [[ $EUID -ne 0 ]]; then
    echo "must be run as root (use sudo)" >&2
    exit 1
fi

step() { echo; echo "==> $*"; }
info() { echo "    [info] $*"; }
skip() { echo "    [skip] $*"; }
do_or_dry() {
    if $DRY_RUN; then info "would: $1"; else info "$1"; shift; "$@"; fi
}

# ---------- service --------------------------------------------------------
step "removing systemd unit"

if systemctl list-unit-files 2>/dev/null | grep -q '^hound\.service'; then
    do_or_dry "stop hound.service"    systemctl stop hound.service    || true
    do_or_dry "disable hound.service" systemctl disable hound.service || true
    do_or_dry "delete /etc/systemd/system/hound.service" rm -f /etc/systemd/system/hound.service
    do_or_dry "systemctl daemon-reload" systemctl daemon-reload
else
    skip "hound.service not present"
fi

# ---------- firewall -------------------------------------------------------
step "closing firewall ports"

if command -v ufw >/dev/null 2>&1; then
    do_or_dry "ufw delete allow 53/udp"   ufw --force delete allow 53/udp   || true
    do_or_dry "ufw delete allow 53/tcp"   ufw --force delete allow 53/tcp   || true
    do_or_dry "ufw delete allow 8080/tcp" ufw --force delete allow 8080/tcp || true
elif command -v firewall-cmd >/dev/null 2>&1; then
    do_or_dry "firewall-cmd remove 53/udp"   firewall-cmd --permanent --remove-port=53/udp   || true
    do_or_dry "firewall-cmd remove 53/tcp"   firewall-cmd --permanent --remove-port=53/tcp   || true
    do_or_dry "firewall-cmd remove 8080/tcp" firewall-cmd --permanent --remove-port=8080/tcp || true
    do_or_dry "firewall-cmd reload"          firewall-cmd --reload
else
    skip "no known firewall tool detected"
fi

# ---------- binary ---------------------------------------------------------
step "removing binary"

if [[ -f /usr/local/bin/hound ]]; then
    do_or_dry "delete /usr/local/bin/hound" rm -f /usr/local/bin/hound
else
    skip "/usr/local/bin/hound not present"
fi

# ---------- user + data ---------------------------------------------------
step "user + data"

if id hound >/dev/null 2>&1; then
    do_or_dry "userdel hound" userdel hound || true
fi

if [[ -d /var/lib/hound ]]; then
    if $KEEP_DATA; then
        skip "keeping /var/lib/hound (--keep-data)"
    else
        do_or_dry "delete /var/lib/hound" rm -rf /var/lib/hound
    fi
fi

echo
echo "==> hound removed. Revert your router's DNS settings manually if you want."
