package skill

// What `shell` used to say on every message, and now says once per action.
//
// It was the second-largest entry in the tool block at 854 tokens, behind only
// delegation. None of what follows was wrong — most of it is a rule learned by
// watching a turn go badly — but the model needs each of these a single time,
// not on every request for the life of the conversation.
//
// What deliberately did NOT move, and stayed in the block: which shell the
// commands are run by. That is not judgment. It is a fact about how to write
// THIS call, it changes the moment the user points the workspace at a distro,
// and a model told the wrong one writes the wrong dialect on every command of
// the turn with no way to find out except by failing. Guidance sent once could
// not correct a switch made after it.
//
// See guidance.go for the standard this follows.

import "strings"

func (s *shellSkill) Guidance(args map[string]any) string {
	action, _ := args["action"].(string)
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "", "run":
		return shellRunGuidance
	case "output":
		return shellOutputGuidance
	case "kill":
		return shellKillGuidance
	case "list":
		return shellListGuidance
	}
	return ""
}

// run carries the trigger for run_in_background, and that placement is the
// point rather than convenience.
//
// A model that does not set it on a dev server does not get a wrong answer, it
// gets a turn that spends its whole clock and then kills the thing it started.
// The first `run` of a session is almost always an ordinary command that exits,
// so this arrives well before the command that needs it — which is exactly the
// case guidance-on-first-use handles well, and worth contrasting with `wait` in
// the browser, where the trigger had to ride on a different action entirely.
const shellRunGuidance = "Use this to verify your own work after editing rather than reporting that a change should work.\n" +
	"A command that never exits on its own, a dev server, a watch build, a REPL, a log tail, MUST set run_in_background. Without it the call spends its whole timeout and the command is then killed, which looks like the command failing.\n" +
	"After 60 seconds (or timeout_seconds) a foreground command reports back as still running rather than being killed. Call again with the same command to look in on it.\n" +
	"Paths follow the same rule the file tools do: inside the folders this session may use, and a command naming anything outside is refused before it runs. A path the command assembles while running cannot be checked, so write paths out literally.\n" +
	"For looking at files, prefer read/grep/glob/list. They are faster and their output is shaped for you."

const shellOutputGuidance = "Output is CONSUMED: each call returns only what is new since the last one, so nothing is a real answer when nothing has been printed.\n" +
	"Prefer wait_for over calling this in a loop. Polling burns rounds to learn that nothing has happened yet; wait_for blocks until the thing you are waiting for appears, or gives up and says so."

const shellKillGuidance = "Ends the command and everything it started. Kill a dev server when you are done with it rather than leaving it holding a port, the next run of the same server will fail on that port, and the reason will not be visible in its error."

const shellListGuidance = "The way back when a handle has fallen out of your context. A background command outlives the turn that started it, so \"I no longer have the id\" is not the same as \"it is gone\"."
