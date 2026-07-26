package devices

import (
	_ "embed"
	"fmt"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed fingerprints.yaml
var fingerprintsYAML []byte

type signature struct {
	Type       string   `yaml:"type"`
	Label      string   `yaml:"label"`
	Confidence int      `yaml:"confidence"`
	Signals    []string `yaml:"signals"`
}

type fingerprintsFile struct {
	Signatures []signature `yaml:"signatures"`
}

// Fingerprinter matches observed (client_ip, domain) pairs against a
// curated set of device signatures. It tracks the current best guess
// per IP and calls a hook whenever the guess improves so persistence
// can happen out-of-band.
type Fingerprinter struct {
	// domain-suffix -> signature index (into signatures slice)
	domainToSig map[string]int
	signatures  []signature

	// last-notified confidence per IP so we don't flap
	mu   sync.Mutex
	best map[string]int // ip -> confidence

	onMatch OnMatch
}

// OnMatch is invoked when Fingerprinter concludes a device is of a
// given type, or when a higher-confidence guess supersedes the previous
// one. label is the human-readable string ("iPhone / iPad"), typ is
// the machine-friendly slug ("iphone"). Called from the same goroutine
// as Observe.
type OnMatch func(ip, typ, label string)

func NewFingerprinter(onMatch OnMatch) (*Fingerprinter, error) {
	var f fingerprintsFile
	if err := yaml.Unmarshal(fingerprintsYAML, &f); err != nil {
		return nil, fmt.Errorf("parse fingerprints yaml: %w", err)
	}

	fp := &Fingerprinter{
		signatures:  f.Signatures,
		domainToSig: make(map[string]int, len(f.Signatures)*4),
		best:        make(map[string]int, 32),
		onMatch:     onMatch,
	}
	for i, sig := range f.Signatures {
		for _, d := range sig.Signals {
			fp.domainToSig[strings.ToLower(strings.TrimSuffix(strings.TrimSpace(d), "."))] = i
		}
	}
	return fp, nil
}

// Observe checks whether the given (ip, domain) pair matches any
// signature and, if it does, promotes the device's current best guess
// when the new confidence is strictly higher. When promoted, onMatch is
// invoked (from this same goroutine, synchronously).
func (fp *Fingerprinter) Observe(ip, domain string) {
	if ip == "" || domain == "" {
		return
	}
	sig, ok := fp.matchSignature(domain)
	if !ok {
		return
	}

	fp.mu.Lock()
	prev, seen := fp.best[ip]
	if seen && sig.Confidence <= prev {
		fp.mu.Unlock()
		return
	}
	fp.best[ip] = sig.Confidence
	fp.mu.Unlock()

	if fp.onMatch != nil {
		fp.onMatch(ip, sig.Type, sig.Label)
	}
}

// matchSignature returns the signature that owns the given domain (or
// any of its parent suffixes). Same algorithm as the categorizer.
func (fp *Fingerprinter) matchSignature(domain string) (signature, bool) {
	d := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	if d == "" {
		return signature{}, false
	}
	if idx, ok := fp.domainToSig[d]; ok {
		return fp.signatures[idx], true
	}
	parts := strings.Split(d, ".")
	for i := 1; i < len(parts)-1; i++ {
		if idx, ok := fp.domainToSig[strings.Join(parts[i:], ".")]; ok {
			return fp.signatures[idx], true
		}
	}
	return signature{}, false
}

// Signatures returns the loaded list, mostly for debugging or exposing
// via an API.
func (fp *Fingerprinter) Signatures() []signature {
	out := make([]signature, len(fp.signatures))
	copy(out, fp.signatures)
	return out
}
