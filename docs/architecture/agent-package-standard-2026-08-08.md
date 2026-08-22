# The Agent Package: One Folder Holds Everything One Worker Knows (2026-08-08)

Approved by the owner 2026-08-08; slices 1 and 2 are built. It answers one
question the office raised the moment `doc` was asked to write a tax invoice:
where does a worker's specialist knowledge live, and what stops it becoming
everyone's?

Live code this touches: [`agenthome.go`](../../internal/config/agenthome.go),
[`profile.go`](../../internal/subagent/profile.go),
[`discovery.go`](../../internal/skill/discovery.go),
[`list.go`](../../internal/skill/list.go),
[`sandbox_open.go`](../../internal/skill/sandbox_open.go).

---

## The rule

**One worker is one folder, and everything that worker knows is inside it.
Nothing specialist sits in a shared place.**

The office already keeps three things per agent — its definition, its memory,
its job history — and keeps them apart by construction. Knowledge is the fourth,
and it is the first one big enough that where it lives is a decision rather than
a detail: a tax-invoice spec that `deck` can see is a tax-invoice spec that
`deck` will eventually use.

## What we adopt, and what we refuse to invent

| Prior art | What it settles | Verdict |
|---|---|---|
| [Agent Skills / SKILL.md](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/overview) — folder + frontmatter, three disclosure levels, `references/` `scripts/` `assets/` | The unit of knowledge and how it loads | **Adopt verbatim** |
| [AGENTS.md](https://agents.md) — one instruction file at a repo root, read by 20+ tools | Project-level instructions | Already ours: AGENT.md per worker |
| [Cursor `.mdc` rules](https://www.morphllm.com/cursor-rules-best-practices) — four activation modes: always / glob-attached / agent-requested / manual | When a piece of knowledge loads | Adopt **agent-requested** as the default; refuse globs |
| [CrewAI knowledge](https://docs.crewai.com/v1.14.7/en/concepts/knowledge) — agent-level vs crew-level sources, separate storage per agent | That per-agent scope is a real, separately-stored thing | Confirms the split; we do it with folders rather than collections |

**Aetox already speaks this format.** `markdownSkill` in
[discovery.go:16](../../internal/skill/discovery.go:16) is documented as
"opencode/Claude Code style: frontmatter name+description, free-form instruction
body" and `DiscoverSkills(paths)` already takes its scan paths as a parameter.
The format is done. The only thing missing is **where it scans from**, which is
exactly the part that must not be shared.

So this standard invents no file format. A "form" is not a new kind of file; it
is a skill whose body happens to describe a document's required skeleton.

## Layout

```
<DataRoot>/agents/<name>/
  AGENT.md              the craft — paid on every request
  MEMORY.md             what it learned, approved by the user
  skills/
    tax-invoice/
      SKILL.md          name + description (index), then the body
      references/       loaded only when the body points at them
      assets/           templates, sample files
  mcp.json              the servers this worker brings (format: v2 below)
```

Bundled workers ship the identical shape inside the binary, so "copy a shipped
agent and change it" stays a copy rather than a translation — the rule
[profile.go:66](../../internal/subagent/profile.go:66) already states for
AGENT.md, extended to the whole package.

## Three levels, and what each costs

| Level | Loaded | Cost | Content |
|---|---|---|---|
| 1 — index | when the worker starts | ~100 tokens per skill | `name` + `description`, as an entry in that worker's tool block |
| 2 — body | when the worker asks for it | one tool result | SKILL.md body: the skeleton, the required fields, the arithmetic |
| 3 — resources | when the body points at them | nothing until read | `references/`, `assets/`, sample documents |

**The tool block is the index — there is no second mechanism.** A skill
registers into the worker's registry like anything else it holds, so its name
and description are already in front of the model and calling it returns the
body. An earlier draft of this document proposed injecting a separate index
into the prompt; that would have been a second list of the same names, kept in
sync by hand, paid for twice.

Level 1 is the only level anybody pays for without asking, so it is the only one
with a budget: an agent carrying so many skills that its index crowds its own
craft has stopped being a specialist. Warn past a threshold rather than cap
silently.

## Before any of this: what makes the folder an agent at all

This section was added on 8 Aug 2026, after a specialist was built to the letter
of everything below and still could not be reached by anybody. The rule it
states is not new — it is written in **COMPANY.md §4, which is where it is
canonical and which wins if this page ever disagrees**. What was missing is that
the page a builder actually reads on the way to creating one never mentioned it,
so the rule was met by the agents that predate it and by nothing after.

An agent is a specialist colleague, and that means three ways in, always, all
three at once:

1. **The assistant can hand it a job** across the counter, and
2. **it hands a result back**, and
3. **the user can pick it and talk to it directly** from the chat page.

An agent missing any of the three is not a narrower agent. It is a folder nobody
can get to.

**One field decides all three: `desk: specialized`** — the office (§85 · §4).
Three separate readers ask for the office's chairs and only the office's:
`ListChairs` builds the ทีมเอเจน roster, the chat page's picker is the same
call, and `NewChairSession` opens a direct chat there and nowhere else. A
profile that names any other desk still parses, still validates, still shows up
in `List()` — and disappears from the roster, from the picker, and from every
door a user could walk through, while looking entirely correct in its own file.

That is the failure mode worth naming, because it is silent from every angle
except one: `TestBundledProfilesAreUsable` asserts the office, and that
assertion is the only thing standing between a specialist and being invisible.
Do not relax it to make a new agent pass. If an agent seems to need another
desk, the thing being asked for is a change to the dispatch star (COMPANY.md
§3), and that is the owner's decision rather than a line in a profile.

`tools:` is the other field a specialist must not omit. Absent means "everything
this desk carries", which is the opposite of specialist and is paid for on every
request the worker takes.

## Frontmatter

Required, from the standard: `name` (≤64 chars, lowercase letters/digits/hyphen)
and `description` (≤1024 chars). The description is the trigger — it must say
**what this is and when to reach for it**, because it is the only thing the
worker sees before deciding to open the file.

One Aetox-specific optional field was specified here: `always: true`, for the
rare skill that should be in the body of the prompt rather than behind a door.
**It was never built** — checked 2026-08-20, nothing reads the field — so a
skill that sets it is a skill behind the ordinary door, and this paragraph is
kept as a proposal rather than deleted because the reason for it still stands:
be miserly, since an always-on skill taxes every request that worker takes.

Refused for now, each for a reason: `globs` (a chat product has no open file to
match against), `allowed-tools` (the profile's tool ceiling already answers
this, and two answers is the debt this codebase exists to refuse), and a version
field (frontmatter is additive; a version number is a migration nobody has
needed yet) — **the version field is reversed in v2 below**, because
distribution is the migration that needed it.

## Isolation — the part with teeth

1. **Discovery scans the running worker's home and nowhere else.** The skills go
   into the child registry `FilterRegistry` already builds per worker, next to
   the scoped `memory` and `ask_main` that are already rebuilt per child rather
   than inherited.
2. **Names are per-home.** Two workers may each hold a skill called `invoice`
   without conflict, because neither can see the other's.
3. **`~/.aetox/skills` stays the shared shelf.** Anything placed there is
   everyone's, by definition — that is what it is for. Specialist knowledge
   never goes there. This is the tempting wrong answer: it works on the first
   day and quietly makes every worker a generalist by the thirtieth.
4. **No file tool reaches any worker's `skills/`, in any mode — including that
   worker's own.** One check alongside `refuseCredentialStore`, which is the
   single place every file tool already asks. Without it the separation held
   only because nothing pointed across: the folders sit inside the data root,
   which is readable on purpose, so a worker that guessed a sibling's path was
   in. `AGENT.md` and `MEMORY.md` stay readable — explaining the team, and
   writing a new `AGENT.md` when the user asks for a teammate, are things this
   product is built to do.
5. **The main assistant gets none of it.** It knows the office has a document
   writer; it does not know what a tax invoice needs. That is the capability /
   implementation boundary the whole office rests on.

## What a form skill must and must not contain

The split the owner set: **the skeleton follows the standard, the content
follows the intent.**

A form skill carries the skeleton — required fields, their legal source, the
arithmetic, what must never be inferred, and which parts are mandatory versus
merely conventional. It does **not** carry finished sentences. A form that locks
its wording produces one document for every intent, which destroys the reason
there is a writer rather than a template.

Every form skill states its source (the section, the announcement, the date it
was checked) so that when the law moves, the file that has to change can be
found by searching for the citation rather than by reading all of them.

## Rollout — each slice ships alone

| # | Slice | Done when | |
|---|---|---|---|
| 1 | Per-agent skill discovery: scan `AgentHome/skills`, register into the child registry | A skill in `doc`'s home reaches `doc` and provably not `deck`, `sheet` or the main assistant | **done** |
| 2 | Ship bundled skills with bundled agents — the embed pattern was `profiles/agents/*/AGENT.md` and held nothing else | A fresh install has the shipped forms with nothing to download | **done** |
| 3 | Isolation rule 4 — the skills folder is refused to every file tool, in every mode | Reading any worker's `skills/` fails; `AGENT.md` and `MEMORY.md` stay open | **done** |
| 3b | Level 3: a worker reading its own `references/` | A skill body can point at a file and have it read | **done** (`supportingFiles` + `skill_view`) |
| 4 | `doc_write` form primitives: plain layout tables, column widths and alignment, inline bold, and a `lineitems` block whose money is **computed in Go** | An invoice lays out correctly and its VAT is arithmetic, not recall | **done** |
| 5 | The rest of the first five forms | quotation, invoice, receipt, withholding-tax certificate (`tax-invoice` shipped with slice 2) | |
| 6 | `sheet` primitives: formulas, number formats, filter | A total row recalculates when a cell changes | **done** |
| 7 | The office page lists what each worker knows | A card shows its skills; the gear edits them | **done** (`AgentSkills` / `AgentNeeds` / `OpenAgentSkillsFolder` in Settings) |

Slice 1 is the whole standard. Everything after it is content.

Slice 3 landed as a refusal rather than a scoped read, and the difference is
worth recording: rather than teaching the file tools which worker is running —
which the registry has no way to say, since a child shares its parent's tool
instances — the skills folder is simply shut to all of them. A worker's own
knowledge reaches it as tools in its own registry, exactly as `~/.aetox/skills`
is reachable through `skills_list` and not through `read`. Level 3 (a skill body
pointing at `references/`) therefore still needs its own door, which is 3b.

Images and page furniture were dropped from slice 4 rather than done badly. The
`tax-invoice` skill says so on the page it would have used them.

Built in [`skills.go`](../../internal/subagent/skills.go), attached from
[`FilterRegistry`](../../internal/subagent/store.go) — the one place that
already answers "what does this worker hold", so a delegation and a direct chat
cannot end up with different answers. Claims held by
[`skills_test.go`](../../internal/subagent/skills_test.go).

**Rule 4 shipped with slice 3** and this paragraph used to say it had not.
`refuseAgentKnowledge` ([sandbox_open.go:566](../../internal/skill/sandbox_open.go:566))
now shuts every worker's `skills/` to every file tool, so isolation is a refusal
rather than the accident of nothing pointing across homes.

## Settled, and still open

**`skills/`** — settled. It matches the ecosystem, so a SKILL.md written for
another agent runtime drops into a worker's folder unchanged.

Still open:

- **Does a newly-dropped skill need the user's approval before a worker uses
  it?** A skill is instructions, and Anthropic's own guidance is to treat
  installing one like installing software. Dropping a file is the hiring gesture
  this product is built on, so the answer cannot simply be "yes" — but a worker
  silently gaining behaviour from a file nobody reviewed is a real edge.
  **Still open.** v2 puts the answer on the install screen, and a folder copied
  in by hand will still walk past it — deliberately, because that gesture is the
  product, and because a person moving a folder is a person who chose to.
- ~~**Are packages distributable?**~~ **Answered in v2 below: yes.**

---

# v2 — The Package Travels (2026-08-20)

Approved by the owner 2026-08-20. This section answers the last open question
above — *"Are packages distributable?"* — with **yes**, and states what that
costs. Everything in v1 stands; nothing here replaces a rule, and the two
places where it revises one say so out loud.

The occasion is a shop. Agents are to be sold as files and installed by people
who did not write them, which turns three things that were conveniences into
requirements: a package has to be **complete** (installing it must not leave the
buyer editing a global config file by hand), it has to be **honest** (what it
will add to the machine is visible before it is added), and it has to be
**identifiable** (two sellers may both call theirs `doc`).

## What was already true

Worth stating first, because it is most of the work and it is done:

- **An agent already reads the machine's shared shelf.** `mergeShelf`
  ([progressive.go:76](../../internal/skill/progressive.go:76)) takes
  `DefaultDiscoveryPaths()` as the base and lays the worker's own skills in
  front of it, own name winning. The gate is holding `skills_list` / `skill_view`
  in `tools:`, which is exactly "as far as the system opens it".
- **An agent-only MCP server already waits for the agent.** `Deferred`
  ([client.go:52](../../internal/mcp/client.go:52)) keeps it out of the startup
  connect; `RegisterFor` brings it up at first use.
- **`needs:` already declares without granting**, and already folds the unmet
  list into the running agent's own prompt ([needs.go](../../internal/subagent/needs.go)).
- **The zip road already installs whole-or-nothing** through a staging directory
  ([zip_install.go](../../internal/skill/zip_install.go)).

The gap is not the runtime. It is the **package boundary**: what travels in the
folder, and what a stranger's machine does with it on arrival.

## The three laws

1. **The folder is the whole worker.** Anything that is this agent's identity
   travels with it. The v1 form of this rule covered knowledge and the opening
   cards; v2 extends it to the servers it brings.
2. **A package declares; it never grants.** `mcp.json` and `needs:` are
   declarations. `for:` on the server and on the connection remains the only
   thing that grants, and the human at the install screen is the only one who
   writes it. There is no second permission tier and a package cannot open one.
3. **One question has one answer on disk.** `mcp-servers.json` stays the truth
   about which servers exist on this machine. `mcp.json` in a package is an
   **instruction consumed at install**, not a second store. A package that kept
   its own live server list would give Settings, the permission rules and the
   manager a second place to disagree with.

## Layout

```
agents/<name>/
  AGENT.md      identity + tools + needs + package/version/requires-app
  skills/       what this one knows
  STARTERS.md   how it opens a conversation
  mcp.json      the servers it brings          (new in v2)
  MEMORY.md     what it learned — never packed (new in v2)
```

**MEMORY.md never travels.** Not as an option, not behind a checkbox. It is
what this worker learned doing the seller's jobs, which means client names,
document numbers and paths on somebody else's disk; and on the buyer's side it
is a stranger's recollections presented as the agent's own. What is sold is a
capability, not an experience. The exporter drops it.

## Identity: two names, and they are not the same name

Owner's call, 2026-08-20.

- **The folder name is the local id.** It stays a single path segment. It is
  what `task` selects, what `for: agent:<name>` places, what `AgentHome`
  resolves, and what the user types. It belongs to the user: a buyer may rename
  a package on install, and everything keeps working because everything already
  keys on the folder.
- **`publisher:` + `package:` in the frontmatter are the store's id.** They
  identify the goods across machines — for the update check, for "who wrote
  this", for a second copy of the same product. Nothing in the resolver reads
  them; they exist so the shop and the install screen have something stable to
  hold.

This is deliberately the cheap answer. Making the folder itself
`publisher-name` would have bought namespacing at the price of every path, every
`for:` entry and every mention growing a prefix the user did not choose.
A name collision on install is a rename, and a rename is a dialog.

## Frontmatter, v2

Added:

| Field | Meaning |
|---|---|
| `publisher:` | who made it — display text, never a permission |
| `package:` | stable id across machines, e.g. `mike/tax-doc` |
| `version:` | the package's own version |
| `requires-app:` | the lowest Aetox version this is known to work on |

All four are **optional, and belong to a package that travels**. The five
workers that ship inside the binary carry none of them and are not missing
anything: see "What a shipped worker carries" below for why adding them would
be a second place to keep the app's own version.

**This reverses v1's refusal of a version field**, which said "frontmatter is
additive; a version number is a migration nobody has needed yet". That was
right while every agent on a machine was written by the person sitting at it.
It stops being right the moment somebody has paid for one: a build that removes
a tool name — `slides_write` left in `2151be7` — silently breaks goods that were
bought, and the only thing that can tell the buyer *why* is a version the file
carries. The rule the reversal keeps is the one underneath it: an unknown field
is still ignored, never fatal.

The same law `needs:` established covers the rest of it: **a tool name this
build does not know produces a sentence on screen, never a silently empty tool
list.**

## `mcp.json`

The file is an array of server entries in the **same schema as
`mcp-servers.json`** ([config.go:600](../../internal/config/config.go:600)), so
a working server can be lifted from one file to the other by copy-paste. Two
differences, both because a package declares rather than grants:

- **`for:` is not read.** The installer writes `["agent:<name>"]`, which is what
  makes the server agent-only and therefore `Deferred`.
- **A secret is a request, not a value.** An `env` value of `${ask:NAME}` (with
  an optional `${ask:NAME|label}`) is a field on the install screen. Owner's
  call 2026-08-20: what the buyer types is written where MCP env already lives,
  because that is where a hand-edit would have put it — the install screen
  replaces the hand-edit and must not quietly change the security posture in
  either direction. Moving MCP env into the credential vault is a real and
  separate piece of work (the rule is stated from the other side in
  [connections.go](../../internal/config/connections.go)); it is not this one.
  A literal secret committed into a package's `mcp.json` is refused at install:
  a package that ships a working key is shipping the seller's account.

**Collision.** A server name is the tool-name prefix, so two packages that both
bring `github` cannot both have it. The rule:

- same name, same command/url → **reuse it**, and only add `agent:<name>` to its
  `for:`. Buying two agents that need one server installs one server.
- same name, different command/url → the install screen asks, and the answer is
  the buyer's. Never silently overwrite: the existing entry may be the one
  another agent is working from.

**`source:`** is a new field on `MCPServerConfig` recording that an entry
arrived with a package. Removing that agent can then remove exactly what it
brought and nothing the user added themselves. Provenance cannot be inferred
from `for:`, because a user who later places the server on a second agent has
not thereby made it theirs to keep.

## Refused at install, always

- **`desk:`** — stripped. `applyHomeRules` gives every file in the agents home
  the office already; a package that writes any other desk still parses, still
  validates, and vanishes from the roster, the picker and every door a user can
  walk through. It is the one mistake that is silent from every angle.
- **Anything addressed to `~/.aetox/skills`** — a package installs into its own
  home and nowhere else. One bought agent must not be able to make every worker
  on the machine a generalist.
- **Any `for:` the package wrote for itself** — see law 2.
- **Paths that climb out of the package** — already the zip road's rule.

## The two roads

**Export** comes first, and not because it is easier: it is the test of the
standard. Anything that fails to travel is something still coupled to the app.
`ExportAgentPackage(name)` writes a zip of the home minus `MEMORY.md`, with an
`mcp.json` generated from the servers currently placed on that agent, and every
secret put back as `${ask:...}` in place of the value the seller typed.

**Install** extends the zip road rather than opening a second one: an archive
whose root holds `AGENT.md` is an agent package and lands in the agents home;
anything else is still skills. Before writing anything, a **preflight report** —
a pure function over the staged archive — answers what the buyer is about to
agree to: who wrote it, what it will be called locally, which servers it adds or
reuses, which secrets it will ask for, which connections it needs, and which of
its `tools:` this build does not know. The old open question — *does a dropped
skill need approval before a worker uses it* — is what that screen will answer
for anything arriving as a package: a package is software, and this is the
moment it is installed. A folder copied in by hand still walks past it, and is
meant to.

The GitHub road (`plugin_install`) gets the same treatment afterwards, through
the same detector, so there is one answer to "what is in this archive".

## What a shipped worker carries, and what it deliberately does not

Asked on 2026-08-20: should the five workers that ship inside the binary be
brought up to this standard, or should the standard be rewritten to describe
them? Measured before answering, in
[bundled_export_test.go](../../internal/subagent/bundled_export_test.go):

- **All five already pack.** `automation` (5 files), `doc` (4), `github` (7),
  `research` (6), `sheet` (3) each export to a package with `AGENT.md` and
  `STARTERS.md` at the root and every skill they carry beneath it.
- **The whole road works.** `github` was exported, unzipped into a data root
  that had never seen it, **under a different name**, and came back a working
  colleague: on the roster, four skills in the running registry, both languages
  of its opening question, and its two `needs:` correctly reported unmet on a
  machine with nothing connected.

So the structural half of the standard is already true of them, and the answer
to the question is neither of the two it offered. **The shipping label is for
packages that travel, and a bundled worker is not one.** It carries none of
`publisher:`, `package:`, `version:`, `requires-app:`, on purpose:

- `version:` would put the app's version in five more files, bumped by hand
  every release. `internal/version` exists precisely because that number used to
  live in five places and now lives in one, with `version_test.go` failing the
  build over every copy that disagrees. Five bundled AGENT.md files would be
  five new copies with nothing checking them.
- `publisher:` and `package:` answer "where did this come from", and
  `Profile.Builtin` already answers it for these — from the fact of where the
  file is, which cannot drift.
- `requires-app:` cannot be false for something compiled into the app it would
  be checked against.

A worker that leaves this machine gets a label; one that ships with the binary
is identified by shipping with the binary. Both are the same format, and the
label is optional in it — which is what "optional" was always for.

Two real gaps did come out of the measurement, and neither is a standard
mismatch:

- **`sheet` ships with no `skills/` at all** — the only one of the five without
  any. **Parked by the owner on 2026-08-20, deliberately outside this standard**:
  the worker has never been tuned, and tabular work is a subject made almost
  entirely of formulas, so what it needs is its own design pass rather than a
  SKILL.md written in passing to make a table in this document look complete.
  Noted here only so nobody reads the export measurement as saying `sheet` is
  finished. Do not close it from inside the packaging work.
- **`github` and `research` declare `needs: mcp:...` that nothing can satisfy in
  one click.** `ReasonUnplaced` has a button because Aetox can write a `for:`;
  `ReasonMissing` has none, because Aetox cannot install a server the user never
  configured. A bundled `mcp.json` would be exactly the recipe that button
  needs — the package proposes, the user presses, `for:` is still what grants.
  Not built, and worth deciding on its own rather than inside this section.

## Measured — what a dropped folder actually gets, 2026-08-20

Not an estimate. One complete package folder was written into `<DataRoot>/agents/`
and every door was asked, in `TestDropInMeasure`
([dropin_measure_test.go](../../internal/subagent/dropin_measure_test.go)).
Seven of eight doors open on the copy alone, with no restart and nothing
registered:

| Door | Result |
|---|---|
| `Load` sees the folder, desk defaults to the office | works |
| On the ทีมเอเจน roster and in the chat picker | works |
| Counted as an agent by `KindOf` (so `task` can reach it) | works |
| `OwnSkills` finds its `skills/` | works |
| The skill is in the **running** registry, through `FilterRegistry` | works |
| `skills_list` / `skill_view` are there, so the machine's shared shelf is readable beside its own | works |
| `STARTERS.md` reaches the empty chat | works |
| The `mcp.json` it brought | **nothing reads it** |

So the promise "drop the folder in and it works" is true today for everything a
worker *is*, and false for everything a worker *brings*. An `mcp.json` in a
hand-copied folder is inert: the server is not configured, and if the same
package also declares `needs: mcp:<that server>`, the roster correctly marks the
need unmet — which is the honest failure, but it is still a failure the person
who copied the folder has to fix by hand in `mcp-servers.json`. Closing it is
slice 2.

### Two ways a hand-copied folder was wrong without saying so — closed 2026-08-20

Both were measured silent, and both now speak. The owner's constraint was that
the working system must not change, so **neither fix changes any behaviour**:
the same workers run, at the same desks, holding the same tools. All that is
added is a sentence where somebody will read it.

- **A folder that writes its own `desk:`** parses, validates, and is absent from
  the office roster and the chat picker. It now carries `Profile.Notice` saying
  which desk it named and how to undo it. Deliberately **not** `Invalid`, which
  would remove it from `Chairs()` and turn a warning into a deletion; and the
  desk line is left exactly as the user typed it, because rewriting it would be
  the resolver overruling them.
- **A folder naming a tool this build does not have** — `slides_write`, removed
  in `2151be7`, or a typo — runs with what is left. The office roster now draws
  `Chair.Missing`, which is the difference between two lists `ListChairs`
  already computed: what `tools:` asked for, and what the child registry handed
  over. Nothing new is looked up. MCP tools are skipped, because a deferred
  server is legitimately not connected while the page is drawn.

Two causes are folded into one list on purpose: a name this build never had and
a name the office ceiling does not carry are different mistakes, and the
sentence a person needs is the same either way. Held by
`TestDroppedFolderThatIsWrongSaysSo` and `desktop/office_missing_test.go`.

`requires-app:` (slice 4) is still worth building on top of this: the roster can
say *what* is missing, and only a version can say *why*.

## Rollout

| # | Slice | Done when | |
|---|---|---|---|
| 1 | `mcp.json` + v2 frontmatter + `ExportAgentPackage` | An agent with a placed server exports to a zip that carries it, and MEMORY.md is not in it | **done 2026-08-20** |
| 2 | The detector, the preflight report, and install | A package built on one machine installs on another and works without opening any config file | |
| 3 | The install screen and the agents management page | The buyer sees what they are agreeing to, and can later see, place and remove what they bought | |
| 4 | `requires-app:` enforcement + unknown-tool notice | A package built for a newer build says so instead of failing downstream | |
| 5 | Update check against `package:` + publisher signing | A buyer learns there is a new version, and can tell who made the file | |

Not in scope, deliberately: **copy protection**. A package is markdown and JSON,
the licence keeps the user's own extensions theirs, and a lock in the app would
be both breakable and a broken promise. What is sold is provenance and updates.
