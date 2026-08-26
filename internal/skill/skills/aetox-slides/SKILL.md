---
name: aetox-slides
description: สไลด์และงานนำเสนอในแอตทอกซ์ — เด็คคือไฟล์ .html เดียวที่เปิดในห้องสไลด์ แล้วเปิดหน้า นำเสนอเต็มจอ ส่งออก .pptx .pdf รูป ได้จากตรงนั้น. อ่านตัวนี้ก่อนเขียนเด็คเสมอ ถึงจะมีสกิลสไลด์ตัวอื่นติดตั้งอยู่ก็ตาม เพราะพวกนั้นเขียนไว้สำหรับไฟล์ที่เปิดเดี่ยว ๆ ในเบราว์เซอร์ ส่วนนี่คือข้อเท็จจริงของห้องนี้ที่อ่านจากไฟล์เองไม่ได้. ตัวนี้บอกว่าเด็คประกอบขึ้นยังไง ส่วนว่าจะวางอะไรลงบนสไลด์ — โครงเด็ค เลย์เอาต์ กราฟ ถ้อยคำ — อยู่ที่ aetox-design-system
---

A deck here is one `.html` file that opens in the slides room. Everything about
how it looks is yours. What follows is the short list of things about this room
that cannot be worked out from the file itself.

## The marker

A slide is `<section class="slide">`. Four things key on exactly that: the desk
deciding an `.html` is a deck at all rather than source, the room paging it, the
exporter flattening it, and every export cutting on it.

`<div class="slide">` is a fallback, read only in a document with no
`<section class="slide">` anywhere — and the export flattener does not know it.
A div deck pages on screen and prints as one long page.

## The room moves it

The room pages, presents full-screen and exports, so those controls are already
around your deck: a toolbar with arrows and a page count sits above every deck
that opens here.

**So a deck must not draw its own.** No floating arrows in a corner, no
prev/next buttons, no page counter of its own — the room has all three, and a
second set is two of everything in one frame: two pairs of arrows, two numbers
that can disagree. A deck that brings them still *works* (the room drives it by
pressing the key it listens for) and only the room's survives an export, so the
extra pair is pure duplication on screen and gone on paper. Leave the corner
empty and let the room drive.

## The room marks the slide on screen

The room knows which slide is showing — it must, to page — and it writes that
onto the live document: the slide on screen carries the class `onstage`, the rest
do not. Hang an entrance on that class and every slide animates as it is reached,
whichever way it arrived — a stacked deck that never scrolls, a flow deck paged
by an arrow key, a slide a reader jumps to — because the room re-marks it however
it came.

That last part is why the room, and not the deck's own CSS, carries the signal.
A scroll-driven entrance (`animation-timeline`) runs only while a scroll is
*continuous*; a slide paged or jumped to snaps straight to its end frame with no
entrance at all, and the room pages by a programmatic scroll. `onstage` fires on
arrival by any route, so it is the trigger to build on. Scroll timelines are a
fine extra for a deck read by hand-scrolling outside the room, never the thing a
paged deck leans on.

Like the way the room hides the scrollbar, this is a runtime touch to the DOM and
not a line written into the file. The deck on disk stays plain HTML that owes the
class to nobody — open it in a bare browser and `onstage` simply never arrives,
which the resting-visible rule already covers.

## The box is 1280 x 720

That is what the exporter builds its webview at, and 13.333 x 7.5in at 96dpi,
which is a PowerPoint widescreen page. The room scales the whole slide to fit
whatever panel it is in, so `px` here is a fixed page rather than a small one; a
deck in `vw`/`vh`/`clamp()` is laid out twice, differently, and you only ever
see one of them.

It is a page with a hard edge. The box is `overflow:hidden`, so a slide whose
content runs past 720px is cut there, not scrolled onto a taller page — the last
line goes missing in silence, in the room and the export both. There is no fit
check here that a screenshot would not also miss, so count the lines to the fold
yourself: a slide that has to be squeezed to fit is two slides.

## The export runs off-screen, and it does not wait

It loads the deck in a webview nobody is looking at and prints when navigation
finishes — not when a third-party host answers. Two things follow.

