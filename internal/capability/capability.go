// Package capability installs the programs that let the agent read a file the
// user attached: text in an image, a PDF, a video, a recording.
//
// These used to arrive with the Windows installer, which downloaded and ran
// them at install time. That is what got aetox-amd64-installer.exe classified
// as Program:Win32/Wacapew.C!ml by Defender's cloud model on 2026-08-20 — an
// unsigned NSIS installer that curls four payloads and ExecWaits one of them is
// shaped exactly like a downloader, whatever the pins and checksums around it
// say. docs/architecture/capability-install-2026-08-21.md has the measurements.
//
// So the installer goes back to putting files on disk, and this package does
// the fetching later, from inside a running Aetox, only when someone asks.
//
// The destination had to move with it. The NSIS installer runs elevated, so
// $INSTDIR is under Program Files and a normally-launched Aetox cannot write
// there. Everything here lands under config.DataRoot() instead, which is the
// same judgment internal/rtk/install.go already made for the rtk binary, and
// the reason none of this needs an administrator.
package capability

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Mikedev115/Aetox/internal/config"
)

// Kind is how a component's download turns into files on disk.
type Kind string

const (
	// KindZip is an archive with a subtree worth taking.
	KindZip Kind = "zip"
	// KindFile is the download itself, saved under its own name. Model
	// weights arrive this way: one big blob, no container.
	KindFile Kind = "file"
)

// Component is one downloadable piece.
//
// URL and SHA256 are pinned together and bumped together. The URL must point
// at something immutable — a dated tag or a commit — because a pin against a
// name whose bytes are rebuilt in place starts failing on somebody else's
// schedule rather than on ours. That reasoning is inherited verbatim from the
// NSIS macros these replace.
type Component struct {
	// ID is the stable name used in the manifest, the UI and the directory.
	//
	// It is also the only name that crosses to the frontend. The sentence a
	// user reads ("read PDFs") lives in the locale files, keyed off this, and
	// deliberately not here: a string in Go would be written once in one
	// language and shown to everyone, which is what the first draft of this
	// file did.
	ID string

	// Capability groups components that buy the user one thing. Speech needs
	// two of them (the engine and a model) and neither is any use alone, so the
	// screen must say "turn speech into text" once, not once per download.
	//
	// A key, not a sentence: the words live in the locale files.
	Capability string

	URL    string
	SHA256 string
	Kind   Kind

	// SubPath is the directory inside the archive to take, and Strip is how
	// many leading segments of each entry's path to drop before writing. Both
	// carry the archive's own layout, which is why they move with the pin.
	// KindFile ignores them.
	SubPath string
	Strip   int

	// Dest is where it lands, relative to config.DataRoot().
	Dest string

	// Probe is the one file whose presence proves the install worked, relative
	// to Dest. Checked on every status read, not just after installing: a
	// half-deleted tree should reinstall rather than be trusted.
	Probe string

	// Marker is a zero-byte file named after the pinned version, alongside
	// Probe. Reinstalling the same version does nothing; bumping the pin
	// changes this name and so forces a full replace. Empty when the pinned
	// identity is already in the file's own name, as it is for model weights.
	Marker string

	// ApproxBytes is for the sentence on the screen, so someone can decide
	// before they press rather than after. Never used as a progress
	// denominator — that comes from Content-Length, which is the real number.
	ApproxBytes int64
}

