# Where these came from

47 scenes in 105 files, renamed from studio/brand names to plain descriptions of
what each one actually is — the same reason `aetox-slide-templates` never named
a slide after where its inventory was cross-checked. Originals are untouched
next to this folder (`../html-video/`, `../huashu-design/`), kept as the
provenance trail; everything here is a renamed copy, not a move.

## motion/ — 22 scenes

Source: [nexu-io/html-video](https://github.com/nexu-io/html-video),
Apache-2.0. The file name changed, from the template's marketing name to what it
does — `frame-vignelli` → `bold-portrait-title`, `frame-pentagram-stat` →
`stat-anchor`, and so on for all 22.

**Three edits to the markup, 31 ส.ค. 2569, and each one closed a defect that was
silent.** The licence permits editing outright; this list exists so that nobody
reading a scene later mistakes our line for upstream's.

- **A composition root on `<body>` in all 13 single-file scenes**, carrying
  `data-composition-id`, `data-width`, `data-height`, `data-duration` and
  `data-no-timeline`. Without the frame the renderer produced 1080x1920 for every
  one of them and cropped designs drawn 1920 wide; without the duration `video
  check` could not sample the layout, and four scenes would not render at all.
  The numbers are measured from real renders, not from the files' own comments,
  which overstate every length they give.
- **Tailwind compiled in, in the 6 scenes that used it.** The
  `cdn.tailwindcss.com` script is gone and the CSS it generated for each file
  sits in a `<style>` block at the end of `<head>` — same position the script
  injected it, same pixels (SSIM 1.000000), 5 KB instead of 282 KB fetched per
  render.
- **Nothing changed for GSAP.** The 9 folders still name `cdn.jsdelivr.net` and
  are byte-for-byte upstream's; `video new` rewrites that address in the copy it
  makes, so the shelf keeps a file that opens in a browser as upstream wrote it.

**Nine of them are copied whole, and were not at first (fixed 30 ส.ค. 2026).**
Upstream ships every template as a folder; the first pass took only its
`index.html`, which is the one file that cannot stand alone in those nine. Each
mounts scenes of its own through `data-composition-src`, three of them play
audio off `assets/`, and every one of those referenced files was missing. A
scene like that does not fail loudly — it renders as a frame with its contents
absent. The folders are now byte-for-byte upstream's, renamed:

| upstream folder | here | what came with it |
|:--|:--|:--|
| `frame-vignelli` | `bold-portrait-title/` | 2 compositions |
| `frame-decision-tree` | `decision-flowchart/` | 1 composition |
| `frame-nyt-graph` | `editorial-chart/` | 1 composition |
| `frame-warm-grain` | `grain-texture-hero/` | 3 compositions |
| `frame-kinetic-type` | `kinetic-type/` | 1 composition |
| `frame-play-mode` | `playful-bounce/` | 3 compositions |
| `frame-product-promo-30s` | `product-launch-30s/` | 8 compositions, 4 audio tracks, 3 screenshots, 2 logos |
| `frame-product-promo` | `product-showcase/` | 3 compositions, 3 svg |
| `frame-swiss-grid` | `structured-grid/` | 3 compositions, 1 svg |

Their `template.html-video.yaml` travels with them rather than being stripped,
and that is deliberate: it carries upstream's own licence block and attribution
alongside the default length and frame size, so the credit for those nine lives
inside the folder as well as in this file.

Six of the 22 are themselves derived by nexu-io from two upstream design
skills, credited in nexu-io's own `ATTRIBUTIONS.md` (copied to
`../html-video/ATTRIBUTIONS.md`) — carried through here rather than dropped:

