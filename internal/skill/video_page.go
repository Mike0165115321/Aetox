package skill

// A video link, read as what it actually is: a title, a channel, and words.
//
// `web_fetch` on a YouTube URL used to come back with the HTML shell — a page
// whose whole content arrives later by script, so what the model got was
// navigation chrome and nothing it asked for. The question was right and the
// answer was garbage.
//
// **Captions, never transcription.** Every tool that does this well reads the
// caption track and stops there, NotebookLM included — its own documentation
// says video analysis "depends on transcript availability and quality", which is
// the polite way of saying it does not watch anything. Human-written captions
// beat speech recognition, arrive instantly, and cost nothing. Owner, 24 ส.ค.:
// *"ผมว่าเอาแค่ถอดคำบรรยายก็น่าจะพอ เพราะหลายที่ก็ทำแบบนั้น"*. A video with no
// captions is reported as having none — it is not quietly downloaded and run
// through whisper, which is minutes of somebody's machine for a result nobody
// asked for.
//
// **Not a new tool.** This rides inside `web_fetch` because it is the same
// question — "what is the content at this URL" — answered correctly for a kind
// of URL where the old answer was wrong. A `video_read` beside it would be a
// second door onto one question, and the tool block has no room for one anyway.
//
// **Why this works here and not in a datacenter.** YouTube's bot check leans
// hardest on datacentre IP ranges; the standing advice is to run from a
// residential connection, or with cookies from a real browser. Aetox is on the
// user's own machine, which is the situation that advice describes. This is a
// desktop agent doing something a hosted one cannot do reliably.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/config"
	"github.com/Mikedev115/Aetox/internal/proc"
)

// videoHosts are the hosts whose pages are a video rather than a document.
//
// YouTube only, deliberately. It is the one whose caption track is public,
// reliable and worth the dependency; the others either require a login or hand
// back nothing a caption reader can use, and a list that quietly fails on four
// of its five entries teaches the model to stop trusting the tool.
var videoHosts = map[string]bool{
	"youtube.com":     true,
	"www.youtube.com": true,
	"m.youtube.com":   true,
	"youtu.be":        true,
	"www.youtu.be":    true,
}

// isVideoPage reports whether this URL names one video.
//
// A channel, a playlist or a search is NOT one: yt-dlp would happily walk all of
// them, and "read this link" turning into four hundred fetches is not a thing to
// do because it is possible.
func isVideoPage(u *url.URL) bool {
	if u == nil || !videoHosts[strings.ToLower(u.Host)] {
		return false
	}
	path := strings.TrimSuffix(u.Path, "/")
	switch {
	case strings.EqualFold(u.Host, "youtu.be") || strings.EqualFold(u.Host, "www.youtu.be"):
		return len(strings.Trim(path, "/")) > 0
	case path == "/watch":
		return u.Query().Get("v") != ""
	case strings.HasPrefix(path, "/shorts/"), strings.HasPrefix(path, "/live/"), strings.HasPrefix(path, "/embed/"):
		return true
	}
	return false
}

// videoMeta is the part of yt-dlp's JSON worth reading. Everything else in that
// object is formats, thumbnails and fragment URLs — megabytes of it.
type videoMeta struct {
	Title       string  `json:"title"`
	Uploader    string  `json:"uploader"`
	Channel     string  `json:"channel"`
	Duration    float64 `json:"duration"`
	UploadDate  string  `json:"upload_date"`
	Description string  `json:"description"`
	WebpageURL  string  `json:"webpage_url"`
	ID          string  `json:"id"`
}

const (
	// videoFetchTimeout bounds the whole thing. yt-dlp on a page YouTube is
	// challenging can sit for a long time, and a research turn that stops dead
	// on one link is worse than a link that reports it could not be read.
	videoFetchTimeout = 90 * time.Second
	// videoDescMax keeps a description from being the answer. Some channels put
	// their entire back catalogue under every video.
	videoDescMax = 1500
)

