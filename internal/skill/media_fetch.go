package skill

// media_fetch downloads one image or sound file from the web into the
// workspace — the download step of the picture recipe, owned by the system.
//
// The recipe (aetox-design SKILL.md) was four moves: web_search finds the page,
// web_fetch lists its file URLs, then a shell Invoke-WebRequest downloads and a
// `read` verifies what landed. The last two are a deterministic procedure that
// was being paid for in model turns — and in the rooms that carry no shell (the
// deck desk, measured ปลายเดือน ส.ค.) the recipe simply died at the download.
// Choosing the picture, judging its fit and its licence stay with the model;
// moving the bytes and refusing a lie about them is this file.
//
// **The lie it exists to catch:** a saved HTML error page named `hero.jpg`,
// which downloads fine, sits quietly, and ruins an export later. Every body is
// judged by its own first bytes — never by the server's content type, never by
// the URL's extension — and a body that is not an image or a sound this app
// knows is refused with its real shape named.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Mikedev115/Aetox/internal/model"
	"github.com/Mikedev115/Aetox/internal/proc"
)

// mediaFetchMaxBody is the biggest file worth pulling this way. Far above
// web_fetch's 2MB because the subject is media, not pages: a music bed is
// 3–8MB, a print-quality photo can pass 10. Video stays out on purpose —
// footage is kinocut's material and arrives by other doors.
const mediaFetchMaxBody = 30 << 20

type mediaFetchSkill struct {
	root         string
	outputSubdir func() string
	files        *FileState
	httpClient   *http.Client
}

func (*mediaFetchSkill) Name() string { return "media_fetch" }

func (*mediaFetchSkill) Description() string {
	return "ดาวน์โหลดรูปหรือไฟล์เสียงจาก URL ลงในโปรเจกต์ พร้อมตรวจว่าไฟล์ที่ได้เป็นชนิดนั้นจริง"
}

func (*mediaFetchSkill) ToolDefinition() model.ToolDefinition {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url":  map[string]any{"type": "string"},
			"path": map[string]any{"type": "string"},
		},
		"required":             []string{"url", "path"},
		"additionalProperties": false,
	}
	payload, _ := json.Marshal(schema)
	return model.ToolDefinition{
		Type: "function",
		Function: model.ToolFunction{
			Name:        "media_fetch",
			Description: "Download an image or sound file from a direct URL to a relative path in the workspace.",
			Parameters:  payload,
		},
	}
}

// Guidance carries the judgment the block entry may not (block_standard_test):
// how to find a real file URL, what is accepted, and the licence duty.
func (*mediaFetchSkill) Guidance(map[string]any) string {
	return "media_fetch saves jpg/png/gif/webp/svg and mp3/wav/ogg/flac/m4a, and refuses everything else by its " +
		"first bytes — including the HTML page a hotlink guard returns in place of a file. Get the file URL by " +
		"running web_fetch on the page that shows it (it lists every image URL with alt text); a page URL here " +
		"is the ordinary mistake. Not for video — footage has its own doors. Prefer sources whose page states " +
		"the licence (Unsplash, Pexels, Wikimedia Commons, Pixabay), and tell the user where each file came " +
		"from and under what licence; a file found on an unknown page is not cleared for their published work."
}

func (s *mediaFetchSkill) Execute(ctx context.Context, input Input) (Output, error) {
	args := stringSlice(input["args"])
	if len(args) < 2 {
		err := errors.New("usage: media_fetch <url> <path>")
		return newToolOutput("media_fetch", "media_fetch", "", time.Now(), false, err), err
	}
	return s.fetch(ctx, args[0], args[1])
}

func (s *mediaFetchSkill) ExecuteTool(ctx context.Context, args map[string]any) (Output, error) {
	rawURL, _ := args["url"].(string)
	requestPath, _ := args["path"].(string)
	return s.fetch(ctx, strings.TrimSpace(rawURL), strings.TrimSpace(requestPath))
}

