# The First Cut: A `video` Chair That Hires Kinocut, Not an Engine Aetox Builds (2026-08-25)

Plan for giving Aetox the ability to actually cut, join, and re-mux video —
not just read it. Went through four shapes in one night, three reconsidered
by the owner in turn — see
[docs/video-edit-study/README.md](../video-edit-study/README.md) (local only,
gitignored) for the full search trail (รอบ 1 through รอบ 6). This doc is the
plan that came out the other end, not the survey.

**Owner, 25 ส.ค. 2026:** *"มาเขียนเอกสารทำระบบตัดคริปกัน โดยเราจะไม่นั่งงมทำเอง แต่จะศึกษา
Opensource ดูว่าเขาทำยังไง"* — go look first, then draw the plan.

**The arc, same night:**
1. First draft: build a native `video_edit` Go tool wrapping ffmpeg directly.
   Rejected — *"ผมจะให้มันทำงานกับโปรแกรมข้างนอกอ่ะครับ จะได้ไม่ต้องแบก Tool อะไรเยอะ"*.
2. Second draft: drive `melt` (Shotcut/Kdenlive's CLI render engine) instead.
   Free, no Python, single Windows installer — genuinely good, kept as a
   documented fallback (§4.2) — but bare: no templates, no captions, no
   effects. Owner: *"เอาแบบวิดีโอจริงนะแบบพร้อมจริงๆ สเกลเหมือนแคปคัตมีของพร้อมอ่ะ"* — a
   CLI render engine isn't what "ready" means when the comparison is CapCut.
3. Third draft: **OpenChatCut** — free, open-source, CapCut-scale feature
   set, its own MCP server built in. Owner confirmed the *shape* was right —
   *"ติดตั้งแบบนี้ก็ดีไปอีกแบบเพราะหลายๆ Tool ของเราเช่น n8n หรือตัวช่วยอื่นๆก็ทำประมาณนี้"* — but
   its license (AGPL-3.0, §4.5 as it then read) was a real, unresolved flag.
4. **Fourth draft, chosen:** **kinocut** — the guardrailed MCP video server
   from the very first search pass (§video-edit-study รอบ 1), originally set
   aside for being a Python process this doc assumed Aetox would have to
   *bundle*. Under the connect-to-an-external-program architecture §3-§4
   settled on, that objection doesn't apply — a user-installed Python MCP
   server is exactly as external as OpenChatCut or n8n. Owner picked it by
   name for its license: *"Kinocut ⭐ (แนะนำ) ... License Apache 2.0
   (permissive, ใช้เชิงพาณิชย์ได้)"* — clean where OpenChatCut's AGPL needed a
   flag. §4.3-4.6 below is what changed and what didn't.

---

## 1. What exists today, and why it isn't enough

Three tools already touch video, and all three are read-only:

| Tool | Does |
|---|---|
| [`video_ocr`](../../internal/skill/video_ocr.go) | frame-sample + Tesseract → on-screen text |
| [`audio_transcribe`](../../internal/skill/audio_transcribe.go) | ffmpeg → 16kHz WAV → whisper.cpp → transcript |
| [`video_page`](../../internal/skill/video_page.go) | reads captions off a video link |

Not one of them writes a video file. Ask Aetox to cut a clip today and it has
one path left: `shell` calling ffmpeg by hand, guessing flags, on a machine
whose bundled ffmpeg build the model has never seen documentation for. That is
exactly the failure mode kinocut's README names in one sentence worth keeping:
*"AI agents can write FFmpeg commands, but they should not have to guess
flags."*

## 2. Found while checking — the two read tools already share a hidden seam

**Owner, 25 ส.ค. 2026:** *"เราจะดึง TOOL ที่อ่านเสียงและวิดีโอ ที่แยกเฟรมออกมาด้วย ไปเช็คที
เพราะผมจะไม่เอามาปนกับตัวหลักแล้ว"* — before `video_edit` gets written, check whether the
existing read tools are already tangled with each other, because the new write
tool must not become a fourth thread through that tangle.

They are, once — one seam, checked and confirmed by reading both files in
full:

```
video_ocr.go:215   func missingFFmpegError() error { ... }   ← defined here
audio_transcribe.go:170   return missingFFmpegError()        ← called from the other file
```

`missingFFmpegError()` says nothing specific to video — the message is generic
("ไม่พบโปรแกรม ffmpeg ในเครื่อง") — but it happens to live inside `video_ocr.go`,
so `audio_transcribe.go` reaches across a file boundary to use it. Nothing
about that ownership was decided; it is just where the first tool that needed
the message happened to be written. If `video_edit` needed the same message
(it does — every ffmpeg call site needs one), it would have reached into
`video_ocr.go` too, making the new write tool depend on a read tool's file for
no reason connected to what either one does.

`bundledBinary()` (resolves the ffmpeg path itself) is **not** part of this
problem — it already lives in the correct place, [`bundled.go`](../../internal/skill/bundled.go),
a file built specifically to be the shared home for programs `video_ocr` and
`audio_transcribe` both need (its own header comment says so). It is the
model to copy, not a second instance of the seam.

There is no separate "extract frames" tool to find. Frame extraction is a
private step inside `video_ocr.go` (`extractFrames`, line 184), wired directly
to the Tesseract OCR call that follows it — not something the model can invoke
on its own today, and not called from anywhere outside `video_ocr.go`.

**Decision (owner confirmed):** move both `missingFFmpegError()` and
`extractFrames()` out of `video_ocr.go` and into `bundled.go`, next to
`bundledBinary()`/`bundledRoots()`, so `video_ocr` and `audio_transcribe`
call the same neutral home instead of one owning code the other depends on.

**Still worth doing after §4's pivot, on smaller grounds.** The original
reasoning here was written against a plan where a third tool, `video_edit`,
would join these two and inherit the seam — that tool doesn't exist any
more (§4.3-4.5). What survives is the seam itself: two shipped tools with an
undecided, accidental ownership between them, worth the same small,
separately-reviewable code-move regardless of what does or doesn't get
built on top of them later.

## 3. Tool visibility — off the main desk, onto a dedicated agent

**Owner, 25 ส.ค. 2026:** *"ผมว่าจะให้ tool ไม่ส่งไปโต๊ะหลักอ่ะครับ เหมือน TOol อื่นๆไง หรือ
ทำเอเจนเฉพาะ แล้วให้สิทธ์แค่เอเจนนั้นเข้าถึง tool นั้น มาตรฐานเดียวกับออโตเมชั่นไงครับ"* — §2's
seam wasn't the real concern; visibility was. `video_ocr`/`audio_transcribe`,
and whatever editing capability §4 eventually settles on, should not sit in
the default desk's own tool block at all — reachable only by hiring a
specialist, the same standard automation connections already get. (Written
before §4's pivot away from a native `video_edit` tool — the mechanism below
turned out to fit an MCP-connected chair just as well as a tool-carrying
one; nothing here needed to change when §4 did.)

### 3.1 The mechanism already exists, unused for media

Checked `internal/mode` (ARCHITECTURE.md §83) and `internal/subagent` in full:

- Each desk (`assistant.md`, `coding.md`, `specialized.md`) declares
  `categories:` — the capability groups (`internal/skill/category.go`) it
  carries directly. **`assistant.md` currently reads
  `categories: agent, web, media, files, shell`** — `media` is right there,
  which is why `video_ocr`/`audio_transcribe` sit in the main desk's tool
  block today.
- A tool a desk doesn't carry directly can still be *in the room for its
  agents* — `specialized.md`'s `chairs:` is exactly that list (it named those
  nine tools when this was written and names three category words since §212:
  `files, shell, deliverables`), and it's the reason the main
  assistant can never call `doc_write` itself but can hire the `doc` agent to
  do it (owner's call, 2026-08-06, recorded in `mode.go`'s own comment: *"the
  assistant must not carry the document, workbook and deck writers — that is
  why the agents exist"*).
- `assistant.md` already has `dispatch: specialized` — the hiring door to the
  desk that holds those chairs is already open. Removing `media` from
  `assistant.md`'s own `categories:` costs nothing on the delegation side; it
  only removes the tools from the *main assistant's own* schema.
- Every existing chair (`doc`, `sheet`, `github`, `research`, `automation` —
  all five, checked in full) already lists `video_ocr, audio_transcribe` in
  its own `tools:` line, as part of a shared "can sense any material handed
  to me" kit alongside `pdf_read`/`image_ocr`. That's
  `subagent.Profile.Tools` — a second, independent allowlist checked in
  `FilterRegistry` ([store.go:226](../../internal/subagent/store.go:226)),
  layered on top of the desk's own ceiling, not instead of it.

So the pieces already stack: `Mode.Categories` (what the desk's own assistant
carries) → `Mode.Chairs` (what the desk keeps in the room without carrying it
itself) → `Profile.Tools` (which agent in that room actually gets it). Three
layers, all already built for `doc_write`, none of them currently used to keep
media off the main desk.

### 3.2 Decision (owner confirmed) — a dedicated video agent, not the shared kit

Two ways to place this inside that machinery, both raised and one chosen:

- Add `video_ocr`/`audio_transcribe` (and whatever editing capability §4
  settles on) to the same `tools:` line the five existing chairs already
  carry media-reading on — cheapest, but keeps video capability spread
  across five files that don't otherwise touch it.
- **A dedicated agent**
  (`internal/subagent/profiles/agents/video/AGENT.md`, same shape as
  `profiles/agents/doc/`) that holds `video_ocr`, `audio_transcribe`, and
  the editing capability **exclusively** — and the five existing agents
  (`doc`, `sheet`, `github`, `research`, `automation`) lose
  `video_ocr`/`audio_transcribe` from their own `tools:` lines, so video
  capability has exactly one owner instead of five duplicate copies of it.

Chosen: **the dedicated agent.** This is precisely "มาตรฐานเดียวกับออโตเมชั่น" —
automation capability already lives behind one `automation` agent that
declares `needs: connection:n8n | connection:windmill` rather than being
spread across every chair as a maybe-useful extra; video capability reads the
same way from today.

### 3.3 What actually changes (documented here, not yet coded)

1. **`internal/mode/modes/assistant.md`** — drop `media` from `categories:`.
   New line: `categories: agent, web, files, shell`. The default desk stops
   carrying `video_ocr`, `audio_transcribe`, `image_ocr`, and `pdf_read` in
   its own tool block.
2. **New `internal/subagent/profiles/agents/video/AGENT.md`** — a sixth chair
   at the `specialized` desk (no `desk:` line needed — `applyHomeRules`
   defaults an agent-home file with none to `mode.Office`, same as the other
   five). `tools: video_ocr, audio_transcribe` plus the same baseline
   file/shell/web kit every other chair carries (`read, write, edit, edits,
   delete, grep, list, glob, shell, git, desk_terminal, desk_open,
   desk_list, browser, web_fetch, web_search, memory, skills_list,
   skill_view`) plus `needs: mcp:openchatcut` once §4 is wired up.
3. **`doc`, `sheet`, `github`, `research`, `automation` `AGENT.md` files** —
   remove `video_ocr, audio_transcribe` from each `tools:` line. `pdf_read`/
   `image_ocr` stay (still genuinely general-purpose, unrelated to this
   change) unless a later pass decides those belong single-owner too — not
   this decision.
4. **`specialized.md`** — left alone for now. It carries `media` in its own
   `categories:`, so a session opened directly at the specialized desk still
   senses media itself without hiring the new chair. Whether the Office
   desk's own assistant should also lose direct media access is a separate
   question the owner didn't raise here — left open in §7.

None of this is `internal/skill` code — it's five markdown frontmatter edits
and one new markdown file, the same weight as hiring any other chair. Still
true to tonight's rule: documented, not written.

## 4. Decision — hire an external program over MCP; write no edit engine at all

### 4.1 First candidate, rejected: build native

Every serious *MCP* wrapper found in the first search pass was a Python/
FastMCP process or a headless-browser stack, so writing Aetox's own trim/
concat/mux logic in Go looked like the only local, dependency-light path.
Owner rejected the framing itself: the question was never "MCP server or
native Go", it was "does an external program already do this", and the
first search never asked that because every query had "MCP" or "AI editor"
baked into it.

### 4.2 Second candidate, kept as a documented fallback: `melt`

**`melt`** — the command-line renderer of the **MLT Framework**, the engine
underneath both **Shotcut** and **Kdenlive**. Free, single Windows binary
(`C:\Program Files\Shotcut\melt.exe`, ships inside the ordinary Shotcut
installer), no Python, genuinely built for this — its own docs say
*"integrating editing workflows into agents"*
([mltframework.org/docs/melt](https://www.mltframework.org/docs/melt/)).

Still real and still worth having documented: builds a timeline straight
from CLI arguments or a `.mlt` XML file, renders through its own `avformat`
consumer with its own ffmpeg encoders (§5). What it lacks is everything
"CapCut-scale" means — no templates, no auto-captions, no built-in effects
library, no transitions beyond `-mix`. A real editing *engine*, not a
finished *product*. §4.3 is the product; §7 keeps `melt` as the answer for
"I don't want to install a whole editor app," not the primary path.

### 4.3 Chosen: **kinocut**, connected the same way n8n already is

**[kinocut](https://kinocut.dev)** (`KyaniteLabs/kinocut`, formerly
`mcp-video`) — the guardrailed MCP video server from the first search pass
(video-edit-study รอบ 1), rejected there for being a Python process this
plan then assumed Aetox would bundle. §3-§4's external-connection
architecture removes that objection entirely (§4.4). Owner picked it back
up by name:

- **License: Apache-2.0** — permissive, commercial use explicitly allowed,
  no network-copyleft clause to reason about. This is the concrete
  improvement over OpenChatCut's AGPL-3.0 (§4.5's flag): nothing here needs
  a "probably fine, but check with someone qualified" caveat.
- **196 MCP tools / 167 CLI subcommands**, organized in categories: core
  editing (trim, merge, resize, crop, rotate, overlays, subtitles),
  workflow engine (validate → plan → render with receipts), AI-assisted
  media (transcription, scene detection, upscaling, stem separation),
  Hyperframes (procedural motion design, background removal, local TTS),
  repurposing (one clip → platform-specific vertical/horizontal/square
  variants with manifests), procedural audio, still/plate editing, visual
  effects, analysis (thumbnails, storyboards, quality comparison).
- **Local-first, no account** — `pip install kinocut` or via the MCP
  registry (`io.github.KyaniteLabs/kinocut`), runs entirely on the user's
  machine, requires ffmpeg on PATH (the user's own system ffmpeg, not
  Aetox's pinned LGPL copy — §5).
- **MCP transport: stdio**, not OpenChatCut's HTTP endpoint — `kino --mcp`
  registers all 196 tools over stdin/stdout, matching the *other* half of
  what `internal/mcp/client.go` already supports (`CommandTransport`,
  client.go:200) — the same shape as any `npx`/`uvx`-launched MCP server a
  user adds today. Also unlike OpenChatCut, kinocut's guardrails (preflight
  validation, dry-run `video_intent`, "video receipts" with per-step hashes
  for lineage) are built for exactly the caller Aetox is: an agent that
  can't watch the video it's editing and needs the tool layer to catch a
  bad cut before it renders, not a human who can just look at a timeline.
- **What it doesn't have that OpenChatCut does:** no GUI, no timeline a
  human can open and manually fix, no motion-graphics template library, no
  auto-caption styling UI. It's a tool surface for an agent, not a consumer
  product a person also drives — CapCut-scale *breadth of capability*, not
  CapCut-scale *polish*. Worth being explicit about, since §4's own trigger
  last time was "พร้อมจริงๆ สเกลเหมือนแคปคัต" — kinocut answers that on tool
  count and guardrail maturity, OpenChatCut answered it on product finish;
  the owner's license concern settled which one wins for now (§4.5 keeps
  OpenChatCut as a documented alternative, not a dropped one).

### 4.4 Why this is the right shape for Aetox specifically

The same reasoning that made OpenChatCut/n8n acceptable makes kinocut
acceptable too, and resolves this doc's own earlier objection to it:
**"Aetox has never bundled Python" was true and stays true — it was never
actually about whether a *user's own separately-installed program* happens
to be written in Python.** That confusion is what round 1 of the study made
and round 6 caught: a Python MCP server the user installs and runs
themselves is exactly as external as an n8n instance the user installs and
runs themselves. Nothing about Aetox's own binary changes either way.

Checked against `internal/mcp/client.go`: *"Two transports: local/stdio (a
subprocess speaking MCP over stdin/stdout, e.g. npx/uvx-based servers) and
remote streamable HTTP"* (client.go:3-4) — kinocut's `kino --mcp` is a
`Command`, which selects `CommandTransport` at connect time (client.go:200).
**Nothing in `internal/mcp` needs to change**, same as the OpenChatCut plan
concluded — a user adds `kino --mcp` as a command-based MCP server in
Settings exactly the way they'd add any other stdio server, and
`needs: mcp:kinocut` on the `video` chair (§3) gates it to that one agent,
unchanged from how n8n/windmill are gated on the `automation` chair.

