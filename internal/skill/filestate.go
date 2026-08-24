package skill

// What Aetox last saw a file as, so a whole-file write can tell "I am replacing
// this" from "somebody changed it under me".
//
// Owner, 24 ส.ค., on `write`: *"อันตรายมาก"*. He was right, and the reason is
// structural: `os.WriteFile` truncates, there is no lock anywhere in this
// program, and the last writer wins. Two things share the tree at all times —
// the agent's tools and the person typing in the editor beside them — and until
// now neither could see the other move.
//
// `edit` and `edits` were already safe by construction: they match
// the `find` text against the bytes on disk, so an edit aimed at text somebody has
// since changed fails cleanly and writes nothing. The whole-file writers had no
// equivalent, and that is what this is.
//
// **What it is not.** This is not a lock. Between the check and the write there
// is still a gap, and two processes with no lock cannot be made exclusive by
// looking harder. It turns "silently overwritten" into "almost always refused",
// which is the honest description and the whole of the improvement.
//
// **Why a record rather than a read-before-write.** A whole-file `write` usually
// has not read the file — writing without reading is what the tool is FOR, and
// refusing that would refuse the ordinary case. So the question asked here is
// narrower and answerable: *has this file changed since the last time anything
// in this app looked at it?* No record at all means nobody here has looked, and
// the write goes through.

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// FileState is the record. One per app, shared by every conversation's tools
// and by the editor's own save path, because the point is to catch two of them
// touching one file.
type FileState struct {
	mu   sync.Mutex
	seen map[string]stamp
}

// stamp is what a file looked like. Size and mod time rather than a hash: this
// runs before every whole-file write, including of files that are megabytes,
// and the failure it is catching is another writer — which moves both.
//
// Two writes inside one filesystem timestamp tick with identical sizes would
// read as unchanged. On the reference platform that is a 100ns tick; elsewhere
// it can be a second, which is why size is in the comparison as well.
type stamp struct {
	size int64
	mod  time.Time
}

func NewFileState() *FileState {
	return &FileState{seen: map[string]stamp{}}
}

// Note records what the file looks like now: called after anything in this app
// reads or writes it, so the next writer has something to compare against.
//
// A path that cannot be stat'ed is forgotten rather than recorded — a file that
// is not there has no state worth remembering, and a stale entry would make the
// next write refuse for a file nobody touched.
func (f *FileState) Note(path string) {
	if f == nil || path == "" {
		return
	}
	info, err := os.Stat(path)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err != nil || info.IsDir() {
		delete(f.seen, path)
		return
	}
	f.seen[path] = stamp{size: info.Size(), mod: info.ModTime()}
}

// Forget drops what is known about a path. Called when a file is deleted, so
// the next write of the same name is a creation rather than a refusal.
func (f *FileState) Forget(path string) {
	if f == nil || path == "" {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.seen, path)
}

// ChangedSinceSeen reports whether the file has moved since this app last
// looked at it.
//
// False whenever there is no record — that is a file nobody here has read or
// written, so there is nothing to be stale against and a whole-file write is
// exactly what was asked for. False too when the file is now gone: writing it
// is a creation, and refusing that would be refusing to do the obvious thing.
func (f *FileState) ChangedSinceSeen(path string) bool {
	if f == nil || path == "" {
		return false
	}
	f.mu.Lock()
	was, known := f.seen[path]
	f.mu.Unlock()
	if !known {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Size() != was.size || !info.ModTime().Equal(was.mod)
}

// errStaleWrite is what a whole-file writer says when the file moved under it.
//
// The message is written for the model, because the model is who has to act on
// it: it names the act to take (read it again) rather than describing the
// state. Without that, a refusal it cannot resolve becomes a retry loop.
func errStaleWrite(requestPath string) error {
	return fmt.Errorf(
		"%s changed on disk since this session last read it — someone else is editing it. "+
			"Read it again first: that both shows what they changed and clears this refusal, "+
			"and then `edit` keeps their work where a whole-file write would replace it",
		requestPath)
}

// guardStale is the check itself, as one line for every whole-file writer.
func (f *FileState) guardStale(targetPath, requestPath string) error {
	if f.ChangedSinceSeen(targetPath) {
		return errStaleWrite(requestPath)
	}
	return nil
}
