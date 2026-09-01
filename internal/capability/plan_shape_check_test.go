package capability

import "testing"

// What the install confirmation shows for the card that makes videos, printed
// rather than asserted: the numbers come from the manifest and the point is to
// read them once with human eyes before somebody presses a button that spends
// a quarter of a gigabyte of their connection.
func TestWhatTheMakeVideoCardOffersToFetch(t *testing.T) {
	t.Setenv("AETOX_DATA_ROOT", t.TempDir())
	var total int64
	for _, c := range MissingFor([]string{"video", "video-make"}) {
		t.Logf("%-24s %6.1f MB  %-34s %s", c.ID, float64(c.ApproxBytes)/(1<<20), c.Title, c.License)
		total += c.ApproxBytes
		if c.Title == "" || c.License == "" || c.Homepage == "" {
			t.Errorf("%s has a blank title, licence or homepage — the confirmation dialog draws all three", c.ID)
		}
		if c.ApproxBytes <= 0 {
			t.Errorf("%s reports no size, so the button would offer to fetch 0 MB", c.ID)
		}
	}
	t.Logf("%-24s %6.1f MB", "total", float64(total)/(1<<20))
}
