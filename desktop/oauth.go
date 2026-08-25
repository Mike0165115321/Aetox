package main

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/oauth"
)

// Sign-in bindings — "use the plan you already pay for" instead of pasting a
// key. The flows themselves live in internal/oauth; everything here is the
// desktop's half: hold the in-flight authorization between the two calls the
// UI makes, and re-bootstrap the engine when a sign-in lands on the provider
// that is currently active.

// SignInPrompt is what the UI shows while waiting. Which fields are filled
// depends on Kind: "device" fills UserCode and VerificationURI (type this code
// into that page), "browser" and "paste" fill URL.
type SignInPrompt struct {
	Provider        string `json:"provider"`
	Kind            string `json:"kind"`
	URL             string `json:"url"`
	UserCode        string `json:"user_code,omitempty"`
	VerificationURI string `json:"verification_uri,omitempty"`
}

// pendingSignIns holds authorizations between StartSignIn and CompleteSignIn.
// ponytail: package-level rather than a field on App because there is exactly
// one App per process; move it onto App if the desktop ever hosts two.
var pendingSignIns = struct {
	sync.Mutex
	byProvider map[string]*pendingSignIn
}{byProvider: map[string]*pendingSignIn{}}

type pendingSignIn struct {
	pending *oauth.Pending
	// ctx spans both calls: CompleteSignIn blocks on it, so CancelSignIn's
	// cancel is what actually unblocks a user who changed their mind while a
	// device flow was still polling.
	ctx    context.Context
	cancel context.CancelFunc
}

// SignInMethods lists every provider Aetox can sign into, with the risk note
// the UI must show before the user commits.
func (a *App) SignInMethods() []oauth.Method {
	return oauth.Methods()
}

// SignInStatus reports whether one provider is signed in, and as whom. It
// never returns a token.
func (a *App) SignInStatus(providerName string) oauth.Status {
	return oauth.StatusFor(providerName)
}

// StartSignIn opens a sign-in and returns what to show the user. Nothing is
// stored until CompleteSignIn succeeds.
func (a *App) StartSignIn(providerName string) (SignInPrompt, error) {
	canonical := model.NormalizeProvider(providerName)
	method, ok := oauth.MethodFor(canonical)
	if !ok {
		return SignInPrompt{}, fmt.Errorf("%s has no sign-in — add an API key instead", canonical)
	}

	// Starting a second sign-in for the same provider abandons the first
	// rather than leaking its listener: the user clicking the button twice
	// means they gave up on the first attempt.
	a.CancelSignIn(canonical)

	ctx, cancel := context.WithCancel(context.Background())
	pending, err := oauth.Start(ctx, canonical)
	if err != nil {
		cancel()
		return SignInPrompt{}, err
	}

	pendingSignIns.Lock()
	pendingSignIns.byProvider[canonical] = &pendingSignIn{pending: pending, ctx: ctx, cancel: cancel}
	pendingSignIns.Unlock()

	return SignInPrompt{
		Provider:        canonical,
		Kind:            method.Kind,
		URL:             pending.URL,
		UserCode:        pending.UserCode,
		VerificationURI: pending.VerificationURI,
	}, nil
}

// CompleteSignIn finishes what StartSignIn began and stores the credential.
//
// It blocks: a device flow polls until the user approves in their browser, and
// a browser flow waits for the redirect. The UI shows the prompt and awaits
// this call; CancelSignIn is how the user backs out.
//
// pasted carries the code for providers that make the user copy one and is
// ignored by the rest.
func (a *App) CompleteSignIn(providerName, pasted string) (ModelInfo, error) {
	canonical := model.NormalizeProvider(providerName)

	pendingSignIns.Lock()
	entry := pendingSignIns.byProvider[canonical]
	pendingSignIns.Unlock()
	if entry == nil {
		return ModelInfo{}, fmt.Errorf("no %s sign-in in progress", canonical)
	}

	err := oauth.Finish(entry.ctx, entry.pending, strings.TrimSpace(pasted))

	pendingSignIns.Lock()
	delete(pendingSignIns.byProvider, canonical)
	pendingSignIns.Unlock()
	entry.cancel()
	entry.pending.Cancel()

	if err != nil {
		return ModelInfo{}, err
	}
	return a.reloadAfterCredentialChange(canonical)
}

// CancelSignIn abandons an in-flight sign-in and frees whatever it holds.
// Safe to call when nothing is in progress.
func (a *App) CancelSignIn(providerName string) {
	canonical := model.NormalizeProvider(providerName)

	pendingSignIns.Lock()
	entry := pendingSignIns.byProvider[canonical]
	delete(pendingSignIns.byProvider, canonical)
	pendingSignIns.Unlock()

	if entry != nil {
		entry.cancel()
		entry.pending.Cancel()
	}
}

// ImportableSignIns lists providers whose official CLI already holds a session
// on this machine, so Settings can offer "use the one you have" instead of a
// second authorization for the same account.
//
// One entry again since §69: the Codex CLI writes a session Aetox can adopt, so
// someone already signed into it never authorizes the same ChatGPT account
// twice. Settings hides the button on an empty list.
func (a *App) ImportableSignIns() []string {
	var out []string
	if oauth.CodexCLIAvailable() {
		out = append(out, "codex")
	}
	return out
}

// ImportSignIn adopts that existing session. Explicit action only — the button
// says which tool it is reading from.
func (a *App) ImportSignIn(providerName string) (ModelInfo, error) {
	canonical := model.NormalizeProvider(providerName)

	var err error
	switch canonical {
	case "codex":
		err = oauth.ImportCodexCLI()
	default:
		return ModelInfo{}, fmt.Errorf("%s has no session to import", canonical)
	}
	if err != nil {
		return ModelInfo{}, err
	}
	return a.reloadAfterCredentialChange(canonical)
}

// SignOut forgets a provider's credential. If it was the active provider the
// engine is re-bootstrapped, which is what surfaces "needs credentials" in the
// UI instead of failing on the user's next message.
func (a *App) SignOut(providerName string) (ModelInfo, error) {
	canonical := model.NormalizeProvider(providerName)
	a.CancelSignIn(canonical)
	if err := oauth.Logout(canonical); err != nil {
		return ModelInfo{}, err
	}
	return a.reloadAfterCredentialChange(canonical)
}

// reloadAfterCredentialChange re-bootstraps only when the change touches the
// provider in use — signing into OpenRouter while running on Ollama should not
// restart anything.
func (a *App) reloadAfterCredentialChange(canonical string) (ModelInfo, error) {
	// Before anything else, and for every credential change rather than only
	// for sign-out: what this provider last reported was about the credential
	// being replaced. Signing in over an existing session never passes through
	// Logout, so hanging this on SignOut alone would leave the commonest way of
	// switching accounts showing the previous account's numbers. See
	// App.forgetQuotas.
	a.forgetQuotas(canonical)
	if strings.EqualFold(model.NormalizeProvider(a.cur().cfg.ModelProvider), canonical) {
		next := a.cfg
		next.ModelAPIKey = resolveAPIKeyForProvider(canonical)
		if strings.TrimSpace(next.ModelName) == "" {
			next.ModelName = model.ResolveDefaultModel(canonical, next.ModelBaseURL, next.ModelAPIKey)
		}
		a.applyConfig(a.cur(), next)
	}
	return a.modelSwitchResult()
}
