package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/oauth"
	"github.com/Mikedev115/Aetox/internal/proc"
)

// runAuthCommand handles `aetox login`, `aetox logout`, `aetox auth` and
// `aetox account`. It reports whether it consumed the invocation, plus the
// exit code.
//
// The first three are about a model provider — which account pays for a
// request. `account` is about Aetox itself and is handled in account.go; the
// two are kept apart so that "signed in" never has to mean both at once.
func runAuthCommand(args []string) (handled bool, code int) {
	if len(args) == 0 {
		return false, 0
	}
	rest := args[1:]
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "login":
		return true, runLogin(rest)
	case "logout":
		return true, runLogout(rest)
	case "auth":
		return true, runAuthStatus()
	case "account":
		return true, runAccountCommand(rest)
	default:
		return false, 0
	}
}

func runLogin(args []string) int {
	if len(args) == 0 {
		printSignInMethods()
		return 0
	}
	provider := strings.TrimSpace(args[0])

	// Adopting a session the matching official CLI already holds on this
	// machine beats making the user authorize the same account twice. Opt-in
	// by name, never automatic — reading another tool's credential file is
	// something you ask for.
	if len(args) > 1 && strings.EqualFold(strings.TrimSpace(args[1]), "--import") {
		return runImport(provider)
	}
	if _, ok := oauth.MethodFor(provider); !ok {
		fmt.Fprintf(os.Stderr, "no sign-in for %q.\n\n", provider)
		printSignInMethods()
		return 2
	}
	method, _ := oauth.MethodFor(provider)

	// Ctrl-C during a sign-in cancels the wait instead of killing the process
	// mid-flow: a device-code poll can run for minutes and the user needs a way
	// out that still cleans up the local listener.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	pending, err := oauth.Start(ctx, provider)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sign-in failed: %v\n", err)
		return 1
	}
	defer pending.Cancel()

	// Printed for every method, not only the restricted ones it used to gate on:
	// since §70 the Note carries what the flow needs rather than a caveat about
	// it — which port must be free, whose quota pays — and that is worth reading
	// before the browser opens, not after it fails.
	fmt.Printf("\n  %s\n  %s\n\n", method.Label, method.Note)

	pasted := ""
	switch method.Kind {
	case "device":
		fmt.Printf("  1. open  %s\n", pending.VerificationURI)
		fmt.Printf("  2. enter %s\n\n", pending.UserCode)
		openBrowser(pending.URL)
		fmt.Println("  waiting for approval… (ctrl-c to cancel)")
	case "browser":
		fmt.Printf("  opening %s\n\n", pending.URL)
		openBrowser(pending.URL)
		fmt.Println("  waiting for the browser to come back… (ctrl-c to cancel)")
	case "paste":
		fmt.Printf("  1. open %s\n", pending.URL)
		fmt.Println("  2. click Authorize, then copy the code it shows")
		fmt.Println()
		openBrowser(pending.URL)
		fmt.Print("  code: ")
		reader := bufio.NewReader(os.Stdin)
		line, readErr := reader.ReadString('\n')
		if readErr != nil && strings.TrimSpace(line) == "" {
			fmt.Fprintln(os.Stderr, "\nno code entered")
			return 1
		}
		pasted = strings.TrimSpace(line)
	}

	if err := oauth.Finish(ctx, pending, pasted); err != nil {
		fmt.Fprintf(os.Stderr, "\nsign-in failed: %v\n", err)
		return 1
	}

	status := oauth.StatusFor(provider)
	label := status.Label
	if label == "" {
		label = provider
	}
	fmt.Printf("\n  signed in — %s\n", label)
	fmt.Printf("  run: aetox --model-provider %s\n\n", oauth.StatusFor(provider).Provider)
	return 0
}

func runLogout(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: aetox logout <provider>")
		return 2
	}
	provider := strings.TrimSpace(args[0])
	if !oauth.Has(provider) {
		fmt.Printf("not signed in to %s\n", provider)
		return 0
	}
	if err := oauth.Logout(provider); err != nil {
		fmt.Fprintf(os.Stderr, "sign-out failed: %v\n", err)
		return 1
	}
	fmt.Printf("signed out of %s\n", provider)
	return 0
}

func runAuthStatus() int {
	fmt.Println()
	for _, method := range oauth.Methods() {
		status := oauth.StatusFor(method.Provider)
		state := "—"
		if status.SignedIn {
			state = "signed in"
			if status.Account != "" {
				state += " as " + status.Account
			}
			if status.ExpiresAt > 0 {
				left := time.Until(time.UnixMilli(status.ExpiresAt))
				if left > 0 {
					state += fmt.Sprintf(" (token %s left)", left.Round(time.Minute))
				}
			}
		}
		fmt.Printf("  %-16s %-24s %s\n", method.Provider, method.Label, state)
	}
	fmt.Printf("\n  credentials: %s\n\n", oauth.StorePath())
	return 0
}

func runImport(provider string) int {
	var err error
	switch strings.ToLower(provider) {
	case "codex":
		err = oauth.ImportCodexCLI()
	default:
		fmt.Fprintf(os.Stderr, "%s has no session to import — run `aetox login %s` instead\n", provider, provider)
		return 2
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "import failed: %v\n", err)
		return 1
	}
	status := oauth.StatusFor(provider)
	fmt.Printf("\n  signed in — %s\n\n", status.Label)
	return 0
}

func printSignInMethods() {
	fmt.Printf("\n  usage: aetox login <provider>\n\n")
	for _, method := range oauth.Methods() {
		mark := " "
		if oauth.Has(method.Provider) {
			mark = "*"
		}
		fmt.Printf("  %s %-16s %s\n", mark, method.Provider, method.Note)
	}
	fmt.Println("\n  * = already signed in")
	// Only offered when there is actually something to adopt — advertising an
	// import for a CLI the user does not have is noise.
	if oauth.CodexCLIAvailable() {
		fmt.Println("\n  aetox login codex --import         adopt the Codex CLI session on this machine")
	}
	fmt.Println()
}

// openBrowser is best effort. Every flow prints the URL first, so a machine
// with no browser (a container, a remote shell) is inconvenienced, not blocked.
func openBrowser(url string) {
	if strings.TrimSpace(url) == "" {
		return
	}
	// proc-detached: the user's browser is meant to outlive the CLI — the
	// sign-in finishes in it, long after this function has returned.
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	proc.HideConsole(cmd)
	_ = cmd.Start()
}