This answers "ไม่ต้องแบก Tool อะไรเยอะ" completely: Aetox writes **zero**
editing logic. No `op` enum, no trim/concat/mux code, no encoder decisions,
no guardrail/receipt system of its own — kinocut already has one, and §7 of
this doc had explicitly decided *not* to build one. The 196 skills arrive
live from the connected server, the same way any MCP server's tools do —
nothing about them is compiled into Aetox or counted against
`desktop/tool_budget_test.go`.

### 4.5 OpenChatCut, kept as a documented alternative

Not dropped — demoted to the same shelf `melt` sits on (§4.2), for the same
reason: still real, still worth revisiting if the product-polish gap in
§4.3's last bullet ever matters more than it does today. If a user wants a
timeline they can also open and drive by hand, or the template/caption UI
specifically, OpenChatCut over MCP is exactly the same connection shape as
kinocut — swapping which one a `needs: mcp:` entry names, not a different
architecture. Its AGPL-3.0 flag from the original §4.5 still applies if it's
ever picked up again.

### 4.6 What's flagged, not resolved

- **196 tool schemas on one chair's context.** Not a compiled-tool-block
  problem (§4.4) — those never touch the default desk, and the whole point
  of §3 was keeping them off it — but every one of those 196 schemas still
  lands in the `video` chair's *own* delegate context once connected,
  against OpenChatCut's 26. kinocut's own docs mention a `search_tools()`
  discovery call for narrowing this from the *client* side; whether
  `internal/mcp`'s `Tools()` (client.go:234, a flat enumeration of
  everything the server reports, no pagination or filtering) can make use
  of that, or whether the `video` chair simply carries a much larger tool
  list than any other chair does today, wasn't checked. Real, and worth
  measuring before this ships — this doc's own §6's earlier "26 tools" note
  is now stale and this is why.
