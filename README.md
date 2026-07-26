<div align="center">

```
  ██╗  ██╗ ██████╗ ██╗   ██╗███╗   ██╗██████╗
  ██║  ██║██╔═══██╗██║   ██║████╗  ██║██╔══██╗
  ███████║██║   ██║██║   ██║██╔██╗ ██║██║  ██║
  ██╔══██║██║   ██║██║   ██║██║╚██╗██║██║  ██║
  ██║  ██║╚██████╔╝╚██████╔╝██║ ╚████║██████╔╝
  ╚═╝  ╚═╝ ╚═════╝  ╚═════╝ ╚═╝  ╚═══╝╚═════╝
```

**Peek at what's happening on your family's WiFi.**

A retro-terminal DNS monitor for home networks — no hardware, no fuss, just clarity.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
![Status](https://img.shields.io/badge/status-early--dev-orange)
![Go](https://img.shields.io/badge/go-1.22+-00ADD8?logo=go)

</div>

---

> ⚠️ **Early development.** Core is working (DNS server, categorization, UI window, SQLite storage) but there's no packaged release yet — build from source for now. Star & watch for the v0.1 release with pre-built binaries.

## What is this?

`hound` is a self-hosted DNS server that runs on any machine in your home network. It captures every DNS query from every connected device, categorizes them (adult, social, gaming, streaming, education, etc.), and displays the activity in a beautiful retro-terminal web UI.

## Who it's for

- 👨‍👩‍👧 Parents who want a clear view of their kids' online activity
- 👨‍💻 The tech-savvy older sibling curious about what's happening on the WiFi
- 🔧 Home lab tinkerers who love a good dashboard
- 🕵️ Anyone curious about who's talking to whom on their home network

## Who it's NOT for

- Ad-blocking → use [Pi-hole](https://pi-hole.net)
- Deep packet inspection → this is DNS-level only
- Reading URLs, messages, or app content → HTTPS makes this impossible without MITM (a v2+ opt-in feature)

## Features (v1 scope)

- 🌐 **Zero-hardware DNS monitoring** — runs as a single binary or Docker container on any machine already in your home
- 📊 **Per-device tracking** — see what each phone, laptop, TV, or console is doing
- 🏷️ **Automatic domain categorization** — 50k+ curated domains bundled in
- ⏱️ **Human-readable timeline** — `iPhone-Lea → 14:32 tiktok.com` instead of raw logs
- ✍️ **Custom device names** — rename `192.168.1.42` into `iPhone-Lea`
- 🖥️ **Retro-terminal UI** — CRT scan lines, ASCII borders, green phosphor. Optional CRT effect toggle.
- 🕳️ **Honest about gaps** — the UI clearly shows when the tracker was offline

## What's next (v2+)

- ⏰ Time-based blocking (*"no YouTube after 21:00"*)
- 🚨 Real-time alerts
- 🌐 Browser extension for full URL visibility
- 👤 User profiles (group devices per person)
- 📧 Weekly digest emails
- 🤖 LLM fallback for uncategorized domains

## Quick start

**Docker** (easiest on Linux / NAS / homelab):

```bash
git clone https://github.com/AnteurAbderraouf/hound.git
cd hound
docker compose up -d
```

Then point your router's **primary DNS** at your host (secondary DNS →
`1.1.1.1` as fallback). See [ROUTER-SETUP.md](docs/ROUTER-SETUP.md) for
the exact clicks per ISP box.

**From source** (Windows / macOS / dev):

```bash
git clone https://github.com/AnteurAbderraouf/hound.git
cd hound
go build -o bin/hound ./cmd/hound
./bin/hound        # sudo on linux/mac to bind :53
```

A retro-terminal UI window pops open automatically (no browser needed).

Read the [full install guide](docs/INSTALL.md), and if you're weighing
"should I even install this?", read the [FAQ](docs/FAQ.md) — especially
the sections on what hound does and doesn't see.

## How it works

```
  📱 iPhone   💻 PC      📺 TV      🎮 PS5
      │          │          │          │
      └──────────┴──────────┴──────────┘
                     │
                     ▼
             🌐 Home Router
        (primary DNS → your host)
        (secondary DNS → 1.1.1.1)
                     │
                     ▼
        ┌────────────────────────┐
        │  💻 Any always-on box  │
        │  ┌──────────────────┐  │
        │  │ hound (single    │  │
        │  │ binary or docker)│  │
        │  │                  │  │
        │  │ • DNS server :53 │  │
        │  │ • Web UI  :8080  │  │
        │  │ • SQLite storage │  │
        │  └──────────────────┘  │
        └────────────────────────┘
```

## Contributing

The project is in early development. Once the v0.1 lands, contribution guidelines will follow. Feel free to open issues for feature requests or bug reports.

## License

MIT © 2026 [Anteur Abderraouf](https://github.com/AnteurAbderraouf)
