package devices

import (
	"sync"
	"testing"
)

func TestFingerprinterMatchesAndPromotes(t *testing.T) {
	var (
		mu   sync.Mutex
		hits []string // "ip=type"
	)
	fp, err := NewFingerprinter(func(ip, typ, label string) {
		mu.Lock()
		hits = append(hits, ip+"="+typ)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("NewFingerprinter: %v", err)
	}

	// Same IP: android hit at confidence 9, then iphone at 10 wins.
	fp.Observe("192.168.1.10", "1e100.net")           // -> android (9)
	fp.Observe("192.168.1.10", "push.apple.com")      // -> iphone  (10)
	fp.Observe("192.168.1.10", "gsp10-ssl.ls.apple.com") // still iphone, no new hit
	// Another IP, subdomain suffix match.
	fp.Observe("192.168.1.42", "livestream.xboxlive.com") // -> xbox (10)
	// No match at all — must not trigger onMatch.
	fp.Observe("192.168.1.99", "example.com")

	mu.Lock()
	got := append([]string(nil), hits...)
	mu.Unlock()

	want := []string{
		"192.168.1.10=android",
		"192.168.1.10=iphone",
		"192.168.1.42=xbox",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, g := range got {
		if g != want[i] {
			t.Errorf("hit[%d] = %q, want %q", i, g, want[i])
		}
	}
}

func TestFingerprinterEmptyInputs(t *testing.T) {
	fp, err := NewFingerprinter(func(ip, typ, label string) {
		t.Fatalf("onMatch called for empty inputs (ip=%q typ=%q)", ip, typ)
	})
	if err != nil {
		t.Fatalf("NewFingerprinter: %v", err)
	}
	fp.Observe("", "push.apple.com")
	fp.Observe("192.168.1.1", "")
	fp.Observe("", "")
}
