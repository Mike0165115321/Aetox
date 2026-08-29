---
name: aetox-web-templates
description: เทมเพลตหน้าเว็บจริงที่ก๊อปไปใช้ได้เลย 18 ส่วน ตั้งแต่ nav hero ราคา FAQ ไปจนถึงแดชบอร์ด ตาราง ฟอร์ม หน้า 404 บทความ และหน้าเอกสาร ไฟล์เดียวจบ responsive ไม่มี framework ไม่มี CDN ไม่ต้อง build ใช้ตอนกำลังจะสร้างหน้าเว็บหรือหน้าจอแล้วไม่อยากประดิษฐ์โครงขึ้นใหม่ทุกครั้ง
---

# Web templates

Markup that gets copied, not advice about markup. **Web pages only.**

The name says the medium on purpose, the same way `aetox-slide-templates` does.
A deck is one file at a fixed 1280x720 that an off-screen renderer prints; a web
page is responsive and lives in a browser somebody resizes, zooms and reads with
a screen reader. A layout correct for one is wrong for the next, so they are two
skills and neither borrows from the other.

## Why this exists

Six skills already describe web and UI work: `aetox-frontend-design` decides the
look, `aetox-ui-design` carries nine implementation guides, `aetox-shadcn` and
`aetox-radix-to-base` cover component libraries, `aetox-design-system` the
tokens, `aetox-design` the pictures. Sixty-six files between them, and on
29 ส.ค. not one of them was markup: every file was prose, a CSV or a licence.

That is the same gap the slide tables had, measured the same way. Descriptions
tell a model what a hero *should be*. They do not stop it building a different
hero every time, which is what a page assembled from scratch on every run
actually looks like.

## The sections

Paste `page-shell.html` first; it carries the tokens, the reset, the landmarks
and dark mode that every section below assumes. Then paste sections into its
`<main>` in the order the page needs.

| Section | File | Reach for it when |
|---------|------|-------------------|
| Page shell | `sections/page-shell.html` | Always first. Tokens, reset, skip link, light and dark. |
| Nav | `sections/nav.html` | Site header. Opens on mobile with no script. |
| Hero | `sections/hero.html` | The thesis of the page, above the fold. |
| Logo bar | `sections/logo-bar.html` | Named social proof under the hero. |
| Features | `sections/features.html` | What it does. Scales 3 to 12 with no new layout. |
| How it works | `sections/how-it-works.html` | Ordered steps, where the order is the meaning. |
| Stats | `sections/stats.html` | Numbers, each with its source. |
| Testimonial | `sections/testimonial.html` | One quote, attributed to a real person. |
| Pricing | `sections/pricing.html` | Up to three tiers, one marked. |
| FAQ | `sections/faq.html` | Objections. Accordion with no script. |
| CTA band | `sections/cta-band.html` | Mid-page and end-of-page conversion block. |
| Newsletter | `sections/newsletter.html` | Email capture or short contact form. |
| Footer | `sections/footer.html` | Closure and the alternative paths. |
| Dashboard shell | `sections/dashboard-shell.html` | Sidebar, topbar, content well for an app. |
| Stat cards | `sections/stat-cards.html` | The number row at the top of a dashboard. |
| Data table | `sections/data-table.html` | Tabular data that scrolls on a narrow screen. |
| Form card | `sections/form-card.html` | Sign in, sign up, any short form. |
| Empty state | `sections/empty-state.html` | A list with nothing in it yet. |
| Error page | `sections/error-page.html` | 404, 500, anything that went wrong. |
| Article | `sections/article.html` | A post or case study, read rather than scanned. |
| Docs page | `sections/docs-page.html` | Sidebar, prose, on-this-page rail. |

**Page order for a marketing page**, which is the visitor's own decision
journey: nav → hero → logo bar → features → how it works → testimonial →
pricing → FAQ → CTA band → footer. Cut freely; do not reorder without a reason,
because each section answers the question the one before it raises.

## The contract every template obeys

- **One self-contained file.** No framework, no CDN, no build step, no
  `node_modules`. The page opens from disk, from a USB stick, from an email
  attachment. Same reason a deck is one file.
- **Responsive without breakpoint bookkeeping.** `clamp()` for type and space,
  `repeat(auto-fit,minmax(min(280px,100%),1fr))` for grids, so the column count
  follows the content and the viewport rather than a list of widths somebody has
  to maintain. Media queries appear only where the layout genuinely changes
  shape, such as the dashboard sidebar becoming a top scroller.
- **No script unless the semantics cannot be had without it.** The FAQ is
  `<details>`, the mobile menu is a checkbox. Both are keyboard operable and
  survive a JS error, which a scripted version of either does not.
- **Real landmarks and real elements.** `<nav>`, `<main>`, `<footer>`,
  `<blockquote>` with `<cite>`, `<ol>` when the order is the meaning, `<table>`
  with `scope` on every header. This is not decoration; it is what a screen
  reader navigates by.
- **Every class is namespaced `w-`.** `aetox-frontend-design` warns that
  generated CSS most often breaks when a class selector and an element selector
  fight over the same padding. Two templates pasted into one page must not be
  able to reach into each other.
- **Light and dark from tokens**, defined once in the shell. No template hard
  codes a colour.
- **Accessible by construction.** Visible focus ring, a skip link, labels that
  stay visible when the field is filled, `prefers-reduced-motion` respected, and
  status never carried by colour alone.

## Using one

Copy the section, change the words, delete what the page does not need. A
template is a starting composition, not a form to fill in. Three identical
card rows down one page is the tell `aetox-anti-slop` is written about: vary
the cell count, let one section be a single sentence, let another be a number.

Read `aetox-frontend-design` before choosing the look, and `aetox-design-system`
for the token layers if the page needs a palette of its own. This skill decides
none of that. It only means you do not rebuild the skeleton of a pricing table
from memory again.

## Not in here

Video, because Aetox produces none; `video_ocr` reads one. Slides, which are
`aetox-slide-templates` and a different contract entirely. Documents and sheets,
which leave through `doc_write` and `sheet_write` as OOXML and are not HTML at
all. If any of those earn templates, they earn a skill of their own rather than
a folder in here: a template nobody can tell the medium of is a template that
gets pasted into the wrong file.
