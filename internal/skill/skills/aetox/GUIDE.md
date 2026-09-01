# The long answers

`SKILL.md` beside this file is the index: where everything lives, and the
rules in one line each. This file holds the reasoning and the craft behind
them. Open the section the job actually needs, not the whole file.

## Delegation, in full

Work handed to an เอเจน or ซับเอเจน **outlives the turn that handed it over**
(§105): a delegate the assistant did not collect before answering keeps
working, is collectable in a later turn by the same task id, and a question it
parked on can be answered then too. **Four run at once; anything past that
waits its turn and starts on its own the moment a slot frees** (30 ส.ค.).
Nothing is refused, however wide the fan-out: twenty jobs asked for are twenty
jobs done, four at a time. A waiting one says **รอคิว** rather than spinning
over a clock, it has its id already, and it is collectable and stoppable like
any other. What ends one early is the user: **Stop** in the composer ends
every running delegate, **ยกเลิกทั้งคิว** ends the whole waiting line and
leaves the four that are working, and the card below the conversation — as
well as the card drawn in the transcript itself — carries a stop on each
delegate and on each declared run, which ends that one alone. Either way it is
a statement about the work rather than about the turn.

The user watches all of this on that card: each uncollected delegation, its
last few tool calls, what it has read and written in tokens, and, for one
parked on a question, the question with a box to answer it.

A card the engine can no longer vouch for reads **"เทิร์นจบก่อน งานนี้เลย
ไม่ได้รายงานผล"** rather than spinning. That is a turn that died mid-flight,
or an app closed and reopened, taking with it the only channel the delegate
had to report back, so it is not a delegate that failed at its job, and if the
user asks, say that rather than guessing at what the work found.

Nobody has to press anything to get a result. The moment a delegation
finishes, a `[ระบบ]` message arrives saying so; collect it with
`task action=collect` and report what it found.

**A delegate the user stopped sends no such message, and you must not go
looking for one.** Nothing collects it, on purpose: they ended that work
deliberately, and restarting the same job, or spending a turn reporting on the
wreckage of it, is the opposite of what the click meant. If a stopped delegate
mattered to the answer, say plainly that the work was stopped and what is
therefore missing, and let the user decide whether to ask again. An answer
typed on the card arrives as "ตอบ task_N ด้วย task action=answer ว่า …". Both
are ordinary user messages, do exactly what they say.

Work that takes more than one wave of them is declared first, with **`task`
action=plan**: a name, a sentence for the user, and the phases in order
including the ones that have not happened yet. Every `task` after it names one
of those phases, and only those. Nothing is enforced by it and nothing needs
to be: a phase that was promised and never filled sits at zero on the user's
card for the whole run, so a checking round skipped because the answer already
looked finished is visible rather than silent. There is **no token ceiling**
on a run (owner, 16 ส.ค.), the card shows what it is spending, split into what
each delegate read and what it wrote, and Stop ends it. Both halves of that
bargain are the user's to act on, not yours to ration: never refuse or shorten
work to save tokens on your own initiative.

**Why delegates cannot touch the user's panel** (31 ส.ค.): `desk` and
`desk_terminal` are refused to every delegate, เอเจน included, because their
whole output is what the person is looking at and nobody is watching a
delegate's loop, while several run at once and would write over each other. It
costs them nothing — `shell` runs the same command and a file they wrote is on
disk — and an เอเจน gets both back the moment somebody opens a direct chat
with it. The browser is **not** in that group: a delegate browses in a tab of
its own, never the one on screen.

## Addressing an เอเจน with @

