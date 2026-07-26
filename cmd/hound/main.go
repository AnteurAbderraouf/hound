package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/AnteurAbderraouf/hound/internal/api"
	"github.com/AnteurAbderraouf/hound/internal/categorizer"
	"github.com/AnteurAbderraouf/hound/internal/config"
	"github.com/AnteurAbderraouf/hound/internal/devices"
	"github.com/AnteurAbderraouf/hound/internal/dns"
	"github.com/AnteurAbderraouf/hound/internal/storage"
	"github.com/AnteurAbderraouf/hound/internal/window"
)

// version is overridden at release time by goreleaser via ldflags.
// See .goreleaser.yml -> builds.ldflags.
var version = "0.1.0"

// sinkAdapter bridges dns.Query into storage.Query so the two packages
// don't depend on each other's types, and enriches each query with its
// category before persisting. It also notifies the device tracker and
// the DNS fingerprinter so MAC/vendor/type enrichment can happen off
// the DNS hot path.
type sinkAdapter struct {
	store       *storage.Store
	cat         *categorizer.Categorizer
	tracker     *devices.Tracker
	fingerprint *devices.Fingerprinter
	log         *slog.Logger
}

func (a *sinkAdapter) Log(q dns.Query) {
	sq := storage.Query{
		Timestamp: q.Time,
		ClientIP:  q.ClientIP,
		Domain:    q.Domain,
		Type:      q.Type,
		Responded: q.Responded,
		Category:  a.cat.Categorize(q.Domain),
	}
	if err := a.store.InsertQuery(sq); err != nil {
		a.log.Error("failed to store query", "err", err)
	}
	// fire-and-forget: both are O(1) map lookups + optional non-blocking
	// enqueue.
	a.tracker.Observe(q.ClientIP, q.Time)
	a.fingerprint.Observe(q.ClientIP, q.Domain)
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	log.Info("hound starting", "version", version)

	cfg := config.Load()
	log.Info("config loaded",
		"db", cfg.DBPath,
		"dns_addr", cfg.DNSAddr,
		"http_addr", cfg.HTTPAddr,
		"upstream", cfg.Upstream,
		"headless", cfg.Headless,
	)

	cat, err := categorizer.New()
	if err != nil {
		log.Error("failed to load categorizer", "err", err)
		os.Exit(1)
	}
	log.Info("categorizer loaded", "categories", len(cat.Categories()))

	store, err := storage.Open(cfg.DBPath)
	if err != nil {
		log.Error("failed to open storage", "err", err)
		os.Exit(1)
	}
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tracker := devices.NewTracker(devices.NewResolver(), store, log)
	tracker.Start(ctx)

	fingerprinter, err := devices.NewFingerprinter(func(ip, typ, label string) {
		if err := store.SetDeviceType(ip, typ, time.Now()); err != nil {
			log.Warn("set device type failed", "ip", ip, "type", typ, "err", err)
			return
		}
		log.Info("device fingerprinted", "ip", ip, "type", typ, "label", label)
	})
	if err != nil {
		log.Error("failed to load fingerprinter", "err", err)
		os.Exit(1)
	}
	log.Info("fingerprinter loaded", "signatures", len(fingerprinter.Signatures()))

	sink := &sinkAdapter{
		store:       store,
		cat:         cat,
		tracker:     tracker,
		fingerprint: fingerprinter,
		log:         log,
	}

	dnsServer := dns.New(cfg.DNSAddr, cfg.Upstream, sink, log)
	apiServer := &api.Server{
		Addr:        cfg.HTTPAddr,
		Store:       store,
		Categorizer: cat,
		Log:         log,
	}

	go func() {
		if err := dnsServer.Start(ctx); err != nil && err != context.Canceled {
			log.Error("dns server crashed", "err", err)
			cancel()
		}
	}()

	go func() {
		if err := apiServer.Start(); err != nil {
			log.Error("http server crashed", "err", err)
			cancel()
		}
	}()

	uiURL := "http://localhost" + cfg.HTTPAddr
	if !strings.HasPrefix(cfg.HTTPAddr, ":") {
		uiURL = "http://" + cfg.HTTPAddr
	}
	log.Info("ready · " + uiURL)

	if !cfg.Headless {
		time.Sleep(300 * time.Millisecond)
		if err := window.Open(uiURL, log); err != nil {
			log.Warn("failed to open ui window; open the url manually", "err", err)
		}
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sig:
		log.Info("shutdown signal received")
	case <-ctx.Done():
	}

	log.Info("bye")
}
