package main

// One chat's own engine, and the coordinates it was built at.
//
// **The rule this file exists to enforce: the core has no "current session".**
// Which chat the user is looking at is a fact about the window, and the window
// is welcome to hold it. Everything under here is addressed by id — a turn
// names the conversation it runs in, a callback carries the conversation it was
// built for, and a row is written where its conversation says.
//
// Aetox was the opposite of that for its whole life. `App` held one agent, one
// registry, one transcript and one `sessionID` cursor, and ten places in the
// turn path asked `turnSessionID()` to *guess* whose work they were doing —
// honestly, and correctly, for exactly as long as only one turn could exist.
// Every refusal the owner kept hitting ("เอเจนกำลังทำงานอยู่ รอให้เสร็จ") was
// that single cursor defending itself: opening a second chat meant overwriting
// the first one's mind mid-thought, so the doors were shut instead (§134.2).
//
// The shape here is the one Codex uses and the one opencode uses: a single
// process holding many conversations in a map, each addressed by id, with the
// UI naming which one it is talking to. Claude Code reaches the same isolation
// with a process per conversation, which one Wails window cannot do (§134.4).
// What is expensive is still shared — the MCP manager, the provider clients,
// the database — and what IS the conversation is not: the agent and its
// context. `bootstrap.Engine` was measured against exactly this before any of
// it was built (internal/bootstrap/two_engines_test.go): callable twice, two
// separate memories, ~10ms for the second because the costly halves are shared.

import (
	"sync"

	"github.com/Mike0165115321/Aetox/internal/config"

	aetoxapp "github.com/Mike0165115321/Aetox/internal/app"
	"github.com/Mike0165115321/Aetox/internal/cognitive"
	"github.com/Mike0165115321/Aetox/internal/mode"
	"github.com/Mike0165115321/Aetox/internal/skill"
	"github.com/Mike0165115321/Aetox/internal/subagent"
)

