package skill

// Reading a video link as captions.
//
// The parts worth pinning are the ones that decide whether the answer is usable:
// which URLs are one video, and whether a rolling automatic caption track comes
// back as prose or as the same sentence three times.

import (
	"net/url"
	"strings"
	"testing"
)

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestOneVideoIsRecognisedInEveryShapeYouTubeUses(t *testing.T) {
	for _, raw := range []string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtube.com/watch?v=dQw4w9WgXcQ&list=PL123",
		"https://m.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ",
		"https://www.youtube.com/shorts/abc123",
		"https://www.youtube.com/live/abc123",
		"https://www.youtube.com/embed/abc123",
	} {
		if !isVideoPage(mustURL(t, raw)) {
			t.Errorf("%s is one video and was not recognised", raw)
		}
	}
}

// A channel or a playlist is not one video. yt-dlp would walk all of them, and
// "read this link" turning into four hundred fetches is not a thing to do
// because it is possible.
func TestAChannelOrPlaylistIsNotOneVideo(t *testing.T) {
	for _, raw := range []string{
		"https://www.youtube.com/@somechannel",
		"https://www.youtube.com/c/somechannel/videos",
		"https://www.youtube.com/playlist?list=PL123",
		"https://www.youtube.com/results?search_query=cats",
		"https://www.youtube.com/watch",
		"https://vimeo.com/12345",
		"https://example.com/watch?v=abc",
	} {
		if isVideoPage(mustURL(t, raw)) {
			t.Errorf("%s is not one video and was taken for one", raw)
		}
	}
}

// The whole reason this file has a parser. An automatic track scrolls: every cue
// repeats the tail of the one before it, so a naive read of a 40-minute talk
// comes back three times too long and unreadable.
func TestARollingAutoCaptionBecomesProse(t *testing.T) {
	raw := "WEBVTT\n" +
		"Kind: captions\n" +
		"Language: en\n" +
		"\n" +
		"00:00:00.120 --> 00:00:02.500 align:start position:0%\n" +
		"the first thing\n" +
		"\n" +
		"00:00:02.500 --> 00:00:04.720 align:start position:0%\n" +
		"the first thing\n" +
		"<00:00:03.100><c>and the second</c>\n" +
		"\n" +
		"00:00:04.720 --> 00:00:07.000\n" +
		"and the second\n" +
		"the third\n"

	got := transcriptFromVTT(raw)
	want := "the first thing\nand the second\nthe third"
	if got != want {
		t.Errorf("transcript =\n%q\nwant\n%q", got, want)
	}
}

// Timestamps, cue numbers and the header are structure, not speech.
func TestTheTranscriptKeepsOnlyWhatWasSaid(t *testing.T) {
	raw := "WEBVTT\n\n1\n00:00:01.000 --> 00:00:02.000\nhello\n\n2\n00:00:02.000 --> 00:00:03.000\nworld\n"

	if got := transcriptFromVTT(raw); got != "hello\nworld" {
		t.Errorf("transcript = %q", got)
	}
}

// A number that is what somebody said must survive. Dropping every digit line
// would eat "42" out of a talk about the number 42.
func TestASpokenNumberIsNotMistakenForACueNumber(t *testing.T) {
	raw := "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nthe answer is\n\n00:00:02.000 --> 00:00:03.000\n42 of them\n"

	if got := transcriptFromVTT(raw); !strings.Contains(got, "42 of them") {
		t.Errorf("transcript = %q, want the spoken line kept", got)
	}
}

// A video with no captions still has a title, a channel and a description —
// more than the HTML shell ever gave — and the absence has to be said out loud,
// or a model reads an empty transcript as an empty video.
func TestAVideoWithNoCaptionsSaysSo(t *testing.T) {
	page := renderVideoPage(videoMeta{
		Title: "เรื่องที่พูดไว้", Channel: "ช่องหนึ่ง", Duration: 3725,
		UploadDate: "20260824", WebpageURL: "https://youtu.be/abc",
	}, "")

	for _, want := range []string{"เรื่องที่พูดไว้", "ช่องหนึ่ง", "1:02:05", "2026-08-24"} {
		if !strings.Contains(page, want) {
			t.Errorf("page is missing %q:\n%s", want, page)
		}
	}
	if !strings.Contains(page, "no caption track") {
		t.Errorf("the page does not say the captions are missing:\n%s", page)
	}
	// And it must not invite the model to describe a video it has not heard.
	if !strings.Contains(page, "Nothing was transcribed") {
		t.Errorf("the page does not rule out inventing one:\n%s", page)
	}
}

// A description long enough to be the answer is cut, and says it was.
func TestAHugeDescriptionIsCut(t *testing.T) {
	page := renderVideoPage(videoMeta{
		Title: "t", Description: strings.Repeat("ก", videoDescMax*2),
	}, "words")

	if !strings.Contains(page, "…") {
		t.Error("a cut description does not admit it was cut")
	}
	if len(page) > videoDescMax*3 {
		t.Errorf("page is %d bytes — the description was not cut at all", len(page))
	}
}