- **User needs Python + `pip`/`pipx` already set up.** Not a new category of
  requirement — any stdio MCP server the user points Aetox at needs
  *something* runnable on PATH — but worth being plain that this one is a
  Python install, unlike `melt`'s single Windows binary, when writing the
  install instructions §6 still owes.

## 5. The LGPL constraint — moot on the editing path, unchanged on the read path

Aetox's bundled ffmpeg is deliberately the **LGPL build, not GPL**
([capability.go:165](../../internal/capability/capability.go:165), comment:
*"the safer build to put on someone else's machine, and nothing here needs
the GPL-only encoders"*), and ships without `libx264`/`libx265`.

That still constrains `video_ocr` and `audio_transcribe` — both call Aetox's
own pinned ffmpeg directly for frame extraction and audio stripping, neither
of which needs an encoder anyway. It constrains nothing about editing:
`melt` (§4.2) carries its own ffmpeg build via Shotcut, OpenChatCut (§4.5)
never touches Aetox's ffmpeg at all, and kinocut (§4.3, chosen) uses
whatever ffmpeg is on the *user's* PATH — a separate install from Aetox's
pinned copy, so whatever encoders that build happens to carry (GPL or not)
is between the user and their own ffmpeg, not something this decision
constrains either way. Kept here rather than deleted because it's exactly
the trap the first draft of this doc would have walked into; worth a
sentence saying so on every path, not a silent removal.

**Corrected 2026-08-30 — see §10.7.** The last clause above is measurably
wrong. kinocut writes `-c:v libx264` into 38 call sites, so the user's own
ffmpeg is not free to be any build: it has to be one carrying libx264. That
also closes the shortcut this section left open, of handing kinocut Aetox's
pinned copy to save the user an install.

## 6. Scope — what Aetox actually builds

Almost nothing, and that's the point of §4.3-4.4, not a gap in this plan:

1. **`internal/mode` / `internal/subagent` changes from §3**, unchanged by
   this revision — the `video` chair, `assistant.md` losing `media`, the
   five existing chairs losing `video_ocr`/`audio_transcribe`. Still the
   foundation everything else sits on.
2. **The `video` chair's `needs:`** gains `mcp:kinocut` — the same field
   shape `automation`'s `needs: connection:n8n | connection:windmill`
   already uses (`needs.go`). No new Go code: `mcpState()` (needs.go:284)
   already answers "is a server named this configured, enabled, and pointed
   at this agent" for any MCP server name, kinocut included the moment a
   user adds a command-based entry (`kino --mcp`) in Settings.
3. **`video_ocr`/`audio_transcribe` stay on the `video` chair's `tools:`**
   as the fallback for a user who hasn't installed kinocut — reading a
   video is still useful without it, same as today.
4. **Documentation, not code**: how a user installs kinocut
   (`pip install kinocut` or the MCP registry entry
   `io.github.KyaniteLabs/kinocut`) and adds it as a command-based MCP
   server (`kino --mcp`) in Settings. This is the one piece of actual
   *work* left in this plan, and it's writing, not Go.

No `video_edit` tool. No `op` enum. No ffmpeg/melt invocation code, no
guardrail or receipt system of Aetox's own. The `internal/skill` package
gains nothing from this decision — only §2's prep fix (moving
`missingFFmpegError`/`extractFrames` into `bundled.go`) touches it at all,
and that was already true before any of §4's candidates entered the
picture.

