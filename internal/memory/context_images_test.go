package memory

// The pictures nobody could see the cost of.
//
// Measured on 28 ส.ค. (DECISIONS §204): one turn took 22 screenshots of a
// 7-slide deck and re-sent them 620 times between them. Neither half of that
// was a picture being wrong. The budget counted a message carrying a 130 KB
// screenshot as its 45-character caption, so no conversation ever grew too big
// because of one and no screenshot was ever dropped for being old.

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/model"
)

// shot is a picture of a given size, the way a browser capture arrives.
func shot(t *testing.T, w, h int) model.Image {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Not a flat colour: PNG compresses one to almost nothing, and a test whose
	// picture is 200 bytes cannot tell "priced by pixels" from "priced by
	// bytes", which is the distinction this file exists to hold.
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding the test picture: %v", err)
	}
	return model.Image{MediaType: "image/png", Data: buf.Bytes()}
}

func toolShot(t *testing.T, w, h int) model.Message {
	t.Helper()
	return model.Message{
		Role:           model.RoleUser,
		Content:        "[the image returned by browser follows]",
		Images:         []model.Image{shot(t, w, h)},
		ImagesFromTool: true,
	}
}

// A picture has to weigh something, and it has to weigh roughly what it costs.
func TestBudgetCountsPictures(t *testing.T) {
	c := NewContext("system", 0, 1_000_000)
	before := totalChars(c.Messages())

	c.AddMessage(toolShot(t, 1280, 720))
	after := totalChars(c.Messages())

	added := after - before
	// A caption is 38 characters. Anything in that neighbourhood means the
	// picture is still invisible, which is the whole bug.
	if added < 1000 {
		t.Fatalf("a 1280x720 screenshot added %d chars to the budget, want it counted as a picture and not as its caption", added)
	}
	// And priced by area, not by bytes: a PNG this size is over a megabyte of
	// data, which as characters would evict the whole conversation at once.
	if added > 20000 {
		t.Errorf("a 1280x720 screenshot added %d chars, want a token-shaped estimate rather than its byte count", added)
	}

	// Twice the pixels, near enough twice the price. This is the property that
	// makes the number usable rather than merely non-zero.
	c2 := NewContext("system", 0, 1_000_000)
	c2.AddMessage(toolShot(t, 1280, 1440))
	double := totalChars(c2.Messages()) - before
	if double < added*3/2 {
		t.Errorf("a picture with twice the area cost %d against %d, want it to scale with area", double, added)
	}
}

// Bytes that are not a picture must not be free either, or an unreadable
// header becomes a way to smuggle a megabyte past the budget.
func TestUnreadablePictureIsNotFree(t *testing.T) {
	junk := model.Image{MediaType: "image/png", Data: []byte("not actually a png")}
	if got := junk.CharCost(); got <= 0 {
		t.Errorf("CharCost() = %d for bytes with no readable header, want a real estimate", got)
	}
	if got := (model.Image{}).CharCost(); got != 0 {
		t.Errorf("CharCost() = %d for no picture at all, want 0", got)
	}
}

// Old screenshots stop being pictures. The newest two stay, because two is what
// a before-and-after comparison needs.
func TestOldToolPicturesAreForgotten(t *testing.T) {
	c := NewContext("system", 0, 1_000_000)
	for i := 0; i < 5; i++ {
		c.AddMessage(toolShot(t, 800, 600))
	}

	msgs := c.Messages()
	withPictures, notes := 0, 0
	for _, m := range msgs {
		if len(m.Images) > 0 {
			withPictures++
		}
		if strings.Contains(m.Content, "ถูกถอดออกจากบทสนทนา") {
			notes++
		}
	}
	if withPictures != imagesKept {
		t.Errorf("%d messages still carry pictures, want %d", withPictures, imagesKept)
	}
	if notes != 5-imagesKept {
		t.Errorf("%d messages say their picture is gone, want %d — a caption promising a picture that is not there is a lie", notes, 5-imagesKept)
	}

	// The newest two are the ones kept. Forgetting the picture just taken to
	// keep the one from nineteen actions ago would be worse than not forgetting
	// at all.
	for i, m := range msgs[len(msgs)-imagesKept:] {
		if len(m.Images) == 0 {
			t.Errorf("message %d from the end lost its picture, want the newest %d kept", imagesKept-i, imagesKept)
		}
	}

	// And it says so on the meter, like the other two compaction layers. A
	// third mechanism reclaiming space in silence is what those counters exist
	// to prevent.
	items, chars, _ := c.MaintenanceStats()
	if items != 5-imagesKept || chars <= 0 {
		t.Errorf("MaintenanceStats() = %d items, %d chars, want %d items and a positive saving", items, chars, 5-imagesKept)
	}
}

// A picture the user attached is the subject of the conversation, not a step in
// it. Dropping it to save room breaks the job the room was asked to do.
func TestTheUsersOwnPicturesAreNeverForgotten(t *testing.T) {
	c := NewContext("system", 0, 1_000_000)
	c.AddMessage(model.Message{
		Role:    model.RoleUser,
		Content: "อันนี้รูปที่ผมถ่ายมา ดูให้หน่อย",
		Images:  []model.Image{shot(t, 800, 600)},
	})
	for i := 0; i < 5; i++ {
		c.AddMessage(toolShot(t, 800, 600))
	}

	mine := c.Messages()[1]
	if len(mine.Images) != 1 {
		t.Fatal("the picture the user attached was dropped to make room for screenshots")
	}
	if strings.Contains(mine.Content, "ถูกถอดออกจากบทสนทนา") {
		t.Error("the user's own message was annotated as if its picture had been swept")
	}
}

// Sweeping twice must not annotate twice, or a long turn accumulates the same
// sentence over and over in place of the picture.
func TestForgettingAPictureIsIdempotent(t *testing.T) {
	c := NewContext("system", 0, 1_000_000)
	for i := 0; i < 4; i++ {
		c.AddMessage(toolShot(t, 800, 600))
	}
	first := c.Messages()[1].Content

	c.Add(model.RoleAssistant, "still working") // another AddMessage, another sweep
	got := c.Messages()[1].Content
	if got != first {
		t.Errorf("the note was appended again on the next sweep:\n%q\n%q", first, got)
	}
	if strings.Count(got, "ถูกถอดออกจากบทสนทนา") > 1 {
		t.Errorf("the note appears more than once in one message: %q", got)
	}
}
