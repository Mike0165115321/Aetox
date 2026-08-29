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
	"path/filepath"
	"sort"
	"strings"
)

// DefaultMaxNodes is how many files the graph keeps. Sixty is where a
// force-laid graph still reads as districts and hubs rather than as felt.
const DefaultMaxNodes = 60

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
	if maxNodes <= 0 {
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
	outDeg := make(map[string]int)
	spokesperson := goSpokespersons(a)
	for _, e := range a.edges {
		if _, ok := resolveTargetFile(a.byRel, e.target); ok {
			outDeg[e.from]++
		} else if _, ok := spokesperson[e.target]; ok {
			outDeg[e.from]++
		}
	}

	nodes := make([]Node, 0, maxNodes)
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

	edges := resolveEdges(a, index, spokesperson)
	return nodes, edges, a.total, nil
}

// resolveEdges turns the walk's raw import records into drawn lines between
// kept nodes. Script and Python targets resolve to a file directly; a Go
// target names a package directory, and the line lands on that package's
// fullest file — a spokesperson, chosen deterministically, because fanning one
// import out to every file of the package would draw edges nobody wrote.
func resolveEdges(a *analysis, index map[string]int, spokesperson map[string]string) []Edge {
	var edges []Edge
	seen := make(map[[2]int]bool)
	for _, e := range a.edges {
		to := ""
		if f, ok := resolveTargetFile(a.byRel, e.target); ok {
			to = f.rel
		} else if rep, ok := spokesperson[e.target]; ok {
			to = rep
		}
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
