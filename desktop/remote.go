package main

// PARKED 2026-08-14. This file compiles, is tested, and opens no port: nothing
// calls StartMobileRemote now that the pairing panel is unmounted
// (MobileRemote.svelte), so an install that never asks for a phone never
// listens on anything. It is kept because the parts below survive whatever the
// phone surface turns out to be — picking the right LAN address among a
// machine's virtual adapters, the same-subnet gate, and the device table — and
// because throwing away a tested foundation to rewrite it later is the
// expensive way to reach the same place.
//
// Two things must change before it is unparked, both settled on 14 Aug:
//
//   - **Pairing must be confirmed on the desktop.** Today a scan is enough,
//     which means anyone who can see the screen can walk in. The fix is a
//     6-digit code on both screens and a yes/no on the desktop.
//   - **The wire must be encrypted.** Plain http on a LAN was defensible while
//     the phone could only watch; it is not once the phone can command this
//     machine, because a passive listener on the same Wi-Fi reads the device
//     token and needs no QR at all.
//
// And one rule for whoever picks this up: **do not shape the API around a
// browser.** Bearer token in a header, not a cookie; pairing returns JSON, not
// Set-Cookie; no reliance on the URL-fragment trick. A native client is the
// likely answer (only it can pin a self-signed certificate handed over by QR,
// which is what buys encryption with no CA and no setup), and a server built
// for a browser would have to be rewritten to serve one.
//
// Full reasoning: docs/architecture/mobile-remote-2026-08-14.md
//
// ---
//
// The phone as a remote control for the Aetox already running on this machine
// — never as a second Aetox (docs/architecture/mobile-remote-2026-08-14.md).
//
// Three rules hold this file's shape, and every awkward-looking choice below is
// one of them being obeyed:
//
//  1. **The phone has no settings.** Not one field. Model, provider, keys,
//     permissions, agents, projects — the phone inherits all of them because it
//     is a second screen on this process, not a second install. A setting that
//     exists in two places is a setting that will disagree with itself.
//  2. **No second gate.** Every endpoint here is a thin adapter over an App
//     method the desktop already calls (ApprovePendingChange, ListSessions, …).
//     The moment an endpoint answers a question differently from its Wails
//     binding there are two Aetoxes, so this file computes nothing it could
//     instead ask for.
//  3. **Same LAN is enforced, not assumed.** `gate` refuses anything off the
//     subnet the listener is bound to. That is what makes the promise a
//     property of the system rather than a description of the usual case.
//
// What is deliberately NOT here: the tool-approval prompt (turn.ApprovalPromptFunc).
// That one blocks a running turn, and a host that answers it must be able to
// park and wait for a real human rather than time out into a yes — the design
// doc's §"The hard part is Approve". This slice carries the durable
// `pending_changes` queue only, which is state in the store and safe to decide
// from anywhere.

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "embed"

	qrcode "github.com/skip2/go-qrcode"
)

// remotePage is the phone's whole UI. One embedded file rather than a second
// Vite entry: it ships inside the same binary as the desktop frontend, so the
// two can never be different versions of each other — which is the argument
// against a native app restated as a build decision.
//
//go:embed remote_page.html
var remotePage string

const (
	// remotePortFirst is where the listener tries first. A fixed, memorable
	// port beats an ephemeral one because the phone's stored address has to
	// survive a restart of the desktop.
	remotePortFirst = 7317
	// remotePortTries bounds the walk upward when something else holds the
	// port, so a busy machine still gets a server instead of an error.
	remotePortTries = 8

	// pairWindow is how long a QR stays good. Short, because the token is on
	// screen and possibly in a screenshot; long enough to find your phone.
	pairWindow = 3 * time.Minute
)

// RemoteStatus is what the pairing panel shows. Never nil-valued: the panel
// renders the same shape whether the server is up or down (§34).
type RemoteStatus struct {
	Running bool `json:"running"`
	// Address is what the phone would open, "" when not running.
	Address string `json:"address"`
	// Subnet is the range the gate admits, shown so the user can tell at a
	// glance which network they are exposing this on.
	Subnet string `json:"subnet"`
	// Devices already paired, newest first.
	Devices []RemoteDevice `json:"devices"`
	// Error is a Thai sentence when the last start attempt failed.
	Error string `json:"error"`
}

// RemoteDevice is one paired phone as the panel lists it.
type RemoteDevice struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	PairedAt string `json:"pairedAt"`
	LastSeen string `json:"lastSeen"`
}

type remoteServer struct {
	app *App

	mu      sync.Mutex
	srv     *http.Server
	ip      net.IP
	subnet  *net.IPNet
	port    int
	lastErr string

	// pairToken is the one-time secret currently encoded in the QR. Cleared
	// the moment it is spent, so a QR left on screen is not a standing key.
	pairToken string
	pairUntil time.Time
}

