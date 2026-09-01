// The map's second face: the same analysis the model reads as budgeted text,
// handed to the UI as nodes and edges. One walk, one ranking, one ignore list
// — the owner's requirement was that the system's view "ซิงค์กับ repo_map
// ที่โมเดลเขียนไว้", and the only sync that cannot drift is sharing the
// computation itself (analyze/rank above), never a cached copy of its output.
//
// What differs is only what each reader can afford: the model pays per token,
// so Build cuts to ~1k and eight symbols a file; a screen pays per pixel, so
// Graph keeps whole files and real edges and cuts only the node COUNT, because
// a thousand circles is not a picture of anything.
package repomap

import (
	"context"
	"math"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultMaxNodes is how many files the graph keeps when the caller has no
// opinion. Sixty is where a force-laid graph still reads as districts and hubs
// rather than as felt — the right first sentence, not the only one the reader
// is allowed to hear.
const DefaultMaxNodes = 60

// AllNodes, passed as maxNodes, lifts the ceiling entirely: every file the walk
// mapped becomes a node. There is no number to invent here, because the honest
// ceiling was always the project's own size — and a caller that wants the whole
// thing should not have to guess how big the whole thing is to ask for it.
const AllNodes = -1

// Node is one file the graph kept, ranked the same way the text map ranks.
type Node struct {
	Path    string `json:"path"`
	Dir     string `json:"dir"`
	Refs    int    `json:"refs"`
	Symbols int    `json:"symbols"`
}

// Edge points from the importing node to the imported one, by index into the
// node list. Indices rather than paths because the consumer is a renderer,
// and a renderer joining strings to find its endpoints is doing this
// package's job with worse tools.
type Edge struct {
	From int `json:"from"`
	To   int `json:"to"`
}

// Graph analyzes root and returns the top maxNodes files with the import
// edges that run between them, plus how many mapped files exist in total —
// the number that tells the reader how much of the repository the picture is.
func Graph(ctx context.Context, opts Options, maxNodes int) ([]Node, []Edge, int, error) {
	switch {
	case maxNodes == AllNodes:
		maxNodes = math.MaxInt
	case maxNodes <= 0:
		maxNodes = DefaultMaxNodes
	}
	a, err := analyze(ctx, opts)
	if err != nil {
		return nil, nil, 0, err
	}
	a.rank()

	// A file with nothing but an outgoing import (an entry point, a thin
	// wiring file) is invisible to the ranking — no symbols, no incoming refs
	// — and it is exactly the arrow-tail a graph exists to draw. Counted here
	// so selection can keep it.
	spokesperson := goSpokespersons(a)

	// Resolving one import target can cost a scan of every mapped file: when
	// the target names nothing in the tree — "fmt", "react", any dependency —
	// resolveTargetFile falls through to a suffix search over the whole map.
	// That was paid once per EDGE and the whole pass was then run TWICE, once
	// to count out-degree and again to draw the lines, so a project importing
	// "context" from fifty files searched all 2,363 of them fifty times over,
	// twice. Memoised per DISTINCT target and shared by both passes: 2.4s to
	// under a second on this repository, with the same edges out the far end.
	resolved := make(map[string]string)
	resolve := func(target string) string {
		if to, ok := resolved[target]; ok {
			return to
		}
		to := ""
		if f, ok := resolveTargetFile(a.byRel, target); ok {
			to = f.rel
		} else if rep, ok := spokesperson[target]; ok {
			to = rep
		}
		resolved[target] = to
		return to
	}

	outDeg := make(map[string]int)
	for _, e := range a.edges {
		if resolve(e.target) != "" {
			outDeg[e.from]++
		}
	}

	// Reserve for what the tree can actually supply, never for the ceiling:
	// AllNodes is math.MaxInt, and a slice asked to reserve that much panics.
	nodes := make([]Node, 0, min(maxNodes, len(a.files)))
	index := make(map[string]int)
	for _, f := range a.files {
		if len(nodes) >= maxNodes {
			break
		}
		if len(f.symbols) == 0 && f.refs == 0 && outDeg[f.rel] == 0 {
			continue
		}
		index[f.rel] = len(nodes)
		nodes = append(nodes, Node{
			Path:    f.rel,
			Dir:     filepath.ToSlash(filepath.Dir(f.rel)),
			Refs:    f.refs,
			Symbols: len(f.symbols),
		})
	}

	edges := resolveEdges(a, index, resolve)
	return nodes, edges, a.total, nil
}

// resolveEdges turns the walk's raw import records into drawn lines between
// kept nodes. Script and Python targets resolve to a file directly; a Go
// target names a package directory, and the line lands on that package's
// fullest file — a spokesperson, chosen deterministically, because fanning one
// import out to every file of the package would draw edges nobody wrote.
func resolveEdges(a *analysis, index map[string]int, resolve func(string) string) []Edge {
	var edges []Edge
	seen := make(map[[2]int]bool)
	for _, e := range a.edges {
		to := resolve(e.target)
		if to == "" || to == e.from {
			continue
		}
		fi, ok := index[e.from]
		if !ok {
			continue
		}
		ti, ok := index[to]
		if !ok {
			continue
		}
		key := [2]int{fi, ti}
		if seen[key] {
			continue
		}
		seen[key] = true
		edges = append(edges, Edge{From: fi, To: ti})
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
	return edges
}

// goSpokespersons picks, per directory, the non-test Go file with the most
// symbols (ties to the shorter then earlier path) — where a package-level
// import edge lands.
func goSpokespersons(a *analysis) map[string]string {
	best := make(map[string]*file)
	for _, f := range a.files {
		if !strings.HasSuffix(f.rel, ".go") || strings.HasSuffix(f.rel, "_test.go") {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(f.rel))
		cur := best[dir]
		if cur == nil ||
			len(f.symbols) > len(cur.symbols) ||
			(len(f.symbols) == len(cur.symbols) && f.rel < cur.rel) {
			best[dir] = f
		}
	}
	out := make(map[string]string, len(best))
	for dir, f := range best {
		out[dir] = f.rel
	}
	return out
}