- **statement-title, section-marker, offset-title, split-quote** — style
  presets from [frontend-slides](https://github.com/zarazhangrui/frontend-slides)
  by Zara Zhang (MIT).
- **stat-anchor, radial-node-diagram** — design philosophies from
  [huashu-design](https://github.com/alchaincyf/huashu-design) by alchaincyf
  (MIT), themselves stylistic homages to real studios (Pentagram/Michael
  Bierut, Takram) that nexu-io names as inspiration only, not affiliation.

Not included: `frame-data-rollup` (would have been `number-rollup`) — its
`source_entry` is a Remotion `.ts` component, not an HTML file, so it needs
the Remotion toolchain rather than a browser. Left in `../html-video/` only.

## graphic-scenes/explainer-diagram-poster.html — 1 file, adapted not just copied

Source: [amigoscode/infographics-skill](https://github.com/amigoscode/infographics-skill),
MIT, Copyright (c) 2026 Amigoscode. The source skill's workflow needs a
`GEMINI_API_KEY` (calls Gemini to draw the diagram) plus Playwright
(screenshots the result) — both wrong for Aetox, which calls no paid image
API and already has its own screenshot path (the WebView2 + `CapturePreview`
capture the slide renderer uses). That coupling lives in the *workflow*, not
the template file: `assets/template.html` itself has zero API calls, zero
Gemini/Playwright references — 140 lines of plain HTML with five
`{{PLACEHOLDER}}` tokens.

**Kept as-is:** the frame — fixed 1200×1581 poster, top-left tech icon,
top-right ALL-CAPS title, full-bleed content area between them, bottom bar
with footer text + logo wordmark. One Google Fonts `@import` (fonts-only,
degrades to a fallback face).

**Cut:** `generate-diagram.ts`, `install.sh`, `config.example.json`, the
Amigoscode branding assets, and every Gemini/Playwright step of its
`SKILL.md` workflow — none of that is in the file kept here.

**Adapted — how the placeholders get filled now:** `{{ICON_PATH}}` /
`{{LOGO_PATH}}` are small brand marks, fetched from the real source's own
site via `web_fetch` — the same mechanism `DECISIONS.md` already documents as
how Aetox gets a picture at all ("there is no image search in Aetox and
there never was; what there was is a page fetch that hands back the picture
URLs it saw"). `{{DIAGRAM_SRC}}`, the main illustration, has no Aetox-native
equivalent to Gemini's generation — either `web_fetch` finds an image that
already exists for the exact topic, or the `<img>` is replaced with inline
SVG the model draws directly, consistent with `motion/decision-flowchart`
and `motion/radial-node-diagram` in this same library.

## slide-scenes/, graphic-scenes/, web-scenes/ — 24 files

Source: [huashu-design](https://github.com/alchaincyf/huashu-design) by
alchaincyf (MIT), `assets/showcases/`. These are **static** — no GSAP, no
`@keyframes`, checked directly in the markup — so they sit next to
`aetox-slide-templates`/`aetox-web-templates` in spirit, not next to
`motion/`. Split by medium for the same reason those two skills are separate
skills: a deck, a web page and a standalone graphic don't share a contract.

The source names each scene `{scenario}-{studio}` (e.g. `ppt-pentagram`,
`ainav-takram`) — three real design studios' names (Pentagram, Build, Takram)
used as style labels. Renamed here by what the style actually looks like
instead, since none of those names mean anything on sight otherwise:

| Studio label (source) | Renamed to | Look |
|---|---|---|
| Pentagram | `-editorial` | black/white, Swiss grid, red accent, high contrast |
| Build | `-minimal` | 70%+ whitespace, ultra-thin type, warm-gold hairlines |
| Takram | `-organic` | rounded, soft-tech, natural tones, curved connectors |

Scenario names renamed the same way — `ainav` → `directory-nav`,
`aiwriting` → `writing-tool`, `homepage` → `portfolio-home` — describing the
page's purpose rather than the fictional product huashu-design demoed it with.

Studio names are recorded here as the original design inspiration, exactly as
huashu-design's own license requires attribution for — not a claim that
Aetox is affiliated with Pentagram, Build, or Takram.

## From the hyperframes registry itself (1 ก.ย. 2569)

Source: [heygen-com/hyperframes](https://github.com/heygen-com/hyperframes)
(Apache-2.0), `registry/examples/`. Four full productions vendored whole —
folder shape, GSAP timelines already registered at `window.__timelines`,
already pinned to gsap@3.14.2, engine-checked with 0 errors before landing:

| Scene | Source example | Notes |
|---|---|---|
| `motion/airbnb-deck/` | `registry/examples/airbnb-deck` | 108s, 12 beats, ships its own per-beat SFX (`sfx/*.mp3`, same licence) and a `DESIGN.md` |
| `motion/startup-pitch/` | `registry/examples/startup-pitch` | 84s pitch deck |
| `motion/slideshow-demo/` | `registry/examples/slideshow-demo` | 32s branching slideshow |
| `motion/motion-blur/` | `registry/examples/motion-blur` | 4s title; its "DRIFT" overflow is the design and `check` reports it on purpose |

Their upstream `registry-item.json` metadata was folded into each folder's
`template.html-video.yaml` rather than carried as a second metadata file. Two
more registry examples were inspected and NOT taken: `product-promo` and
`play-mode` are the upstream originals of `product-showcase` and
`playful-bounce` already on this shelf (via hyperframes-student-kit), and
`vscode-theme-visualizer` was deferred because it needs its own build scripts,
which breaks this shelf's copy-and-edit contract.
