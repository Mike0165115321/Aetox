package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// newRemote builds a server holder over a test App, bound to a subnet the test
// controls so gate decisions are deterministic rather than dependent on
// whatever network the machine running the suite happens to be on.
func newRemote(t *testing.T) *remoteServer {
	t.Helper()
	a := newJobApp(t)
	_, subnet, err := net.ParseCIDR("192.168.1.0/24")
	if err != nil {
		t.Fatalf("parse cidr: %v", err)
	}
	return &remoteServer{
		app:       a,
		ip:        net.ParseIP("192.168.1.40"),
		subnet:    subnet,
		port:      remotePortFirst,
		pairToken: "opensesame",
		pairUntil: time.Now().Add(pairWindow),
	}
}

// The promise the feature is sold on is "same Wi-Fi". If that is only true of
// the usual case it is not a promise, so it is checked here as a property of
// the handler and not of the network.
func TestGateAdmitsOnlyTheBoundSubnet(t *testing.T) {
	r := newRemote(t)
	h := r.gate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot) // proof the request got past the gate
	}))

	for _, tc := range []struct {
		peer string
		want int
	}{
		{"192.168.1.55:5000", http.StatusTeapot},    // the phone
		{"127.0.0.1:5000", http.StatusTeapot},       // the desktop itself
		{"172.21.224.1:5000", http.StatusForbidden}, // WSL's vEthernet — a real
		{"10.0.0.9:5000", http.StatusForbidden},     // adapter on the same box
		{"203.0.113.7:5000", http.StatusForbidden},  // the open internet
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
		req.RemoteAddr = tc.peer
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("peer %s: got %d, want %d", tc.peer, w.Code, tc.want)
		}
	}
}

// A server that has not been started has no subnet, and "no subnet" must read
// as "admit nobody" rather than as "no restriction" — the failure mode of the
// opposite default is a machine wide open the moment start() errors.
func TestGateWithNoSubnetRefusesEveryone(t *testing.T) {
	r := &remoteServer{}
	h := r.gate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	req.RemoteAddr = "192.168.1.55:5000"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403 — an unstarted server must not be open", w.Code)
	}
}

func pairRequest(token string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/pair",
		strings.NewReader(url.Values{"t": {token}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0)")
	req.RemoteAddr = "192.168.1.55:5000"
	return req
}

// The QR is on a screen and probably in a screenshot, so the token it carries
// has to stop working the moment it is spent.
func TestPairTokenIsSingleUse(t *testing.T) {
	r := newRemote(t)

	w := httptest.NewRecorder()
	r.apiPair(w, pairRequest("opensesame"))
	if w.Code != http.StatusNoContent {
		t.Fatalf("first pair: got %d, want 204", w.Code)
	}
	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == deviceCookie {
			cookie = c
		}
	}
	if cookie == nil || cookie.Value == "" {
		t.Fatal("pairing issued no device cookie")
	}
	if !cookie.HttpOnly {
		t.Error("device cookie must be HttpOnly — page scripts have no reason to read it")
	}

	w2 := httptest.NewRecorder()
	r.apiPair(w2, pairRequest("opensesame"))
	if w2.Code != http.StatusForbidden {
		t.Fatalf("replayed token: got %d, want 403", w2.Code)
	}
}

