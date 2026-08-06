# The Desk's Four Doors, and Where Every File Lands (2026-08-06)

Reference for the right-hand workbench — the desk. Two questions, answered once:
**what the `+` menu is for**, and **which pane draws which file**. Read this before
adding a menu entry or teaching the desk a new file type; both have a shape they
are supposed to keep, and both have been got wrong by reasonable people.

Live code: [`Workbench.svelte`](../../desktop/frontend/src/lib/workbench/Workbench.svelte),
[`workbench.svelte.ts`](../../desktop/frontend/src/lib/stores/workbench.svelte.ts),
[`filehost.go`](../../desktop/filehost.go). Decisions: §76, §79, §80, §81, §87.

---

## 0. Owner's rule (2026-08-06): this desk is where the work happens, so nothing arrives on it unwritten

*"ถ้าหากอันไหนมันต้องสร้างเพิ่ม ต้องเพิ่มจากตรงนี้ก่อนด้วย เพราะมันคือโต๊ะทำงาน"*

**Anything new that will appear on the desk is added to this document first, then
built.** A new pane, a new file type, a new entry in the `+` menu, a new way a
tab is born — it gets its row here, saying which door it comes from, which pane
draws it, and what it cannot do, *before* the code exists. Code that lands
without its row is exactly the drift this file was written to stop.

This is not ceremony. The desk is the one surface where the user finds out what
the agent actually produced, and it has four entries and four routing questions —
small enough that any addition changes its shape, and small enough that the shape
can be checked in one reading. The order matters because the reverse order is how
a fifth menu entry, a second router, or a second reason-for-the-same-refusal gets
in: each is defensible alone and wrong against the whole, and the whole only
exists written down.

The table in §3 is therefore the plan as much as the record. A row with no code
behind it is fine and should say so; code with no row is the error.

---

## 1. The four entries are sources, not file types

```
＋  เทอร์มินัล          a shell, with nothing typed into it yet
    เบราว์เซอร์  Ctrl+T  a page, with no address yet
    ไฟล์        Ctrl+P  the project tree, so a file can be found
    เครื่องมือ           the list of skills and MCP tools the agent can use
```

Every one of them starts something from empty. **Not one of them opens a file** —
`openFilesTab` opens the *tree*, which is where you go to find one.

This is why the menu did not grow when the desk learned PDFs, video and audio
(§87). A "PDF" entry would have to answer the click by asking *which* PDF, and
the answer to that question is already the third entry. Two doors to one room.

**What earns a fifth entry:** a surface that begins with nothing chosen — a
scratch canvas, a notebook that opens blank. Not a file type. A file type earns a
row in §3 below and nothing else.

| Entry | Pane | Tab kind |
|---|---|---|
| เทอร์มินัล | [`Terminal.svelte`](../../desktop/frontend/src/lib/Terminal.svelte) — a real ConPTY session, one per tab | `terminal` |
| เบราว์เซอร์ | [`BrowserPane.svelte`](../../desktop/frontend/src/lib/workbench/BrowserPane.svelte) — a native WebView2 window composited over the pane, not an iframe (§18) | `browser` |
| ไฟล์ | [`FilesPane.svelte`](../../desktop/frontend/src/lib/workbench/FilesPane.svelte) — the project tree; filters nothing, every file is clickable | `files` |
| เครื่องมือ | [`ToolsPane.svelte`](../../desktop/frontend/src/lib/workbench/ToolsPane.svelte) | `tools` |

All four are singletons except terminals and browser tabs.

## 2. A file tab is born at one of three doors

Never from the menu:

1. **A row in the file tree** — `openFileTab(node.path)`
2. **A drop on the desk** — from Explorer, another app, or a produced-file card;
   anything outside the project is copied in first (§80)
3. **A file card in the chat** — the agent's own output, dragged or clicked (§81)

All three land in `openFileTab` → `loadFileTab`, which is the single place that
decides what a file *is*. There is no second router.

## 3. Where every file lands

`loadFileTab` asks four questions in order. The first `yes` wins.

### 3.1 Does it have a pane of its own? → `fileView(path)`

