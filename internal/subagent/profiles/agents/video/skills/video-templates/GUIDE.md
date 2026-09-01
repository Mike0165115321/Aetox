# The long answers

`SKILL.md` states each rule in one line; this file is the why and the
measurements behind them. Open the section you are about to act against,
not the whole file.

## The contract these are written to

**Most of these are CSS `@keyframes` and nothing else** — no library, no build
step, no bundler, which is why they open in a browser as fast as they render.

**13 of the 26 motion scenes hold to that.** The other **13** —
`bold-portrait-title`, `decision-flowchart`, `editorial-chart`,
`grain-texture-hero`, `kinetic-type`, `playful-bounce`, `product-launch-30s`,
`product-showcase`, `structured-grid`, and since 1 ก.ย. `airbnb-deck`,
`startup-pitch`, `slideshow-demo`, `motion-blur` — drive their animation with
**GSAP**.
Checked, not assumed: each of the 9 registers its timeline at
`window.__timelines["..."]`, the exact hook Hyperframes seeks to render
frame-by-frame, so this is not a scene built for the wrong engine. It is the
second of the two things the renderer can drive, CSS keyframes being the first.

The file itself is on this machine, not on a CDN. The library still spells the
`cdn.jsdelivr.net` address, because a scene on the shelf has to open in a
browser as it is — but `video new` rewrites it to `vendor/gsap.min.js` in the
copy it makes, from the pinned copy the งานวิดีโอ page installs. If that has not
been installed the address is left alone, and then those 9 need the network up
at render time or they produce a still picture with nothing saying why.

**6 of the 22** — `bar-chart-counter`, `cinematic-light-leak`, `glitch-title`,
`liquid-gradient-hero`, `logo-outro`, `typewriter-cursor` — lay themselves out
with **Tailwind** utility classes. Upstream fetched the browser JIT from
`cdn.tailwindcss.com` on every render; these six now carry the CSS that script
generated for them, about 5 KB each, in a `<style>` block at the end of
`<head>`. Same pixels (SSIM 1.000000, measured 31 ส.ค.), nothing fetched.

**What that costs you:** the utility set in those six is now fixed to the
classes each scene already uses. A Tailwind class you add that is not in the
block does nothing at all — it will not error, the element simply will not move.
Write plain CSS for anything new, which is what the other 16 scenes do.

## Thirteen of them are folders, not files

The same 13 that drive their motion with GSAP are **compositions**: a host
`index.html` that mounts smaller scenes of its own through
`data-composition-src`, and in three cases plays audio tracks laid out on a
timeline with `data-start`, `data-duration` and `data-volume`. They travel as
folders because they have to:

```
motion/structured-grid/
  index.html                    the host, and the file you open
  compositions/intro.html       the scenes it mounts
  compositions/graphics.html
  compositions/captions.html
  assets/swiss-grid.svg         what those scenes draw
  template.html-video.yaml      its own licence block, default length and frame
  poster.svg
```

Open `index.html`. Change the words there or in the composition it mounts, never
both hoping they agree. Copy the **whole folder** when you copy one of these:
its host alone renders as an empty frame with the parts missing, which is a
failure that looks like a working render.

`product-launch-30s` is the fullest example in the library and worth reading
before writing anything long: eight mounted scenes, four audio tracks, and
thirty seconds of timing somebody already argued about.

**`__VIDEO_SRC__` and `__VIDEO_DURATION__` are not broken markup.** They are the
two slots the engine fills when it scaffolds a project from the template — the
footage you point it at, and the length you asked for. Leave them alone in the
library; they disappear in the copy that renders.

## How the renderer decides a scene's length

**The renderer freezes the page, waits for the fonts, then steps the clock.**
That is what makes a headless machine able to produce smooth motion without
watching it play in real time. It also means the length of a scene is the length
of its keyframes: an animation that finishes in 2 seconds inside a 6 second
scene leaves 4 seconds of a held frame, and one still moving at the end is cut
off mid-move.

**All 13 single-file scenes now state their length on `<body>`**, beside the
frame, on the same composition root:

```html
<body data-composition-id="scene" data-no-timeline
      data-width="1920" data-height="1080" data-start="0" data-duration="3.0">
```

That is the one place the length lives in those 13; change the number to change
the clip. It is stated rather than derived for two reasons. Four of them —
`glitch-title`, `typewriter-cursor`, `liquid-gradient-hero` and
`cinematic-light-leak` — have no end to derive: the first three loop for ever
and the fourth has no `@keyframes` at all, and the renderer refuses all four
with "Composition has zero duration". The other nine derive fine at render time
and still could not be **checked**: `video check` reports "Could not determine
composition duration" without it, so the cheap half of the loop was returning
nothing. `data-no-timeline` is the third piece, and it stops the renderer
waiting 45 seconds for a GSAP timeline that is never coming.

The numbers in those 13 are measured, not intended — each is what a real render
produced on the owner's machine, rounded up to a tenth of a second so nothing is
cut mid-move.

