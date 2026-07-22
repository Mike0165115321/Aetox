# internal/rtk — optional RTK output-filtering hook

> Module map: [ARCHITECTURE.md §4.1](../../ARCHITECTURE.md) · Design decision: [ARCHITECTURE.md §13](../../ARCHITECTURE.md) (settled 2026-07-22)

**What it is:** a thin, optional bridge to the owner's own `rtk` CLI (a token-optimizing output filter, unrelated to this repo) — shrinks what a tool call sends back to the model. Wired into two call sites: [internal/skill/shell.go](../skill/shell.go) (rewrites the command before running it) and [internal/turn/executor.go](../turn/executor.go)'s `modelToolReceipt` (filters git's output after running it).

## Key seams

Two independent mechanisms, one per shape of tool call — chosen after checking how rtk's *own* OpenCode/Claude Code hook plugins do this (`rtk init -g --opencode --dry-run`): those plugins are thin wrappers around a single `rtk rewrite` call, no hand-maintained command list. Aetox follows the same minimalism, split across the two places where it actually fits:

| Seam | Used by | What it does |
|---|---|---|
| `Available()` | both | `exec.LookPath("rtk")`, cached via `sync.Once` — checked once per process, not per call. |
| `Rewrite(command)` | `internal/skill/shell.go` | Calls `rtk rewrite <command>` — the exact call rtk's own OpenCode plugin makes, nothing hand-guessed. Returns the rtk-equivalent command (e.g. `git status` → `rtk git status`) to run *instead of* the original, or `("", false)` if rtk has no equivalent. Same underlying side effects either way (rtk actually runs the real command); only what Aetox's `exec.Cmd` targets changes. **Success is judged by stdout content, not exit code** — `rtk rewrite`'s own `--help` claims exit 0 on success, but a live check (v0.34.3) showed a successful rewrite exiting 3. |
| `FilterForTool(toolName, args)` + `Filter(filter, content)` | `internal/turn/executor.go`'s `modelToolReceipt`, for `git` only | Aetox's git skill already validates and parses the exact subcommand itself (`internal/skill/git.go`) — a direct name→filter mapping (`status`→`git-status`, `diff`/`show`→`git-diff`, `log`→`git-log`) is simpler than reconstructing a command string just to hand it to `Rewrite`. `Filter` runs `rtk pipe -f <filter>` with content on stdin, 5s timeout, falls back to the original content unchanged on any error. |

## Why not every tool

- `read.go` is deliberately excluded. `rtk read --level minimal` was tested live against a real file in this repo and **silently dropped every doc comment** — a correctness risk for a tool whose output the agent may use to edit that same file next. `rtk`'s file-reading path is a different mechanism (`rtk read --level`, needs a path) from the `pipe -f <filter>` mechanism this package uses (needs already-produced text), so folding it in would be a second, separate integration — not done.
- `write`/`delete`/`echo`/`time`/`help`/`input`/`output`/`github_repo_summary`/`plugin_install`/`image_ocr` have no matching `rtk pipe` filter — `FilterForTool` returns `""` for all of them, so they pass through exactly as before.

## Rules of thumb

- This package never talks to a provider and never changes what gets **approved** — `safety.AssessCommand`/`turn.resolveApproval` and the audit log always see the real, original command (`git status`, not `rtk git status`; the un-rewritten `commandLine`, not `execLine`). RTK only ever changes what actually executes (`shell.go`) or the result string after execution (`git`, via the receipt) — never what the user is asked to approve or what gets logged.
- Adding a new `FilterForTool` mapping = one more `case`, checked against `rtk pipe -f <bogus>`'s own error output for the current valid filter list (it can drift as `rtk` is upgraded — don't hardcode from memory, re-run the check). `Rewrite` needs no such list — it defers entirely to rtk's own registry.
- If `rtk` isn't installed, every caller behaves exactly as it did before this package existed. Never make it a hard dependency.
