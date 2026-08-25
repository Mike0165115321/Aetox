package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/account"
)

// runAccountCommand handles `aetox account`, which is a different thing from
// `aetox login` next door: that one signs in to a model provider so a request
// can be paid for, this one signs in to Aetox itself. Keeping them separate
// verbs keeps "signed in" from meaning two things in the same sentence.
func runAccountCommand(args []string) int {
	if len(args) == 0 {
		return runAccountStatus()
	}
	rest := args[1:]
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "login", "signin":
		return runAccountLogin(rest)
	case "logout", "signout":
		return runAccountLogout()
	case "whoami":
		return runAccountWhoami()
	default:
		printAccountUsage()
		return 2
	}
}

func printAccountUsage() {
	fmt.Printf("\n  usage: aetox account <command>\n\n")
	fmt.Println("    login [github|google]   sign in to Aetox through that door")
	fmt.Println("    logout                  sign out on this machine and on the server")
	fmt.Println("    whoami                  ask the server who this session belongs to")
	fmt.Println()
	fmt.Println("  Signing in is optional. Everything in Aetox works signed out.")
	fmt.Println()
}

func runAccountStatus() int {
	// Not an error and not a failure to sign in: there is nothing to sign in
	// to in this build yet.
	if !account.Configured() {
		fmt.Printf("\n  %v\n\n", account.ErrNotOpen)
		return 0
	}
	user, ok := account.Current()
	fmt.Println()
	if !ok {
		fmt.Println("  not signed in to Aetox")
		fmt.Println("  run: aetox account login github")
	} else {
		fmt.Printf("  signed in as %s\n", user.Display())
		if user.Email != "" && user.Email != user.Display() {
			fmt.Printf("  %s\n", user.Email)
		}
	}
	fmt.Printf("\n  id server: %s\n", account.BaseURL())
	fmt.Printf("  session:   %s\n\n", account.StorePath())
	return 0
}

func runAccountLogin(args []string) int {
	provider := "github"
	if len(args) > 0 {
		provider = strings.ToLower(strings.TrimSpace(args[0]))
	}

	// Ctrl-C cancels the wait rather than killing the process mid-flow: the
	// local listener needs to come down with it.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	pending, err := account.Start(provider)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		fmt.Fprintf(os.Stderr, "doors: %s\n", strings.Join(account.Providers(), ", "))
		return 2
	}
	defer pending.Cancel()

	fmt.Printf("\n  opening %s\n\n", pending.URL)
	openBrowser(pending.URL)
	fmt.Println("  waiting for the browser to come back… (ctrl-c to cancel)")

	sess, err := pending.Wait(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nsign-in failed: %v\n", err)
		return 1
	}
	fmt.Printf("\n  signed in as %s\n\n", sess.User.Display())
	return 0
}

func runAccountLogout() int {
	if _, ok := account.Current(); !ok {
		fmt.Println("not signed in to Aetox")
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// The local half of SignOut always happens; an error means only that the
	// server was not told. Saying so is the difference between a sign-out and
	// a sign-out the user believes is complete.
	if err := account.SignOut(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "signed out on this machine, but the server was not told: %v\n", err)
		return 1
	}
	fmt.Println("signed out")
	return 0
}

func runAccountWhoami() int {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	user, err := account.Me(ctx)
	if errors.Is(err, account.ErrSignedOut) {
		fmt.Fprintln(os.Stderr, "not signed in to Aetox")
		return 1
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not ask the id server: %v\n", err)
		return 1
	}
	fmt.Printf("%s <%s>\n", user.Display(), user.Email)
	return 0
}
