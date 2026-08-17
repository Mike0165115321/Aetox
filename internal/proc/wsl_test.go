package proc

import (
	"runtime"
	"strings"
	"testing"
)

// The whole WSL backend rests on these two functions agreeing about which file
// a path names. A translation that is merely plausible is worse than none: the
// sandbox guard resolves what comes out of HostPath, so a wrong answer either
// refuses a command that was inside the workspace or admits one that was not.

// The note is the only place the model is told it is already standing in the
// distro, and it was empty for as long as the backend existed. An agent
// mid-session wrapped its command in `wsl -d mikedev --`, got `command not
// found` from a shell that was already there, and concluded the machine had no
// WSL at all. An empty note is not a neutral default here; it is the default
// that costs a turn and then lies to the user.
func TestWSLNoteSaysTheCommandIsAlreadyInsideTheDistro(t *testing.T) {
	note := WSL("mikedev").Note()
	if strings.TrimSpace(note) == "" {
		t.Fatal("the WSL backend tells the model nothing about where its command starts")
	}
	for _, want := range []string{"wsl", "/mnt/"} {
		if !strings.Contains(note, want) {
			t.Errorf("the note does not mention %q, which is the habit it exists to correct:\n%s", want, note)
		}
	}
	// The native shell's note is about its own syntax and must not have picked
	// any of this up.
	if strings.Contains(Native().Note(), "distro") {
		t.Errorf("the native note talks about a distro:\n%s", Native().Note())
	}
}

