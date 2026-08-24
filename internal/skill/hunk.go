package skill

import (
	"fmt"
	"strings"
)

// What a write actually changed, in the shape git says it in.
//
// The line counts (LineDelta) answer "how much", and for most of Aetox's work
// that is the whole answer: a person watching the assistant finish a document
// does not want the document's diff. The โค้ด desk is the room where "how much"
// is not enough — an agent that says `+42 -17` and nothing else is asking to be
// trusted about the one thing the user came here to check. So the tools that
// write files carry the change itself up to the UI, and the chat draws it under
// the row that made it.
//
// The format is git's, deliberately: unified hunks, three lines of context, one
// character of prefix per line. It is not produced by shelling out to git —
// nothing here is committed, half these files are untracked, and the change
// wanted is *this call's*, not the working tree's, which has every other edit of
// the turn mixed into it. It is the same format because that is the format
// every programmer already reads, and because a row expanded a week later must
// show what THAT call did, which only the call itself knows.
//
// One line is not git's:
//
//	~N   N further diff lines exist and were dropped
//
// A transcript is stored, and an unbounded diff per tool call would put a
// generated lockfile into the session store. The sentinel exists so the cut is
// visible: a diff that will not say how truncated it is reads as complete, and
// that is the one thing this must never do.
const (
	// diffContext is git's default, and matches it for the same reason the rest
	// of the format does.
	diffContext = 3
	// diffMaxLines caps one call's diff. Generous enough that an ordinary edit
	// — even a large one — arrives whole, small enough that a rewritten
	// lockfile does not.
	diffMaxLines = 400
	// diffMaxCells bounds the LCS table. Beyond it the change is reported as
	// one block replaced rather than line-matched: a precise diff of two
	// thousand-line regions is not worth a hundred megabytes of table, and
	// nobody reads it line by line anyway.
	diffMaxCells = 1 << 20
)

// UnifiedDiff renders before → after as git-style hunks, or "" when nothing
// changed. Line endings are normalised to LF first: on the reference platform
// every checked-out file is CRLF (see lineendings.go), and a diff that renders
// a `\r` per line is noise about the file's convention rather than about the
// change.
func UnifiedDiff(before, after string) string {
	oldLines, newLines := splitDiffLines(toLF(before)), splitDiffLines(toLF(after))

	// The common head and tail are the bulk of any real edit, and they are
	// found by walking rather than by matching. This is what keeps the table
	// below small: an exact-string replacement in a ten-thousand-line file
	// leaves a handful of lines between the two walks.
	pre := 0
	for pre < len(oldLines) && pre < len(newLines) && oldLines[pre] == newLines[pre] {
		pre++
	}
	suf := 0
	for suf < len(oldLines)-pre && suf < len(newLines)-pre &&
		oldLines[len(oldLines)-1-suf] == newLines[len(newLines)-1-suf] {
		suf++
	}
	if pre == len(oldLines) && pre == len(newLines) {
		return ""
	}

	midOld, midNew := oldLines[pre:len(oldLines)-suf], newLines[pre:len(newLines)-suf]

	ops := make([]diffOp, 0, len(midOld)+len(midNew)+2*diffContext)
	// Context from the head, taken by index — the walk above already proved
	// these lines identical, so there is nothing to match here.
	head := pre - diffContext
	if head < 0 {
		head = 0
	}
	for i := head; i < pre; i++ {
		ops = append(ops, diffOp{kind: ' ', text: oldLines[i]})
	}
	ops = append(ops, diffMiddle(midOld, midNew)...)
	tail := len(oldLines) - suf
	for i := tail; i < tail+diffContext && i < len(oldLines); i++ {
		ops = append(ops, diffOp{kind: ' ', text: oldLines[i]})
	}

	return renderHunks(ops, head+1, head+1)
}

// FileDiff is UnifiedDiff with the file it belongs to named above it, for the
// one tool that changes several files in a single call (edits). Every
// other writer touches one file and the row's own label already names it.
func FileDiff(path, before, after string) string {
	body := UnifiedDiff(before, after)
	if body == "" {
		return ""
	}
	return "+++ " + path + "\n" + body
}

// JoinDiffs stacks per-file diffs into one call's diff, keeping the whole
// thing inside the same budget a single-file diff gets.
func JoinDiffs(parts []string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			kept = append(kept, p)
		}
	}
	if len(kept) == 0 {
		return ""
	}
	return capDiff(strings.Join(kept, "\n"))
}

type diffOp struct {
	kind byte // ' ' context, '-' removed, '+' added
	text string
}