// fetchVideoPage returns the video's metadata and its caption track as text.
//
// Two attempts at the captions and no more: the languages worth having, then
// whatever the video actually has. A third pass would be guessing.
func fetchVideoPage(ctx context.Context, rawURL string) (string, error) {
	bin, err := findVideoTool()
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "aetox-video-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	ctx, cancel := context.WithTimeout(ctx, videoFetchTimeout)
	defer cancel()

	meta, err := runVideoTool(ctx, bin, dir, rawURL, "th.*,en.*")
	if err != nil {
		return "", err
	}
	transcript := readCaptions(dir)
	if transcript == "" {
		// The video may simply be in another language. Ask for what it has
		// rather than deciding it has nothing.
		if _, second := runVideoTool(ctx, bin, dir, rawURL, "all,-live_chat"); second == nil {
			transcript = readCaptions(dir)
		}
	}
	return renderVideoPage(meta, transcript), nil
}

// runVideoTool asks yt-dlp for the metadata and the caption files in one go.
//
// One process, not two: `-J` prints the JSON on stdout while the same run writes
// the subtitle files, and a second invocation would be a second bot check on a
// service that counts them.
func runVideoTool(ctx context.Context, bin, dir, rawURL, langs string) (videoMeta, error) {
	// Built as a slice rather than spread over the call, so HideConsole below
	// sits next to the command it hides — the rule proc's coverage test checks,
	// and it reads better for a list this long.
	args := []string{
		"--skip-download",
		"--no-playlist", // a link with &list= in it is still one video
		"--no-warnings",
		"--write-subs",
		"--write-auto-subs",
		"--sub-langs", langs,
		// VTT is what YouTube serves, and taking it as-is keeps ffmpeg out of
		// this path: --convert-subs would make a caption read depend on a
		// 63MB capability that has nothing to do with reading text.
		"--sub-format", "vtt",
		"-o", filepath.Join(dir, "%(id)s.%(ext)s"),
		"-J",
		rawURL,
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	proc.HideConsole(cmd)
	out, err := cmd.Output()
	if err != nil {
		return videoMeta{}, videoToolError(err)
	}
	var meta videoMeta
	if jsonErr := json.Unmarshal(out, &meta); jsonErr != nil {
		return videoMeta{}, fmt.Errorf("could not read the video's details: %w", jsonErr)
	}
	return meta, nil
}

// videoToolError turns a failed run into something a reader can act on.
//
// yt-dlp writes its real reason to stderr, and the two that actually happen are
// worth naming: YouTube's bot challenge, and a video that is private or gone.
// Everything else is passed through rather than flattened, because a message
// nobody wrote is a message nobody can fix.
func videoToolError(err error) error {
	var stderr string
	if exitErr, ok := err.(*exec.ExitError); ok {
		stderr = strings.TrimSpace(string(exitErr.Stderr))
	}
	switch {
	case strings.Contains(stderr, "confirm you"), strings.Contains(stderr, "not a bot"):
		return fmt.Errorf("YouTube asked this machine to prove it is not a bot, so the captions could not be read. " +
			"Signing in to YouTube in a browser on this machine usually clears it")
	case strings.Contains(stderr, "Private video"), strings.Contains(stderr, "unavailable"):
		return fmt.Errorf("that video is private or no longer available")
	case stderr != "":
		return fmt.Errorf("could not read that video: %s", firstLine(stderr))
	}
	return fmt.Errorf("could not read that video: %w", err)
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// readCaptions turns whatever .vtt files landed in dir into one transcript,
// preferring a hand-written track over an automatic one.
//
// yt-dlp names an automatic track `<id>.<lang>.vtt` exactly like a real one, so
// they cannot be told apart by name. What CAN be told apart is quality, and the
// proxy for it is length: an auto track repeats every line as the caption rolls,
// so it is the longer file for the same speech. Preferring the shortest is
// therefore preferring the human one when both exist — and harmless when only
// one does.
func readCaptions(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	best := ""
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".vtt") {
			continue
		}
		raw, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			continue
		}
		text := transcriptFromVTT(string(raw))
		if text == "" {
			continue
		}
		if best == "" || len(text) < len(best) {
			best = text
		}
	}
	return best
}

