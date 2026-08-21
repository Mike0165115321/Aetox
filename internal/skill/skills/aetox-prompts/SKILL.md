---
name: aetox-prompts
description: เขียนชุดคำสั่ง (/ชื่อ) ให้ผู้ใช้ — คำสั่งที่เขาพิมพ์ซ้ำ ๆ เก็บเป็นไฟล์เรียกด้วยสแลช, กติกาชื่อ, $ARGUMENTS, และโครงที่ชุดคำสั่งที่แถมมาใช้จริง. อ่านตัวนี้เมื่อผู้ใช้ขอให้ทำชุดคำสั่ง หรือเมื่อเห็นว่าเขาพิมพ์คำขอแบบเดิมซ้ำ
---

A prompt preset is one thing: a request the user makes often enough to be tired
of typing, saved under a short name and invoked as `/name`. This is the cheapest
extension point in the app — a file, no tools, no tokens until it is typed — and
the one users are least likely to build for themselves.

Unlike a skill, **this one you can write end to end**, so the craft below is
what decides whether it is any good.

## 1. It has to come from something they actually repeat

Do not invent a preset for a job the user has never asked for. The material is
one of two things:

- **Something you watched them do twice.** If this conversation contains a
  request they have clearly made before — same shape, different subject — say so
  and offer to save it. That is the best source there is.
- **Something they tell you when asked.** Two questions: what do they end up
  asking for over and over, and what do they always have to add afterwards
  because the first answer missed it? The second question is where the good
  material is — the corrections are the preset.

Write it in **their words and their voice**, first person, as a request to you.
It is going to be sent as if they had typed it, so it must read like something
they would.

## 2. The shape that works here

Every preset that ships with Aetox has the same bones, and they are load-bearing
rather than decorative:

```
<one line saying what is wanted — this line becomes the card's description>

**วิธีทำ**            what to do, in order, numbered when order matters
**สิ่งที่อยากได้**     what the finished answer must contain
**ห้าม**              the failures they keep having to correct

**<subject>:** $ARGUMENTS
```

Three notes on why:

- **The first non-empty line is the description** shown on the card in
  ตั้งค่า → ชุดคำสั่ง. A vague first line makes an unfindable preset.
- **The "ห้าม" section is the whole value.** Anyone can ask for a summary; the
  preset earns its place by carrying the four corrections the user is tired of
  making. Ask for those explicitly.
- **`$ARGUMENTS`** is replaced by whatever follows the name — `/review src/x.go`
  puts `src/x.go` there. Put it at the end, labelled, so the invocation reads
  naturally. If the body never mentions `$ARGUMENTS`, the arguments are appended
  at the bottom instead, which works but reads worse.

## 3. The name

- One word, no spaces. At most 40 characters.
- Not `\ / : * ? " < > |`.
- Short enough to type without thinking — that is the entire point of the
  feature. `/quote` beats `/quotation-template`.
- Thai names work.

A user preset with the same name as a bundled one **replaces it**, which is how
a shipped preset gets edited: copy it out under the same name. Deleting the copy
restores the original. Aetox's own slash commands win over both, so a preset
cannot shadow one.

## 4. Saving it

The file is `<DataRoot>/prompts/<name>.md` — the body, exactly as written,
nothing else. `<DataRoot>` is in the `aetox` skill; on Windows it is
`%APPDATA%\aetox`.

Unlike the skill shelf, **this folder is not closed to file tools**. You can
write it yourself with `write` and an absolute path. It sits outside the session
workspace, so expect the safety gate to ask the user first — that prompt is the
user seeing what you are about to add to their app, which is correct. Say what
you are writing and why before it appears.

If writing is refused or you are on a desk without `write`: show the finished
body in the chat and send them to **ตั้งค่า → ชุดคำสั่ง → + สร้างใหม่**, which
is the same file by another door. A preset they paste in themselves is not a
worse preset.

A cover picture is optional: `<name>.png` (or `.jpg`, `.webp`) beside the `.md`,
under 4MB. Without one the card shows `/name`, which is perfectly good.

## 5. Show them how it is used, once

End by typing the invocation out: "`/quote บริษัท ก ค่าออกแบบ 40,000`". A preset
whose first use the user has to work out is one they will not use twice.

Then say where to edit it (ตั้งค่า → ชุดคำสั่ง) so it is theirs to change. A
preset is a first draft of a habit — expect to revise it after they run it once,
and offer to.