func (s *mediaFetchSkill) fetch(ctx context.Context, rawURL, requestPath string) (Output, error) {
	start := time.Now()
	command := "media_fetch " + rawURL
	if rawURL == "" || requestPath == "" {
		err := errors.New("usage: media_fetch <url> <path>")
		return newToolOutput("media_fetch", command, "", start, false, err), err
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		// The observed misuse is a file:// URL for a sound already on this
		// machine (31 ส.ค.) — that caller wants a copy, not a download, and
		// the refusal that only says "http(s)" leaves it guessing which door.
		err := fmt.Errorf("media_fetch รับเฉพาะ http(s) URL, ได้ %q — ไฟล์ที่อยู่ในเครื่องแล้วไม่ต้องดาวน์โหลด: คัดลอกด้วย shell (Copy-Item) หรืออ่านแล้วเขียนตามปกติ", rawURL)
		return newToolOutput("media_fetch", command, "", start, false, err), err
	}

	// The same placement rule write and sheet_write follow (write.go): a
	// relative path in an unfocused session lands in the session's output
	// folder, and the receipt names where it really went.
	original := requestPath
	requestPath = placedWrite(s.outputSubdir, requestPath)
	targetPath, err := resolveSandboxPath(s.root, requestPath)
	if err != nil {
		return newToolOutput("media_fetch", command, "", start, false, err), err
	}

	body, err := s.download(ctx, rawURL)
	if err != nil {
		return newToolOutput("media_fetch", command, "", start, false, err), err
	}

	kind := sniffMediaKind(body)
	if kind == "" {
		err := refuseNonMedia(body)
		return newToolOutput("media_fetch", command, "", start, false, err), err
	}

	if err := ensureWriteDir(targetPath); err != nil {
		return newToolOutput("media_fetch", command, "", start, false, err), err
	}
	if err := s.files.guardStale(targetPath, requestPath); err != nil {
		return newToolOutput("media_fetch", command, "", start, false, err), err
	}
	if err := os.WriteFile(targetPath, body, 0o644); err != nil {
		return newToolOutput("media_fetch", command, "", start, false, err), err
	}
	s.files.Note(targetPath)

	report := fmt.Sprintf("ได้ %s: %s (%s", requestPath, kind, humanBytes(len(body)))
	if w, h, ok := imageDims(kind, body); ok {
		report += fmt.Sprintf(", %dx%d px", w, h)
	}
	if secs, ok := audioSeconds(ctx, kind, targetPath); ok {
		report += fmt.Sprintf(", %.1f วินาที", secs)
	}
	report += ")"
	if ext := strings.TrimPrefix(strings.ToLower(pathExt(requestPath)), "."); ext != "" && ext != kind &&
		!(ext == "jpeg" && kind == "jpg") {
		report += fmt.Sprintf("\nนามสกุลในชื่อไฟล์ (%s) ไม่ตรงกับเนื้อไฟล์ (%s) — เปลี่ยนชื่อก่อนใช้", ext, kind)
	}
	if requestPath != original {
		report += onDiskNote(s.root, targetPath)
	}
	report += "\nบอกผู้ใช้ว่าไฟล์นี้มาจากหน้าไหนและ licence อะไร"
	return newToolOutput("media_fetch", command, report, start, false, nil), nil
}

func (s *mediaFetchSkill) download(ctx context.Context, rawURL string) ([]byte, error) {
	client := s.httpClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Aetox/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d จาก %s", resp.StatusCode, rawURL)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, mediaFetchMaxBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > mediaFetchMaxBody {
		return nil, fmt.Errorf("ไฟล์ใหญ่เกิน %d MB — media_fetch มีไว้สำหรับรูปและเสียง ไม่ใช่วิดีโอ", mediaFetchMaxBody>>20)
	}
	if len(body) == 0 {
		return nil, errors.New("ได้ไฟล์ว่างกลับมา")
	}
	return body, nil
}

