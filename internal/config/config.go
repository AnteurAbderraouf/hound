package config

import (
	"os"
	"strings"
)

type Config struct {
	DBPath   string
	DNSAddr  string
	HTTPAddr string
	Upstream []string
	Headless bool
}

func Load() Config {
	return Config{
		DBPath:   getEnv("HOUND_DB_PATH", "hound.db"),
		DNSAddr:  getEnv("HOUND_DNS_ADDR", ":53"),
		HTTPAddr: getEnv("HOUND_HTTP_ADDR", ":8080"),
		Upstream: splitCSV(getEnv("HOUND_UPSTREAM", "1.1.1.1:53,8.8.8.8:53")),
		Headless: getEnv("HOUND_HEADLESS", "") != "",
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
