# Remote: Aetox Is a Server in Your Own House, and the Phone Is Its Remote (2026-08-14)

> ## Status: PARKED 2026-08-14, with a working foundation in the tree
>
> The direction was locked the same day — *"ยุ ก เต็มๆ"*, the server runs on the
> owner's own machine, never a rented one — and a working slice was built,
> tested, and then deliberately shelved. **The slice is what settled it:** a
> phone that can only watch is not worth a surface, and a phone that can drive
> the machine needs its own design for display, navigation and security. That
> is a product, not a panel, and it did not fit under the button it started as.
>
> **What is in the tree and stays there:** [`desktop/remote.go`](../../desktop/remote.go)
> (LAN address discovery, same-subnet gate, one-time pairing, device table,
> the phone's JSON API), [`desktop/remote_page.html`](../../desktop/remote_page.html),
> [`desktop/remote_test.go`](../../desktop/remote_test.go) (9 tests),
> schema migration v14 `remote_devices`, and
> [`MobileRemote.svelte`](../../desktop/frontend/src/lib/MobileRemote.svelte).
>
> **What is switched off:** the menu entry. Nothing calls `StartMobileRemote`,
> so **no port is opened on any install** — the code is dead, tested, and inert.
>
> **What must change before it is unparked** — both found by building it:
> pairing must be confirmed on the desktop (a scan alone lets in anyone who can
> see the screen), and the wire must be encrypted (plain http was defensible
> while the phone could only watch; it is not once the phone can command the
> machine). See §Security below.
>
> **The rule for whoever picks this up:** do not shape the API around a browser.
> See §"Decide the client later" below.

Written before the build for the same reason [§106](../DECISIONS.md) was: the
shape of this decision reaches backwards into work already on the map, and
learning it late means rework. Everything below is the design as it stood when
it was parked.

Prior note this finally picks up: the owner's own — *"Mobile monitor, จำไว้ ควรทำ"*,
with Codex Remote as the thing that prompted it.

Live code this touches:
[`bootstrap.go`](../../internal/bootstrap/bootstrap.go) ·
[`pending.go`](../../desktop/pending.go) ·
[`background_tasks.go`](../../desktop/background_tasks.go) ·
[`loopback.go`](../../internal/oauth/loopback.go) ·
[`app.go`](../../desktop/app.go).

---

## The rule

**Aetox may become a server, but only the kind that runs in your own house.
The phone is a remote control, not a third desk.**

Three sentences follow from it, and they are the whole design:

- The phone shows work that is already happening and lets a human unblock it.
  It does not become a place where the workbench, the terminal, or the file
  panes are re-drawn small.
- Nothing the phone can do is absent from the desktop, and nothing the desktop
  can do appears on the phone as a *new right*. One gate, one permission list.
- **The phone has no settings page at all — not one field** (owner, 14 Aug:
  *"ไม่เอาแบบมาตั้งค่าซ้ำซ้อนนะครับ"*). Model, provider, keys, permissions,
  agents, projects: the phone inherits every one of them because it is not a
  second Aetox, it is a second screen on the same one. Anything configurable is
  configured on the desktop, once. A setting that exists in two places is a
  setting that will disagree with itself.

## What "server" must not mean

| | Server **in your house** | Server **you rent** |
|---|---|---|
| Where the work happens | The owner's machine | A cloud box |
| Reaches your files, terminal, logged-in browser, open project | ✅ | ❌ |
| Keeps the one sentence the product is sold on | ✅ | ❌ |
| Verdict | **This document** | **Refused** |

The refusal is not about positioning; it is about the product not working.
Aetox in the cloud has no hands — no files, no terminal, no browser already
logged in, no project open. What is left is a chat window, which is a category
where we are the worst entrant. Everything Aetox is worth is downstream of
*"it does the work on your machine"*, and renting a box throws away the only
thing we have.

## No separate app — a page, not a build target

**The phone gets a web page (PWA) served by the same Go process. No App Store,
no Play Store, no second toolchain, no Apple Developer account.**

The deciding fact is that a native app buys *nothing on the Go side*. The
phone cannot use the Wails bindings (`wailsjs/go/main/App.*` need the Wails
runtime, which exists only inside the desktop webview), so the phone talks
HTTP either way — native or web, the server work is identical. What native
would add is a store review queue and a release channel we do not control, for
a solo maintainer who ships on Windows.

The frontend is already Svelte 5 + Vite. The phone page is a second Vite entry
sharing [`styles/`](../../desktop/frontend/src/styles), not a second frontend.

**The one thing native buys, and its price:** notifications that reliably wake
a locked phone. Web push works on Android freely and on iOS only for a PWA the
user has added to the home screen (16.4+) — **and only over HTTPS**. That
single requirement is what pins the transport choice below; without
notifications the whole feature degrades into *"remember to check your phone"*,
which is the problem it exists to solve.

## What already exists — more than expected

`internal/bootstrap` was written with this host in mind. Its own doc comment on
`Options`:

> *"The callbacks are deliberately all optional except Approve: a host that
> draws nothing still gets a working engine, **which is what a server needs**.
> They are also deliberately NOT split into 'desktop' and 'headless' sets — **an
> HTTP host streaming over SSE** wants OnStatus and OnContentPreview for exactly
> the reason a window does."*

and on `Surface`:

> *"Zero value means prompt.SurfaceDesktop, which is what every graphical and
> **headless** host wants"*

So: no new `prompt.Surface`, no engine change, no new agent identity.
`bootstrap.Engine(cfg, Options{...})` is the door, and a server host walks
through the same one the desktop does.

| Piece | State |
|---|---|
| Engine runs with nothing drawing it | ✅ designed for, `bootstrap.Engine` |
| Live reply / status / preview streaming | ✅ `RunOnceStream` + `OnStatus` / `OnContentPreview` → SSE |
| Approval queue with a durable home | ✅ [`pending.go`](../../desktop/pending.go) — rows in the store, with an audit trail, not an in-memory popup |
| A network listener | ❌ the only `net.Listen` today is the OAuth loopback, bound to `127.0.0.1` |
| Any notion of an authenticated caller | ❌ **the one genuinely new thing** |
| A phone-shaped page | ❌ |

## The hard part is `Approve`, and the code already said so

`Options.Approve` refuses `nil`, and the comment explaining why was written for
exactly this host:

> *"turn reads a nil Approve as 'approved' — so accepting nil here would
> silently disarm whatever approval mode the config asked for, and **it would
> fail open, quietly, on the host least likely to have a human watching**."*

A server is the host least likely to have a human watching. So the server's
`Approve` must **park and wait for a real answer from a real phone** — never
return true because nobody replied, and never time out into a yes. A timeout
is a no, and a no is safe.

This is the majority of the work. The rest is plumbing.

## Two dependencies, one of them already on the map

**1. Background work has no durable home yet.**
[`BackgroundTasks()`](../../desktop/background_tasks.go) reads
`a.delegations.Snapshot()` — an in-memory register, scoped to the session the
desktop currently has open. A phone asking *"what is it doing right now?"* can
only be answered by something that outlives the window's current session.
[ROADMAP.md](../../ROADMAP.md) already names the gap — *"ขาดแค่อายุที่ไม่ผูก
กับเทิร์น + ที่เก็บสถานะ"* — and this is the reason to close it with a second
reader in mind. **Designed for one reader now, it is a rewrite later; designed
for two now, it is free.**

**2. There are already two engine wirings, and this would be the third.**
The desktop calls `bootstrap.Engine` ([app.go:2818](../../desktop/app.go)); the
CLI wires `app.NewApp` itself ([cmd/aetox/main.go:360](../../cmd/aetox/main.go)).
Two places already answer *"how is an engine assembled"*. **The server host
goes through `bootstrap.Engine` or it does not get built** — and the pressure
that creates to migrate the CLI onto the same door is a benefit, not a cost.

Related but separate: the HTTP layer must be a **thin adapter over the same
`App` methods**, never a second implementation. The moment an endpoint answers
a question differently from its Wails binding, there are two Aetoxes.

## Transport — recommendation

One implementation covers the first two columns. They are the same code
listening on an interface; the only difference is which address the phone is
told to open. **That is the argument for not choosing between them.**

| | Same Wi-Fi (LAN) | Tailscale / WireGuard | Our own relay |
|---|---|---|---|
| Works away from home | ❌ | ✅ | ✅ |
| Extra code for us | — | **none** | a service to run, pay for, and secure |
| HTTPS (⇒ web push, PWA install) | ✗ private-IP http only | ✅ certs handled | ✅ |
| Setup burden on the user | none | installs a VPN app | none |
| Traffic through a third party | no | no (peer-to-peer) | **yes — ours** |

**Recommended: build the listener once; ship LAN as the default and document
Tailscale as the away-from-home path. Never build a relay.** A relay would make
*us* the rented server this whole document refuses, plus an attack surface and
a running cost carried by a solo maintainer.

Consequence to accept honestly: **on plain LAN there are no push
notifications** — no HTTPS, no service worker. LAN is *"I am home but not at
the desk"*, which is a real and common case. Notifications arrive with
Tailscale, and that is a fair line to draw as long as the UI says which one
you are on rather than letting the user discover it by missing an approval.

### Plain `http://` is the right call on LAN, and self-signed is the wrong one

Counter-intuitive but firm: over a private IP, plain HTTP opens with **no
warning at all**, while a self-signed certificate opens with a full-page red
interstitial the user has to defeat. Reaching for HTTPS here would buy the
service worker at the cost of the one thing this feature is being judged on —
that it just works when you scan it. **HTTPS arrives with Tailscale, which
issues real certificates, or it does not arrive.**

What that costs, stated plainly so it is not discovered later: on LAN the page
must be open to show an approval. It cannot wake a locked phone.

## Pairing: scan it and it works

**Target (owner, 14 Aug): "เปิดมาแสกนแล้วเช็คว่าในวงแลนเดียวกัน เจอแล้ว ใช้ได้เลย."
Zero fields typed, on either device.** Everything in this section exists to
hold that line.

**Desktop side** — Settings grows one page with one switch. Turning it on
immediately shows a QR and the plain-language line *"กำลังฟังอยู่บน `<ชื่อ Wi-Fi>`"*.
No port field, no address field, no password field. Below it, the list of
paired phones with a revoke button each. That is the entire UI.

**Which address goes in the QR is the whole problem.** A Windows machine has
many interfaces — Wi-Fi, Ethernet, WSL `vEthernet`, Hyper-V, Docker, VPN,
link-local — and a QR pointing at the wrong one is a dead page with nothing to
retry. Do not enumerate and guess. Ask the OS which interface it would actually
use to leave the house:

```go
c, _ := net.Dial("udp", "8.8.8.8:80")   // sends nothing; UDP dial only binds
defer c.Close()
local := c.LocalAddr().(*net.UDPAddr).IP // the default-route interface
```

That is the LAN address in effectively every real case, and it sidesteps the
whole interface-ranking problem. Virtual adapters are not on the default route.

**Phone side** — the camera opens the URL and there is nothing more to do. The
QR carries the one-time token in the URL **fragment** (`…/p#t=<token>`), which
browsers never transmit; the page reads it, POSTs it once to trade it for a
long-lived cookie bound to that device, and rewrites the address bar. The token
is single-use and expires in minutes, so a QR left on screen or in a screenshot
is not a standing key.

**Same-LAN is enforced, not assumed.** The server refuses any request whose
`RemoteAddr` is outside the subnet it is bound to. This is what makes the
promise in the owner's sentence a property of the system rather than a
description of the usual case, and it is a handful of lines.

**Returning later does not mean scanning again.** The cookie is bound to the
install, not the address, so opening the page again just works. The one thing
that breaks it is the machine's DHCP lease changing, which on a home router is
rare — and the answer is to scan again, which costs two seconds. Registering
an mDNS name (`aetox.local`) as a fallback the page tries when the stored
address goes quiet is a cheap second layer, worth doing only if lease churn
turns out to be real.

**Where this is honestly weaker than it sounds:** on an untrusted network —
a café, a co-working space — "same LAN" is not a meaningful boundary and the
traffic is unencrypted. The switch's own copy must say so in one sentence
rather than leaving the user to infer it, and Tailscale is the answer for
anyone who wants this away from a network they own.

## Slices

1. **Listener + pairing.** HTTP server inside the desktop process, off by
   default, switched on from Settings. Pairing by QR shown on the desktop;
   the token is stored, listed, and revocable from the same page. No engine
   changes.
2. **Read-only monitor.** What is running, what it has done, what it is stuck
   on. Requires dependency 1 above.
3. **Approve from the phone.** The `pending` queue, plus a real waiting
   `Approve`. **This is the feature; 1 and 2 exist to make it possible.**
4. **A short reply into a session that already exists.** Not a new desk — one
   line of text into work that is already running.

Anything past slice 4 is a second desk and is out of scope by the rule at the
top.

## Deliberately not in this

- Starting new work from the phone with a fresh project, file panes, or a
  terminal. That is the desk, and the desk stays where it is.
- A separate daemon binary. **One process owns the store, the MCP children,
  and the browser.** Two processes over one SQLite file is two versions of the
  truth. The window minimises; it does not need to close.
- Anything a cloud account would be required for.

## Security — the two holes the working slice exposed

Both were found by having something to attack rather than something to read,
which is the argument for having built the slice at all.

**1. A scan is not consent.** The QR is on a screen, and a screen can be
shoulder-surfed, screen-shared, or caught in a screenshot pasted into a bug
report. Nothing on the desktop asks whether the phone that just scanned is
yours. The fix is the pattern Bluetooth and Tailscale already use: show a
6-digit code on **both** screens and make the desktop answer yes or no. Pairing
then needs the screen *and* the machine, not just the screen.

**2. Plain `http://` is the bigger hole, and it is not the QR.** A passive
listener on the same Wi-Fi reads the device token off the wire and is in
permanently, **having never scanned anything**. This was defensible while the
phone could only watch things the user could already see; it stops being
defensible the moment the phone can command the machine. *Changing what the
phone may do changes what the transport must provide* — that link is the whole
lesson here.

Related: "same LAN" is a real boundary at home and a meaningless one in a café
or an office. The gate is worth keeping either way, but it must never be
mistaken for authentication.

## Decide the client later — but only if the server is built for it

The Go side is identical whether the client is a PWA or a native app, so the
choice does not have to be made now. It only stays open if the API refuses to
assume a browser:

| Browser-shaped (what the parked slice does) | Client-agnostic (what to build) |
|---|---|
| `Set-Cookie` device token | bearer token in a header |
| token read from the URL fragment | pairing returns JSON |
| server serves the HTML page | JSON + SSE only, no HTML |

**What tilts it toward native, and it is not notifications.** Encryption with
no public CA needs a certificate the client will trust, and a QR is a perfect
out-of-band channel for a fingerprint: scan once, pin forever, real TLS on any
network with no setup. A native app can pin. **A browser cannot** — it shows
the full-page interstitial and offers no way to say "I verified this out of
band." Push notifications are a second, smaller reason.

Note this reverses the earlier "no separate app" conclusion, and it should:
that conclusion was drawn for a phone that could only watch. The premise
changed; the answer changed with it.

## What the owner still has to settle

1. ~~LAN-only for slice 1, or Tailscale from the start?~~ **Settled 14 Aug:
   LAN, zero-config, scan-and-go.** Tailscale stays a documented path for
   away-from-home, never a setup step. The cost accepted with it is that the
   page cannot wake a locked phone.
2. ~~Does the phone get slice 4 at all?~~ **Overtaken 14 Aug.** Watch-and-approve
   turned out not to be worth a surface on its own — the owner's verdict on the
   working build was *"ได้แค่เข้ามาดูประวัติแชท"*. Commanding the machine is the
   floor, not an extra, and that is what pushed this from a panel to a product.
3. **Is the phone a remote or a third desk?** The rule at the top of this
   document says remote. A phone with features *เทียบเคียงกัน* with the desktop
   may not fit under it. Whoever unparks this decides — and should overturn the
   rule deliberately if that is the answer, rather than drift past it.
4. **Is this 0.9.x side-work, or the first thing after 1.0.0?** It is on none
   of the five 1.0.0 criteria in [ROADMAP.md](../../ROADMAP.md), and the Unix
   CI failures still are. Recommendation: **after 1.0.0** — except for the
   background-task storage shape in dependency 1, which should be decided now
   because it is free now and expensive later.