A lookup table on the extension. These are the types the webview renders
natively, so the routing *is* the implementation — the pane is a tag with a
`src` pointing at the file host (`/aetox-file/…`, §87). **No bytes cross the
Wails binding and there is no size ceiling.**

| Extensions | Pane | What you see |
|---|---|---|
| `png jpg jpeg gif webp bmp avif ico svg` | [`ImagePane`](../../desktop/frontend/src/lib/workbench/ImagePane.svelte) | The picture, on a checkerboard so transparency reads as transparency. Click toggles fit-to-pane ↔ 1:1. |
| `mp4 webm mov mkv avi` | [`MediaPane`](../../desktop/frontend/src/lib/workbench/MediaPane.svelte) | A player. Scrubbing works without downloading the file — the host answers Range requests. |
| `mp3 wav m4a flac ogg` | `MediaPane` | A transport bar, no black rectangle. |
| `pdf` | [`PdfPane`](../../desktop/frontend/src/lib/workbench/PdfPane.svelte) | The webview's own PDF reader in an iframe: scroll, zoom, find, print. Nothing here renders the document; bundling a renderer to redo the one already shipped would cost a megabyte to arrive somewhere worse. |

`svg` is deliberately here and not with the text files. It is a picture that
happens to be written down; showing its source was the old behaviour by
accident, not by choice.

**Failure is the element's, not the loader's.** Nothing is read ahead of time, so
a missing file or an undecodable codec surfaces where it happens — `MediaPane`
catches `error` and says the container is fine but the codec is not (`.mkv` and
`.avi` routinely carry H.265 or DivX, which Chromium refuses). `PdfPane` cannot
detect its own failure at all: an `<iframe>` reports `load` either way. That is
why the way out to the real reader is in the header from the first frame rather
than behind a fallback — a blank document area must never be a dead end.

### 3.2 Is it `.xlsx`? → [`SheetPane`](../../desktop/frontend/src/lib/workbench/SheetPane.svelte)

`ReadWorkbook` → [`ooxml.ReadXLSX`](../../internal/ooxml/xlsx_read.go) renders the
workbook to display strings and the pane draws a grid. Read-only by construction
(§79): it answers *"what did I just get?"*, and Excel is still one button away.

Truncation is honest — `Truncated` + `TotalRows` reach the UI, which says how
many rows it is showing of how many. **This is the house pattern for anything too
big to show whole: cut it and say so, never refuse.**

A workbook this reader cannot parse falls through to §3.4's card, because a file
this app cannot read is still a file Excel can open.

### 3.3 Is it `.docx` or `.pptx`? → the card, without reading

Straight to [`ExternalFilePane`](../../desktop/frontend/src/lib/workbench/ExternalFilePane.svelte)
with `workbench.officeNoPreview`. Deliberately *before* any read attempt: routing
a deck through the text path made the card report whichever gate fired first
("file too large to preview" for a 1.5 MB pptx) for a file that was never
previewable at any size. **The reason shown must be the real one.**

`internal/ooxml` writes these formats and does not read them (§77, §78) — there
is no reader to call even if the pane wanted one.

### 3.4 Everything else → read it as text

`ReadFile` → [`FileEditor`](../../desktop/frontend/src/lib/FileEditor.svelte)
(Monaco, editable, Ctrl+S saves). `.md` / `.markdown` open in the rendered view
with a toggle to the source, sharing the chat's renderer.

Two gates decide whether the text path answers at all, and both currently answer
by refusing:

| Gate | Where | Effect |
|---|---|---|
| 1 MB ceiling | [`app.go`](../../desktop/app.go) `ReadFile` | A larger file gets the card, whatever it is |
| any `0x00` byte | same | Binary gets the card — **and so does UTF-16 text**, which on Windows is what PowerShell's default redirect writes |

A read that fails becomes the card carrying the real reason. A file dropped from
outside that could not be brought in at all becomes a card with no path — no
button, because a button that cannot work is a lie.

**Known gaps in this path, unfixed as of this document:** the two refusals above,
and non-UTF-8 text (a Thai CSV saved as TIS-620 from Excel) which renders as
mojibake with no error — worse than refusing, because the file looks broken when
it is fine. The fix is the same shape as §3.2: read a bounded head, mark the
editor read-only, and say what was cut. **Read-only is not optional there** —
`WriteFile` replaces the whole file, so saving a truncated buffer would destroy
what was not shown.