## 7. What this plan explicitly does not attempt

- **No fork or vendoring of kinocut.** It stays the user's own
  separately-installed program, exactly like n8n — Aetox never bundles it,
  never pins a version, never patches it, never ships Python to make it
  work.
- **No new visibility/spawn logic beyond what `internal/mcp` already has.**
  kinocut connects over the *stdio* transport (§4.3), and
  `CommandTransport` already spawns and manages that subprocess on demand
  (client.go:200) — unlike §4.5's OpenChatCut, there's no separate "is the
  app running" question to solve here; connecting *is* launching it. See
  §8 for what that leaves genuinely open.
- **`melt` and OpenChatCut stay documented alternatives, not built ones.**
  §4.2 and §4.5 are both real and could become a second or third connector
  later, but nothing in this plan schedules building either — they're
  paragraphs in this doc and the study doc, not line items.

## 8. Open questions before anything ships

1. **196 tool schemas on one chair's context** (§4.6) — the real open cost
   question this revision leaves, replacing the old "is it running" concern
   §4.3's transport choice already answers. Whether `internal/mcp`'s
   `Tools()` can narrow this via kinocut's own `search_tools()` discovery,
   or whether the `video` chair simply carries a much bigger tool list than
   any other chair today, needs measuring before this is tried for real.
