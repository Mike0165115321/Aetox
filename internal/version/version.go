// Package version is the one place Aetox's own release number lives.
//
// It used to live in five: cmd/aetox/main.go's const, desktop/wails.json's
// productVersion, README's status heading, docs/index.html's badge, and
// scoop/aetox.json. Bumping five copies by hand worked only because nothing
// ever read them back — the moment the app starts *checking* for updates, a
// missed bump stops being a typo and becomes the app lying to the user about
// which release they are running, in the one place they went to find out.
//
// The four non-Go copies cannot import a constant, so version_test.go asserts
// they agree with this one. That test rides along with `go test ./...`, which
// CI already runs on all three platforms, so a stale copy fails the build
// instead of shipping.
package version

// Current is the release this build calls itself. Bump it here first — the
// test will then name every other file that still disagrees.
//
// Deliberately a plain constant rather than an -ldflags stamp: `wails dev` and
// `go run` are how this app is actually run all day, and a stamp leaves both
// of them reporting an empty or "dev" version — i.e. the update check would be
// dark exactly where it is being developed.
const Current = "1.5.6"

// Credit is printed by both --version paths and shown in Settings → About. A
// rebranded fork has to delete this line by hand rather than just swap a logo,
// which is the whole point of it being here — and since v1.3.0 deleting it is
// also a breach of the licence rather than merely rude (LICENSE §5(d)).
//
// Settings → About reads this through App.AppCredit rather than restating it,
// because the one string in the running app that names the licence must not be
// able to name a different licence than the LICENSE file does.
const Credit = "© 2026 Chayaphon Phromsawana (Mike) · All rights reserved · github.com/Mike0165115321/Aetox"
