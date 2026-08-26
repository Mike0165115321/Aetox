# Reference: Presentations & Slide Decks

Read alongside `../SKILL.md` when building a presentation, slide deck, or pitch deck
(HTML slides, or content destined for slides). The hard rules and self-audit in SKILL.md
still apply, this adds the tells specific to decks, each with the concrete fix.

## The deck "house style" (what betrays a generated deck)

- **Kicker on every slide.** A tiny uppercase, wide-tracked, accent-colored eyebrow label
  above the heading on slide after slide ("THE BIG IDEA", "STRATEGY BRIEFING"). This is the
  most repeated AI slide motif. **Fix:** drop it, or use a section marker only at real
  section breaks, never one per slide.
- **The bullet template.** "icon + 3–4 verb-led, equal-length bullets" repeated deck-wide
  is *the* single most reliable deck tell. Human decks vary bullet count, length, and form.
  **Fix:** some slides are one bold sentence; some a single image; some a number. Vary
  ruthlessly. Rarely 3–4 tidy parallel bullets.
- **Uniform titles + thank-you closer.** "Title Case, ~6 Words: With a Subtitle" on every
  slide, plus a "Thank you for your attention" final slide. **Fix:** sentence-case, varied
  headers that say something specific; end on a real ask or a memorable line, not "thank you".
- **Wall-of-text slides.** Cramming a written report onto slides, the audience can't scan
  it in 5–10 seconds. **Fix:** one idea per slide; move detail to speaker notes.
- **Same invisible grid on every slide.** Identical margins, text widths, card layouts,
  radii, "technically clean but visually flat." **Fix:** let some slides be full-bleed,
  some centered, some split; vary the composition to match the beat of the content.
- **Fake variety tricks.** Alternating even/odd slide background tints and decorative
  numbered corner badges ("02"…"09") to *simulate* design variation. **Fix:** delete them;
  create variety through real compositional differences instead.
- **Formulaic openers.** "In today's fast-paced world…", "As businesses navigate
  unprecedented change…" appear in ~40% of AI decks. **Fix:** open on a specific, concrete
  hook, a real number, a sharp question, a scene.

## The second-order deck kit (what a *de-slopped* deck now over-reaches for)

Once the obvious tells are gone, generated decks converge on a new, "tasteful" house style.
In testing, 5/5 rebuilt decks shared this exact kit, which makes the kit itself the tell:

- **A high-contrast editorial serif** (Fraunces / Playfair) as the display face on every deck,
  regardless of topic. **Fix:** the face should come from the subject and audience, a Swiss
  grotesk, a mono, a slab, a bold condensed, a humanist sans are all on the table. Don't reach
  for the editorial serif by reflex; across a batch of decks, *vary the family*.
- **A warm canvas every time**, cream, or warm near-black. **Fix:** cool, white, saturated,
  and true-dark grounds are all valid; pick from the brief, not the habit.
- **The corner page-counter + arrow-nav chrome**, "01 / 10", a dot row, a thin progress bar,
  "← / → to move", or two arrow buttons bottom-right. Present on essentially every AI deck.
  **Fix:** most real decks don't render a visible pager on the title slide. If you need
  navigation, make it quiet and don't pair it with a decorative counter; better, let the
  content imply progress.
- **The "colored last word" headline**, a black headline with its final word in the accent
  color ("explained *from scratch*", "It's the *route*"). A signature move; drop it or use
  emphasis that isn't the same trick twice.
- **A repeated wordmark + a "sample / figures illustrative" disclaimer** at the foot of every
  slide. **Fix:** one honest note where it matters, not a per-slide tic.

## The narrative-arc template

Decks default to a memorized arc regardless of topic:
- **Pitch:** Title → Problem → Solution → How It Works → Market (TAM/SAM/SOM rings) →
  Traction → Business Model → Competition (a check/cross table where you win every row) →
  GTM → Team (monogram avatars, "ex-BigCo" one-liners) → Ask.
- **All-hands:** Title → Agenda → KPIs → Growth chart → Retention → What we shipped →
  Wins → **"Where we fell short"** (the obligatory manufactured-vulnerability beat) →
  Roadmap ("three bets") → Team → "Heads down, momentum up" close.

**Fix:** the arc should be argued from *this* topic and audience, not filled in. Cut slides
that exist only because the template has them. If a competitor table can't honestly show you
losing any row, it's not a real analysis. Don't force "three bets / three wins / three
anything", use the number the content actually has.

## Data & copy on slides

- **No fabricated-precise stats.** "28% of workdays", "3.2 days remote", "+9% vs 2019
  baseline" with no source is a top tell. Use real figures with a source, or label clearly
  as illustrative, and don't over-fit hand-drawn SVG bars to a smooth exponential.
- **No anonymous pull quotes.** A quote credited to "the reframe every leader should
  internalize" (i.e., nobody) reveals it was generated. Quote a real, named source or cut it.
- **Copy rules from SKILL.md apply doubly on slides:** no benefit-speak, no "not X, it's Y",
  no em-dash cadence, no coined jargon ("Northwinders"), no rule-of-three reflex.

## Visual system for decks

- **Typography carries the deck.** One distinctive display face for slide titles + a clean
  body face does more than any other single choice. Not the system stack; not Inter flat.
- **Commit to a topical, ownable palette**, and make it functional. (The bees deck earned
  points precisely by committing to honey/amber over the default slate+indigo.) Avoid the
  Tailwind `slate` dark canvas + `sky/indigo` accent default.
- **Real iconography or none.** Not emoji; not the same flat single-color line icon on every
  slide from one library. If you use icons, choose a weight/style and use them sparingly.
- **`clamp()` on every text size** and negative letter-spacing on every heading are CSS-level
  tells, use a real type scale and set tracking deliberately, per role.

## Deck pre-ship check (in addition to the SKILL.md audit)

- [ ] Is there a kicker/eyebrow on more than section-break slides? → remove.
- [ ] Do most slides have 3–4 equal bullets? → break the rhythm.
- [ ] Is there a "Thank you for your attention" slide? → replace with a real close.
- [ ] Any slide the audience can't absorb in ~8 seconds? → split or cut.
- [ ] Any invented precise stat or unnamed quote? → source it or cut it.
- [ ] Could this template hold any other topic unchanged? → it's not authored yet.
- [ ] Is the display face an editorial serif (Fraunces/Playfair) again? → across decks, vary it.
- [ ] Is there a corner pager, dot row, progress bar, or ← → arrow chrome? → quiet it or cut it.
- [ ] Does the headline color only its last word? → that's the signature move; change it.
- [ ] Warm cream / warm-dark canvas by reflex? → a cool/white/saturated ground may fit better.
- [ ] Building several decks? Do they look like one hand? → diverge ground + face + accent.