2. **Does the `specialized` desk's own assistant keep direct media access?**
   §3.3 point 4 leaves `specialized.md`'s `categories: media, web, agent`
   untouched, so a session opened at that desk still senses video/audio
   itself without hiring the `video` chair — only `assistant.md` was named
   in the owner's original instruction. Worth a deliberate yes/no.
3. **New agent's description/icon** — the five existing agents each have a
   one-line Thai `description:` and an `icon:` (`fileText`, `chartColumn`,
   `zap`, `gitBranch`, `search`) from the app's existing icon set. `video`
   needs both, checked against the real icon set before the file is
   written, not guessed here.

## 9. Relation to other decisions

Still not the agent-team system
([agent-team-standard-2026-08-22.md](agent-team-standard-2026-08-22.md)) —
§3's new `video` agent is a **chair**, the mechanism `doc`/`sheet`/`github`/
`research`/`automation` already use, not a **team** in that document's sense
(its §12 is still about multiple agents that talk to each other and decide
things jointly; a chair is one specialist hired for one job, already shipped
machinery, no dispatch star involved).

COMPANY.md §7 item 5 — *"ขยายเอเจน / โต๊ะเฉพาะทางใหม่ (วิดีโอ ฯลฯ) — เพิ่มไฟล์ ไม่แตะกติกา"* —
names exactly this. A new chair is "เพิ่มไฟล์" (adding a file) at an existing
desk, not a new desk and not new rules, so §3 doesn't jump that queue — it
*is* that queue item.

The bigger shift from where this doc started: `video_edit` as a *tool
Aetox writes* is gone entirely. What remains is `video` as a *chair that
hires an external program over MCP*, which is a smaller, cheaper, and — per
§4.4's owner quote — more familiar shape than anything the first two drafts
proposed.

---

## 10. What actually shipped (2026-08-30), and where it differs from the plan above

Everything from §6 is built. Five things came out different, and each one is a
correction to a line above rather than an addition to it — read this section as
the current state and the earlier ones as how it was reasoned.

### 10.1 Two agents, not one

§3.2 chose a single `video` chair holding both reading and editing. The owner
split it on 30 ส.ค.: **`video` makes a video that does not exist yet, `editor`
cuts one that does.** They are separate packages under
`internal/subagent/profiles/agents/`, both `needs: mcp:kinocut`, and the roster
is eleven profiles now rather than nine.

The split is not a filing preference. The two jobs share a connected program and
nothing else: one starts from material and decides what to lose, the other
starts from nothing and decides what to put on screen. A single agent holding
both would have carried a prompt that spends half its length saying which half
of itself applies.