func TestPairRejectsWrongAndExpiredTokens(t *testing.T) {
	r := newRemote(t)
	w := httptest.NewRecorder()
	r.apiPair(w, pairRequest("guess"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("wrong token: got %d, want 403", w.Code)
	}

	r2 := newRemote(t)
	r2.pairUntil = time.Now().Add(-time.Second)
	w2 := httptest.NewRecorder()
	r2.apiPair(w2, pairRequest("opensesame"))
	if w2.Code != http.StatusForbidden {
		t.Fatalf("expired token: got %d, want 403", w2.Code)
	}
}

// The device table must not be a list of working keys. If the cookie itself
// were stored, reading aetox.db would be enough to walk in.
func TestPairedCookieIsStoredOnlyAsAHash(t *testing.T) {
	r := newRemote(t)
	w := httptest.NewRecorder()
	r.apiPair(w, pairRequest("opensesame"))

	var secret string
	for _, c := range w.Result().Cookies() {
		if c.Name == deviceCookie {
			secret = c.Value
		}
	}
	if secret == "" {
		t.Fatal("no cookie issued")
	}

	db, err := r.app.database()
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	var stored string
	if err := db.QueryRow(`SELECT id FROM remote_devices`).Scan(&stored); err != nil {
		t.Fatalf("read device row: %v", err)
	}
	if stored == secret {
		t.Fatal("the raw cookie is in the table — the device list is a list of keys")
	}
	if stored != hashToken(secret) {
		t.Fatalf("stored id is not the cookie's hash: %q", stored)
	}
}

// Pairing is what a device cookie is for; nothing past it may be reachable
// without one, and revoking has to actually shut the door.
func TestGuardRequiresAPairedDeviceAndHonoursRevoke(t *testing.T) {
	r := newRemote(t)
	guarded := r.guard(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	// No cookie at all.
	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	req.RemoteAddr = "192.168.1.55:5000"
	w := httptest.NewRecorder()
	guarded(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unpaired: got %d, want 401", w.Code)
	}

	// Pair, then use the cookie it issued.
	pw := httptest.NewRecorder()
	r.apiPair(pw, pairRequest("opensesame"))
	var cookie *http.Cookie
	for _, c := range pw.Result().Cookies() {
		if c.Name == deviceCookie {
			cookie = c
		}
	}
	req2 := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	req2.RemoteAddr = "192.168.1.55:5000"
	req2.AddCookie(cookie)
	w2 := httptest.NewRecorder()
	guarded(w2, req2)
	if w2.Code != http.StatusTeapot {
		t.Fatalf("paired device: got %d, want it through", w2.Code)
	}

	// Revoke it and try the same cookie again.
	devices := r.app.PairedDevices()
	if len(devices) != 1 {
		t.Fatalf("want one paired device, got %d", len(devices))
	}
	if err := r.app.RevokeDevice(devices[0].ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	req3 := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	req3.RemoteAddr = "192.168.1.55:5000"
	req3.AddCookie(cookie)
	w3 := httptest.NewRecorder()
	guarded(w3, req3)
	if w3.Code != http.StatusUnauthorized {
		t.Fatalf("revoked device: got %d, want 401", w3.Code)
	}
	if got := r.app.PairedDevices(); len(got) != 0 {
		t.Errorf("revoked device still listed: %+v", got)
	}
}

// Revoking stamps the row rather than deleting it — the audit trail is the
// whole reason the table can be trusted.
func TestRevokeKeepsTheRowForTheAuditTrail(t *testing.T) {
	r := newRemote(t)
	w := httptest.NewRecorder()
	r.apiPair(w, pairRequest("opensesame"))

	devices := r.app.PairedDevices()
	if err := r.app.RevokeDevice(devices[0].ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	db, _ := r.app.database()
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM remote_devices WHERE revoked_at != ''`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("want the revoked row kept and stamped, found %d", n)
	}
}

// A phone shows up in the list as something a person recognises.
func TestDeviceLabelNamesThePhone(t *testing.T) {
	for ua, want := range map[string]string{
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)": "iPhone",
		"Mozilla/5.0 (Linux; Android 14; Pixel 8)":               "Android",
		"Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X)":          "iPad",
		"": "อุปกรณ์",
	} {
		if got := deviceLabel(ua); got != want {
			t.Errorf("deviceLabel(%q) = %q, want %q", ua, got, want)
		}
	}
}

// lanAddress must never hand back an address it cannot describe a subnet for,
// because the gate is built from that subnet — a nil one has to mean "refuse",
// which the gate test above pins, and this pins the pairing side of it.
func TestLanAddressAgreesWithItsSubnet(t *testing.T) {
	ip, subnet := lanAddress()
	if ip == nil {
		t.Skip("no route out of this machine — nothing to check")
	}
	if subnet == nil {
		t.Fatal("an address with no subnet would leave the gate with nothing to enforce")
	}
	if !subnet.Contains(ip) {
		t.Fatalf("subnet %s does not contain its own address %s", subnet, ip)
	}
	if ip.IsLoopback() {
		t.Errorf("picked the loopback address %s — a phone can never reach it", ip)
	}
}

// The phone page is embedded, not read from disk: a missing file has to break
// the build rather than serve a blank screen to a paired phone.
func TestPhonePageIsEmbedded(t *testing.T) {
	if !strings.Contains(remotePage, "/api/state") {
		t.Fatal("the embedded phone page does not talk to the state endpoint")
	}
	if !strings.Contains(remotePage, "manifest.webmanifest") {
		t.Error("no manifest link — the page cannot become a home-screen app")
	}
	// The token must be read from the fragment. If it ever moves into the query
	// string it starts appearing in request lines and logs.
	if !strings.Contains(remotePage, "location.hash") {
		t.Error("the pairing token must come from the URL fragment, not the query")
	}
}