Seven of the motion files write their timeline into a header comment
(`Timeline (5s @ 60fps): corner marks draw -> eyebrow fades -> ...`). **Read it
for the order, not for the length.** Every one of those seven numbers is the
length its author meant; every one of them is longer than what the renderer
actually produces, because the last keyframe lands earlier than the comment
says. Measured on this machine, 31 ส.ค.: `minimal-hero` says 5s and renders
3.8s, `offset-title` says 4.5s and renders 2.8s, `section-marker` says 4s and
renders 2.2s. The `Length` column in SKILL.md's table is the measured number,
and it is the one to quote to a user.

## Google Fonts is the one thing still fetched at render time

21 of the 26 motion scenes pull typefaces over a `<link>` — every one except
`bold-portrait-title`, `structured-grid`, `startup-pitch`, `slideshow-demo`
and `motion-blur`; counted 31 ส.ค. and recounted 1 ก.ย. when four scenes
arrived from the hyperframes registry (only `airbnb-deck` among them fetches),
rather than estimated, because the number here said 17 for a while and was
wrong. Unlike
Tailwind and GSAP this one cannot be cured by putting a file on the machine: the
renderer asks `fonts.googleapis.com` for a stylesheet on every render, subsetted
to the exact characters on the page, and only the font binaries it names are
cached afterwards. Change the words and it asks again.

If the request fails the frames record the fallback face. That is not a crash
and not an error message: it is a scene that came out looking wrong for a reason
nothing in the output explains. Check the render, do not assume it.

## ไทย: no scene in this library has a Thai typeface

Counted 31 ส.ค. over the original 47 scenes: 26 font families between them and
not one covers Thai; the four scenes vendored 1 ก.ย. add only `Nunito Sans`
(airbnb-deck), which does not either. Two cover Chinese (`Noto Sans SC`, `Noto Serif SC`) and
the rest are Latin. There is no `IBM Plex Sans Thai`, no `Sarabun`, no `Noto Sans
Thai`, nowhere.

**What that means for a Thai brief, which is most of them.** Thai text does not
come out blank — Chrome falls back to whatever Thai face the machine has, usually
Leelawadee UI or Tahoma. So the render succeeds, the words are readable, and the
typography is somebody else's: a system UI face sitting inside a layout drawn
around Shrikhand or Libre Baskerville, at a weight and a rhythm that were never
chosen. It looks wrong in a way that is obvious to a person and invisible to
every check in this pipeline. `video check` passes it. `video_ocr` reads the
letters back correctly and says nothing.

**So say it, before the render.** When the copy is Thai, tell the user that the
scene's own typeface does not cover Thai and that the headline will land in a
system font. Then either agree on that, or add a Thai family to the scene's
`<link>` and set it first in the `font-family` stack for the Thai elements —
`Noto Sans Thai` and `IBM Plex Sans Thai` sit closest to what this library
already uses. Do not quietly render and hand it over.

## Frame size is stated in every file, and has to be

Every scene declares its frame on its composition root: on `<body>` for the 13
single-file scenes, on the root `<div>` for the 13 folders. 25 of the 26 say
`data-width="1920" data-height="1080"`; `bold-portrait-title` says 1080x1920 and
is the only one drawn portrait from the start.

**Stated because the renderer does not guess it.** A scene with no declared
frame renders at 1080x1920 whatever its CSS says — it crops a design drawn 1920
wide, and nothing in the output says so. Measured 31 ส.ค. on the owner's
machine: that is how all 13 single-file scenes came out until the frame was
written into them.

Changing the aspect is those two numbers, plus whatever the scene's own CSS
pins. Six of them — `glitch-title`, `typewriter-cursor`, `bar-chart-counter`,
`logo-outro`, `liquid-gradient-hero`, `cinematic-light-leak` — lay themselves
out fluidly, so for those it is only the two numbers. The other 15 draw their own
1920x1080 box in CSS as well, and moving the frame without editing that rule
leaves the scene drawing its old box inside the new one. The `Frame` column in
SKILL.md's table says which is which.

## The sample copy is real copy, not placeholders

There is no `{{ }}` in any of these. Every headline, figure and label is a
finished sentence somebody wrote to show the layout working, which means two
things.

Replace all of it. A scene that ships with one line of the sample copy still in
it is the most visible kind of unfinished work, and it is always the line nobody
looked at because it was small.

And respect the length it was drawn for. These layouts were built around text of
roughly the length that is in them. A headline three times longer does not wrap,
it overflows the frame, and the markup will not tell you: only opening it will.
Preview before you render, and read the render back — `video render` with
`proof: true` OCRs its own output in the same reply.

## The explainer diagram's empty middle

The explainer diagram's middle is empty on purpose — no `{{ }}` placeholder,
just a box sized to whatever fills it. Its two small brand marks (icon, logo)
are the kind of image `media_fetch` saves straight off a real site; the
diagram itself is the one thing in this whole library nothing here draws for
you; either find one that already exists for the exact topic, or draw it as
inline SVG the way `decision-flowchart` and `radial-node-diagram` do. Aetox
generates no image via any paid API — that choice was made once, generally,
and applies here rather than being decided again per scene.

## Where these came from

`CREDITS.md` beside this file records the source repository, the licence and the
upstream design lineage of every one of the 51 files, including six that were
themselves derived from other design skills. The licences are Apache-2.0 and
MIT. Both allow commercial use and both require the attribution to travel with
the work.

So the credit file is not paperwork, it is part of the library. If a scene is
copied out into somewhere else, its line in `CREDITS.md` goes with it.