The cost §4.6 flagged is now doubled and still unmeasured: 196 tool schemas land
in **two** agents' contexts rather than one. Neither carries them until kinocut
is connected, so nothing is paid on a machine that has not set it up — but the
measurement §8.1 asks for is still owed, and is now the first thing to do when
somebody has kinocut installed to measure against.

### 10.2 A room, งานวิดีโอ, which the automation rule had to be re-read for

The owner asked for a room in the nav whose inside offers the choice between the
two. On the same day, ระบบออโตเมชั่น was removed from the nav with the rule that
an agent does not earn a row for being important.

Both stand, and the comment in [`desks.ts`](../../desktop/frontend/src/lib/desks.ts)
now says why: the automation button called `newChairSession('automation')`, the
identical line the roster's own button calls, so it was a shortcut wearing a
room's clothes. งานวิดีโอ opens no chat. It asks the question the work starts
with and routes to one of two agents, which a roster card cannot do — it can
only print two names and leave the reader to work out which is theirs.

The test for the next one is written down there too: not "is this important" but
"does walking in decide something".

### 10.3 `assistant.md` keeps `image_ocr` and `pdf_read`

§3.3 point 1 said to drop `media` from the desk's `categories:`, which would
have taken `image_ocr` and `pdf_read` with it — the main assistant would have
stopped being able to read a screenshot or a PDF. That is not what the
instruction it came from asked for (*"เราจะดึง TOOL ที่อ่านเสียงและวิดีโอ"* named
audio and video).

What shipped: `categories:` loses `media`, and `tools: image_ocr, pdf_read`
grants those two back by name. `Mode.AllowsTool` checks `Tools` before
`Categories`, and `Carries` keeps a packed tool when any of its actions is
allowed, so `media_read` stays on the desk offering `image` alone. Video and
audio left; nothing else did.

### 10.4 `doc` keeps `audio_transcribe`

§3.3 point 3 said all five existing chairs lose both tools. Four of them did.
`doc` kept `audio_transcribe`, because its own AGENT.md names it and it ships a
starter card in both languages for writing meeting minutes out of a recording —
removing the tool would have left a shipped card promising work the agent could
no longer do, and an agent cannot hire another agent to get it back.

All five lost `video_ocr`, and `doc`'s prose was updated to stop naming it.

### 10.5 Only `missingFFmpegError` moved

§2 decided to move both `missingFFmpegError()` and `extractFrames()` into
`bundled.go`. Only the first did.

The seam §2 identified is `audio_transcribe.go` calling a function that lives in
`video_ocr.go`, and that is what moved. `extractFrames` has one caller inside
its own file and takes its frame cap from `videoOCRMaxFrames`, a video-OCR
constant: moving it would put frame sampling for OCR inside a file whose subject
is resolving the path of a bundled program, to fix an ownership problem it does
not have. §2's own later paragraph already said what survived the §4 pivot was
"the seam itself", and the seam is the error message.

### 10.6 Where the templates went

`docs/video-edit-study/` is gitignored, so the 46 scenes the study gathered
shipped nowhere until they were copied to
`profiles/agents/video/skills/video-templates/` — an agent-local skill, embedded
with the profile, `CREDITS.md` travelling with it.

Agent-local rather than a shared `aetox-video-templates` on the global shelf,
because the library is this agent's inventory and no other desk renders a scene.
Two facts about the files that the SKILL.md records and nothing else would have:
the motion is CSS `@keyframes` (**not** GSAP, whatever a general knowledge of
"HTML video" suggests), and the frame size differs per file — 15 scenes pinned to
1920×1080, one (`bold-portrait-title`) built at 1080×1920, six fluid. That last
column is what a request for a vertical cut actually costs, per template, and the
SKILL.md prints it beside every name rather than leaving it to be guessed.

The user-facing setup instructions §6 point 4 owed are
[docs/VIDEO-EDITOR.md](../VIDEO-EDITOR.md).

### 10.7 Installed and measured (2026-08-30) — §8's open questions, answered

kinocut 1.15.0 was installed from PyPI on the owner's machine (Python 3.13.14,
Windows 11) and probed. Four numbers came out of it, and three of them changed a
decision.

**196 tools, ~37,600 tokens, no pagination.** §4.6 and §8.1 flagged this as the
real open cost and it is worse than the guess: the schemas total 150,487
characters, `tools/list` returns them in one page with a null cursor, and every
one lands in the context of any agent holding the server, on every request. The
whole fresh-install tool block is ~10,100 tokens
([tool_budget_test.go](../../desktop/tool_budget_test.go)) — this is over three
times that, on one agent.

`config.MCPServerConfig.Tools` already existed as a per-server allowlist and is
what answers it: the shelf preset ships **54 tools, ~12,400 tokens**, chosen as
the two agents' actual job. `search_tools` is deliberately in the list so the
trim is not silent — the agent can find what was left out and say so, and the
field is one edit away in Settings for anyone who wants all 196.

**kinocut hardcodes `libx264` in 38 places.** This closes a shortcut §5 left
open: Aetox already ships an ffmpeg with ffprobe, and handing kinocut its path
would have removed a whole install step for free. It cannot, because that copy
is the LGPL build and has no libx264. The user needs their own full build, and
the install step points at one that has it.

