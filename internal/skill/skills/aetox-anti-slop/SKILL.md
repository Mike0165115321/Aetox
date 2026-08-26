---
name: aetox-anti-slop
description: กัน UI/หน้าเว็บ/แอป/แดชบอร์ด/สไลด์ ออกมาหน้า "AI ปั่น" (ม่วง-คราม gradient, ฟอนต์ Inter/system, การ์ดมนเหมือนกันหมด, กริดฟีเจอร์ 3 ไอคอน, อีโมจิเป็นไอคอน, kicker ทุกสไลด์, ก็อปปี้ขายของ) อ่านก่อนเขียนมาร์กอัปบรรทัดแรก เลือกสี เลือกฟอนต์ หรือวางสไลด์ บังคับให้ตัดสินใจดีไซน์แบบตั้งใจ เจาะจง มีแบรนด์
source: https://github.com/Vinayak-Shukla-03/anti-ai-slop
license: MIT
copyright: Copyright (c) 2026 Vinayak Shukla. Full terms in LICENSE
---

# anti-ai-slop

Make UI and presentations that don't read as machine-generated.

## Why this happens (so you can fight the actual cause)

An LLM emits the most probable token. For an *unconstrained* visual choice, "most
probable" is the statistical average of every Tailwind tutorial scraped 2019–2024 —
so with no constraint you deterministically produce **the same purple-gradient,
Inter-font, rounded-card page every time.** This is *distributional convergence*:
"technically clean but emotionally invisible." Designers and dev communities can spot
it on sight, and they've catalogued exactly how (see `references/` for the evidence).

**The cure is always the same shape: make a specific, intentional decision wherever you
would otherwise reach for the default.** Restraint + intention + specificity. The best
products (Linear, Notion, Stripe) are distinctive because they make *fewer* choices,
each one deliberate — not because they pile on trends.

## The hard rules (never do these by default)

These are the loudest tells. Treat each as prohibited unless the user explicitly asks
for it:

1. **No unbranded purple/indigo/violet, and no reflexive gradient.** Not `#6366f1`,
   `#8b5cf6`, `#818cf8`, `sky-400 → indigo-400`, no 135° gradient on a logo chip, no
   glowing aurora "orbs" behind a hero. Pick one ownable, slightly-unexpected accent.
2. **Never ship the system font stack or Inter/Geist/Poppins flat.** Choose a real
   **display face + a distinct body face**. Typography is the #1 fastest way out of
   slop — a viewer registers it first. (Self-host or a deliberate, characterful Google
   Font. Sentence case, not Title Case.)
3. **No uniform-everything.** Vary border-radius (a 4/8/16 hierarchy), vary padding, so
   *size and space signal priority*. Not every surface is a bordered rounded card. No
   cards-inside-cards. No single 0.1-opacity shadow on everything.
4. **No emoji as icons.** Use a real, consistent (ideally customized) icon set with a
   chosen weight. Never 🔥/🧘/🛒 as UI glyphs, never rotated-square CSS "diamond" bullets.
5. **No `transition: all` and no blanket fade-in.** Animate specific properties with
   tuned easing, and only to signal state, direct attention, or express brand. Real
   hover/focus/active states; nothing that "snaps" or fades in for decoration.
6. **No benefit-speak copy.** Ban: Elevate, Unlock, Supercharge, Seamless(ly), Empower,
   Streamline, Leverage, "all-in-one", "Build the future of", tapestry/landscape/beacon/
   journey. No "not just X — it's Y". No forced rule-of-three. No em-dash pile-ups or
   decorative "·" middots. Write like **one specific human**; be specific, not aspirational.
7. **No fabricated precision.** Don't invent exact stats ("38% MoM", "3.2 days") with no
   source, no anonymous pull quotes attributed to nobody, no fake "updated 2 minutes ago".
8. **Break the template skeleton.** Don't emit Hero → 3-icon grid → social proof → CTA →
   footer by reflex. Let the *specific* content and audience drive structure.
9. **No second-order monoculture — and if you make a set, force divergence.** The escape
   hatches in this skill have their *own* average, and unguided you will converge on it: **warm
   cream/dark paper + a high-contrast serif display (Fraunces/Playfair) + an earthy accent
   (terracotta/clay/ember/radish) + a corner page-counter + a "colored last word" headline +
   mono micro-labels on everything + a cute "sample data" disclaimer.** Treat that whole kit as
   banned-by-default, exactly like purple/Inter. When you produce **more than one** artifact
   (a deck is many slides; a project is many screens), they must **diverge on the primary
   axes** — ground (light/dark/saturated/neutral), display face *family* (grotesk/serif/mono/
   humanist/display), accent hue, and stance. If two pieces could be mistaken for the same
   designer's, you've re-converged. Derive each from *its own* brief.