// ---------------------------------------------------------------- bindings

// StartMobileRemote brings the listener up and mints a fresh pairing token.
// Called by the panel opening, so the QR on screen is always live.
func (a *App) StartMobileRemote() RemoteStatus {
	r := a.remote()
	if err := r.start(); err != nil {
		r.mu.Lock()
		r.lastErr = err.Error()
		r.mu.Unlock()
	}
	return a.MobileRemoteStatus()
}

// StopMobileRemote closes the door. Paired devices are kept — stopping is not
// revoking, and a user who toggles this off for the night expects their phone
// to still be paired in the morning.
func (a *App) StopMobileRemote() RemoteStatus {
	a.remote().stop()
	return a.MobileRemoteStatus()
}

// MobileRemoteStatus is the panel's poll.
func (a *App) MobileRemoteStatus() RemoteStatus {
	r := a.remote()
	r.mu.Lock()
	st := RemoteStatus{Running: r.srv != nil, Subnet: "", Error: r.lastErr}
	if r.srv != nil && r.ip != nil {
		st.Address = fmt.Sprintf("http://%s:%d", r.ip, r.port)
	}
	if r.subnet != nil {
		st.Subnet = r.subnet.String()
	}
	r.mu.Unlock()
	st.Devices = a.PairedDevices()
	return st
}

// MobileRemoteQR is the pairing URL as a data: URL, so the panel can show it
// with a plain <img> and the token never becomes a fetchable endpoint on the
// LAN. Empty string when the server is not running.
func (a *App) MobileRemoteQR() string {
	r := a.remote()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.srv == nil || r.ip == nil {
		return ""
	}
	if r.pairToken == "" || time.Now().After(r.pairUntil) {
		r.pairToken = randToken()
		r.pairUntil = time.Now().Add(pairWindow)
	}
	// The token rides in the URL *fragment*: browsers never transmit it, so it
	// reaches the page's JavaScript without ever appearing in a request line,
	// a proxy log, or this process's own access logging.
	url := fmt.Sprintf("http://%s:%d/m#t=%s", r.ip, r.port, r.pairToken)
	buf, err := qrcode.Encode(url, qrcode.Medium, 512)
	if err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf)
}

// PairedDevices lists phones that may connect, newest first.
func (a *App) PairedDevices() []RemoteDevice {
	out := []RemoteDevice{} // never nil: §34
	db, err := a.database()
	if err != nil {
		return out
	}
	rows, err := db.Query(
		`SELECT id, label, paired_at, last_seen FROM remote_devices
		  WHERE revoked_at = '' ORDER BY paired_at DESC`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var d RemoteDevice
		if err := rows.Scan(&d.ID, &d.Label, &d.PairedAt, &d.LastSeen); err != nil {
			continue
		}
		out = append(out, d)
	}
	return out
}

