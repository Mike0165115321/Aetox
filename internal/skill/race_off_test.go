//go:build !race

package skill

// underRace says whether this binary was built with -race.
//
// It exists for one test — the one that asserts calc's five-second ceiling is
// generous enough for a real calculation (calc_test.go). That assertion is a
// measurement of the interpreter Aetox actually ships, and -race replaces it
// with an instrumented one that runs several times slower, so under race the
// number being checked is not the number the ceiling was set against. CI runs
// the suite both ways; without this the race pass failed on a timing claim it
// was never in a position to make.
//
// Two files rather than a runtime check because Go offers no runtime "am I
// under race" answer — the `race` build tag is the only handle.
const underRace = false
