// Package devices tracks the physical (or randomized) machines that
// talk to hound. It reconciles IPs with their MAC address via the
// operating system's ARP cache, keeps a first_seen/last_seen history
// per device, and lets the user assign a friendly name.
package devices

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// Resolver looks up the MAC address associated with an IP on the local
// LAN by parsing the OS ARP cache. Zero-value is not usable; use
// NewResolver instead.
type Resolver struct {
	timeout time.Duration
}

func NewResolver() *Resolver {
	return &Resolver{timeout: 3 * time.Second}
}

// ResolveMAC returns the lowercase MAC (aa:bb:cc:dd:ee:ff) for the
// given IPv4 address, empty string if not found. Loopback and non-IPv4
// input short-circuits to "".
//
// Implementation detail: shells out to `arp` because Go's stdlib does
// not expose the neighbour table portably. If arp is missing the
// function returns "" without erroring — MAC lookup is best-effort.
func (r *Resolver) ResolveMAC(ctx context.Context, ip string) (string, error) {
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.IsLoopback() || parsed.To4() == nil {
		return "", nil
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "arp", "-a", ip)
	default: // linux, darwin, freebsd
		cmd = exec.CommandContext(ctx, "arp", "-n", ip)
	}

	out, err := cmd.Output()
	if err != nil {
		// arp may return non-zero even when a match exists (freebsd) or
		// may be missing entirely on some minimal containers. Treat as
		// "no mac found" rather than propagating.
		return "", nil
	}

	return extractMAC(string(out)), nil
}

// extractMAC scans arp output for the first plausible MAC address.
// Accepts both colon (aa:bb:...) and dash (aa-bb-...) separators —
// Windows uses dashes, unix uses colons.
var macPattern = regexp.MustCompile(`(?i)\b([0-9a-f]{2}[:-]){5}[0-9a-f]{2}\b`)

func extractMAC(s string) string {
	m := macPattern.FindString(s)
	if m == "" {
		return ""
	}
	m = strings.ToLower(strings.ReplaceAll(m, "-", ":"))
	if strings.HasPrefix(m, "ff:ff:ff") || m == "00:00:00:00:00:00" {
		// arp cache placeholder / broadcast — not a real device
		return ""
	}
	return m
}

// ReverseHostname does a best-effort reverse DNS lookup on the LAN.
// Many home routers respond to PTR queries for their DHCP leases, so
// this often returns "iPhone-de-Lea" without any extra protocol work.
// Returns "" on failure — hostname discovery is best-effort.
func (r *Resolver) ReverseHostname(ctx context.Context, ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.IsLoopback() {
		return ""
	}

	resolver := &net.Resolver{}
	rctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	names, err := resolver.LookupAddr(rctx, ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	// PTR records are FQDN with a trailing dot, and DHCP-issued
	// hostnames typically end in ".home", ".lan", or ".local". Trim
	// trailing dot and the LAN suffix.
	name := strings.TrimSuffix(names[0], ".")
	for _, suffix := range []string{".home", ".lan", ".local", ".localdomain"} {
		name = strings.TrimSuffix(name, suffix)
	}
	return name
}

// Enrich fills MAC and hostname for an IP using both mechanisms above.
// Never returns an error — the zero values simply mean "not found".
func (r *Resolver) Enrich(ctx context.Context, ip string) (mac, hostname string) {
	if m, _ := r.ResolveMAC(ctx, ip); m != "" {
		mac = m
	}
	hostname = r.ReverseHostname(ctx, ip)
	return mac, hostname
}

// Snitch is a helper used in error messages and logs when we want to
// show what backend we shelled out to.
func (r *Resolver) Snitch() string {
	return fmt.Sprintf("arp(%s)", runtime.GOOS)
}
