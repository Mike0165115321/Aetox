package skill

// Reading a picture off disk for whichever writer is about to embed it.
//
// It lives on its own because it stopped being the deck's problem the moment a
// document could hold a picture too. Every rule below was decided once, in
// slides_write (retired at §149), and every one of them is a rule about
// pictures rather than about slides: where a relative path resolves, how big is
// too big, and what happens when the file is not the picture its name claims. A
// second copy of them is a second set of answers, and the one that drifts is
// always the newer one.

import (
	"bytes"
	"fmt"
	"image"
	// Registering the three decoders is what makes image.DecodeConfig able to
	// measure anything at all. They are linked in for their side effect only —
	// and they are most of what an embedded picture costs the binary, which is
	// why the whole package pays for them once here.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"

	"github.com/Mike0165115321/Aetox/internal/ooxml"
)

// A picture is embedded whole, so it is also the whole file size. 20 MB is far
// past any screenshot or chart and well short of the point where a document
// stops being mailable.
const maxPictureBytes = 20 << 20

// loadPicture reads a picture off disk and measures it.
//
// The dimensions come from the file rather than from the model, because the
// model does not know them and would have to guess — and a guessed aspect ratio
// is a stretched screenshot, the kind of wrong that looks deliberate. Failing
// loudly is on purpose too: a document that silently dropped the figure it was
// asked for is worse than one that was never written.
func loadPicture(root string, outputSubdir func() string, requestPath string) (*ooxml.Picture, error) {
	// Same fallback every file-consuming skill uses, so a picture an earlier
	// tool wrote into the session output folder is found by the name the model
	// remembers (see RegistryOptions.OutputSubdir).
	resolved := PlacedPath(root, outputSubdir, requestPath)
	full, err := resolveSandboxPath(root, resolved)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return nil, fmt.Errorf("%s not found", requestPath)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory", requestPath)
	}
	if info.Size() > maxPictureBytes {
		return nil, fmt.Errorf("%s is %d bytes, over the %d limit for an embedded picture", requestPath, info.Size(), maxPictureBytes)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}

	// Decoding the header also validates the format: a .png that is really a
	// PDF would otherwise produce a file the reader declares damaged, naming no
	// cause the user could act on.
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s is not a readable png, jpeg or gif", requestPath)
	}
	ext := format
	if ext == "jpeg" {
		ext = "jpg"
	}
	return &ooxml.Picture{
		Ext:      ext,
		Data:     data,
		WidthPx:  config.Width,
		HeightPx: config.Height,
		AltText:  filepath.Base(requestPath),
	}, nil
}
