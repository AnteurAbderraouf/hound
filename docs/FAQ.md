# FAQ

Answers to the questions people ask before (and sometimes after) they
install hound. Short version: it's less powerful than most people
expect, and less invasive than most people fear.

---

## What is hound?

A self-hosted DNS server that logs every domain looked up on your home
network and shows the activity in a retro-terminal web UI. Think Pi-hole,
but focused on giving families a clear view of what's happening rather
than on blocking ads.

## What does hound actually see?

For every device on your LAN:

- The **domain** it's asking about (`youtube.com`, `pornhub.com`, …)
- The **query type** (A, AAAA, MX, …)
- The **timestamp**
- The **client IP** (and, later, MAC and hostname)
- The **category** hound has assigned to that domain

That's all we log.

## What does hound *not* see?

- **URLs.** DNS resolves domain names, not paths. `youtube.com` is what
  we see; `youtube.com/watch?v=...` is HTTPS content and stays private.
- **Search queries.** Same reason. "cute cats" typed into YouTube goes
  over HTTPS to `youtube.com` — hound only sees the domain.
- **Messages / emails / passwords.** All HTTPS. Impossible to inspect
  without a MITM cert on every device (not a v1 goal).
- **App content.** Same story — 99% of mobile apps use HTTPS with
  certificate pinning.
- **The identity of the person behind a device.** hound only knows
  devices (IPs, MACs). "iPhone-Lea" is a label *you* apply.

## Is this legal?

hound is a monitoring tool that runs on your own hardware, on your own
network, watching traffic you consented to receive. In most
jurisdictions, this is fine when the network is yours. Rules vary if
you monitor other adults — check your local law.

For parents monitoring minor children, it's generally accepted (and
often recommended). Being upfront with kids about it also tends to work
better than pretending it's not there.

## Will hound slow down my internet?

No, barring hardware pathologies. DNS queries are tiny and hound
responds in milliseconds after asking Cloudflare/Google upstream. On
typical home networks, latency is indistinguishable from asking
Cloudflare directly.

## What happens when the machine running hound is turned off?

Every device falls back to the **secondary DNS** you set in your router
(Cloudflare `1.1.1.1` in our recommended setup). Internet keeps working
normally — you just lose tracking during that window. The UI shows
these gaps visibly so you know when hound wasn't watching.

If you never configured a secondary DNS, powering off the hound host
will break DNS on the LAN. Go [set one](ROUTER-SETUP.md).

## Can my kids bypass hound?

Yes, several ways:

- **Change DNS on their device** to `1.1.1.1` or `8.8.8.8` directly.
  Fix: block outbound UDP:53 to anything but the hound host at your
  router firewall.
- **Use a VPN.** VPNs tunnel DNS through their own resolver. Fix: block
  known VPN endpoints (arms race) or simply have the conversation.
- **Use their phone's cellular data.** hound is only on your WiFi. Fix:
  outside our scope.
- **Use DNS-over-HTTPS (DoH) in browsers.** Chrome and Firefox can send
  DNS over HTTPS to their own resolvers, bypassing hound. Fix: v2 will
  offer DoH interception options.

## Is hound a replacement for Pi-hole?

No, and it's not trying to be. Pi-hole is an ad-blocker with monitoring
on the side. hound is a monitor with (planned) blocking on the side.
If you want silence-the-ads, use Pi-hole. If you want know-what's-happening,
try hound.

## Why should I trust hound with my DNS data?

The data never leaves your machine. hound is a single binary talking to
a local SQLite file. There's no cloud, no telemetry, no phone-home. The
source is on [GitHub](https://github.com/AnteurAbderraouf/hound) and
MIT-licensed — read every line if you want.

## Does hound work with IPv6?

The DNS server already handles AAAA (IPv6) queries — you'll see them
appear in the log. Full IPv6 client tracking (identifying an iPhone by
its IPv6 address across privacy-extension rotations) is planned but not
v0.x-ready.

## Can I contribute?

Yes, once the project reaches v0.1.0 with a proper release. Until then,
issues and discussions are welcome on GitHub.

## Where do I report a bug?

Open a [GitHub issue](https://github.com/AnteurAbderraouf/hound/issues).
Include your OS, hound version, and — if it's a UI bug — a screenshot.
Please redact any real domain names from your logs before pasting them
publicly.
