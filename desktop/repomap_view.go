package main

// The node view's data: internal/repomap's Graph face, served to the frontend.
//
// The owner's requirement (29 ส.ค.) was that the system draw the map itself —
// "ไม่ให้โมเดลทำ" — and stay in sync with what the model's repo_map tool sees.
// So this calls the same analysis under the same two knobs the tool uses
// (skill.RepoMapIgnores, skill.RepoMapTimeBudget), and holds no cache: every
// open is a fresh 1-2s walk of the tree as it is right now, which is cheaper
// than being wrong about staleness ever could be.

import (
	"context"

	"github.com/Mikedev115/Aetox/internal/repomap"
	"github.com/Mikedev115/Aetox/internal/skill"
)

// RepoMapGraph is what the frontend draws. Focused false means there was no
// project root to map — the unfocused desk is the whole machine, and a map of
// a machine is a wrong answer ranked confidently, same rule as the tool.
//
// Mirrors of repomap.Node/Edge rather than the types themselves, because the
// wails binding generator turns every referenced package into a TS namespace
// and one flat shape is the whole of what the pane needs.
type RepoMapNode struct {
	Path    string `json:"path"`
	Dir     string `json:"dir"`
	Refs    int    `json:"refs"`
	Symbols int    `json:"symbols"`
}

type RepoMapEdge struct {
	From int `json:"from"`
	To   int `json:"to"`
}

type RepoMapGraph struct {
	Focused    bool          `json:"focused"`
	Root       string        `json:"root,omitempty"`
	Nodes      []RepoMapNode `json:"nodes"`
	Edges      []RepoMapEdge `json:"edges"`
	TotalFiles int           `json:"totalFiles"`
	Error      string        `json:"error,omitempty"`
}

// GetRepoMapGraph maps the focused project down to maxNodes files. Zero means
// the default sixty; repomap.AllNodes (-1) means every file that was mapped.
// The ceiling is the caller's because it is a question about the SCREEN, not
// about the repository — the walk below costs the same either way, and which
// slice of its answer is worth drawing is the only part the viewer can judge.
func (a *App) GetRepoMapGraph(maxNodes int) RepoMapGraph {
	root := a.cur().cfg.SandboxRoot
	// Same reading applyConfig gives the engine: no focused project means the
	// sandbox is open and there is nothing honest to map.
	if root == "" || !a.projectFocused {
		return RepoMapGraph{Focused: false}
	}
	ctx, cancel := context.WithTimeout(context.Background(), skill.RepoMapTimeBudget)
	defer cancel()
	nodes, edges, total, err := repomap.Graph(ctx, repomap.Options{
		Root:   root,
		Ignore: skill.RepoMapIgnores(),
	}, maxNodes)
	if err != nil {
		return RepoMapGraph{Focused: true, Root: root, Error: err.Error()}
	}
	out := RepoMapGraph{Focused: true, Root: root, TotalFiles: total,
		Nodes: make([]RepoMapNode, 0, len(nodes)), Edges: make([]RepoMapEdge, 0, len(edges))}
	for _, n := range nodes {
		out.Nodes = append(out.Nodes, RepoMapNode{Path: n.Path, Dir: n.Dir, Refs: n.Refs, Symbols: n.Symbols})
	}
	for _, e := range edges {
		out.Edges = append(out.Edges, RepoMapEdge{From: e.From, To: e.To})
	}
	return out
}