**Assets that live beside the file survive; assets from a CDN are a bet.** A
remote image can be missing from a `.pdf` that looked right on screen, and a
chart drawn by a CDN library can print blank. Fetch pictures into the deck's own
folder and reference them relatively (`imgs/hero.png` resolves against the deck,
in the room and in the export both), or inline small ones as `data:`. Same for
fonts. On a plain Windows machine `"Leelawadee UI", Tahoma, sans-serif` is
there; `Prompt`, `Kanit` and `Noto Sans Thai` are not, so naming one of those
with nothing behind it is the same bet as a CDN, lost silently. A Google Fonts
link is a CDN too, and it is worse than a missing picture: the export prints
whatever had loaded by the time it walked the deck. A typeface that has to
arrive belongs in the deck's own folder, loaded with `@font-face` off a
relative path, exactly as the pictures do.

**An animation prints wherever it comes to rest.** Before it prints, the export
walks the deck: it scrolls every slide into view, collapses animations and
transitions to their last frame, and pins the `opacity` and `transform` each
element computed to at that moment. So an entrance is seen and kept, whatever
started it — a `@keyframes` run with `animation-fill-mode: forwards`, or a class
an `IntersectionObserver` adds when its slide arrives.

What the walk cannot rescue is an element whose *resting* state is invisible. It
pins what it finds, so a trigger that never fires leaves `opacity:0` and that is
what prints. **Put the hidden half inside the `@keyframes` and never in the base
rule.** An entrance written that way survives a trigger that missed, a reader
with reduced motion on, and the export, without any of the three being a special
case.

```css
@keyframes rise { from{ opacity:0; transform:translateY(24px) } to{ opacity:1; transform:none } }
.rise { animation:rise .7s ease both; }   /* rests visible; only the keyframe hides it */
```

Animate freely otherwise. An ambient loop is a screen effect: the export freezes
whatever frame it lands on.

## Two kinds of motion, and the one a page cannot hold

That walk draws a line between two kinds of motion, worth naming because a deck
usually wants both and only one of them crosses into a `.pdf`.

An **entrance** happens once, when a slide arrives, and leaves every element at
rest where a reader sees it a moment later. Written the way above — the hidden
half in the `@keyframes`, the rule resting visible — it survives the export, a
missed trigger and reduced motion all three, because the resting state already
*is* the finished slide. Reach for this first.

A **build** is the other thing: one slide holding several states, uncovering a
line at a time as the presenter clicks — the shape reveal.js calls a fragment,
and the shape most decks bolt a small script onto. On screen it works. In an
export it is gone: the walk pins the last state it can reach, so every step
collapses into the single frame where everything shows. A transition *between*
slides is the same story, a screen effect the export has no page to hold.
Neither is a fault to fix. The room already does the freezing; a deck does not
need a script to un-hide its own steps for the export, and carrying one only
fights work that is already done.

**So a build that has to survive is written as more sections, not more states.**
One `<section class="slide">` per step, each adding the next line to the ones
before it. On screen the room pages through them and it reads as a build; in the
export each section is its own page, because a section is exactly where the cut
lands. No library, no click handler, no export flag — it prints every step in
order and stays the same portable HTML as the rest of the deck.

```html
<section class="slide"><h2>ปัญหา</h2>
  <p class="rise">ข้อหนึ่ง</p></section>
<section class="slide"><h2>ปัญหา</h2>
  <p>ข้อหนึ่ง</p><p class="rise">ข้อสอง</p></section>
```

The one build that survives *inside* a single slide is a line drawing itself: an
SVG path animated on `stroke-dashoffset` from its length to `0`, with
`pathLength="1"` so that length is always `1`. It ends fully drawn, and fully
drawn is visible, so the pin keeps it — the rare build whose resting state is the
finished picture. Animate the path's `opacity` from `0` to `1` in the same
keyframe, or an arrowhead placed with `marker-end` shows at the path's end before
the stroke has reached it.

## A small kit of entrances

These are close to the whole of what a deck needs; reach past them for a reason,
not for variety. The trigger is the `onstage` class the room puts on the slide
being looked at, so each slide plays its entrance as it is reached — not all of
them at load, which is the whole of why an entrance written `animation:rise .6s
both` was only ever seen on slide one: below the fold, unwatched, the rest ran
and finished before anyone scrolled down.

