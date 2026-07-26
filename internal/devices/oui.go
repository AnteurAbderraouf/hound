package devices

import (
	_ "embed"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed oui.yaml
var ouiYAML []byte

var (
	ouiOnce  sync.Once
	ouiTable map[string]string
	ouiErr   error
)

// LookupVendor returns the vendor name for a MAC address by matching
// its OUI (first 24 bits). Returns "" for empty, malformed, locally-
// administered (randomized), or unknown addresses.
//
// The lookup table is a curated subset of the IEEE OUI database. Full
// coverage is possible by swapping in the wireshark "manuf" file — see
// oui.yaml for the format.
func LookupVendor(mac string) string {
	loadOUI()
	if ouiErr != nil || len(mac) < 8 {
		return ""
	}
	prefix := strings.ToLower(mac[:8])
	if !validOUIPrefix(prefix) {
		return ""
	}
	if isLocallyAdministered(prefix) {
		// randomized MAC (iOS/Android per-SSID) — OUI is not a real
		// manufacturer prefix, refuse to guess
		return ""
	}
	return ouiTable[prefix]
}

func loadOUI() {
	ouiOnce.Do(func() {
		raw := make(map[string]string, 200)
		ouiErr = yaml.Unmarshal(ouiYAML, &raw)
		if ouiErr != nil {
			return
		}
		ouiTable = make(map[string]string, len(raw))
		for k, v := range raw {
			ouiTable[strings.ToLower(k)] = v
		}
	})
}

// validOUIPrefix returns true iff s looks like "aa:bb:cc".
func validOUIPrefix(s string) bool {
	if len(s) != 8 || s[2] != ':' || s[5] != ':' {
		return false
	}
	for i, r := range s {
		if i == 2 || i == 5 {
			continue
		}
		if !isHex(byte(r)) {
			return false
		}
	}
	return true
}

func isHex(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f')
}

// isLocallyAdministered returns true when the "locally administered" bit
// (bit 1 of the first octet) is set. Randomized MACs have this bit set
// and cannot be reverse-mapped to a manufacturer.
func isLocallyAdministered(prefix string) bool {
	if len(prefix) < 2 {
		return false
	}
	b, err := strconv.ParseUint(prefix[:2], 16, 8)
	if err != nil {
		return false
	}
	return b&0x02 != 0
}