**`kino doctor` answers readiness in JSON**, so nothing in Aetox keeps a list of
kinocut's dependencies. `VideoToolingStatus`
([desktop/videotooling.go](../../desktop/videotooling.go)) runs
`kino --format json doctor` and reports what it says. The version of kinocut
that grows a dependency reports it without Aetox being edited.

**Hyperframes is a separate Node package**, not part of `pip install kinocut`.
The doctor lists `hyperframes` and `@hyperframes/core` as optional-missing on a
machine that has kinocut and Node. Cutting works without it; rendering an HTML
scene does not. §10.6's SKILL.md is written against Hyperframes because that is
what the templates declare, and this is the install step that makes it real.

### 10.8 The install button was built wrong, and is out

First attempt: a button that opened a terminal on the desk and typed
`python -m pip install kinocut` into it. Owner, immediately: *"ปุ่มติดตั้งทำทำเหี้ยไร
แบบนี้ทำไมไม่ดูมาตรฐานเดิมของเรา"*.

Correct. Aetox has one way to install a program it needs and it is
[`internal/capability`](../../internal/capability/capability.go): a pinned,
SHA256-checked archive fetched into `DataRoot`, a progress strip, no elevation,
no package manager, no PATH. It exists because the alternative got the NSIS
installer classified as `Program:Win32/Wacapew.C!ml` (§capability-install-2026-08-21),
and a second mechanism beside it is the whole shape of that mistake returning.

The terminal button is removed. What is left in
[desktop/videotooling.go](../../desktop/videotooling.go) is the part that was
never in dispute: reading `kino --format json doctor` and reporting it. The room
shows the command and links to the instructions rather than pressing anything.

### 10.9 The decision this leaves open

**kinocut cannot go through `internal/capability` as it stands.** It is a Python
package: PyPI only, no standalone build, its v1.15.0 GitHub release carries no
binary asset at all (checked 30 ส.ค.). The manifest ships four components and
none of them is a Python.

Two ways out, and they are opposite:

**A — bring it inside the standard.** `.github/workflows/tools.yml` already does
exactly this shape for Tesseract: take a third party's pinned bytes, unpack them
once in a script anyone can read, republish as `tools-<name>-<version>`, print
the manifest snippet to paste into `capability.go`. The kinocut version is a job
that takes python.org's embeddable Windows zip plus `pip install --target` at a
pinned version, and ships one archive of ~30-40MB. The room's button then calls
`InstallCapabilities` like every other tool, and the MCP entry points at
`<DataRoot>/tools/kinocut/python.exe -m kinocut --mcp`.

This reverses §7's *"never ships Python to make it work"* — which was written on
25 ส.ค. under the premise that the user installs the editor themselves, and that
premise is what changed.

**B — leave it outside, and say so.** No button. The room shows the command and
the instructions, kinocut stays the user's own program exactly as n8n is, and
§7 stands unedited. Costs the user a real install step on a product being
shipped to people who did not read this document.

Not decided. The code today is B, because B is what removing the wrong thing
leaves behind, not because it won.

**Decided 1 ก.ย. 2569: A won — see §13.**

## §12 เราถือฟอร์กของ Hyperframes เอง (31 ส.ค. 2569)

> วิธีดูแลฟอร์ก ลงแพตช์ ดึงต้นทาง บิ้ว และทางกลับ อยู่ที่
> [../HYPERFRAMES-FORK.md](../HYPERFRAMES-FORK.md) ข้อนี้เก็บเฉพาะว่าทำไม

ตัดสินแล้ว ทำแล้ว ฟอร์กอยู่ที่ `Mikedev115/hyperframes` สาขา `aetox` ตัดจาก
`v0.8.20` ของอัปสตรีม ใบอนุญาต Apache-2.0 เหมือนเดิม และ job `hyperframes` ใน
`tools.yml` บิ้วจากฟอร์กนั้นแทนการ `npm install`

**เหตุผลไม่ใช่ "อยากปรับแต่ง" ลอย ๆ** วันเดียวกันนั้นเจอข้อบกพร่องสามข้อที่ทางแก้
จริงอยู่ข้างในเอนจิน ไม่ใช่ข้างนอก และสองข้อแรกผมแก้อ้อมไปแล้วในโค้ดเรา

1. **ฟอนต์** `fetchGoogleFont` ยิงหา `fonts.googleapis.com` ทุกครั้งที่เรนเดอร์
   ขอ stylesheet ที่ซับเซ็ตตามตัวอักษรบนหน้านั้น แล้วแคชเฉพาะไฟล์ฟอนต์ที่
   stylesheet ชี้ ไม่มีทางอ่านจากเครื่องก่อนเลย แปลว่าวางไฟล์ฟอนต์ไว้ล่วงหน้า
   เท่าไรก็ไม่ช่วย นี่คือข้อที่เหลืออยู่ข้อเดียวที่ทำให้เรนเดอร์ออฟไลน์ไม่ได้
2. **fast capture** เปิดตัวเองเมื่อโพรบเจอ GPU ฉากเดียวกันจึงเรนเดอร์ผ่านบน
   เครื่องหนึ่งและตายบนอีกเครื่อง (`HF_DE_COMPOSITION_ROOT_MISSING`) เราพินปิดไว้
   ที่ `hyperframesEnvironment` ควรเป็นสวิตช์ ไม่ใช่การเดา
