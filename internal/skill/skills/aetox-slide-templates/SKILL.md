---
name: aetox-slide-templates
description: เทมเพลตจริงที่ก๊อปไปใช้ได้เลย ไม่ใช่คำอธิบายว่าควรหน้าตาแบบไหน เริ่มที่เลย์เอาต์สไลด์ 14 แบบ กล่อง 1280x720 ไฟล์เดียวจบ ไม่มี CSS ภายนอก ไม่มีสคริปต์ ไม่มีกราฟจาก CDN ใช้ตอนกำลังจะเขียนสไลด์หรือหน้าจอแล้วไม่อยากประดิษฐ์องค์ประกอบขึ้นใหม่ทุกครั้ง
---

# Slide templates

Markup that gets copied, not advice about markup. **Slides only.**

The name says the medium on purpose. Aetox produces five kinds of thing and they
do not share a contract: a deck is one self-contained HTML file at a fixed
1280x720 that an off-screen renderer prints, a web page is responsive and lives
in a browser somebody resizes, a document goes out through `doc_write` as OOXML,
a sheet through `sheet_write`, and a picture is SVG or a real photograph found
on the web. A layout that is correct for one is wrong for the next.

So this skill holds slides and nothing else. If web pages or documents ever earn
templates, they earn a skill of their own rather than a folder in here, because
a template nobody can tell the medium of is a template that gets pasted into the
wrong file. (Video is not on the list at all: Aetox reads video with `video_ocr`
and produces none.)

## Why this is its own skill

The design skills already describe every layout a deck could want.
`aetox-design-system` carries `data/slide-layouts.csv`, twenty-five rows, each
naming a layout and its content zones and its visual weight. What it gives for
the composition is a `css_structure` column holding one line:

```
display:flex; flex-direction:column; justify-content:center; align-items:center; text-align:center
```

That is a hint. Everything else had to be invented at write time, every time,
and the result was measurable: decks built from the same table came out
differently on every run, and mostly came out as the one skeleton in
`aetox-slides` with different words in it. A run on 29 ส.ค. read
`slide-strategies.csv` and `slide-layouts.csv` before writing a line and still
produced seven ad-hoc slides that ended on a step.

Descriptions were never the missing part. This is the missing part.

It is separate from the skills that describe, rather than folded into them, for
two reasons. A skill's file listing is capped at 40 entries, so a folder of
templates inside `aetox-design-system` would push its own tables past the edge
and hide them silently. And templates are one question, "what does this look
like in markup", where the tables are another, "which one should I reach for".
One home each.

## Slide layouts

Read the row from `aetox-design-system` `data/slide-layout-logic.csv` first, so
you know which layout the slide wants. Then open only that file here. Each is a
`<section>` plus the CSS it adds, ready to paste.

| Layout | File | Reach for it when |
|--------|------|-------------------|
| Title | `slides/title.html` | Opening. One idea, enormous, room around it. |
| Agenda | `slides/agenda.html` | What is coming. Five rows at most; six is two slides. |
| Section divider | `slides/section-divider.html` | A breath between acts. Allowed to be nearly empty. |
| Two column split | `slides/two-column.html` | An idea and its evidence, side by side, unequal. |
| Big number | `slides/big-number.html` | One number that is the whole slide. |
| Metrics | `slides/metrics.html` | Three or four numbers that belong together. |
| Timeline | `slides/timeline.html` | Steps where the order is the message. |
| Before after | `slides/before-after.html` | Two states of one thing. |
| Comparison table | `slides/comparison.html` | Options against criteria. Three columns at most. |
| Chart | `slides/chart-bars.html` | A quantity comparison, drawn in CSS. |
| Quote | `slides/quote.html` | Somebody's words, attributed. |
| Full bleed image | `slides/full-bleed.html` | The picture is the slide. |
| Feature grid | `slides/feature-grid.html` | Three or six parts of one thing. |
| Code | `slides/code.html` | A command or an excerpt, one screen, never scrolled. |
| Synthesis | `slides/synthesis.html` | Second to last. What the middle added up to. |
| Close | `slides/close.html` | Last. The one thing to do, remember or decide. |

Synthesis and close are not optional and are the two most often missing: a deck
that stops on its final step has not concluded, only stopped. See
`aetox-design-system`, "How a deck ends".

## The contract every template obeys

The deck is **one self-contained HTML file**. There is no stylesheet to link, no
script to load, no CDN. `aetox-slides` carries the skeleton these drop into and
the full reasoning; this is the short form.

- **The slide box is fixed 1280x720 px** with `overflow:hidden`, declared once
  by the skeleton. No template here redefines `.slide`. A deck sized in
  `vw`/`vh` is laid out twice, differently, once in the room and once in the
  off-screen exporter, and you only ever see one of those.
- **Slides sit one after another in normal flow.** Never `position:absolute`
  with `opacity:0` and a script switching between them. The exporter prints what
  the document says, so a deck built that way exports as one slide followed by
  blank rectangles.
- **Entrances go through `.rise`**, which rests visible and animates only on the
  slide the room marks `.onstage`. A slide that starts hidden waiting for script
  exports empty.
- **Charts are CSS or SVG.** A chart from a CDN-fetched library prints blank.
- **Each template adds its own classes only.** Its CSS goes in the deck's one
  `<style>` block, its markup in the body.

Sizes assume the skeleton's 120px side padding. A template that wants the edge
says so and sets its own.

## Using one

Copy the block, change the words, delete what the slide does not need. A
template is a starting composition, not a form to fill in: two slides from the
same template with the same number of cells and the same sentence lengths is the
uniformity these exist to break. Vary the cell count, let one slide be a single
sentence, let another be a number.

## Where the inventory came from

The list of layouts a deck system needs was cross-checked against
`html-ppt-skill` by lewis (MIT), which catalogues 31 and named four that
`slide-layouts.csv` did not: Code, Terminal, Diff and Process Steps.

Its markup was **not** taken, and could not have been. Its slides are `100vw` by
`100vh`, stacked with `position:absolute` and `opacity:0` for a runtime to
switch between, and each of its layout files is a fragment depending on four
external stylesheets and a script. Under the contract above that deck exports as
one slide and a run of blank rectangles. Structure borrowed, ornament not.