// sniffMediaKind names a body by its own first bytes, or "" for anything this
// tool does not save. The allowlist IS the security boundary: an archive, an
// executable, a script — everything not an image or a sound — falls out here.
func sniffMediaKind(body []byte) string {
	switch {
	case len(body) > 3 && body[0] == 0xFF && body[1] == 0xD8 && body[2] == 0xFF:
		return "jpg"
	case len(body) > 8 && string(body[:8]) == "\x89PNG\r\n\x1a\n":
		return "png"
	case len(body) > 6 && (string(body[:6]) == "GIF87a" || string(body[:6]) == "GIF89a"):
		return "gif"
	case len(body) > 12 && string(body[:4]) == "RIFF" && string(body[8:12]) == "WEBP":
		return "webp"
	case len(body) > 12 && string(body[:4]) == "RIFF" && string(body[8:12]) == "WAVE":
		return "wav"
	case len(body) > 4 && string(body[:4]) == "OggS":
		return "ogg"
	case len(body) > 4 && string(body[:4]) == "fLaC":
		return "flac"
	case len(body) > 3 && string(body[:3]) == "ID3",
		len(body) > 2 && body[0] == 0xFF && (body[1]&0xE0) == 0xE0:
		return "mp3"
	case len(body) > 12 && string(body[4:8]) == "ftyp" &&
		(string(body[8:11]) == "M4A" || string(body[8:12]) == "mp42" || string(body[8:12]) == "isom"):
		return "m4a"
	}
	head := strings.TrimSpace(strings.ToLower(string(body[:min(len(body), 256)])))
	if strings.HasPrefix(head, "<svg") ||
		(strings.HasPrefix(head, "<?xml") && strings.Contains(head, "<svg")) {
		return "svg"
	}
	return ""
}

// refuseNonMedia says what the body actually was, because the ordinary failure
// is a page, not a file — a hotlink guard, a login wall, a 200 that is really
// an error — and "not an image" without the shape sends the model back to the
// same URL.
func refuseNonMedia(body []byte) error {
	head := strings.TrimSpace(strings.ToLower(string(body[:min(len(body), 256)])))
	if strings.HasPrefix(head, "<!doctype") || strings.HasPrefix(head, "<html") ||
		strings.Contains(head, "<head") {
		return errors.New("URL นี้ตอบกลับมาเป็นหน้าเว็บ ไม่ใช่ตัวไฟล์ — เปิดหน้านั้นด้วย web_fetch เพื่อหา URL ของไฟล์จริง")
	}
	return fmt.Errorf("ไฟล์ที่ได้ไม่ใช่รูปหรือเสียงที่รู้จัก (ขึ้นต้นด้วย %q) — media_fetch เซฟเฉพาะ jpg/png/gif/webp/svg และ mp3/wav/ogg/flac/m4a", firstBytes(body))
}

func firstBytes(body []byte) string {
	n := min(len(body), 8)
	printable := true
	for _, b := range body[:n] {
		if b < 0x20 || b > 0x7E {
			printable = false
			break
		}
	}
	if printable {
		return string(body[:n])
	}
	return fmt.Sprintf("% X", body[:n])
}

func imageDims(kind string, body []byte) (w, h int, ok bool) {
	switch kind {
	case "jpg", "png", "gif":
		cfg, _, err := image.DecodeConfig(strings.NewReader(string(body)))
		if err != nil {
			return 0, 0, false
		}
		return cfg.Width, cfg.Height, true
	}
	return 0, 0, false
}

// audioSeconds asks the bundled ffprobe, best effort: a machine without the
// video toolchain still downloaded the file, and the length is a courtesy in
// the receipt, not a gate.
func audioSeconds(ctx context.Context, kind, path string) (float64, bool) {
	switch kind {
	case "mp3", "wav", "ogg", "flac", "m4a":
	default:
		return 0, false
	}
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, bundledBinary("ffmpeg", "ffprobe"),
		"-v", "error", "-show_entries", "format=duration", "-of", "default=nw=1:nk=1", path)
	proc.HideConsole(cmd)
	out, err := cmd.Output()
	if err != nil {
		return 0, false
	}
	secs, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || secs <= 0 {
		return 0, false
	}
	return secs, true
}

func pathExt(p string) string {
	if i := strings.LastIndexByte(p, '.'); i >= 0 && !strings.ContainsAny(p[i:], `/\`) {
		return p[i:]
	}
	return ""
}
