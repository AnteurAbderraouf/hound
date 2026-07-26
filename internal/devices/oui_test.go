package devices

import "testing"

func TestLookupVendor(t *testing.T) {
	cases := map[string]string{
		"":                    "",                          // empty
		"aa":                  "",                          // too short
		"invalid-mac":         "",                          // malformed
		"00:03:93:11:22:33":   "Apple, Inc.",               // known Apple OUI
		"00:1F:A7:aa:bb:cc":   "Sony (PlayStation)",        // uppercase, mixed case
		"00:03:93":            "Apple, Inc.",               // just the prefix
		"b8:27:eb:00:00:00":   "Raspberry Pi Foundation",   // Pi
		"ff:ff:ff:ff:ff:ff":   "",                          // broadcast / locally-administered
		"02:00:00:11:22:33":   "",                          // locally-administered bit set (randomized)
		"6a:aa:bb:cc:dd:ee":   "",                          // locally-administered
		"99:99:99:aa:bb:cc":   "",                          // unknown OUI
	}
	for in, want := range cases {
		if got := LookupVendor(in); got != want {
			t.Errorf("LookupVendor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsLocallyAdministered(t *testing.T) {
	// The "locally administered" bit is bit 1 of the first octet. So
	// any first octet whose second hex digit is 2, 3, 6, 7, A, B, E,
	// or F has the bit set.
	cases := map[string]bool{
		"02:00:00": true,
		"06:00:00": true,
		"0a:00:00": true,
		"0e:00:00": true,
		"12:00:00": true, // 0x12 = 0001 0010 -> bit 1 set
		"00:00:00": false,
		"04:00:00": false,
		"08:00:00": false,
		"0c:00:00": false,
	}
	for in, want := range cases {
		if got := isLocallyAdministered(in); got != want {
			t.Errorf("isLocallyAdministered(%q) = %v, want %v", in, got, want)
		}
	}
}