// Manifest is every component Aetox knows how to install, in the order a
// first-run screen should read them out.
//
// Windows only, which is not a limitation this package invented: every URL
// below is a win64 build, and these are the same four the Windows installer
// used to fetch. The desktop app is Windows-only today (PLATFORM-SUPPORT.md).
// On any other OS the answer is an empty manifest, so every caller degrades to
// "nothing to offer" instead of offering a download that cannot run.
//
// Tesseract is the one entry that does not point at its upstream. Its only
// official Windows channel is the UB-Mannheim NSIS installer, which elevates,
// and the project publishes no plain archive at all
// (https://tesseract-ocr.github.io/tessdoc/Downloads.html) — so
// .github/workflows/tools.yml takes those same pinned bytes, extracts them
// once somewhere anyone can read the script, and republishes the result. The
// provenance is unchanged; only the container is.
func Manifest() []Component {
	if runtime.GOOS != "windows" {
		return nil
	}
	return []Component{
		{
			ID:         "tesseract",
			Capability: "image",
			// Repackaged by .github/workflows/tools.yml from the UB-Mannheim
			// installer pinned in project.nsi, with Thai added — the official
			// build carries English and osd only, and Thai is the language half
			// this app exists for.
			//
			// 62MB is the honest size and the largest thing here. Most of it is
			// libtesseract-5.dll at 97MB unpacked plus 29MB of ICU data, neither
			// of which is optional. The official installer looks smaller (48MB)
			// because NSIS uses solid LZMA where a zip uses deflate; it is the
			// same program.
			URL:    "https://github.com/Mikedev115/Aetox/releases/download/tools-tesseract-5.4.0.20240606/tesseract-win64.zip",
			SHA256: "550a87a862558fafc783462d6c15cab8348f6e1d9057ec1b2e131dc1b16496fb",
			Kind:   KindZip,
			// No SubPath prefix to strip: this archive is ours and has the layout
			// image_ocr wants at its root already.
			SubPath:     ".",
			Strip:       0,
			Dest:        filepath.Join("tools", "tesseract"),
			Probe:       "tesseract.exe",
			Marker:      "tesseract-5.4.0.20240606.ok",
			ApproxBytes: 62 << 20,
		},
		{
			ID:          "poppler",
			Capability:  "pdf",
			URL:         "https://github.com/oschwartz10612/poppler-windows/releases/download/v26.02.0-0/Release-26.02.0-0.zip",
			SHA256:      "993e4a94376ed712fafc7058d724ea0b943d118bbd2305cd9ed55174eb85cda5",
			Kind:        KindZip,
			SubPath:     "poppler-26.02.0/Library",
			Strip:       2,
			Dest:        filepath.Join("tools", "poppler"),
			Probe:       filepath.Join("bin", "pdftotext.exe"),
			Marker:      "poppler-26.02.0.ok",
			ApproxBytes: 20 << 20,
		},
		{
			ID:         "ffmpeg",
			Capability: "media",
			// LGPL, not GPL: the safer build to put on someone else's machine,
			// and nothing here needs the GPL-only encoders. Shared, not static.
			//
			// Mirrored by .github/workflows/tools.yml rather than taken from BtbN
			// directly, and that is not caution — it is a repair. project.nsi
			// pinned one of BtbN's dated autobuild tags because the tag's bytes
			// never change, which is true; what it misses is that BtbN deletes
			// those tags after about five days. The pin written on 27 July
			// answered 404 when the live test finally fetched it on 21 August, so
			// every fresh v1.4.0 install in between had quietly gone without
			// ffmpeg. A pin is only as durable as the thing it points at.
			URL:    "https://github.com/Mikedev115/Aetox/releases/download/tools-ffmpeg-n9.0.1/ffmpeg-win64-lgpl.zip",
			SHA256: "b91abbbde7ab1b45e7d9b8d3baea9d98362b31303f403acf15642b95f2c11bfd",
			Kind:   KindZip,
			// Our archive is bin/ already, so there is no wrapper to strip.
			SubPath:     ".",
			Strip:       0,
			Dest:        filepath.Join("tools", "ffmpeg", "bin"),
			Probe:       "ffmpeg.exe",
			Marker:      "ffmpeg-n9.0.1.ok",
			ApproxBytes: 63 << 20,
		},
		{
			ID:         "whisper",
			Capability: "speech",
			// The engine binary itself, which nothing has ever installed. The
			// NSIS installer shipped a 31MB model for it and stopped there, so
			// audio_transcribe has been dead on a stock install: findBinary looks
			// on PATH, whisper-cli is not on PATH, and the error told the user to
			// go and run scoop or build from source (found 2026-08-21).
			//
			// The whole Release/ folder is taken rather than whisper-cli.exe
			// alone: it loads whisper.dll and a fan of ggml-cpu-*.dll picked at
			// runtime for the host CPU, and guessing which of those this machine
			// will reach for is how a working extract becomes a missing DLL on
			// someone else's laptop. 8MB compressed for the certainty.
			URL:         "https://github.com/ggml-org/whisper.cpp/releases/download/b4938/whisper-bin-x64.zip",
			SHA256:      "c2a4b60edb11f7e11a9191ffb50929535527d4d91c9903dbe3e554583bbbc63d",
			Kind:        KindZip,
			SubPath:     "Release",
			Strip:       1,
			Dest:        filepath.Join("tools", "whisper"),
			Probe:       "whisper-cli.exe",
			Marker:      "whisper-b4938.ok",
			ApproxBytes: 8 << 20,
		},
		{
			ID:         "speech-model",
			Capability: "speech",
			// tiny, not base: 31MB against 141MB. The pin is a commit rather
			// than a branch because a branch's bytes move.
			URL:    "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-tiny-q5_1.bin",
			SHA256: "818710568da3ca15689e31a743197b520007872ff9576237bda97bd1b469c3d7",
			Kind:   KindFile,
			// internal/stt already resolves models from <DataRoot>/models, so
			// this one needed no lookup change at all — only a different
			// program putting the file there.
			Dest:        "models",
			Probe:       "ggml-tiny-q5_1.bin",
			ApproxBytes: 31 << 20,
		},
	}
}