3. **exit code ของ `check`** "เจอปัญหา" กับ "รันไม่ได้" ออกมาเป็นรหัสเดียวกัน
   `desktop/video_tool.go` ต้องแยกเองด้วยการดู `exec.ExitError` กับว่ามีข้อความ
   ออกมาไหม

**ฟอร์กไม่ใช่การฝัง** ก้อนที่ลงเครื่องยัง 374MB เพราะแบก Node กับ dependency tree
ทั้งต้น การเป็นเจ้าของซอร์สไม่ได้ทำให้เล็กลงสักไบต์ มันยังถูกโหลดตอนคนกดปุ่มใน
ห้องงานวิดีโอ ไม่ได้ไปอยู่ในตัวติดตั้ง ข้อ §5 เรื่อง *"ไม่ฝังอะไรเข้าไปในระบบ"*
ยังยืนอยู่ทั้งข้อ

**กฎที่ตั้งไว้กับฟอร์กนี้: diff ต้องเล็ก** สามแพตช์ข้างบนคือสิ่งที่ตั้งใจจะมี ไม่ใช่
จุดเริ่มของการเขียนใหม่ ถ้าวันหนึ่งอัปสตรีมรับข้อไหนไป แพตช์นั้นควรหายไปจากฟอร์ก
และถ้าฟอร์กเริ่มแพงกว่าที่แพตช์คุ้ม ทางกลับคือส่ง `hyperframes_repo` เป็น
`heygen-com/hyperframes` ในการดิสแพตช์เดียว job เดิมรับได้อยู่แล้ว

## §13 ทาง A ชนะ และปุ่มเดียวทำครบทั้งสาย (1 ก.ย. 2569)

**เจ้าของ:** *"เราจะทำให้ Aetox ตัดวิดีโอได้ครับตอนนี้ ... ทำยังไงให้มันติดตั้งง่ายด้วย
อย่าลืมเราจะปล่อยให้คนอื่นใช้ด้วยนะครับ"* — สองครึ่งของประโยคเดียว: ปล่อยของจริง
และอย่าให้คนอื่นต้องอ่านเอกสารนี้ถึงจะใช้ได้

**ครึ่งแรก: §10.9 จบที่ A** job `kinocut` ใน tools.yml ถูกรันครั้งแรก รีลีส
`tools-kinocut-1.15.0` ขึ้น GitHub (28.8MB) และแฮชถูกวางใน capability.go
ความสามารถ video-edit จึงถูกเสนอบนการ์ดจริงเป็นครั้งแรก ประโยค *"never ships
Python to make it work"* ใน §7 ถูกกลับตามเหตุผลที่ §10.9-A เขียนรอไว้แล้ว

**ครึ่งหลัง: การเชื่อมต่อเลิกเป็นขั้นของผู้ใช้** ก่อนหน้านี้ กดติดตั้งในห้องงานวิดีโอแล้ว
การ์ดยังถูกม่านคลุม เพราะ `needs: mcp:kinocut` ต้องการรายการใน mcp-servers.json
ที่มีแต่ชั้นวางใน ตั้งค่า จะเขียนให้ ผู้ใช้ใหม่ไม่มีทางรู้ว่าครึ่งหลังอยู่ห้องไหน สามอย่างแก้มัน:

1. `connectVideoEditor` (desktop/videotooling.go) — ดาวน์โหลด kinocut ลงเสร็จ
   เมื่อไร รายการ MCP พร้อม allowlist 54 ตัวและการชี้ไป `agent:editor` ถูกเขียนให้
   ในจังหวะเดียวกัน idempotent และไม่ทับของเดิม: entry ที่ผู้ใช้แต่งเองอยู่ครบทุกช่อง
   อย่างมากแค่เติมการชี้ กติกาบนชั้นวาง ("a button, not a default") ไม่ถูกข้าม
   เพราะกติกานั้นกันการผูกเครื่องเข้ากับ endpoint ภายนอกที่ไม่มีใครเลือก ส่วนนี่คือ
   โปรแกรมในเครื่องที่เพิ่งถูกโหลดเพราะผู้ใช้กดปุ่มบนการ์ดของเอเจนที่ต้องใช้มัน
   การกดคือคำยินยอม สิ่งที่ถูกตัดคือการจับคู่เอง ไม่ใช่การถาม
2. allowlist 54 ตัวย้ายไปมีสำเนาเดียวที่ Go (`videoEditorTools`) ชั้นวางใน
   Settings.svelte อ่านผ่าน `VideoEditorTools()` แบบเดียวกับที่อ่าน command และ
   environment อยู่แล้ว รายการที่วัดต้นทุนมาแล้ว (§10.7) เลิกมีสองบ้าน
3. `CapabilityForServer("kinocut")` เปลี่ยนจาก `video` เป็น `video-edit` — ปุ่มบน
   ม่านของการ์ด editor นอกห้องงานวิดีโอเคยเสนอโหลด ffmpeg 90MB ที่ไม่มีตัว editor
   อยู่ข้างใน ตอนนี้เสนอตัว editor เอง แล้ว ffmpeg ค่อยโผล่ในเทิร์น "incomplete"
   ของ gate ตามชื่อจริงของมัน

เส้นทางผู้ใช้ใหม่ทั้งสาย: เข้าห้องงานวิดีโอ กดติดตั้งทั้งหมด รอแถบโหลด จบ ไม่มีขั้นไหน
ต้องรู้จักคำว่า MCP
