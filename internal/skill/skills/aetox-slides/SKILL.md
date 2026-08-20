---
name: aetox-slides
description: สไลด์และงานนำเสนอในแอตทอกซ์ — เด็คคือไฟล์ .html เดียวที่เปิดในห้องสไลด์ แล้วเปิดหน้า นำเสนอเต็มจอ ส่งออก .pptx .pdf รูป ได้จากตรงนั้น. อ่านตัวนี้ก่อนเขียนเด็คเสมอ ถึงจะมีสกิลสไลด์หรือสกิลดีไซน์ตัวอื่นติดตั้งอยู่ก็ตาม เพราะพวกนั้นเขียนไว้สำหรับไฟล์ที่เปิดเดี่ยว ๆ ในเบราว์เซอร์ ส่วนนี่คือข้อเท็จจริงของห้องนี้ที่อ่านจากไฟล์เองไม่ได้
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
fonts: `"Kanit", "Noto Sans Thai", "Leelawadee UI", Tahoma, sans-serif` is on a
plain Windows machine; a Google Fonts link is a CDN.

**An animation prints wherever it comes to rest.** The flattener forces
`opacity:1` on a *slide* that computes to zero, and does nothing to what is
inside it — so an element waiting at `opacity:0` for something to reveal it
prints at `opacity:0`. An entrance built from `@keyframes` with
`animation-fill-mode: forwards` runs at load and has finished by then; one that
waits for a class an `IntersectionObserver` adds waits forever, because in an
export nothing scrolls.

```css
@keyframes rise { from{ opacity:0; transform:translateY(24px) } to{ opacity:1; transform:none } }
.rise { animation:rise .7s ease both; }
.rise:nth-child(2){ animation-delay:.10s }
.rise:nth-child(3){ animation-delay:.18s }
```

Animate freely otherwise. An ambient loop is a screen effect: the export freezes
whatever frame it lands on.

## The house look

Taken from the deck the owner picked as the standard, and it is a reference
rather than a rule — a dark stage, one accent doing every job, gradients rather
than flat fills, three weights of light instead of white-on-black, and the same
furniture on every slide so it reads as a deck and not as a long page.

A deck with no pictures in it is the one thing here that reads as unfinished.
Go and get them before laying anything out.

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
  body{ background:var(--stage); font-family:"Prompt","Noto Sans Thai","Leelawadee UI",Tahoma,sans-serif }

  .slide{ width:1280px; height:720px; position:relative; overflow:hidden;
          padding:78px 120px; display:flex; flex-direction:column; justify-content:center;
          background:radial-gradient(circle at 80% 15%,#242630 0,#101116 28%,#07080a 70%);
          color:var(--text) }
  h1{ font-family:"Kanit",sans-serif; font-size:96px; line-height:1.05; letter-spacing:-.035em }
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

1. Find and download the pictures first.
2. `write` the `.html`; the receipt says where it landed. Reference the pictures
   relatively.
3. `desk open` that path, so the user sees it in the room.

The room's export bar writes `.pptx` (editable), `.pptx` as pictures, `.pdf`,
and `.png`/`.jpg`/`.webp`. There is no tool for it: the user is already looking
at the deck. A request for a PowerPoint is answered by the deck plus a sentence
saying where that button is — nothing here builds a `.pptx` from scratch, and
the one the room writes is made from your HTML anyway.

Nothing of Aetox goes into the file. The deck the user keeps is a plain HTML
file that opens in any browser on any machine, which is why decks are HTML here.
