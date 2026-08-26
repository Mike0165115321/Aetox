# Reference: Web & App UI

Read alongside `../SKILL.md` when building a website, landing page, web app, component,
or dashboard. The hard rules and self-audit in SKILL.md still apply — this adds the
tells specific to product UI, each with the concrete fix.

## Reliability comes first (a blank page can't be un-slopped)

Learned the hard way: in one test batch, 4 of 10 React apps shipped as **blank pages** and
still passed every code-level review, because nothing rendered them. Rules:

- **It must render before it can be judged.** Open the file in a browser (or headless
  screenshot) and confirm content appears. A page that doesn't mount is a failure regardless
  of how good the code reads.
- **Prefer the most robust medium for a single self-contained file: plain HTML + CSS +
  vanilla JS.** It has no build, no CDN, no transpile step, renders offline, and can't fail to
  mount. For a demo app this is usually enough and is *less* sloppy than shipping a React +
  in-browser-Babel bundle (which no real product does).
- **If you do use React via CDN + Babel-standalone:** never write ESM `import`/`export` in a
  `text/babel` block — it throws `Cannot use import statement outside a module` and the app
  renders blank. Use the globals. And still verify it mounts.
- **Rendering catches data bugs too**, not just crashes — e.g. sample data that makes every
  streak read "0" looks broken even though the code is "correct." You only see it by looking.

## Dashboard & app-skeleton tells

**The default dashboard is instantly recognizable:** dark sidebar → a 4-up KPI stat row →
two-column cards → a donut chart in the top-right → a recent-activity table. Onboarding
defaults to big headline + single CTA + progress dots. If you're producing this exact
arrangement, you're reproducing a memorized template.

- **Don't** open every dashboard with a symmetric 4-KPI band. Show only the metrics that
  drive a decision — often that's 2 or 3, at different visual weights, with the single
  most important one dominant.
- **Don't** make every panel an identical bordered rounded card on a `repeat(N, 1fr)` grid.
  Let the primary view be large and asymmetric; supporting data smaller. Balance by
  **weighting, not mirroring**.
- **Don't** clone a recognizable third-party visual (the GitHub contribution heatmap, a
  donut + legend, TAM/SAM/SOM rings) verbatim. If you need that chart, restyle it into
  your own system (your colors, your radius, your type).

## Cloned-product tokens

A subtler slop mode than "invent a generic look" is **copying a famous product's exact
design tokens** — e.g. Atlassian/Trello's `#0052cc / #172b4d / #5e6c84 / #ebecf0`, or
Linear's `#5E6AD2`. It looks competent but has no identity of its own and reads as
"cloned a known product."

- **Do** derive tokens from *your* brief. If the user has no brand, invent one ownable
  color deliberately (see `toolkit.md`) rather than defaulting to a recognizable product's.

## Fabricated brand & data furniture

Baseline output reflexively invents: a punny one-word SaaS name (Habitat, Ledgerly,
Nimbus, Stillwater), a "Premium plan" badge, a personalized "Good afternoon, {Name}",
a diversity-balanced cast of fake people as monogram avatars, and recognizable-brand seed
data ("Whole Foods", "Spotify Premium", "Chase •••• 4821").

- **Do** use obviously-placeholder, neutral sample data and say it's sample data. Don't
  dress fabrication as real (no fake "updated 2 minutes ago" precision a static demo can't
  know). If you invent a brand name, make it deliberate, not a reflexive pun.

## Second-order app slop (what a de-slopped UI now over-reaches for)

Once purple/Inter/emoji are gone, product UIs converge on a *new* safe look. Watch for it:

- **Warm cream paper + a serif display + a terracotta accent** on an app that isn't editorial
  (a recipe app, a task board). It's tasteful, but it's now the reflex — and it spreads: build
  two apps this way and they're visibly one designer. Pick the ground and face from *this*
  product's job (a task board reads fine cool/dark/neutral; a finance view reads fine
  monochrome), not from the warm-minimal habit. See the anti-monoculture rule in `toolkit.md`.
- **Mono micro-labels on everything** — column headers, eyebrows, timestamps all in a
  monospace face to signal "intentional." Used across every surface it becomes the same cheap
  tell the uppercase-tracked eyebrow was. Use mono where it means *data/code*, not as texture.
- **The self-aware disclaimer** ("sample data — names are made up") on every screen. One honest
  note is fine; a per-screen tic is a new house habit.

## Design for real, hostile content (the biggest craft signal)

AI layouts look great with placeholder content and fall apart on real data. This is the
single most effective way to *not* look generated, because the model normally skips it.

- **Design the states explicitly:** empty, loading, error, success — not just the
  happy path. Empty/error states are the highest-yield "personality injection" points.
- **Pressure-test** with long usernames, a 3-word and a 30-word title, zero items,
  overflowing numbers, and (if relevant) a longer translated string.
- **Voice the system copy.** Not "Error: Invalid input detected." → "That doesn't look
  right — mind checking the date?" One clear CTA in an empty state, never stacked actions.

## Component-level defaults to avoid

- The untouched shadcn card: `rounded-2xl shadow-lg p-6` on everything. Tune per context.
- The colored 3–4px **left-border strip** on a card ("the single most reliable AI tell").
- A hairline 1px border **and** a wide diffuse shadow on the same card — pick the elevation
  intent; don't apply both to everything.
- Frosted glassmorphism / `backdrop-filter` sticky headers and floating pill navs added
  where there's no real layering to communicate.
- Over-rounding: 24px+ radius on small controls; pills for everything.
- The "badge above the H1" + "numbered 1·2·3 step row" + "three feature cards" reflex on
  a landing page. Three-across *by count* is itself a tell — vary it, or earn it.

## A concrete "instead of" table

| Default (slop) | Do instead |
|---|---|
| `sky-400 → indigo-400` gradient CTA | One solid ownable accent; gradient only if it means something |
| `font-family: -apple-system…` / Inter flat | Display face + distinct body face (see toolkit) |
| Every card `border-radius: 16px; padding: 24px` | 4/8/16 radius hierarchy; padding varies by importance |
| `transition: all .15s` + `translateY(-3px)` hover | Transition specific props; hover reflects real affordance |
| 🔥 / 🧘 / 🛒 as icons | One consistent icon set, chosen weight, used with intent |
| "Supercharge your workflow" | The specific thing it does, in a real voice |
| 4 equal KPI cards up top | 2–3 metrics that drive a decision, weighted |
| Symmetric `repeat(3,1fr)` everywhere | Asymmetric layout balanced by weight |