// RevokeDevice cuts one phone off. The row is kept and stamped rather than
// deleted, for the same reason pending_changes keeps decided rows: "this phone
// was allowed in on that day and cut off on this one" is the audit trail, and
// a table that deletes could never grow one.
func (a *App) RevokeDevice(id string) error {
	db, err := a.database()
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE remote_devices SET revoked_at = ? WHERE id = ?`,
		time.Now().Format(time.RFC3339), id)
	return err
}

// remote lazily builds the server holder. One per App.
func (a *App) remote() *remoteServer {
	a.remoteOnce.Do(func() { a.remoteSrv = &remoteServer{app: a} })
	return a.remoteSrv
}

// ---------------------------------------------------------------- lifecycle

func (r *remoteServer) start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.srv != nil {
		return nil
	}

	ip, subnet := lanAddress()
	if ip == nil {
		return errors.New("หาที่อยู่ในวงแลนไม่เจอ — เครื่องนี้ต่อเน็ตอยู่หรือเปล่า")
	}

	ln, port, err := listenNear(remotePortFirst)
	if err != nil {
		return fmt.Errorf("เปิดพอร์ตไม่ได้: %w", err)
	}

	r.ip, r.subnet, r.port, r.lastErr = ip, subnet, port, ""
	r.pairToken, r.pairUntil = randToken(), time.Now().Add(pairWindow)

	mux := http.NewServeMux()
	mux.HandleFunc("/m", r.page)
	mux.HandleFunc("/manifest.webmanifest", r.manifest)
	mux.HandleFunc("/icon.png", r.icon)
	mux.HandleFunc("/api/pair", r.apiPair)
	mux.HandleFunc("/api/state", r.guard(r.apiState))
	mux.HandleFunc("/api/decide", r.guard(r.apiDecide))
	mux.HandleFunc("/api/session", r.guard(r.apiSession))

	r.srv = &http.Server{
		Handler:           r.gate(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func(srv *http.Server, ln net.Listener) {
		_ = srv.Serve(ln)
	}(r.srv, ln)
	return nil
}

func (r *remoteServer) stop() {
	r.mu.Lock()
	srv := r.srv
	r.srv, r.pairToken = nil, ""
	r.mu.Unlock()
	if srv != nil {
		_ = srv.Close()
	}
}

// listenNear walks upward from a preferred port rather than failing, so
// something else already holding 7317 costs a different number, not the
// feature. Binds all interfaces: the gate, not the bind address, is what
// decides who may talk to it.
func listenNear(first int) (net.Listener, int, error) {
	var lastErr error
	for p := first; p < first+remotePortTries; p++ {
		ln, err := net.Listen("tcp", ":"+strconv.Itoa(p))
		if err == nil {
			return ln, p, nil
		}
		lastErr = err
	}
	return nil, 0, lastErr
}

// lanAddress asks the OS which interface it would use to leave the house, then
// finds the subnet that address sits in.
//
// The naive alternative — enumerate interfaces, take the first non-loopback —
// is wrong on exactly the machines this has to work on. A developer's Windows
// box carries a VPN adapter, a WSL vEthernet, sometimes Docker, and several
// down Ethernet ports, of which three or four can be "up" with an IPv4
// address. Encoding the wrong one in a QR produces a dead page with nothing to
// retry. A UDP "dial" sends no packet; it only asks the routing table, and
// virtual adapters are not on the default route.
func lanAddress() (net.IP, *net.IPNet) {
	c, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, nil
	}
	defer c.Close()
	ua, ok := c.LocalAddr().(*net.UDPAddr)
	if !ok || ua.IP == nil {
		return nil, nil
	}
	ip := ua.IP

	ifaces, err := net.Interfaces()
	if err == nil {
		for _, ifc := range ifaces {
			addrs, _ := ifc.Addrs()
			for _, a := range addrs {
				if n, ok := a.(*net.IPNet); ok && n.IP.Equal(ip) {
					return ip, n
				}
			}
		}
	}
	// The address is known but its mask is not — assume the /24 that every
	// home router hands out rather than admitting everything.
	if v4 := ip.To4(); v4 != nil {
		m := net.CIDRMask(24, 32)
		return ip, &net.IPNet{IP: v4.Mask(m), Mask: m}
	}
	return ip, nil
}

// ---------------------------------------------------------------- gates

// gate is rule 3: same LAN, enforced. It runs before routing, so there is no
// endpoint — not the manifest, not the icon — reachable from off-subnet.
func (r *remoteServer) gate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		host, _, err := net.SplitHostPort(req.RemoteAddr)
		if err != nil {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		peer := net.ParseIP(host)
		r.mu.Lock()
		subnet := r.subnet
		r.mu.Unlock()
		if peer == nil || (!peer.IsLoopback() && (subnet == nil || !subnet.Contains(peer))) {
			http.Error(w, "อยู่นอกวงแลนของเครื่องนี้", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, req)
	})
}

// guard is the device check on everything past pairing.
func (r *remoteServer) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if !r.knownDevice(req) {
			http.Error(w, "ยังไม่ได้จับคู่", http.StatusUnauthorized)
			return
		}
		next(w, req)
	}
}

const deviceCookie = "aetox_remote"

func (r *remoteServer) knownDevice(req *http.Request) bool {
	c, err := req.Cookie(deviceCookie)
	if err != nil || c.Value == "" {
		return false
	}
	db, err := r.app.database()
	if err != nil {
		return false
	}
	id := hashToken(c.Value)
	var revoked string
	err = db.QueryRow(`SELECT revoked_at FROM remote_devices WHERE id = ?`, id).Scan(&revoked)
	if errors.Is(err, sql.ErrNoRows) || err != nil || revoked != "" {
		return false
	}
	_, _ = db.Exec(`UPDATE remote_devices SET last_seen = ? WHERE id = ?`,
		time.Now().Format(time.RFC3339), id)
	return true
}

// ---------------------------------------------------------------- handlers

func (r *remoteServer) page(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// No caching: the page is served by the same binary it talks to, so a
	// stale copy is the one way the two could ever disagree about a version.
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(remotePage))
}

// apiPair trades the one-time token for a device cookie. Only what the token
// proves is stored: the cookie's hash, never the cookie, so the table is not a
// list of working keys if it is ever read.
func (r *remoteServer) apiPair(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if err := req.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	r.mu.Lock()
	want, until := r.pairToken, r.pairUntil
	got := req.FormValue("t")
	ok := want != "" && time.Now().Before(until) &&
		subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
	if ok {
		r.pairToken = "" // single use
	}
	r.mu.Unlock()
	if !ok {
		http.Error(w, "รหัสจับคู่ใช้ไม่ได้แล้ว — เปิด QR ใหม่", http.StatusForbidden)
		return
	}

	db, err := r.app.database()
	if err != nil {
		http.Error(w, "no store", http.StatusInternalServerError)
		return
	}
	secret := randToken()
	now := time.Now().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO remote_devices (id, label, paired_at, last_seen) VALUES (?,?,?,?)`,
		hashToken(secret), deviceLabel(req.UserAgent()), now, now,
	); err != nil {
		http.Error(w, "no store", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: deviceCookie, Value: secret, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		MaxAge: 60 * 60 * 24 * 365,
	})
	r.app.emitLearningChanged() // the desktop panel repaints its device list
	w.WriteHeader(http.StatusNoContent)
}