// conversation is one chat: its engine, the coordinates that engine was built
// from, and the record of what has been said.
//
// Every field here was an `App` field until 2026-08-19. They moved together
// because they were always one thing — none of them describes the app, all of
// them describe a chat — and keeping half of them on `App` would leave exactly
// the ambiguity this change exists to remove.
type conversation struct {
	// cfg is what this chat's engine runs on: its model, its provider and wire
	// format, its thinking level, its approval mode, its delegate switches —
	// and, riding along because bootstrap.Engine takes one config, the
	// machine-level facts every chat shares (the sandbox root, the workspace).
	//
	// The dials moved here on 2026-08-20. They were `App.cfg` alone, which was
	// true while the app was one conversation and became a lie the moment it
	// was several: switching the model in one chat changed the name every chat
	// reported while rebuilding only the engine on screen, so a chat working in
	// the background went on answering with the old provider under the new
	// name. ApprovalMode was the same field and the worse case — the safety
	// gate of a turn already running, moved by a picker in another chat.
	//
	// `App.cfg` is still there and still means something, but something else:
	// what a NEW chat is born with. Changing a dial writes both — this one, so
	// the chat on screen changes, and that one, so the next chat inherits the
	// choice — and rebuilds this engine and no other.
	cfg config.Config

	// id is the session id these rows are stored under. Empty until the first
	// turn names it, which is the state a freshly launched app sits in.
	id string

	// The engine. Rebuilt in place by applyConfig on a model, stance or
	// workspace change; never rebuilt because the user opened another chat,
	// which is the whole point of holding one of these per conversation.
	chat        *aetoxapp.App
	agent       *cognitive.Agent
	registry    *skill.Registry
	delegations *subagent.Delegations

	// modelStatus is what the engine resolved to, for the UI's model row.
	modelStatus string
	// modelErr is why the last bootstrap could not reach the configured
	// provider. Non-nil with a live chat means the engine is running the
	// built-in aetox fallback while the UI names something else — see
	// modelSwitchResult.
	modelErr error

	// desk is the mode this session was created at (ARCHITECTURE.md §83), nil
	// for the full desk — every session from before modes existed, and the
	// state the app starts in. Set when a session is opened and never while one
	// is running: a session is born at a desk and stays there, which is what
	// lets its context be trusted never to have held another desk's tools.
	desk *mode.Mode
	// chair is the session's second coordinate (§85): which of the office's
	// agents the user is talking to directly, "" for every session held with
	// the main assistant. Only ever non-empty alongside desk = the office.
	chair string
	// space is the session's third coordinate: which โปรเจกต์ this chat is held
	// inside, "" for a chat held outside every project. It moves no wall — see
	// COMPANY.md §84.
	space string
	// stance is the session's fourth coordinate (DECISIONS.md §106): how the
	// turn runs, as opposed to what is on the desk. The zero value is ลงมือ, and
	// this is the one coordinate that may change mid-conversation, because a
	// stance only ever subtracts from the desk.
	stance mode.Stance

	// transcript is what has been said, as the store holds it. The turn that is
	// running appends to the transcript of the conversation it was started in —
	// never to "the app's", which is how an answer used to be able to land in
	// whichever chat the user had opened by the time it finished.
	transcript []SessionMessage
	// turnOpened is true between openTurn and appendTurn: the user message for
	// the turn now running is already in the store, so the closing write must
	// not add it a second time.
	turnOpened bool

	// lastSnapshot is the tree as it stood before THIS chat's last turn — what
	// its undo goes back to. "" when there is nothing to go back to.
	//
	// It was one field on App, which was true while one turn could exist. With
	// two chats working, the second turn's capture overwrote the first's, so
	// undo in one conversation restored the other one's work — the only item
	// on §155's leftover list that damaged data rather than confusing a screen.
	//
	// The snapshot STORE stays shared: it is the project's git object database,
	// one per working tree, and every chat in that tree is looking at the same
	// files. What is per chat is the point in it that this conversation may go
	// back to. Guarded by App.snapshotMu with the store.
	lastSnapshot string

	// userSaves is every file the PERSON saved from the editor since
	// lastSnapshot was taken. An undo of this chat leaves them alone.
	//
	// §157 settled which POINT an undo goes back to. It left open which FILES it
	// drags along, and the answer was "all of them": Restore(id, nil) puts back
	// every file that differs from the snapshot, and the snapshot store is the
	// whole work tree that every chat in the project shares. So pressing undo to
	// reject one answer also threw away whatever the user had typed while the
	// turn ran, with nothing saying so.
	//
	// An exclusion rather than the obvious inclusion — "restore only what the
	// agent's tools wrote" — because that list cannot be completed. Writing
	// tools are enumerable (session_edits.go), but `shell` runs anything, and a
	// tool that produces a file without being one of the writers (a rendered
	// picture, a screenshot) would silently stop being undoable the day it is
	// added. What CAN be enumerated exactly is what came through this app's own
	// editor, because App.WriteFile is the only door it has. So undo keeps its
	// reach and gives up only the files it can prove are the user's.
	//
	// Filled for every live conversation, not just the one on screen: a save is
	// a fact about the tree, and the chat whose undo might eat it is often not
	// the chat being looked at. Reset by captureSnapshot. Guarded by
	// App.snapshotMu.
	userSaves []string

	// openTabs is what the window reports is open on THIS chat's workbench, so
	// the agent can read its own desk (desk_list).
	//
	// The window has kept the layout per session all along
	// (switchWorkbenchSession); what was one per app was the mirror on this
	// side, because there was only ever one chat to mirror. With two working at
	// once a tool call from the chat nobody is looking at read the tab strip of
	// the chat on screen — someone else's desk, described as its own.
	openTabs deskState

	// taskChips is the side work THIS chat flagged with suggest_task and the
	// user has not started or dismissed.
	//
	// One list per app read as "things Aetox noticed", which is not what a chip
	// is: it is something this conversation saw while doing this conversation's
	// job, and its prompt is written to stand alone from THIS chat. Shown in
	// another one it is a suggestion with no visible origin — and with two chats
	// working, two agents were filling one tray.
	taskChips taskChips

	// toolHistory is this chat's command log, for the Inspector's panel. Per
	// conversation rather than per app because the panel has always claimed to
	// show "this session's" calls, and two chats working at once would have
	// made that claim false. Guarded by App.toolHistoryMu, which covers every
	// conversation's slice: a delegate writes from its own goroutine (§44.11).
	toolHistory []string

	// A worker addressed with @name that stopped to ask something, and is still
	// holding everything it had done when it stopped. The next message in THIS
	// chat answers it (runAnswer) unless that message addresses somebody else.
	pendingTask  string
	pendingAgent string

	// askCh is the in-flight ask_user question's answer channel, nil when this
	// chat is not waiting on one. Per conversation on the owner's instruction
	// (19 ส.ค.: *"ask_user หากมี ให้ขึ้นแจ้งเตือนที่เซสชั่นปกติเลยครับ"*) — the
	// question belongs to the chat that asked it, so it is drawn there and
	// answered there, and two chats can be waiting on different questions
	// without either one seeing the other's. Guarded by App.askMu.
	askCh chan string
}