// Status is one row on the screen: a capability, not a download. Speech is two
// components and one row, because "install the engine but not the model" is not
// a choice anyone can make well — it is two ways to end up with nothing that
// works.
type Status struct {
	Capability  string `json:"capability"`
	Installed   bool   `json:"installed"`
	ApproxBytes int64  `json:"approx_bytes"`
}

// Statuses reports one row per capability, in manifest order, with the size of
// only what is still missing — quoting the full size of something half present
// would overcharge for a download that will not happen.
func Statuses() []Status {
	var out []Status
	index := map[string]int{}
	for _, c := range Manifest() {
		i, seen := index[c.Capability]
		if !seen {
			index[c.Capability] = len(out)
			out = append(out, Status{Capability: c.Capability, Installed: true})
			i = len(out) - 1
		}
		if c.Installed() {
			continue
		}
		// One missing part is enough to make the whole capability missing.
		out[i].Installed = false
		out[i].ApproxBytes += c.ApproxBytes
	}
	return out
}

// Missing is every component not on this machine, whatever it is for.
func Missing() []Component { return MissingFor(nil) }

// MissingFor is every missing component belonging to one of the named
// capabilities. A nil list means all of them; an unknown name matches nothing,
// which is the safe way to read a stale request from a window that was open
// across an update.
func MissingFor(capabilities []string) []Component {
	want := map[string]bool{}
	for _, k := range capabilities {
		want[k] = true
	}
	var out []Component
	for _, c := range Manifest() {
		if capabilities != nil && !want[c.Capability] {
			continue
		}
		if !c.Installed() {
			out = append(out, c)
		}
	}
	return out
}

// Root is where this component's tree lives.
func (c Component) Root() (string, error) {
	root, err := config.DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, c.Dest), nil
}

// Installed reports whether the component can be used right now, from either
// address: the one this package writes, or the one the old NSIS installer
// wrote next to aetox.exe. Someone upgrading from v1.4.0 or earlier already
// has these files and must not be asked to download them again.
func (c Component) Installed() bool {
	if root, err := c.Root(); err == nil && c.completeIn(root) {
		return true
	}
	return c.completeIn(c.legacyRoot())
}

// legacyRoot is where the Windows installer used to unpack this component:
// alongside the executable, under the same last path segment. "" when the
// executable's own location cannot be determined.
func (c Component) legacyRoot() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), filepath.Base(c.Dest))
}

// completeIn asks the two questions separately on purpose. Probe alone would
// trust a tree left behind by an older pin; Marker alone would trust a marker
// that outlived the files it vouched for.
func (c Component) completeIn(root string) bool {
	if root == "" {
		return false
	}
	if !isFile(filepath.Join(root, c.Probe)) {
		return false
	}
	if c.Marker == "" {
		return true
	}
	return isFile(filepath.Join(root, c.Marker))
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// Progress is one update from an install in flight. Done and Total are bytes
// for the component named by ID; Total is 0 when the server sent no
// Content-Length, which a caller should render as "working" rather than as 0%.
type Progress struct {
	ID    string
	Index int
	Of    int
	Done  int64
	Total int64
}

// Percent is Done over Total, or -1 when the server sent no Content-Length.
// Two callers wanted the same three lines and one of them rounded differently,
// which is how a bar and its label end up disagreeing on the same download.
func (p Progress) Percent() int {
	if p.Total <= 0 {
		return -1
	}
	pct := int(p.Done * 100 / p.Total)
	if pct > 100 {
		pct = 100
	}
	return pct
}

// Install fetches and unpacks each component in turn, reporting progress.
//
// One component's failure stops the run and is returned, rather than being
// swallowed so the rest can continue: the caller asked for a set, and a screen
// that says "installed" while one of four silently did not is the kind of lie
// that surfaces days later as a tool that does not work. Anything already
// finished stays finished, so pressing the button again resumes rather than
// starts over.
func Install(ctx context.Context, comps []Component, onProgress func(Progress)) error {
	for i, c := range comps {
		if err := ctx.Err(); err != nil {
			return err
		}
		report := func(done, total int64) {
			if onProgress != nil {
				onProgress(Progress{ID: c.ID, Index: i, Of: len(comps), Done: done, Total: total})
			}
		}
		if err := c.install(ctx, report); err != nil {
			return fmt.Errorf("%s: %w", c.ID, err)
		}
	}
	return nil
}

func (c Component) install(ctx context.Context, report func(done, total int64)) error {
	root, err := c.Root()
	if err != nil {
		return err
	}
	tmp, err := c.download(ctx, report)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)

	// Replace rather than merge. A tree from an older pin with files this pin
	// no longer ships would otherwise keep them, and the mixture is a build
	// nobody tested. The marker is written last, so an interrupted unpack
	// leaves an incomplete tree that Installed() correctly rejects.
	if err := os.RemoveAll(root); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}

	switch c.Kind {
	case KindZip:
		if err := c.unzip(tmp, root); err != nil {
			return err
		}
	case KindFile:
		if err := moveFile(tmp, filepath.Join(root, c.Probe)); err != nil {
			return err
		}
	default:
		return fmt.Errorf("ไม่รู้จักวิธีติดตั้งแบบ %q", c.Kind)
	}

	if !isFile(filepath.Join(root, c.Probe)) {
		return fmt.Errorf("แตกไฟล์แล้วแต่ไม่พบ %s", c.Probe)
	}
	if c.Marker != "" {
		f, err := os.Create(filepath.Join(root, c.Marker))
		if err != nil {
			return err
		}
		return f.Close()
	}
	return nil
}