func TestHostFromGuest(t *testing.T) {
	const distro = "mikedev"
	cases := []struct {
		name  string
		mount string
		in    string
		want  string
		ok    bool
	}{
		{"drive under the default mount", "/mnt", "/mnt/d/GitHub/proj/main.go", `D:\GitHub\proj\main.go`, true},
		{"the drive itself", "/mnt", "/mnt/d", `D:\`, true},
		{"the drive with a trailing slash", "/mnt", "/mnt/d/", `D:\`, true},
		{"drive letter is upper-cased", "/mnt", "/mnt/c/Users", `C:\Users`, true},
		// The Git Bash habit: automount root=/ makes /d a drive, and reading it
		// as a directory named d would check containment against the wrong file.
		{"root=/ makes the first segment a drive", "/", "/d/GitHub/proj", `D:\GitHub\proj`, true},
		{"root=/ still maps the distro's own tree", "/", "/home/mikedev/x", `\\wsl.localhost\mikedev\home\mikedev\x`, true},
		// Not a hole to be special-cased: it lands outside every Windows
		// workspace root, so the ordinary containment rule refuses it.
		{"the distro's own filesystem", "/mnt", "/etc/passwd", `\\wsl.localhost\mikedev\etc\passwd`, true},
		{"the distro root", "/mnt", "/", `\\wsl.localhost\mikedev`, true},
		{"under the mount root but not a drive", "/mnt", "/mnt/wsl/docker", `\\wsl.localhost\mikedev\mnt\wsl\docker`, true},
		// The model mixing spellings is routine; mangling a path that was
		// already the answer would turn a checkable path into a nonsense one.
		{"a Windows path passes through", "/mnt", `D:\GitHub\proj`, `D:\GitHub\proj`, true},
		{"a UNC path passes through", "/mnt", `\\wsl.localhost\mikedev\home`, `\\wsl.localhost\mikedev\home`, true},
		{"a relative path passes through", "/mnt", "src/main.go", "src/main.go", true},
		{"empty passes through", "/mnt", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := hostFromGuest(distro, tc.mount, tc.in)
			if ok != tc.ok || got != tc.want {
				t.Errorf("hostFromGuest(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// An unnamed distro cannot build the \\wsl.localhost\<name> form, and guessing
// one would check containment against a filesystem the command may not run in.
func TestHostFromGuestRefusesGuestPathWithoutADistroName(t *testing.T) {
	if _, ok := hostFromGuest("", "/mnt", "/etc/passwd"); ok {
		t.Error("a guest path translated without a distro name — containment would be checked against the wrong filesystem")
	}
	// Drive paths do not need the name, so they must still translate.
	if got, ok := hostFromGuest("", "/mnt", "/mnt/d/x"); !ok || got != `D:\x` {
		t.Errorf(`hostFromGuest("", "/mnt", "/mnt/d/x") = %q, %v; want D:\x, true`, got, ok)
	}
}

func TestGuestFromHost(t *testing.T) {
	const distro = "mikedev"
	cases := []struct {
		name  string
		mount string
		in    string
		want  string
		ok    bool
	}{
		{"a drive path", "/mnt", `D:\GitHub\proj`, "/mnt/d/GitHub/proj", true},
		{"the drive itself", "/mnt", `D:\`, "/mnt/d", true},
		{"forward slashes are accepted", "/mnt", "D:/GitHub/proj", "/mnt/d/GitHub/proj", true},
		{"root=/ drops the mnt", "/", `D:\GitHub`, "/d/GitHub", true},
		{"this distro's UNC", "/mnt", `\\wsl.localhost\mikedev\home\mikedev`, "/home/mikedev", true},
		{"the older wsl$ spelling", "/mnt", `\\wsl$\mikedev\etc\wsl.conf`, "/etc/wsl.conf", true},
		{"the distro name is matched case-insensitively", "/mnt", `\\wsl.localhost\MikeDev\tmp`, "/tmp", true},
		{"a guest path passes through", "/mnt", "/home/mikedev", "/home/mikedev", true},
		{"a relative path passes through", "/mnt", `src\main.go`, "src/main.go", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := guestFromHost(distro, tc.mount, tc.in)
			if ok != tc.ok || got != tc.want {
				t.Errorf("guestFromHost(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// Another distro's files are reachable from Windows and unreachable from inside
// this one. Answering with this distro's copy of the same path would hand back
// a different file wearing the right name.
func TestGuestFromHostRefusesAnotherDistroAndNetworkShares(t *testing.T) {
	for _, in := range []string{`\\wsl.localhost\other\home`, `\\wsl$\other\home`, `\\fileserver\share\x`} {
		if got, ok := guestFromHost("mikedev", "/mnt", in); ok {
			t.Errorf("guestFromHost(%q) = %q, true; want a refusal", in, got)
		}
	}
}

func TestPathTranslationRoundTrips(t *testing.T) {
	for _, mount := range []string{"/mnt", "/"} {
		for _, host := range []string{`D:\GitHub\proj\main.go`, `C:\Users\me`} {
			guest, ok := guestFromHost("mikedev", mount, host)
			if !ok {
				t.Fatalf("guestFromHost(%q) refused", host)
			}
			back, ok := hostFromGuest("mikedev", mount, guest)
			if !ok || back != host {
				t.Errorf("round trip with root=%q: %q -> %q -> %q", mount, host, guest, back)
			}
		}
	}
}

func TestParseAutomountRoot(t *testing.T) {
	cases := []struct {
		name string
		conf string
		want string
	}{
		{"the common override", "[automount]\nroot = /\n", "/"},
		{"quoted value", "[automount]\nroot = \"/mnt/win\"\n", "/mnt/win"},
		{"comments and blank lines", "# hi\n\n[automount]\n; c\nenabled = true\nroot=/w\n", "/w"},
		// root belongs to [automount]; the same key elsewhere is a different
		// setting and reading it would move every drive.
		{"root in another section is not this root", "[network]\nroot = /nope\n", ""},
		{"no automount section", "[boot]\nsystemd=true\n", ""},
		{"empty file", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseAutomountRoot(strings.NewReader(tc.conf)); got != tc.want {
				t.Errorf("parseAutomountRoot() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseDistroList(t *testing.T) {
	// Docker's two are installations serving another product, not shells anyone
	// works in; offering them in a picker is offering a dead end.
	got := parseDistroList("mikedev\r\ndocker-desktop\r\nDocker-Desktop-Data\r\nUbuntu-22.04\r\n\r\n")
	want := []string{"mikedev", "Ubuntu-22.04"}
	if len(got) != len(want) {
		t.Fatalf("parseDistroList() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parseDistroList()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// An older wsl.exe ignores WSL_UTF8 and answers in UTF-16LE. Read as bytes that
// is one NUL after every character, which without the trim becomes one distro
// per letter — a picker full of single characters rather than an empty one.
func TestParseDistroListSurvivesUTF16Output(t *testing.T) {
	var utf16ish strings.Builder
	for _, r := range "mikedev\r\n" {
		utf16ish.WriteRune(r)
		utf16ish.WriteByte(0)
	}
	got := parseDistroList(utf16ish.String())
	if len(got) != 1 || got[0] != "mikedev" {
		t.Errorf("parseDistroList(utf16) = %v, want [mikedev]", got)
	}
}

// The two backends have to disagree about POSIX, because that is what the
// sandbox guard reads the command line with: a WSL command line is bash, on a
// machine where the native shell is cmd.
func TestBackendPOSIXAndName(t *testing.T) {
	if Native().Name() != ShellName() {
		t.Errorf("Native().Name() = %q, want %q", Native().Name(), ShellName())
	}
	wsl := WSL("mikedev")
	if !wsl.POSIX() {
		t.Error("the WSL backend reports a non-POSIX command line")
	}
	if !strings.Contains(wsl.Name(), "mikedev") {
		t.Errorf("WSL().Name() = %q, which does not say which distro", wsl.Name())
	}
}

// The picker's word and the model's word are two different words. The
// regression this guards is the tempting one: making the label friendlier by
// editing Name, which quietly stops telling the model which syntax to write —
// and nothing fails until a command comes back with `&&` that cmd cannot parse.
func TestLabelNamesTheMachineNotTheShell(t *testing.T) {
	wsl := WSL("mikedev")
	if got := Label(wsl); got != "WSL: mikedev" {
		t.Errorf("Label(WSL) = %q, want %q", got, "WSL: mikedev")
	}
	if !strings.Contains(wsl.Name(), "bash") {
		t.Errorf("WSL().Name() = %q — the model is no longer told whose syntax to write", wsl.Name())
	}

	want := map[string]string{"windows": "Windows", "darwin": "macOS", "linux": "Linux"}[runtime.GOOS]
	if want == "" {
		t.Skipf("no label decided for GOOS %q", runtime.GOOS)
	}
	if got := Label(Native()); got != want {
		t.Errorf("Label(Native()) = %q, want %q", got, want)
	}
	if Label(Native()) == Native().Name() {
		t.Errorf("Label(Native()) = %q is the model's word, not a person's", Label(Native()))
	}
}