// newConversation is a chat with nothing in it yet: no engine (applyConfig
// builds that) and no id (the first turn names it).
func newConversation() *conversation { return &conversation{} }

// conversations is every chat this process currently holds an engine for,
// keyed by session id, plus which one the window is looking at.
//
// Deliberately not "the current one plus a parking lot for the others". That
// shape was considered and rejected: it makes `App` the answer to "which chat
// is this?" all over again, one field deep, and the first piece of code to read
// the wrong half of it would fail in exactly the way this change is meant to
// end. There is one home per conversation and the cursor is only a cursor.
type conversations struct {
	mu sync.Mutex
	// byID holds every live conversation. A conversation stays here while its
	// turn runs, whether or not the window is showing it.
	byID map[string]*conversation
	// open is the conversation the window last said it was looking at. It is a
	// UI fact kept here so the bindings that legitimately mean "the chat on
	// screen" have somewhere honest to read it from — never consulted by the
	// turn path, which is handed its conversation outright.
	open *conversation
}

func newConversations() *conversations {
	open := newConversation()
	return &conversations{byID: map[string]*conversation{}, open: open}
}

// current returns the conversation the window is looking at. Never nil: a
// launched app with no session yet is a conversation with no id, which is a
// real state rather than a missing one.
func (c *conversations) current() *conversation {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.open
}

// show makes conv the one the window is looking at, and files it under its id
// if it has one. Filing here rather than at birth is deliberate: a conversation
// gets its id when its first turn is stored, and a map keyed by "" would hold
// every unsaved chat under one key.
func (c *conversations) show(conv *conversation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.open = conv
	if conv.id != "" {
		c.byID[conv.id] = conv
	}
}

// cur is the conversation the window is looking at.
//
// Named short because it appears everywhere a binding legitimately means "the
// chat on screen" — which is most of them: drawing the model row, listing the
// tools, reading the transcript for the panel. It is the ONE thing the turn
// path may not call. A turn is handed its conversation when it begins and
// keeps it, because by the time it ends the window may be somewhere else, and
// asking again is how an answer lands in a chat that did not ask for it.
func (a *App) cur() *conversation {
	// Once, not a bare nil check. NewApp builds the manager, so in the running
	// app this does nothing at all — but a zero App is how most of this
	// package's tests construct one, and two goroutines reaching a bare `if
	// a.convs == nil` at the same time each build their own manager and then
	// disagree about which conversation is open. That is not theoretical: the
	// approval test asks a question on one goroutine and waits for it on
	// another, and it failed exactly this way the first time it ran.
	a.convsOnce.Do(func() {
		if a.convs == nil {
			a.convs = newConversations()
		}
	})
	return a.convs.current()
}

// all returns every live conversation, the one on screen included.
//
// A snapshot of the map rather than the map itself: the caller holds a
// different lock (App.snapshotMu) while it walks the result, and handing out
// the live map would put two locks in an order nobody has checked.
func (c *conversations) all() []*conversation {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*conversation, 0, len(c.byID)+1)
	seen := map[*conversation]bool{}
	if c.open != nil {
		out = append(out, c.open)
		seen[c.open] = true
	}
	for _, conv := range c.byID {
		if !seen[conv] {
			out = append(out, conv)
		}
	}
	return out
}

// find returns the live conversation with this id, or nil. A conversation this
// process holds no engine for is not an error — it is one that has to be built.
func (c *conversations) find(id string) *conversation {
	if id == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.byID[id]
}

// forget drops a conversation the process no longer needs an engine for. The
// caller decides that; this only stops holding it.
func (c *conversations) forget(id string) {
	if id == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.byID, id)
}

// The manager stays this small on purpose. Walking every live conversation, and
// filing one under an id it earns later than birth, are both written and
// neither is here: nothing calls them yet, and a method waiting for its caller
// is the same debt as a caller waiting for its method. golangci-lint said so on
// the first run, when four of these existed and none had a caller.

// sessionEvent is anything the engine says, with the conversation that said it.
//
// Every `agent:*` event used to go out bare, which was honest while only one
// chat could be working: there was nothing to confuse it with. The moment two
// can, an unstamped event is one the window has to guess the home of, and the
// window's guess would be "whatever is on screen" — the exact failure this
// whole change exists to end, moved from Go into TypeScript.
//
// Generic so the payload keeps its own type all the way to the binding, rather
// than every listener re-parsing an `any`.
type sessionEvent[T any] struct {
	SessionID string `json:"sessionId"`
	Data      T      `json:"data"`
}