// remoteState is everything the phone shows, in one round trip. One endpoint
// rather than four: a phone on a slow link should cost one request per poll.
type remoteState struct {
	Approvals []PendingChange `json:"approvals"`
	Sessions  []SessionMeta   `json:"sessions"`
}

func (r *remoteServer) apiState(w http.ResponseWriter, _ *http.Request) {
	// Rule 2: both of these are the App methods the desktop calls. Nothing is
	// recomputed here, so the phone cannot show a different answer.
	st := remoteState{
		Approvals: r.app.ListPendingChanges(),
		Sessions:  r.app.ListAllSessions(),
	}
	if len(st.Sessions) > 20 {
		st.Sessions = st.Sessions[:20]
	}
	writeJSON(w, st)
}

func (r *remoteServer) apiDecide(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if err := req.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	id, err := strconv.ParseInt(req.FormValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	// Same two methods the review UI calls — including their side effects, so
	// approving from the phone writes the memory file exactly as approving at
	// the desk does.
	if req.FormValue("ok") == "1" {
		err = r.app.ApprovePendingChange(id)
	} else {
		err = r.app.RejectPendingChange(id)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (r *remoteServer) apiSession(w http.ResponseWriter, req *http.Request) {
	id := req.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "no id", http.StatusBadRequest)
		return
	}
	msgs, err := r.app.LoadSessionAnyProject(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if len(msgs) > 60 {
		msgs = msgs[len(msgs)-60:] // the tail is what a phone is for
	}
	writeJSON(w, msgs)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

// ---------------------------------------------------------------- PWA bits

// manifest and icon are the entire difference between "a web page" and an app
// with its own home-screen icon that opens without browser chrome. No service
// worker: that needs HTTPS, which plain-LAN cannot have without a certificate
// warning that would cost more than it buys.
func (r *remoteServer) manifest(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/manifest+json")
	_, _ = w.Write([]byte(`{
  "name": "Aetox",
  "short_name": "Aetox",
  "start_url": "/m",
  "scope": "/",
  "display": "standalone",
  "background_color": "#0b0f16",
  "theme_color": "#0b0f16",
  "icons": [{"src":"/icon.png","sizes":"192x192","type":"image/png","purpose":"any maskable"}]
}`))
}

func (r *remoteServer) icon(w http.ResponseWriter, _ *http.Request) {
	const s = 192
	img := image.NewRGBA(image.Rect(0, 0, s, s))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{0x0b, 0x0f, 0x16, 0xff}}, image.Point{}, draw.Src)
	fg := color.RGBA{0x6e, 0xa8, 0xfe, 0xff}
	c := float64(s) / 2
	for y := 0; y < s; y++ {
		for x := 0; x < s; x++ {
			dx, dy := float64(x)-c, float64(y)-c
			d := math.Sqrt(dx*dx + dy*dy)
			if (d > 46 && d < 68) || (dy > 0 && math.Abs(dx) < 11 && d < 68) {
				img.Set(x, y, fg)
			}
		}
	}
	w.Header().Set("Content-Type", "image/png")
	_ = png.Encode(w, img)
}

// ---------------------------------------------------------------- helpers

func randToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// hashToken is what goes in the table. The cookie itself is a bearer token;
// storing it would make the device list a list of working keys.
func hashToken(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// deviceLabel turns a user-agent into something a person recognises in the
// paired list. Best-effort by design: a wrong guess costs a fuzzy row label,
// so this stays a short list of the words phones actually send rather than a
// parser with opinions.
func deviceLabel(ua string) string {
	switch {
	case strings.Contains(ua, "iPhone"):
		return "iPhone"
	case strings.Contains(ua, "iPad"):
		return "iPad"
	case strings.Contains(ua, "Android"):
		return "Android"
	case ua == "":
		return "อุปกรณ์"
	}
	if len(ua) > 40 {
		ua = ua[:40]
	}
	return ua
}
