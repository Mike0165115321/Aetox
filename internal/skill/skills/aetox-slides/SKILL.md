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
around your deck. One that brings its own still works — the room drives it by
pressing the key such a deck listens for — you just get two sets in one frame,
and only the room's survives an export.

## The box is 1280 x 720

That is what the exporter builds its webview at, and 13.333 x 7.5in at 96dpi,
which is a PowerPoint widescreen page. The room scales the whole slide to fit
whatever panel it is in, so `px` here is a fixed page rather than a small one; a
deck in `vw`/`vh`/`clamp()` is laid out twice, differently, and you only ever
see one of them.

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

## The house look

Taken from the deck the owner picked as the standard, and it is a reference
rather than a rule — a dark stage, one accent doing every job, gradients rather
than flat fills, three weights of light instead of white-on-black, and the same
furniture on every slide so it reads as a deck and not as a long page.

A deck with no pictures in it is the one thing here that reads as unfinished.
Go and get them before laying anything out — `aetox-design` says how, and this
app finds real ones rather than generating any.

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

To make `Prompt` certain rather than likely, carry it: an `@font-face` in the
deck's own `<head>` pointing at a `.woff2` beside the file, or inlined as a
`data:` URL when the deck travels as one file. A name on its own is a wish, and
the export prints exactly what the screen showed.

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
  body{ background:var(--stage); font-family:"Prompt","Leelawadee UI",Tahoma,sans-serif }

  .slide{ width:1280px; height:720px; position:relative; overflow:hidden;
          padding:78px 120px; display:flex; flex-direction:column; justify-content:center;
          background:radial-gradient(circle at 80% 15%,#242630 0,#101116 28%,#07080a 70%);
          color:var(--text) }
  h1{ font-size:96px; line-height:1.05; letter-spacing:-.035em }
  p { font-size:22px; line-height:1.7; color:var(--body); max-width:720px }

  .kicker{ color:var(--accent); font-weight:600; letter-spacing:.14em;
           text-transform:uppercase; font-size:16px; margin-bottom:14px }
  .brand { position:absolute; left:120px; top:28px; font-weight:700 }
  .page  { position:absolute; right:120px; top:28px; color:var(--muted); font-size:13px }
  .footer{ position:absolute; bottom:25px; left:120px; right:120px; display:flex;
           justify-content:space-between; color:var(--muted); font-size:13px }

  @keyframes rise{ from{opacity:0;transform:translateY(24px)} to{opacity:1;transform:none} }
  .rise{ animation:rise .7s ease both }
  .rise:nth-child(2){ animation-delay:.10s }
  .rise:nth-child(3){ animation-delay:.18s }
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
