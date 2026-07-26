package devices

import "testing"

func TestExtractMAC(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"linux arp -n", "? (192.168.1.42) at aa:bb:cc:dd:ee:ff [ether] on eth0", "aa:bb:cc:dd:ee:ff"},
		{"windows arp -a dashes", "  192.168.1.42          aa-bb-cc-dd-ee-ff     dynamic", "aa:bb:cc:dd:ee:ff"},
		{"macos arp -n", "? (192.168.1.42) at aa:bb:cc:dd:ee:ff on en0 ifscope [ethernet]", "aa:bb:cc:dd:ee:ff"},
		{"uppercase MAC", "1.2.3.4          AA:BB:CC:DD:EE:FF", "aa:bb:cc:dd:ee:ff"},
		{"broadcast rejected", "? (192.168.1.42) at ff:ff:ff:ff:ff:ff", ""},
		{"zero mac rejected", "1.2.3.4 at 00:00:00:00:00:00", ""},
		{"no mac in text", "192.168.1.42 -- <incomplete>", ""},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractMAC(c.in)
			if got != c.want {
				t.Errorf("extractMAC(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