## 4. Where the ceilings live, and why

The rule: **a ceiling belongs only where the bytes must enter JavaScript whole.**

| Path | Ceiling | Reason |
|---|---|---|
| image · video · audio · pdf | none | Streamed over HTTP by the webview; a 4 GB video costs what a 4 MB one costs |
| `.xlsx` | 8 MB file, plus 500 rows × 64 columns × 20 sheets | The whole preview is marshalled to JSON and crosses the binding. The row cap is the real guard; the byte cap only limits how much XML a zip bomb can make the reader walk |
| text | 1 MB | The string enters the DOM. This is the number §3.4 says is the wrong shape, not the wrong size |
| chat image attachments | 20 MB (`ReadImageDataURL`) | Unchanged and unrelated — that path still builds a data: URL for the composer |

## 5. Adding a file type later

For anything the webview renders natively, three edits and no new plumbing:

1. `viewByExt` in `workbench.svelte.ts` — extension → view name
2. `contentTypes` in `filehost.go` — pin the MIME type rather than letting
   `mime.TypeByExtension` ask the Windows registry, where another program's
   install can have claimed the extension
3. A branch in `Workbench.svelte`'s file-tab chain

For anything that needs understanding first (an archive listing, a database, a
notebook), the shape to copy is §3.2: read it in Go, return *display strings*,
draw them in a pane, keep the way out to the real program. Not a round-trip, not
an editor.

## 6. The agent's own reach onto the desk

*"มันควรจะเปิดอะไรได้เองหมด ถ้าหากอยากเปิด"* — owner, 2026-08-06.

**Written ahead of its code per §0, and built the same day.** Lives in
[`workbench_desk.go`](../../desktop/workbench_desk.go) — named for the workbench
because `desk` already means the mode a session was opened at (§83), and the
file names should not make that collision worse. See §6.6.

### 6.1 What the agent can reach today: one channel, and it is blind

The whole of the agent's power over the desk is `workbench:open-browser`
([`workbench.go`](../../desktop/workbench.go)) behind the `browser_open` skill.
It opens a **browser tab**, not a file tab, and accepts only what
`browserRenderable` lists.

It cannot open a file tab, cannot open a terminal, cannot close or focus
anything, and — the sharpest gap — **cannot ask what is on the desk at all**.
Its depth is entirely *inside* a page it opened itself (`browser_read`,
`browser_click`, `browser_type`): fluent about web pages, ignorant of the desk
they sit on.

The work in §87 widened this. The desk went from four panes to six; the agent
still reaches one. The visible consequence is that an agent which has just
produced a deck can say it is done but cannot put it down in front of anyone —
the user has to go and click the card themselves, on the surface whose entire
job is showing what was produced.

### 6.2 `desk_open(path)` — the missing verb

One skill, calling the `openFileTab` the tree and the drop already call. It does
**not** decide which pane; `fileView()` does, exactly as it does for a human
click. That keeps §2's promise that there is no second router, and it means every
type the desk learns afterwards is reachable by the agent on the same day —
without the skill, its description, or the model knowing anything about panes.

- **Scope:** sandbox-relative paths only, through the same guard as every file tool.
- **Focus:** opens *and* activates. "Look at this" is the whole point; a tab that
  arrives silently behind five others has not been handed to anyone.
- **Return value:** which pane took it, and plainly whether it landed on the card
  — so a model that gets `.pptx` back as "on the desk as a card; the user can
  open it with their own program" stops rather than retrying in a loop.
