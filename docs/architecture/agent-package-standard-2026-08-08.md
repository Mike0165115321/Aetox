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
  mcp.json              (planned) the servers this worker brings
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

One Aetox-specific optional field: `always: true`, for the rare skill that
should be in the body of the prompt rather than behind a door. Be miserly with
it — every always-on skill taxes every request that worker ever takes.

Refused for now, each for a reason: `globs` (a chat product has no open file to
match against), `allowed-tools` (the profile's tool ceiling already answers
this, and two answers is the debt this codebase exists to refuse), and a version
field (frontmatter is additive; a version number is a migration nobody has
needed yet).

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
| 3b | Level 3: a worker reading its own `references/` | A skill body can point at a file and have it read | |
| 4 | `doc_write` form primitives: plain layout tables, column widths and alignment, inline bold, and a `lineitems` block whose money is **computed in Go** | An invoice lays out correctly and its VAT is arithmetic, not recall | **done** |
| 5 | The rest of the first five forms | quotation, invoice, receipt, withholding-tax certificate (`tax-invoice` shipped with slice 2) | |
| 6 | `sheet` primitives: formulas, number formats, filter | A total row recalculates when a cell changes | **done** |
| 7 | The office page lists what each worker knows | A card shows its skills; the gear edits them | |

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

**Rule 4 is not built yet.** Until slice 3, isolation holds because nothing
points across homes — not because a read is refused. That is the honest state of
it, and the reason slice 3 is not last.

## Settled, and still open

**`skills/`** — settled. It matches the ecosystem, so a SKILL.md written for
another agent runtime drops into a worker's folder unchanged.

Still open:

- **Does a newly-dropped skill need the user's approval before a worker uses
  it?** A skill is instructions, and Anthropic's own guidance is to treat
  installing one like installing software. Dropping a file is the hiring gesture
  this product is built on, so the answer cannot simply be "yes" — but a worker
  silently gaining behaviour from a file nobody reviewed is a real edge.
- **Are packages distributable?** If a worker's folder can be zipped and shared,
  the audit question above stops being hypothetical.