```css
@keyframes rise { from{opacity:0;transform:translateY(24px)} to{opacity:1;transform:none} }
@keyframes fade { from{opacity:0} to{opacity:1} }
@keyframes wipe { from{opacity:0;transform:translateX(-28px)} to{opacity:1;transform:none} }
@keyframes grow { from{opacity:0;transform:scale(.96)} to{opacity:1;transform:none} }

/* resting state visible — the export, reduced motion, a bare browser and every
   slide the room has not marked all keep it. Nothing is ever hidden waiting for
   a class: a flow deck shows several slides at once, so hiding the ones that are
   not `onstage` blanks the deck as you scroll. The class only *starts* an
   entrance; it never gates whether content is visible at all. */
.rise,.fade,.wipe,.grow{ opacity:1 }

@media (prefers-reduced-motion:no-preference){
  .slide.onstage .rise{ animation:rise .55s cubic-bezier(.2,.7,.2,1) both }
  .slide.onstage .fade{ animation:fade .55s ease both }
  .slide.onstage .wipe{ animation:wipe .55s ease both }
  .slide.onstage .grow{ animation:grow .55s ease both }   /* .96, never 0 — a card that pops from nothing reads as a toy */
  /* a time delay staggers here, because the trigger is the class, not the scroll */
  .slide.onstage .rise:nth-child(2){animation-delay:.08s}
  .slide.onstage .rise:nth-child(3){animation-delay:.16s}
  .slide.onstage .rise:nth-child(4){animation-delay:.24s}
}
```

A slide arrives once, so ~0.55s of entrance is a rare moment that can carry a
little weight — not a constant micro-interaction that has to stay under 0.3s.
Use `ease` or `ease-out` for something arriving, never `ease-in` alone, which
starts slow and reads as sluggish. Reduced motion drops the whole block and the
slide simply rests visible, so meaning never leans on the motion.

A deck opened as a bare file — no room, nothing marked `onstage` — rests visible
and still. If it must animate on a hand-scroll outside the room too, add a scroll
timeline as a pure extra; it costs nothing where it is unsupported and it never
touches the paged path, because it is gated to where `onstage` is absent:

```css
@supports (animation-timeline: view()){
  @media (prefers-reduced-motion:no-preference){
    html:not(:has(.slide.onstage)) .rise{
      animation:rise linear both; animation-timeline:view(); animation-range:entry 5% entry 60%
    }
  }
}
```

Its own traps: `animation-delay` does nothing on a scroll timeline (stagger by
giving each child a later `animation-range` instead), and `contain` collapses to
a zero-length range on a slide as tall as the frame, so use `entry`.

Two more traps, both from making one element move more than one way:

- **A number that counts up** rests at its final value only if that value is the
  element's text in the DOM and the script merely races it there. Write `240k` as
  the text and let the count run over it; a counter that starts from an empty node
  prints empty.
- **An element with several transforms** — centred *and* sliding, say — must list
  the same transform functions in the same order in every state it passes through
  (`translate(-50%,-50%) translateY(24px)` → `translate(-50%,-50%) translateY(0)`,
  never dropping one). Mismatched lists leave the browser unable to interpolate,
  and the element flickers or vanishes. It is the most common way an entrance
  breaks.

Every recipe here is one accent, one gesture, subtle. The house look below is a
starting reference, not the only skin a deck may wear — but a deck where each
slide enters a different way reads as the sampler, not the argument.

## The house look

Taken from the deck the owner picked as the standard, and it is a reference
rather than a rule — a dark stage, one accent doing every job, gradients rather
than flat fills, three weights of light instead of white-on-black, and the same
furniture on every slide so it reads as a deck and not as a long page.

Picture-carried is the default; a flat slide is the exception. Most slides
should be carried by an image — a full-bleed photograph behind a scrim (`.hero`
below), or a visual on a panel beside the words — because a dark stage with
nothing but text on it, slide after slide, is exactly what reads as unfinished,
and it is where a deck lands when nobody decided otherwise. A bare slide earns
its place only as a deliberate breath between full ones, never as the shape every
slide falls to for lack of a picture. Emptiness here is a choice you make on one
slide, not the thing that happens to all of them.