## Ship-blockers — verify by rendering, never assume

Beautiful code that doesn't run is worth zero, and a blank page trivially "doesn't look AI"
while being useless. Before you call any UI done:

- **Open it in a real browser and look at it.** Confirm content actually renders (for React,
  that `#root` is not empty). This is not optional — in testing, 4 of 10 apps shipped as
  blank pages that passed every code-level check.
- **In a CDN-React + Babel-standalone (`text/babel`) page, never use ESM `import`/`export`.**
  It throws `Uncaught SyntaxError: Cannot use import statement outside a module` and React
  never mounts. Use the globals (`React`, `ReactDOM`, hooks off `React`). Better: if the
  environment allows, use a real bundler — in-browser Babel is itself a fragile smell.
- **Check the console for errors.** One uncaught error usually means a blank screen.

## The method (do this every time)

1. **Extract a brief first.** Before styling, name (even to yourself, in one line each):
   the user, the core job, the emotional tone, and one real constraint. Every visual
   choice should trace to one of these. "Modern dashboard" → slop. "Urgent tasks first,
   calm and focused, one primary action, for beginner freelancers" → specific.
2. **Commit to a distinctive foundation** *before* laying out: one type pairing, one
   dominant color + one ownable accent (functional, meaning-bearing), one spacing/radius
   scale, one aesthetic stance. Write them down; reuse them; don't let defaults creep back.
3. **Design against real, hostile content** — long names, empty/loading/error states,
   truncated text — not lorem/happy-path. The seams are where craft shows.
4. **Self-audit before shipping** (below). If you can't answer "why this, not the
   default?" for type, color, and layout, you haven't escaped the average yet.

## Pre-ship self-audit

Run this checklist against the output. Any "yes" in the left column is a tell to fix:

- [ ] Is the accent purple/indigo, or is there a decorative gradient? → repick.
- [ ] Is the font the system stack or Inter/Geist, used flat? → choose a real pairing.
- [ ] Same radius + padding on every element? → introduce hierarchy.
- [ ] Any emoji-as-icon or diamond bullets? → real icon set.
- [ ] `transition: all` / one fade-in everywhere / dead hovers? → purposeful motion.
- [ ] Any banned buzzword, em-dash pile-up, or "not X — it's Y"? → rewrite specific.
- [ ] Invented precise stats or anonymous quotes? → real data or cut.
- [ ] Is the layout the default skeleton / all symmetric grids? → weight, don't mirror.
- [ ] Could these exact tokens be pasted onto any other product unchanged? → not distinctive.
- [ ] Did you reflexively reach for warm cream/dark paper + a terracotta/clay/ember accent +
      a high-contrast serif (Fraunces/Playfair) + a corner page-counter + a "colored last
      word" headline? → that's the *second-order* slop (this skill's own overused escape
      hatch). Repick from the brief; see the anti-monoculture rule atop `references/toolkit.md`.
- [ ] **Set check:** if this is one of several artifacts (multiple slides, screens, or a batch),
      do they share a ground + display-face family? → force divergence; each derives from its
      own brief. Two pieces that look like one designer = re-converged.
- [ ] **Mechanical copy grep** — search the source for each and rewrite any hit:
      `isn't` / `it's` (the "not X — it's Y" flip), ` — ` pile-ups (3+ em-dashes),
      `Elevate|Unlock|Supercharge|Seamless|Empower|Streamline|Leverage`, `~` before a fake
      stat (`~2.1M`), `Good morning|Good afternoon, {Name}`, and the disclaimer tic
      (`sample data|names are made up|figures illustrative`) if it appears on more than one
      artifact. Don't eyeball this — actually grep.

## Medium-specific guidance

- Building a **website / app / component / dashboard** → also read
  `references/web-ui.md` (dashboard-skeleton tells, product-clone tokens, real-state design).
- Building a **presentation / deck / slides** → also read
  `references/presentations.md` (kicker-every-slide, bullet-rhythm, thank-you-slide tells).
- Need concrete **swap-in palettes, font pairings, and scales** → `references/toolkit.md`.

The point is never "follow a different template." It's to make decisions a statistical
average would never make — and to be able to say why.