// download streams to a temp file, hashing as it goes, and returns the path
// only once the bytes match the pin. Streaming rather than buffering because
// these run to tens of megabytes and the largest thing in the manifest should
// not have to fit in memory.
func (c Component) download(ctx context.Context, report func(done, total int64)) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ดาวน์โหลดไม่สำเร็จ: %s", resp.Status)
	}

	tmp, err := os.CreateTemp("", "aetox-capability-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	sum := sha256.New()
	counter := &progressWriter{total: resp.ContentLength, report: report}
	_, copyErr := io.Copy(io.MultiWriter(tmp, sum, counter), resp.Body)
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(tmpPath)
		return "", errors.Join(copyErr, closeErr)
	}

	got := hex.EncodeToString(sum.Sum(nil))
	if !strings.EqualFold(got, c.SHA256) {
		os.Remove(tmpPath)
		// Named plainly, because the honest reading of a mismatch is that the
		// bytes are not the ones that were reviewed — not that the download
		// "glitched" and should be retried until it passes.
		return "", fmt.Errorf("ไฟล์ที่โหลดมาไม่ตรงกับลายนิ้วมือที่ปักหมุดไว้ (ได้ %s)", got[:12])
	}
	return tmpPath, nil
}

type progressWriter struct {
	done   int64
	total  int64
	report func(done, total int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	if p.report != nil {
		p.report(p.done, p.total)
	}
	return len(b), nil
}

// unzip writes the SubPath subtree of archive into root, dropping Strip
// leading segments from every entry.
func (c Component) unzip(archive, root string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()

	// "." means the archive is already the tree we want, which is the case
	// for the one we build ourselves. Written as a prefix of "" rather than
	// "./" because path.Clean strips a leading "./" from every entry name,
	// so the obvious spelling matches nothing and unpacks an empty folder.
	prefix := ""
	if clean := path.Clean(c.SubPath); clean != "." && clean != "" {
		prefix = clean + "/"
	}
	wrote := false
	for _, f := range zr.File {
		name := path.Clean(f.Name)
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		rel := stripSegments(name, c.Strip)
		if rel == "" {
			continue
		}
		dest, err := safeJoin(root, rel)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := writeZipEntry(f, dest); err != nil {
			return err
		}
		wrote = true
	}
	if !wrote {
		// Reached when a pin is bumped and SubPath was not, which otherwise
		// surfaces one step later as a missing Probe and reads like a failed
		// download rather than a stale constant.
		return fmt.Errorf("ไม่พบ %q ในไฟล์บีบอัด", c.SubPath)
	}
	return nil
}

func writeZipEntry(f *zip.File, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, rc)
	return errors.Join(copyErr, out.Close())
}

func stripSegments(name string, n int) string {
	parts := strings.Split(name, "/")
	if len(parts) <= n {
		return ""
	}
	return strings.Join(parts[n:], "/")
}

// safeJoin refuses an archive entry that would land outside root. Zip files
// are third-party input even when the bytes matched a pin: the pin says these
// are the bytes we reviewed, not that a reviewed archive can never contain
// "../".
func safeJoin(root, rel string) (string, error) {
	dest := filepath.Join(root, filepath.FromSlash(rel))
	cleanRoot := filepath.Clean(root) + string(os.PathSeparator)
	if !strings.HasPrefix(filepath.Clean(dest)+string(os.PathSeparator), cleanRoot) {
		return "", fmt.Errorf("ไฟล์ในอาร์ไคฟ์ชี้ออกนอกโฟลเดอร์ปลายทาง: %q", rel)
	}
	return dest, nil
}

// moveFile prefers a rename and falls back to a copy, because the temp
// directory and DataRoot are routinely on different volumes on Windows and
// os.Rename across volumes fails.
func moveFile(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dest); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	return errors.Join(copyErr, out.Close())
}