So go and get the pictures before laying anything out — `aetox-design` says how,
and this app finds real ones rather than generating any, never an invented one.
Gather a generous set of relevant photographs up front, enough that most slides
have one to stand on; reach for them first and leave space on purpose.

```css
:root{
  --accent:#ff3b30;  /* one accent: kicker, CTA, bullets, the unit on a big number */
  --stage:#050608; --line:#303239;
  --text:#fff;       /* headings */
  --body:#d2d4da;    /* paragraphs */
  --muted:#9ea2ad;   /* captions, footers */
}
.slide { background:radial-gradient(circle at 80% 15%,#242630 0,#101116 28%,#07080a 70%); }
.card  { background:linear-gradient(145deg,#1d2028,#111217); border:1px solid var(--line);
         border-radius:24px; box-shadow:0 18px 40px #0005; }

/* A photograph carries a slide; a scrim over it keeps the words readable. */
.hero  { background-image:linear-gradient(90deg,rgba(5,6,8,.96) 0 28%,rgba(5,6,8,.45) 56%,rgba(5,6,8,.30)),
                          url('imgs/hero.png');
         background-size:cover; background-position:center; justify-content:flex-end; }
/* Or on a panel beside the words — `contain`, so a product is never cropped. */
.visual img { max-width:100%; max-height:500px; object-fit:contain;
              filter:drop-shadow(0 25px 35px #000); }
```

Type at this size: h1 ~96px, h2 ~64px, body ~22px, caption ~13px — below about
13px the export to pictures stops resolving it. Paragraphs hold a `max-width`
around 720px even on a 1280px slide.

**`Prompt` is the deck typeface, and the tail behind it is not decoration.** A
deck opens in an iframe pointed at the file, so it inherits nothing from
Aetox's own stylesheet: a name renders only if that font is installed on the
machine or the deck carries it. `Prompt` is a Google face and a plain Windows
install does not have it, so `"Leelawadee UI",Tahoma,sans-serif` behind it is
what the deck actually falls to, and it is Thai either way.

Which is why **no rule may end at a bare `sans-serif`**. The heading here read
`font-family:"Kanit",sans-serif` with no Thai behind it, and on a machine
without Kanit it fell to the generic sans, which carries no Thai glyphs at all,
so every Thai title came out in whatever the system substituted one character
at a time. Set a different face on an element only with the same tail after it.

To make a face certain rather than likely, carry it: an `@font-face` in the
deck's own `<head>` pointing at a `.woff2` beside the file, or inlined as a
`data:` URL when the deck travels as one file, and `font-display:block` so the
off-screen export waits for the real face instead of snapshotting the fallback.
A name on its own is a wish, and the export prints exactly what the screen showed.

The house pairing, all under the SIL Open Font License and safe to embed:
**Kanit** for headings — loopless, tall, holds a dark wall; **Prompt** for body,
or **Anuphan** where a quieter, lighter body reads better; and **Sarabun** for a
deck that wants the formal, official-document texture. Two weights of two
families is about 100KB, nothing to inline. One caution: the government
`TH Sarabun` (`New`/`PSK`) is GPL rather than OFL and ships no web build — reach
for Google's `Sarabun`, the same face with a clean licence. Every stack still
ends `…,"Leelawadee UI",Tahoma,sans-serif`, so a face that fails to load lands on
Thai and never on the bare generic.

Where a deck quotes a number somebody else published, say whose it is.

## The skeleton

