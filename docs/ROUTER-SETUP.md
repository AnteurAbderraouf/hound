# Router setup

hound only receives DNS traffic if your router is configured to send it
there. The general recipe is:

1. Find your host's LAN IP (the machine running hound) — usually
   `192.168.1.X` or `192.168.0.X`.
2. In your router admin panel, set the **primary DNS** to that IP.
3. Set the **secondary DNS** to a public resolver such as `1.1.1.1`.
4. Reboot the router (or the devices on it) so DHCP hands out the new
   DNS to every device.

The secondary DNS matters. If hound is ever unreachable (host powered
off, upgrade in progress, ...) every device on the LAN falls back to
Cloudflare — no one loses internet, you just miss the tracking window.

The rest of this page has ISP- and vendor-specific paths so you don't
have to hunt through the admin UI. If your router isn't listed, follow
the "generic" recipe at the bottom.

- [Freebox (Free)](#freebox-free)
- [Livebox (Orange / Sosh)](#livebox-orange--sosh)
- [Bbox (Bouygues Telecom)](#bbox-bouygues-telecom)
- [SFR Box](#sfr-box)
- [TP-Link](#tp-link)
- [Netgear](#netgear)
- [ASUS](#asus)
- [Generic recipe](#generic-recipe)
- [Troubleshooting](#troubleshooting)

---

## Freebox (Free)

1. Open **http://mafreebox.freebox.fr** and sign in.
2. **Paramètres de la Freebox → Serveur DHCP** _(or "Paramètres du DHCP"
   depending on Freebox generation)_.
3. Look for the **DNS** section (may be labeled "DNS personnalisés").
4. Enable custom DNS if there's a toggle.
5. Fill:
   - **DNS 1**: your hound host IP (e.g. `192.168.1.42`)
   - **DNS 2**: `1.1.1.1`
6. Save. Freebox pushes the new DNS to devices at the next DHCP renew
   (reboot devices to force it).

---

## Livebox (Orange / Sosh)

1. Open **http://192.168.1.1** or **http://livebox** and sign in.
2. **Configuration avancée → DHCP** (or "Serveur DHCP" on older Liveboxes).
3. In the **Serveurs DNS** section:
   - **DNS 1**: your hound host IP
   - **DNS 2**: `1.1.1.1`
4. Save.

> Some Livebox versions hide DNS behind **Paramètres avancés → Réseau →
> DHCP**. If you can't find it, the setting always exists — just poke
> around the "advanced" menus.

---

## Bbox (Bouygues Telecom)

1. Open **http://192.168.1.254** or **http://gestionbbox.lan** and sign
   in.
2. **Menu principal → Ma Bbox → Paramètres avancés → DHCP**.
3. Set **Serveur DNS 1** to your hound host IP and **Serveur DNS 2** to
   `1.1.1.1`.
4. Save.

---

## SFR Box

1. Open **http://192.168.1.1** and sign in.
2. **Réseau v4 → DHCP** (or "Paramètres réseau → Serveur DHCP").
3. Set **DNS primaire** to your hound host IP and **DNS secondaire** to
   `1.1.1.1`.
4. Save.

---

## TP-Link

1. Open **http://tplinkwifi.net** or **http://192.168.0.1**.
2. **Advanced → Network → DHCP Server** (varies by model).
3. Set **Primary DNS** and **Secondary DNS**.
4. Save and reboot the router.

If your TP-Link runs the newer "Deco" firmware, use the mobile app:
**More → Advanced → IPv4 → DHCP Server → DNS**.

---

## Netgear

1. Open **http://routerlogin.net** or **http://192.168.1.1**.
2. **Advanced → Setup → LAN Setup**.
3. Under **Address Reservation** _(some models)_ or **Use These DNS
   Servers**, fill primary and secondary.
4. Apply and reboot.

---

## ASUS

1. Open **http://router.asus.com** or **http://192.168.1.1**.
2. **LAN → DHCP Server**.
3. Set **DNS Server 1** and **DNS Server 2**.
4. Apply. Some Merlin firmwares also expose a "DNS Director" that
   respects/overrides this — check under **WAN → Internet Connection →
   DNS**.

---

## Generic recipe

If your router isn't listed here:

1. Access the admin panel (URL is often printed on a sticker under the
   router, or is `192.168.1.1` / `192.168.0.1`).
2. Look for a **DHCP** or **LAN** or **DNS** section under "Advanced".
3. Change **Primary DNS** to your hound host IP.
4. Change **Secondary DNS** to `1.1.1.1` (Cloudflare) or `8.8.8.8`
   (Google).
5. Save and reboot the router or wait for the DHCP lease to renew on
   each device (usually a few minutes, or reboot the device).

---

## Troubleshooting

**"I set the DNS but hound sees no traffic."**
- Reboot the router. Some routers don't push new DHCP settings to
  already-connected devices.
- Reboot the client device or run `ipconfig /release && ipconfig /renew`
  (Windows) or `sudo dhclient -r && sudo dhclient` (Linux).
- Verify with `nslookup example.com` — the "Server:" line should show
  your hound host IP.
- Check your firewall: hound needs UDP+TCP :53 reachable from the LAN.

**"Some devices still use their own DNS."**
- Smart TVs, Chromecasts, some IoT devices hardcode DNS to `8.8.8.8` and
  ignore router DHCP. You can catch this by blocking outbound :53 to
  anything but your hound host at the router firewall (advanced).

**"Internet is slow after configuring hound."**
- Check the hound host isn't overloaded. hound itself is very light
  (~30 MB RAM), so it's usually a network config issue: primary DNS
  timing out, fallback kicking in with a 5s delay.
- Look at `docker compose logs -f hound` (or the console) for upstream
  errors.

**"When I turn off my machine, internet breaks."**
- You forgot the secondary DNS. Go back to your router admin and set
  `1.1.1.1` (or any public DNS) as **Secondary DNS**. All devices will
  fall back automatically when hound is unreachable.
