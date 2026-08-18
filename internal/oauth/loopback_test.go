package oauth

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// The provider controls error_description, and the page it lands in is
// same-origin with a listener that is at that moment holding an authorization
// code. The escaping is one call and easy to drop in a later edit, so the
// assertion is here rather than in anyone's memory.
func TestLoopbackEscapesTheProvidersErrorText(t *testing.T) {
	lb, err := startLoopback(0, "/cb")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer lb.close()

	const payload = `<script>alert(1)</script>`
	resp, err := http.Get(lb.RedirectURI + "?error=bad&error_description=" + payload)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	page := string(body)
	if strings.Contains(page, payload) {
		t.Errorf("the provider's error text reached the page unescaped:\n%s", page)
	}
	if !strings.Contains(page, "&lt;script&gt;") {
		t.Errorf("expected the text escaped and still shown, got:\n%s", page)
	}
}

// The other half: a real message still reads as one. Escaping that swallowed
// the text would hide the only thing telling the user why sign-in failed.
func TestLoopbackStillShowsWhatWentWrong(t *testing.T) {
	lb, err := startLoopback(0, "/cb")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer lb.close()

	resp, err := http.Get(lb.RedirectURI + "?error=access_denied&error_description=you+said+no")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "you said no") {
		t.Errorf("the reason for the failure did not reach the page:\n%s", body)
	}
}