// diffMiddle matches the changed region line by line, or gives up honestly and
// reports it as a block replaced. Giving up is not a failure mode worth hiding:
// the counts are still exact and the content is still all there, only unpaired.
func diffMiddle(oldLines, newLines []string) []diffOp {
	if len(oldLines) == 0 || len(newLines) == 0 ||
		len(oldLines)*len(newLines) > diffMaxCells {
		return blockOps(oldLines, newLines)
	}

	// Classic LCS table. It runs only over what the prefix/suffix walks could
	// not account for, which is why a quadratic algorithm is the right one here.
	lcs := make([][]int, len(oldLines)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(newLines)+1)
	}
	for i := len(oldLines) - 1; i >= 0; i-- {
		for j := len(newLines) - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
				continue
			}
			if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	ops := make([]diffOp, 0, len(oldLines)+len(newLines))
	i, j := 0, 0
	for i < len(oldLines) && j < len(newLines) {
		switch {
		case oldLines[i] == newLines[j]:
			ops = append(ops, diffOp{kind: ' ', text: oldLines[i]})
			i, j = i+1, j+1
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, diffOp{kind: '-', text: oldLines[i]})
			i++
		default:
			ops = append(ops, diffOp{kind: '+', text: newLines[j]})
			j++
		}
	}
	for ; i < len(oldLines); i++ {
		ops = append(ops, diffOp{kind: '-', text: oldLines[i]})
	}
	for ; j < len(newLines); j++ {
		ops = append(ops, diffOp{kind: '+', text: newLines[j]})
	}
	return ops
}

func blockOps(oldLines, newLines []string) []diffOp {
	ops := make([]diffOp, 0, len(oldLines)+len(newLines))
	for _, l := range oldLines {
		ops = append(ops, diffOp{kind: '-', text: l})
	}
	for _, l := range newLines {
		ops = append(ops, diffOp{kind: '+', text: l})
	}
	return ops
}

// renderHunks turns a flat op list into hunks, splitting wherever more than
// twice the context separates two changes — the run in between is file nobody
// is being asked to read.
func renderHunks(ops []diffOp, oldStart, newStart int) string {
	// Where every op sits in each version. Computed once: a hunk header needs
	// the number of its first line, and walking back for it per hunk is how
	// off-by-ones get in.
	oldNo, newNo := make([]int, len(ops)), make([]int, len(ops))
	oldLine, newLine := oldStart, newStart
	changes := make([]int, 0, len(ops))
	for i, op := range ops {
		oldNo[i], newNo[i] = oldLine, newLine
		switch op.kind {
		case ' ':
			oldLine, newLine = oldLine+1, newLine+1
		case '-':
			oldLine++
		case '+':
			newLine++
		}
		if op.kind != ' ' {
			changes = append(changes, i)
		}
	}
	if len(changes) == 0 {
		return ""
	}

	var out strings.Builder
	consumed := 0 // exclusive op index the previous hunk already printed
	for k := 0; k < len(changes); {
		start := changes[k] - diffContext
		if start < consumed {
			start = consumed
		}

		// Swallow every further change close enough that the context around
		// the two would overlap; git merges on the same rule.
		last := changes[k]
		next := k + 1
		for next < len(changes) && changes[next]-last <= 2*diffContext+1 {
			last = changes[next]
			next++
		}
		end := last + diffContext + 1
		if end > len(ops) {
			end = len(ops)
		}

		oldCount, newCount := 0, 0
		var body strings.Builder
		for x := start; x < end; x++ {
			switch ops[x].kind {
			case ' ':
				oldCount, newCount = oldCount+1, newCount+1
			case '-':
				oldCount++
			case '+':
				newCount++
			}
			body.WriteByte(ops[x].kind)
			body.WriteString(ops[x].text)
			body.WriteByte('\n')
		}
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", oldNo[start], oldCount, newNo[start], newCount)
		out.WriteString(body.String())

		consumed = end
		k = next
	}

	return capDiff(strings.TrimSuffix(out.String(), "\n"))
}

// capDiff enforces the budget and says so when it bites.
func capDiff(diff string) string {
	if diff == "" {
		return ""
	}
	lines := strings.Split(diff, "\n")
	if len(lines) <= diffMaxLines {
		return diff
	}
	kept := lines[:diffMaxLines]
	return strings.Join(kept, "\n") + fmt.Sprintf("\n~%d", len(lines)-diffMaxLines)
}

// splitDiffLines splits on LF without inventing a trailing empty line for a
// file that ends in a newline, which every well-formed source file does.
func splitDiffLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
