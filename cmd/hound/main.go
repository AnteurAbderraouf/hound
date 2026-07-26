package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/AnteurAbderraouf/hound/internal/api"
	"github.com/AnteurAbderraouf/hound/internal/config"
	"github.com/AnteurAbderraouf/hound/internal/dns"
	"github.com/AnteurAbderraouf/hound/internal/storage"
)

const version = "0.0.2"

// sinkAdapter bridges dns.Query into storage.Query so the two packages
// don't depend on each other's types.
type sinkAdapter struct {
	store *storage.Store
	log   *slog.Logger
}

func (a *sinkAdapter) Log(q dns.Query) {
	sq := storage.Query{
		Timestamp: q.Time,
		ClientIP:  q.ClientIP,
		Domain:    q.Domain,
		Type:      q.Type,
		Responded: q.Responded,
	}
	if err := a.store.InsertQuery(sq); err != nil {
		a.log.Error("failed to store query", "err", err)
	}
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
	)

	store, err := storage.Open(cfg.DBPath)
	if err != nil {
		log.Error("failed to open storage", "err", err)
		os.Exit(1)
	}
	defer store.Close()

	sink := &sinkAdapter{store: store, log: log}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dnsServer := dns.New(cfg.DNSAddr, cfg.Upstream, sink, log)
	apiServer := &api.Server{Addr: cfg.HTTPAddr, Store: store, Log: log}

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

	log.Info("ready · open http://localhost" + cfg.HTTPAddr)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sig:
		log.Info("shutdown signal received")
	case <-ctx.Done():
	}

	log.Info("bye")
}
