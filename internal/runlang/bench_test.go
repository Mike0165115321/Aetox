package runlang

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The whole cost of this package is here, and it is paid once: Runnable walks
// PATH looking for each Script language's interpreters when the app starts.
// Everything after that is a map lookup per fenced block.
//
// Two cases, because they are not the same shape. A machine that HAS the
// interpreter stops at the first hit; a machine that does not walks every PATH
// entry against every PATHEXT before it can say no, which is the honest worst
// case and the one worth knowing the size of.
func BenchmarkRunnableOnThisMachine(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		Runnable()
	}
}

func BenchmarkRunnableWithNothingInstalled(b *testing.B) {
	// A long PATH of real, empty directories: the lookup has to visit all of
	// them and find nothing, which is what a machine with no Python does.
	root := b.TempDir()
	dirs := make([]string, 0, 40)
	for i := range 40 {
		d := filepath.Join(root, string(rune('a'+i%26))+string(rune('a'+i/26)))
		if err := os.MkdirAll(d, 0o755); err != nil {
			b.Fatal(err)
		}
		dirs = append(dirs, d)
	}
	b.Setenv("PATH", strings.Join(dirs, string(os.PathListSeparator)))

	b.ReportAllocs()
	for b.Loop() {
		Runnable()
	}
}