```html
<!DOCTYPE html>
<html lang="th">
<head>
<meta charset="UTF-8">
<title>ชื่อเด็ค</title>
<style>
  :root{ --accent:#ff3b30; --stage:#050608; --text:#fff; --body:#d2d4da; --muted:#9ea2ad; }
  *{ box-sizing:border-box; margin:0; padding:0 }
  /* carry the faces beside the file so the export prints them, not a fallback:
     @font-face{font-family:"Kanit"; src:url("fonts/Kanit-SemiBold.woff2") format("woff2"); font-weight:600; font-display:block}
     @font-face{font-family:"Prompt";src:url("fonts/Prompt-Regular.woff2")  format("woff2"); font-weight:400; font-display:block}
     font-display:block makes the off-screen export wait for the real face instead of snapshotting a fallback */
  body{ background:var(--stage); font-family:"Prompt","Leelawadee UI",Tahoma,sans-serif }

  .slide{ width:1280px; height:720px; position:relative; overflow:hidden;
          padding:78px 120px; display:flex; flex-direction:column; justify-content:center;
          background:radial-gradient(circle at 80% 15%,#242630 0,#101116 28%,#07080a 70%);
          color:var(--text) }
  h1{ font-size:96px; line-height:1.05; letter-spacing:-.035em;
      font-family:"Kanit","Leelawadee UI",Tahoma,sans-serif }
  p { font-size:22px; line-height:1.7; color:var(--body); max-width:720px }

  .kicker{ color:var(--accent); font-weight:600; letter-spacing:.14em;
           text-transform:uppercase; font-size:16px; margin-bottom:14px }
  .brand { position:absolute; left:120px; top:28px; font-weight:700 }
  .page  { position:absolute; right:120px; top:28px; color:var(--muted); font-size:13px }
  .footer{ position:absolute; bottom:25px; left:120px; right:120px; display:flex;
           justify-content:space-between; color:var(--muted); font-size:13px }

  @keyframes rise{ from{opacity:0;transform:translateY(24px)} to{opacity:1;transform:none} }
  /* rests visible always; the room marks the on-screen slide .onstage, which
     starts the entrance as that slide is reached. Never hide the slides it has
     not marked — a flow deck shows several at once */
  .rise{ opacity:1 }
  @media (prefers-reduced-motion:no-preference){
    .slide.onstage .rise{ animation:rise .55s ease both }
    .slide.onstage .rise:nth-child(2){ animation-delay:.08s }
    .slide.onstage .rise:nth-child(3){ animation-delay:.16s }
  }
</style>
</head>
<body>

<section class="slide">
  <div class="brand">แบรนด์</div><div class="page">01 / 08</div>
  <div class="kicker rise">Kicker</div>
  <h1 class="rise">หัวเรื่อง</h1>
  <p class="rise">บรรทัดรอง</p>
  <div class="footer"><span>คำโปรย</span><span>01</span></div>
</section>

<section class="slide">
  <div class="brand">แบรนด์</div><div class="page">02 / 08</div>
  <h1 class="rise">สไลด์ถัดไป</h1>
  <div class="footer"><span>คำโปรย</span><span>02</span></div>
</section>

</body>
</html>
```

Slides one after another in normal flow is the simplest thing that works
everywhere. Stacking them with one shown at a time also works — the room reads
which slide is showing, the exporter lays them back into flow — and it is the
shape that tends to come with navigation attached.

## Making one

1. Decide the shape of the deck before writing any of it. `aetox-design-system`
   carries the tables for that — `data/slide-strategies.csv` for the running
   order of a whole deck, `data/slide-layouts.csv` for what each slide is made
   of, and the rest of `data/` for charts, typography and copy. Read them with
   `skill_view`. They are where the variety comes from: a deck that reaches for
   the skeleton below on every slide reads as one long page, which is the one
   thing this room makes obvious.
2. Find and download the pictures — `aetox-design` has the recipe
   (search the page, not the file; `web_fetch` lists the image URLs it found;
   `shell` downloads the bytes) and the rule about licences.
3. `write` the `.html`; the receipt says where it landed. Reference the pictures
   relatively.
4. `desk open` that path, so the user sees it in the room.

The room's export bar writes `.pptx` (editable), `.pptx` as pictures, `.pdf`,
and `.png`/`.jpg`/`.webp`. There is no tool for it: the user is already looking
at the deck. A request for a PowerPoint is answered by the deck plus a sentence
saying where that button is — nothing here builds a `.pptx` from scratch, and
the one the room writes is made from your HTML anyway.

Nothing of Aetox goes into the file. The deck the user keeps is a plain HTML
file that opens in any browser on any machine, which is why decks are HTML here.
