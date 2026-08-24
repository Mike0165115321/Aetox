package skill

// The head of a page comes back even when nothing in it matched (§180).
//
// A page assembled by web_fetch opens with its title, its URL, its image URLs
// and its links, and only then its prose. BM25 scores prose. So `find` about
// anything textual selected from the middle and dropped the opening — with no
// error, no note, and no sign that a capability had gone missing.
//
// The capability was real and in use: the owner found pictures by fetching
// pages and reading the image URLs out of the result. It worked before 24 Aug
// and stopped that day, and nobody would have noticed from a green build.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func pageWithPicturesAndProse() string {
	var b strings.Builder
	b.WriteString("<html><head><title>Bicycles of the world</title></head><body>")
	for _, n := range []string{"a", "b", "c", "d", "e", "f"} {
		b.WriteString(`<img src="https://pics.test/photo-` + n + `.jpg" alt="a red bicycle">`)
	}
	b.WriteString("<p>" + strings.Repeat("filler prose about unrelated matters. ", 400) + "</p>")
	b.WriteString("<p>" + strings.Repeat("the answer mentions kubernetes ingress here. ", 30) + "</p>")
	b.WriteString("<p>" + strings.Repeat("more filler prose about other things. ", 400) + "</p>")
	b.WriteString("</body></html>")
	return b.String()
}

func servePage(t *testing.T, html string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(html))
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// The regression itself: a text search must not cost the caller the pictures.
func TestFindKeepsThePageImages(t *testing.T) {
	url := servePage(t, pageWithPicturesAndProse())
	s := &webFetchSkill{}

	plain, err := s.ExecuteTool(context.Background(), map[string]any{"url": url})
	if err != nil {
		t.Fatalf("plain fetch: %v", err)
	}
	if !strings.Contains(plain.Content, "pics.test") {
		t.Fatal("the plain fetch lost the images, so this test is measuring the wrong thing")
	}

	found, err := s.ExecuteTool(context.Background(), map[string]any{"url": url, "find": "kubernetes ingress"})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !strings.Contains(found.Content, "pics.test") {
		t.Errorf("a text search dropped every image URL:\n%s", found.Content)
	}
	// And it still answered the question it was asked.
	if !strings.Contains(found.Content, "kubernetes ingress") {
		t.Error("the head came back but the match did not")
	}
}

// Orientation travels with it: which page this is, and how many pictures it has
// beyond the ones shown. Both live in the same first passage, which is why one
// structural fix covers them together.
func TestFindKeepsTitleAndURL(t *testing.T) {
	url := servePage(t, pageWithPicturesAndProse())
	out, err := (&webFetchSkill{}).ExecuteTool(context.Background(),
		map[string]any{"url": url, "find": "kubernetes ingress"})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	for _, want := range []string{"Bicycles of the world", "URL: "} {
		if !strings.Contains(out.Content, want) {
			t.Errorf("the result does not say %q it came from:\n%s", want, out.Content[:min(400, len(out.Content))])
		}
	}
}

// The window is what the caller pays, and it must not move because of how a
// page happened to be laid out. Room for the head is made by dropping the
// weakest hit, never by growing the budget.
func TestTheHeadIsPaidForOutOfTheWindow(t *testing.T) {
	url := servePage(t, pageWithPicturesAndProse())
	out, err := (&webFetchSkill{}).ExecuteTool(context.Background(),
		map[string]any{"url": url, "find": "kubernetes ingress"})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	// The note and the head add a little on top of the passages themselves;
	// what must not happen is the window doubling.
	if len(out.Content) > webFetchWindow+2000 {
		t.Errorf("a find returned %d characters against a window of %d", len(out.Content), webFetchWindow)
	}
}

// (the query has to be words the page genuinely lacks: an earlier version of
// this test said "entirely unrelated" and matched "unrelated matters" in the
// filler, which made it a hit dressed as a miss)
// A miss already had its own answer and keeps it: the top of the page, said to
// be the top of the page. The head fix must not turn that into a silent hit.
func TestAMissStillSaysItMissed(t *testing.T) {
	url := servePage(t, pageWithPicturesAndProse())
	out, err := (&webFetchSkill{}).ExecuteTool(context.Background(),
		map[string]any{"url": url, "find": "quokka xylophone pemmican"})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !strings.Contains(out.Content, "nothing on this page mentions") {
		t.Errorf("a miss stopped saying it missed:\n%s", out.Content[max(0, len(out.Content)-300):])
	}
}