A user reaches a colleague three ways: opening its own chat, letting the
assistant hand it work with `task`, or writing **`@<name>`** in an ordinary
message. Only the middle one is a switch, and there are **two of them**, one
per kind (20 ส.ค.), each on its own settings page. They ship opposite ways:
**เอเจน off** (handing a whole job to a colleague is a decision, and it costs)
and **ซับเอเจน on** (those are the assistant's own hands). With both off there
is no `task` tool at all; with one off the tool is built carrying the other
roster only. Measured per message: 710 for the pair, 629 for เอเจน alone, 599
for ซับเอเจน alone. The other two doors are the user's and no setting closes
them. `@` delivers that single message to the worker word for word, no
paraphrase in between, and leaves the conversation where it is. If that worker
stops to ask something back, the user's next message answers it.

**Addressing takes a choice, not a word** (30 ส.ค.): the `@` menu has to be
opened and the name picked off it, and the token has to still be in the
message when it is sent. Typing or pasting the characters does nothing — a
draft that merely quotes `@reviewer` in a code span is an ordinary message to
the assistant. **Only เอเจน can be addressed**, never ซับเอเจน: those are the
assistant's own hands and take their work from an agent.

## Writing a desk manifest

**The body names acts, never tool ids.** The frontmatter is configuration and
may name anything (`tools:`, `deny:`, `chairs:` are read by the engine and
shown to nobody). The body is prose *you* paraphrase to the user, so a tool id
written into it becomes a code word handed to somebody who has never seen it
and cannot type it. Say "put it on the desk", not the name of the call that
does it. It happened: `desk_open` sat in two bundled bodies, and in คู่คิด,
where no tool definitions are sent and the body is the whole inventory, it was
one of three things the assistant could name when the owner asked what it
could do, beside a panel showing four labelled buttons (22 ส.ค.).

**On the โค้ด desk the user can see your edits, line by line** (DECISIONS
§161). Every `edit`, `write`, `edits` and `notebook_edit` sends its own hunks
up with the result, in git's format, and the row for that call unfolds into
them. So: do not paste a diff of your own work into the answer, and do not
describe a change line by line in prose, it is already on screen, at the step
that made it, and the second copy is the one that goes stale. Say what the
change *does* and where to look. On every other desk that fold-out does not
exist, so a change worth seeing there has to be said in words.

## STARTERS.md, and how to write the cards

Beside `AGENT.md`, a worker may keep a `STARTERS.md`, the question at the top
of an empty chat with it, and the cards under it. Markdown that happens to
parse: the heading is the question, each list item is one card, split on `|`
into title, the sentence that lands in the composer, and optionally an icon
name. `STARTERS.en.md` beside it is the English version. All of it is
optional; a worker without one opens with the cards the app draws for any
colleague, but writing one is what makes a hired worker feel like the shipped
ones, so offer it whenever you write an `AGENT.md`.

**A file holds a pool; the window draws four of it.** Up to 24 cards per file,
four dealt at a time from a shuffled bag, with a "show me another four" button
under the grid, so a worker can get deeper without the empty chat getting
busier. Never write fewer than four: the grid is two columns and three deals a
widow onto the second row. The two halves of a card do different jobs and are
written differently:

- **The title sells the outcome** and is read in about a second, so it names
  what the user ends up holding rather than the ability used to get there.
  "ได้สไลด์นำเสนอ จากไฟล์ที่มีอยู่แล้ว", not "ทำสไลด์ได้". A card that ends in
  a summary rather than something openable is the pattern the owner cut twice.
- **The prompt is the real instruction the model receives.** Nobody has to
  read it, so length is free and every sentence added is quality in the work
  that comes back. Write it the way a prompt engineer would: the order of
  work, the sources allowed, the rule against inventing a number, the exact
  artifact wanted, and what to report about where each finding came from. A
  prompt that merely restates its own title is the mistake this paragraph
  exists to prevent. A prompt ending in `:` is the deliberate half-sentence
  the user finishes; one that names a real subject is a finished sentence and
  takes no colon.

The user also has a form for it: การตั้งค่า › เอเจน → that agent → "ประโยคเปิด
ของเอเจนคนนี้", a headline and four rows. It writes this same file, so a file
you wrote by hand opens in it and a card they typed there is a line you can
edit, there is one opening, not an app copy and a file copy.

```markdown
# จะให้ปิดบัญชีอะไรดี?

- กระทบยอดธนาคาร | ช่วยกระทบยอดธนาคารเดือนนี้: | chartColumn
```

## Writing an AGENT.md body

**Body or skill, decided by scope and not by length.** What is true on
*every* job this worker does stays in `AGENT.md`; what is true on *one kind*
of job goes in a skill. A worker's skills are named in its own prompt with a
one-line description each and opened with `skill_view`, so a document costs
nothing until the job calls for it — but the body is not free, and moving
something that every job needs behind that door charges a round trip to be
told what it needed anyway.

**What the body is for, and what it must not be.** Say who the worker is, what
its subject is, and the judgments of that profession — the things that are
true about the craft and that a model does not reliably arrive at on its own.
Do not write a tool manual: which tool reads a PDF, which parameter
right-aligns a column, which action opens a page are all in the tool block
already, on every request, and a second copy of them is paid for twice and
goes stale the day a tool changes. The test is one sentence at a time —
*would this be equally true written in the tool's own description?* If yes it
does not belong here.

Swept out of all seven shipped agents on 31 ส.ค. after the owner named it:
*"บทบาทของเอเจนแต่ละตัวบางตัวแม่งหนี้ทางเทคนิค ไม่ควรกรอกไปตรง ๆ แบบนั้นด้วยซ้ำ
แบบไปจำกัดความคิดมันหมด แทนที่จะชี้ว่ามันคืออะไรและจะทำอะไรก็พอ"*. Two of the
things it removed were not merely redundant: a list of three occasions to use
the browser, which is a model told to do three things and not a fourth, and an
instruction to use `todo_write`, which is in forcedDenials and no worker has
ever held.

A worker may also keep an `mcp.json`, the tool servers it brings with it, as
an array in `mcp-servers.json`'s own schema. It is a **declaration and never a
grant**, exactly as `needs:` is: what the machine actually connects stays
`mcp-servers.json` and only that, and the file in the folder is read once,
when a package is installed. A secret in it is written as `${ask:NAME|label}`,
which is a field the person installing fills in, a package carrying a working
token would be carrying its author's account.

`needs:` grants nothing and never has, but since 30 ส.ค. it does decide
whether the worker is offered at all: an unmet entry draws a lock over that
worker's card everywhere it appears, saying it cannot be used until the thing
is there. So write on that line only what the worker genuinely cannot work
without. A capability that merely improves the job belongs in the body, where
the worker can say which half of itself it has — `github` reads repositories
through its own tool and only writes to them through a server the user
connects, and that is a paragraph in its file rather than a second `needs:`
entry.

**Neither packaging road is on screen yet.** Exporting exists in the app
(`ExportAgentPackage` writes a .zip of the folder, without `MEMORY.md`, what
that worker learned on *this* machine is never part of what travels) but has
no button on it, and reading a package back in is not built at all. So an
`mcp.json` sitting in a folder is a file nothing acts on today: never tell a
user that dropping one configures a server. The standard is
`docs/architecture/agent-package-standard-2026-08-08.md`.

## Creating a worker yourself

There is no dedicated tool for it. What there is, is the ordinary `write`, and
whether it reaches depends on the mode:

- **No project focused**, the sandbox is open, so `write` with an **absolute**
  path into `<DataRoot>/agents/<name>/AGENT.md` succeeds, and the worker
  appears with no restart: the profile resolver reads the disk on every
  lookup.
- **A project focused**, the wall is up and the same write is refused with
  "path is outside the folders this session can use".

So the accurate answer to "can you add an agent yourself" is *yes, when no
project is focused*, not "I have no way to". But **ask first**. Hiring is the
user's decision, the folder is theirs, and a teammate that appeared because
you inferred one was wanted is a change to their team that nobody chose. Offer
to write it, say where it will go, and let them answer.

`<DataRoot>/modes` behaves the same way for desks. `<DataRoot>/subagents` does
not: a file written there is **not loaded** whatever mode you are in, so
writing one produces a conflict report rather than a helper.

The folder name is the worker's id. It may not contain spaces or
`\ / : * ? " < > |`, and it may not collide with a ซับเอเจน's name — memory
and job history key on the bare name, so one name belongs to one worker. A
name that matches a bundled agent is not an error: it replaces it, and the
office card says so.

## Installing a skill: the details

`plugin_install` accepts both shapes of repository, and the plain one is the
normal case:

- **No manifest** (almost every published skill), the repository tree is read
  and *any folder that directly contains a `SKILL.md`* is a skill, at any
  depth. So `skills/foo/SKILL.md` installs as `foo`. A `SKILL.md` at the
  repository root wins outright: the repository itself is the skill, named
  after the repo. Skipped: dot-directories, and `node_modules`, `vendor`,
  `dist`, `build`, `target`, `out`, `coverage`, `__pycache__`, a `SKILL.md`
  under one of those belongs to somebody else's package. Every file beside the
  `SKILL.md` comes with it.
- **`aetox-plugin.json`** at the repository root, an explicit manifest
  (`type: skill-bundle`, a name, and a `files` list of source→target pairs)
  for a repository that wants to say exactly what it ships.

Never tell a user a repository is unsupported because it lacks
`aetox-plugin.json`. That is the ordinary case and it installs.

**A bundled skill has no folder on disk**, so it cannot be revealed or deleted
from Settings, and `search` and `shell` will never find it, but its own files
do ship with it, and `skill_view` with a `path` serves them. The body lists
what it carries at the end; anything not on that list is not there, including
a file a table *inside* the document names. The two can disagree, because the
table is what the skill was written to have and the list is what the binary is
carrying, and a refused `path` answers with the list rather than sending you
back to the table you just read.

## Several chats at once

Each chat holds its own engine and its own memory (DECISIONS §150). The user
can give you a job here, switch to another chat, type in that one, and open a
third; all three work at the same time, and coming back to any of them shows
its work still going rather than a transcript read back off disk.

What that means for you:

- **Your context is yours.** Another chat's conversation is not in it and
  never was. If the user refers to something "we said in the other chat", it
  is not something you can see; ask, or read the history (`session_search`).
- **Your tools and your desk are fixed at the moment your chat opened.** A
  session is born at a desk and stays there. Another chat being at a different
  desk changes nothing for you.
- **A question you raise (`ask_user`) appears in this chat and is answered
  here.** Two chats can be waiting on different questions at once.
- **Switching chats no longer interrupts anything**, so a long job is a normal
  thing to leave running. What still cannot happen mid-turn is changing the
  *project*, that moves the sandbox root, the workspace folders and the shell,
  and those belong to the machine rather than to any one chat.
- **You are not the only writer.** The other chats share the work tree, and so
  does the person: files open in the window save themselves as they are typed
  in. `write`, `doc_write` and `sheet_write` therefore refuse a file that has
  changed since this session last read it, rather than replacing somebody's
  work without knowing. When that refusal arrives, read the file again, that
  shows what changed and clears the refusal, and prefer `edit`, which keeps
  what is already there. It is not a lock and it is not an error to be worked
  around.
- **What you put on the desk lands on YOUR chat's desk.** A file you open, a
  page you browse: on screen it draws now; while the user reads another chat
  it waits on this chat's desk and they find it on returning (browser pages
  included, since 1 ก.ย. — your tab keeps working in the background, it just
  does not draw over somebody else's desk). One consequence is honest and
  named: a `browser` capture taken while your chat is off screen photographs
  a hidden window, and its answer says the picture may be a stale frame.

## โปรเจกต์ (storefront door), in full

A project is a folder: `<DataRoot>/project/<name>/`, with its context files in
`<DataRoot>/project/<name>/context/`. The folder is the truth, there is no
table of projects to fall out of step with the disk, so a folder made by hand
is a project and a folder deleted is a deleted project.

The room deletes one too (the ลบโปรเจกต์ button on the project's own page,
beside เปิดโฟลเดอร์): it removes the folder and the context copies inside it,
and leaves the chats alone, they stay in the history held outside every
project.

It groups chats and carries those context files into every session held in it.
**It does not move the sandbox wall**, you still reach the machine exactly as
you otherwise would. That is what separates it from the workshop door's
project, which roots the sandbox in a folder on disk and is a fence.

What those files ARE to a session held in it: the opening context of the work,
not background reading. Answer from them where they answer the question and
say which file it came from; where they and what you otherwise know disagree,
give both rather than picking a side quietly.

**The `context/` folder is the user's own, and you do not write into it on
your own** (owner, 30 ส.ค.). Made something the project should keep, or found
something in there that has stopped being true? Say so and ask first. This is
an instruction, not a wall: the approval gate shows no card at all under full
access, and the card it does show elsewhere cannot know that this particular
file is the ground every future chat in the project stands on.

## Reaching a folder outside the project

With a project focused, the workspace is that folder plus a list the user
keeps. On the desktop, a path outside it is **not** the end of the work: the
user is shown a card naming the folder your path lives in, and if they accept,
the folder joins that list and the call you were making goes through. You do
not have to announce the request or talk them through the menu, naming the
path is the request.

Two things follow from that. If the user declines, that is an answer: say what
you could not reach, finish everything else, and do not raise the same folder
again. And the folders on that list are the permission, the user can see them
in the project menu and take one off at any time, which narrows the running
session immediately.

The card cannot open the always-refused folders (`SKILL.md`, last section).
Those are refused after it, so accepting one never reaches a credential store.

In the CLI there is no card: a refusal is final, and the useful thing is to
name the folder the work needed so the user can add it and run again.
