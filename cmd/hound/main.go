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

const version = "0.0.8"

// sinkAdapter bridges dns.Query into storage.Query so the two packages
// don't depend on each other's types, and enriches each query with its
// category before persisting. It also notifies the device tracker so
// MAC/hostname enrichment can happen off the DNS hot path.
type sinkAdapter struct {
	store   *storage.Store
	cat     *categorizer.Categorizer
	tracker *devices.Tracker
	log     *slog.Logger
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
	// fire-and-forget: tracker maintains its own async worker, this
	// call is O(1) and non-blocking.
	a.tracker.Observe(q.ClientIP, q.Time)
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

	sink := &sinkAdapter{store: store, cat: cat, tracker: tracker, log: log}

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
