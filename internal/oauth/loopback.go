package oauth

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"time"
)

// loopback is a one-shot local web server that catches an OAuth redirect.
//
// Providers that let us choose the redirect URI get this instead of asking the
// user to copy a code out of a browser: the browser lands back on 127.0.0.1,
// the code arrives in a query parameter, and the page tells the user they can
// close the tab.
type loopback struct {
	// RedirectURI is what to send as redirect_uri / callback_url.
	RedirectURI string

	port    int
	srv     *http.Server
	results chan loopbackResult
}

type loopbackResult struct {
	code  string
	state string
	err   error
}

// startLoopback listens on 127.0.0.1. Pass port 0 for any free port; pass a
// specific one when the provider has the redirect URI registered against it
// (ChatGPT insists on 1455).
//
// host names the address to *advertise* in RedirectURI, which is not always the
// address we bind. OAuth redirect matching is a string comparison, so a client
// registered against "localhost" rejects "127.0.0.1" even though both resolve
// here. Empty means advertise what we bound.
func startLoopbackAs(host string, port int, path string) (*loopback, error) {
	lb, err := startLoopback(port, path)
	if err != nil {
		return nil, err
	}
	if host != "" {
		lb.RedirectURI = fmt.Sprintf("http://%s:%d%s", host, lb.port, path)
	}
	return lb, nil
}

func startLoopback(port int, path string) (*loopback, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		if port != 0 {
			return nil, fmt.Errorf("cannot listen on 127.0.0.1:%d — another sign-in may already be running: %w", port, err)
		}
		return nil, err
	}

	bound := listener.Addr().(*net.TCPAddr).Port
	lb := &loopback{
		RedirectURI: fmt.Sprintf("http://127.0.0.1:%d%s", bound, path),
		port:        bound,
		results:     make(chan loopbackResult, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		res := loopbackResult{code: q.Get("code"), state: q.Get("state")}
		if e := q.Get("error"); e != "" {
			desc := q.Get("error_description")
			if desc == "" {
				desc = e
			}
			res.err = errors.New(desc)
		} else if res.code == "" {
			res.err = errors.New("the provider redirected back without an authorization code")
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if res.err != nil {
			w.WriteHeader(http.StatusBadRequest)
			// Escaped because this text can be error_description straight off
			// the query string, and the page it lands in is same-origin with a
			// listener that is at that moment receiving an authorization code.
			// Small window, small blast radius, one function call.
			fmt.Fprintf(w, loopbackPage, "Sign-in failed", html.EscapeString(res.err.Error()))
		} else {
			fmt.Fprintf(w, loopbackPage, "Signed in", "You can close this tab and go back to Aetox.")
		}

		select {
		case lb.results <- res:
		default:
		}
	})

	lb.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = lb.srv.Serve(listener) }()
	return lb, nil
}

// wait blocks for the redirect. Cancel ctx to give up; close always runs.
func (l *loopback) wait(ctx context.Context) (code, state string, err error) {
	select {
	case <-ctx.Done():
		return "", "", ctx.Err()
	case res := <-l.results:
		return res.code, res.state, res.err
	}
}

func (l *loopback) close() {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = l.srv.Shutdown(shutdownCtx)
}

// loopbackPage is deliberately plain: it is rendered by the user's browser
// outside the app, and a stylesheet reference would be one more thing that can
// fail on the last step of a sign-in that already worked.
const loopbackPage = `<!doctype html><meta charset="utf-8"><title>Aetox</title>
<body style="font:16px system-ui;margin:0;display:grid;place-items:center;height:100vh;background:#111;color:#eee">
<div style="text-align:center"><h1 style="font-size:20px;margin:0 0 8px">%s</h1><p style="opacity:.7;margin:0">%s</p></div>
`
