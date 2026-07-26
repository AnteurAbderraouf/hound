// hound-query is a small helper that fires DNS queries at a hound
// instance so you can see the UI populate without configuring your
// router. Useful during local development and for producing screenshots
// of the "live" panel.
//
// Usage:
//
//	hound-query [--server host:port] [--rounds N] [--sleep MS] [domain ...]
//
// With no domains, sends a curated demo set spanning every category.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/miekg/dns"
)

var demoDomains = []string{
	// streaming
	"youtube.com", "netflix.com", "twitch.tv", "spotify.com",
	// social
	"tiktok.com", "instagram.com", "reddit.com", "x.com",
	// gaming
	"steampowered.com", "epicgames.com", "roblox.com", "minecraft.net",
	// messaging
	"discord.com", "whatsapp.net", "telegram.org",
	// education
	"wikipedia.org", "github.com", "stackoverflow.com", "khanacademy.org",
	// shopping
	"amazon.fr", "vinted.fr",
	// news
	"lemonde.fr", "bbc.com",
	// productivity
	"notion.so", "slack.com", "figma.com",
	// adult (visible in the UI as red so screenshots include it)
	"pornhub.com",
	// ads_tracking
	"doubleclick.net", "google-analytics.com",
	// other (uncategorized)
	"example.com", "some-random-blog.dev",
}

func main() {
	server := flag.String("server", "127.0.0.1:5300", "hound DNS server address (host:port)")
	rounds := flag.Int("rounds", 1, "how many times to run through the domain list")
	sleepMs := flag.Int("sleep", 80, "milliseconds to sleep between queries (keeps the UI feeling live)")
	flag.Parse()

	domains := flag.Args()
	if len(domains) == 0 {
		domains = demoDomains
	}

	client := &dns.Client{
		Timeout: 5 * time.Second,
	}

	ok, fail := 0, 0
	fmt.Printf("→ querying %s with %d domain(s) × %d round(s)\n\n", *server, len(domains), *rounds)

	for round := 1; round <= *rounds; round++ {
		if *rounds > 1 {
			fmt.Printf("── round %d/%d ──\n", round, *rounds)
		}
		for _, d := range domains {
			msg := new(dns.Msg)
			msg.SetQuestion(dns.Fqdn(d), dns.TypeA)

			resp, _, err := client.Exchange(msg, *server)
			if err != nil {
				fmt.Printf("  ✗ %-30s  error: %v\n", d, err)
				fail++
			} else {
				status := dns.RcodeToString[resp.Rcode]
				answers := len(resp.Answer)
				fmt.Printf("  ✓ %-30s  %-8s  %d answer(s)\n", d, status, answers)
				ok++
			}
			time.Sleep(time.Duration(*sleepMs) * time.Millisecond)
		}
	}

	fmt.Printf("\ndone: %d ok, %d failed\n", ok, fail)
	if fail == ok+fail {
		os.Exit(1)
	}
}