// vttTag matches the inline timing and styling spans YouTube stamps into an
// automatic track — <00:00:04.720><c>word</c> — which are noise in a transcript.
var vttTag = regexp.MustCompile(`<[^>]*>`)

// transcriptFromVTT reduces a WebVTT file to the words that were said.
//
// The deduplication is the part that matters. An automatic track scrolls: each
// cue repeats the tail of the one before it so the caption reads as a moving
// window, which means a naive read of a 40-minute talk comes back three times
// too long and unreadable. Dropping a line that repeats the one before it is
// what turns it back into prose.
func transcriptFromVTT(raw string) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines)/2)
	last := ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case line == "", strings.HasPrefix(line, "WEBVTT"),
			strings.HasPrefix(line, "NOTE"), strings.HasPrefix(line, "STYLE"),
			strings.HasPrefix(line, "Kind:"), strings.HasPrefix(line, "Language:"),
			strings.Contains(line, "-->"):
			continue
		}
		// A cue number on its own line is VTT structure, not speech.
		if isAllDigits(line) {
			continue
		}
		line = strings.TrimSpace(vttTag.ReplaceAllString(line, ""))
		if line == "" || line == last {
			continue
		}
		out = append(out, line)
		last = line
	}
	return strings.Join(out, "\n")
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// renderVideoPage is what the model reads. Metadata first and always, because a
// video with no captions still has a title, a channel and a description — which
// is more than the HTML shell ever gave, and enough to decide whether to go
// looking elsewhere.
func renderVideoPage(meta videoMeta, transcript string) string {
	var b strings.Builder
	title := strings.TrimSpace(meta.Title)
	if title == "" {
		title = "(untitled video)"
	}
	b.WriteString("# " + title + "\n")
	if channel := firstNonEmpty(meta.Channel, meta.Uploader); channel != "" {
		b.WriteString("Channel: " + channel + "\n")
	}
	if meta.Duration > 0 {
		b.WriteString("Length: " + humanDuration(meta.Duration) + "\n")
	}
	if len(meta.UploadDate) == 8 {
		b.WriteString("Published: " + meta.UploadDate[:4] + "-" + meta.UploadDate[4:6] + "-" + meta.UploadDate[6:] + "\n")
	}
	if url := strings.TrimSpace(meta.WebpageURL); url != "" {
		b.WriteString("URL: " + url + "\n")
	}
	if desc := strings.TrimSpace(meta.Description); desc != "" {
		if len(desc) > videoDescMax {
			desc = desc[:videoDescMax] + "…"
		}
		b.WriteString("\n## Description\n" + desc + "\n")
	}
	if transcript == "" {
		// Said plainly, because the next move depends on it: a model told
		// "no transcript" looks elsewhere, and a model told nothing assumes the
		// video was empty.
		b.WriteString("\n## Transcript\nThis video has no caption track, so there is nothing to read. " +
			"Nothing was transcribed from the audio, say so rather than describing a video you have not heard.\n")
		return b.String()
	}
	b.WriteString("\n## Transcript\n" + transcript + "\n")
	return b.String()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func humanDuration(seconds float64) string {
	total := int(seconds + 0.5)
	h, m, s := total/3600, (total%3600)/60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// findVideoTool looks for yt-dlp on PATH first, then in the copy Aetox manages.
//
// PATH first for the same reason whisper does it: somebody who already has the
// tool — and keeps it up to date, which this one needs more than most — should
// not have a second copy installed behind their back.
func findVideoTool() (string, error) {
	for _, name := range []string{"yt-dlp", "yt-dlp_x86", "youtube-dl"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	if root, err := config.DataRoot(); err == nil {
		name := "yt-dlp"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		candidate := filepath.Join(root, "tools", "yt-dlp", name)
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("reading a video link needs yt-dlp, which is not installed on this machine, " +
		"the user can add it from ตั้งค่า ▸ ความสามารถ, or install it themselves")
}
