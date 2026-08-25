package bootstrap

import (
	"errors"

	aetoxapp "github.com/Mikedev115/Aetox/internal/app"
)

// discardConsole is the console for a host that has no terminal: an HTTP
// server, an MCP server over stdio, a test. Writes go nowhere and a read fails
// immediately.
//
// The failing read is the point. app.NewApp requires a non-nil Console, and the
// obvious stand-in — NewStdIO — is wrong for a windowsgui or service process:
// os.Stdin there is an invalid handle, so a prompt does not block politely, it
// returns a confusing OS error from deep inside a tool call. A host with no
// human attached should never reach a console read at all; if it does, this
// says so plainly.
type discardConsole struct{}

// DiscardConsole returns a Console that writes nowhere and cannot read.
func DiscardConsole() aetoxapp.Console { return discardConsole{} }

var errNoConsole = errors.New("bootstrap: this host has no console to read from")

func (discardConsole) Print(any)                {}
func (discardConsole) Printf(string, ...any)    {}
func (discardConsole) Println(...any)           {}
func (discardConsole) Errorf(string, ...any)    {}
func (discardConsole) ReadLine() (string, error) { return "", errNoConsole }
