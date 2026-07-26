package devices

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Store abstracts the persistence needed by Tracker. Kept minimal so
// hound's storage package can implement it without exposing internals.
type Store interface {
	UpsertDevice(ip, mac, vendor, hostname string, seenAt time.Time) error
}

// Tracker sees every DNS query source IP and, the first time it meets
// a new one (or after a cooldown), asks Resolver to enrich it with MAC
// and hostname then persists via Store. All work happens in a
// background goroutine so the DNS hot path stays synchronous and fast.
type Tracker struct {
	resolver *Resolver
	store    Store
	log      *slog.Logger

	mu       sync.Mutex
	lastSeen map[string]time.Time // ip -> last time we enriched
	inFlight map[string]struct{}  // ip -> currently enriching
	cooldown time.Duration        // how often we re-enrich a known ip
	workCh   chan string
	done     chan struct{}
}

// NewTracker returns a Tracker ready to Start. The cooldown controls
// how often we re-run ARP against an already-known IP (its lease may
// have moved, or the OS may have expired the entry). 15 minutes is a
// reasonable default.
func NewTracker(resolver *Resolver, store Store, log *slog.Logger) *Tracker {
	return &Tracker{
		resolver: resolver,
		store:    store,
		log:      log,
		lastSeen: make(map[string]time.Time),
		inFlight: make(map[string]struct{}),
		cooldown: 15 * time.Minute,
		workCh:   make(chan string, 128),
		done:     make(chan struct{}),
	}
}

// Start runs the enrichment worker. Cancel ctx to stop.
func (t *Tracker) Start(ctx context.Context) {
	go func() {
		defer close(t.done)
		for {
			select {
			case <-ctx.Done():
				return
			case ip := <-t.workCh:
				t.enrich(ctx, ip)
			}
		}
	}()
}

// Wait blocks until Start's goroutine has exited.
func (t *Tracker) Wait() { <-t.done }

// Observe is called by the DNS handler for every query. It updates
// last_seen unconditionally (a cheap map write + fire-and-forget
// enqueue) and only kicks off enrichment when the IP is new or its
// cooldown has expired.
func (t *Tracker) Observe(ip string, at time.Time) {
	if ip == "" {
		return
	}
	t.mu.Lock()
	last, seen := t.lastSeen[ip]
	_, busy := t.inFlight[ip]
	stale := seen && at.Sub(last) > t.cooldown
	shouldEnrich := (!seen || stale) && !busy
	if shouldEnrich {
		t.inFlight[ip] = struct{}{}
	}
	t.lastSeen[ip] = at
	t.mu.Unlock()

	if !shouldEnrich {
		return
	}

	select {
	case t.workCh <- ip:
	default:
		// worker queue is full, drop this enrichment; we'll retry next
		// time we see the same ip after the cooldown.
		t.mu.Lock()
		delete(t.inFlight, ip)
		t.mu.Unlock()
	}
}

func (t *Tracker) enrich(ctx context.Context, ip string) {
	defer func() {
		t.mu.Lock()
		delete(t.inFlight, ip)
		t.mu.Unlock()
	}()

	mac, hostname := t.resolver.Enrich(ctx, ip)
	vendor := LookupVendor(mac)
	if err := t.store.UpsertDevice(ip, mac, vendor, hostname, time.Now()); err != nil {
		t.log.Warn("device upsert failed", "ip", ip, "err", err)
		return
	}
	t.log.Debug("device enriched", "ip", ip, "mac", mac, "vendor", vendor, "hostname", hostname)
}