- **No duplicate tabs:** the tab id is already the path, so re-opening the same
  file re-reads it in place (§3's re-read rule) instead of stacking.

### 6.3 `desk_list()` — and the one thing it must not report

Reading the desk is what makes the agent able to *choose* rather than guess: not
re-opening what is already up, seeing that the user is looking at something else.

**The redaction rule is not optional.** §81 already decided that the workbench's
browsing history stays in `localStorage` rather than the `tool_runs` table,
precisely because that table is agent-searchable memory and *"putting the user's
personal browsing there would make it agent-readable, which is a far bigger
decision than a start-page list gets to make."* A `desk_list` that returned
browser tab titles and URLs would hand over the same thing through a different
door.

So: file tabs and terminal tabs report fully (a file path in this project is
something the agent can already read), the agent's own browser tabs report fully
(it opened them), and **a browser tab the user opened reports its existence and
nothing else** — no URL, no title.

### 6.4 The boundary with `browser_open`

Both can point at a local file today, and after `desk_open` exists only one
should:

> **A page runs. A file is looked at.**

A local file goes to `desk_open` — it lands in the pane built for it, with the
header and the way out to the real program. `browser_open` keeps local files only
for the case that genuinely needs a browser: an `.html` that must *execute* and
load its own assets. The `browserRenderable` list should shrink to match when
this lands; leaving both routes open for `.pdf` is two answers to one question.

### 6.5 `desk_terminal(command?)` — and a call this document got wrong

This section first said opening a terminal was *"deliberately not planned"*, on
the reasoning that the agent already has `shell` and a visible terminal would
need a whole shared-shell subsystem to be worth anything. That was wrong twice
over, and the owner said so within the hour: *"มันเปิดเทอร์มินัลไม่ได้ เปิดได้แค่
เบราว์เซอร์"*.

Wrong on cost: `TerminalStart`, `TerminalWrite` and `TerminalClose` have been
bindings since the terminal pane shipped, and `terminal:data:<id>` already
streams a live session to a pane. Nothing had to be built but the way to ask.

Wrong on purpose: `shell` answers *"what does this print?"* — output to the
agent, in the chat. A visible terminal answers *"watch this run"* — a build, a
dev server, a session the user will keep typing into afterwards. Those are
different questions, and only one of them was answerable.

- **Go owns the session first.** Unlike the browser (where the frontend makes the
  window and Go polls for it), `TerminalStart` returns a live id before the event
  is sent, so the frontend only mounts a pane onto something already real.
- **The command is typed, not executed separately** — `TerminalWrite(id, cmd+"\r")`
  is the agent pressing the keys, in the user's own shell.
- **Output does not come back.** The reply says so in as many words, so a model
  waiting for stdout stops waiting and reaches for `shell` instead.

**It reaches the same approval gate as `shell`, by construction.**
`safety.AssessCommand` routes `desk_terminal` into the shell assessment and keeps
the tool's own name on the verdict; `toolCallToArgs` tokenizes its command in the
same `case` as shell's. Anything less would have made this a second, quieter way
to run any command — with a worse blast radius than the `sheet_write` gap that
file already warns about. An *empty* terminal is assessed low: nothing runs until
the user types, and what they then type is theirs.

### 6.6 Known terminology debt: `desk` means two things

§83 uses **desk** for the mode a session was opened at (`a.desk.DeskName()`,
`desk: specialized` in a chair's profile). §80 uses **desk** for this surface,
because that is what the owner calls it — โต๊ะทำงาน.

The tools here are named `desk_*` after the second meaning. The collision was
already in the tree before them and is recorded here rather than quietly widened:
if it is ever resolved, these three names and `internal/mode` are what move.

### 6.7 Deliberately not built

| | Why |
|---|---|
| `desk_close` | Closing a tab the user put there is not the agent's call. Its own tabs are arguable; not worth the surface until something actually needs it |
| Opening the tools tab | It lists the agent's own tools. It has nothing to tell itself |

## 7. Deliberately not built

| | Why |
|---|---|
| `.doc` `.xls` `.ppt` (pre-2007 binary) | `.xls` is BIFF8 and genuinely reachable — it would land in `SheetPane` unchanged. `.doc` and `.ppt` address their text through a piece table and a record tree with no usable Go reader; writing one is not a feature this product owes anyone. A converter on the user's machine (`soffice --headless --convert-to pdf`) would answer all six Office formats at once through §3.1's PDF pane, and is the route to take if this is ever taken |
| `.psd` `.ai` `.sketch`, 3D/CAD, DRM'd media | An engine per format, for files the agent does not produce |
| A worse Excel / Word / PowerPoint | The program that opens these is already on the machine, and Aetox's promise is that the work happens there (OFFICE-EXPORT-PLAN.md §8) |

The card is not a failure state for these. It is the honest answer, with the
button that leads to the program that *is* the answer.
