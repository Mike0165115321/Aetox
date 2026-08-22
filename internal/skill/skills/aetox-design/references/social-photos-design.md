# Social Photos Design Guide

Design social media images by writing HTML/CSS and exporting a picture of it.
In Aetox the export runs through the slides room, so read `aetox-slides`
before writing the file; `aetox-brand` and `aetox-design-system` supply the
voice and the tokens.

## Platform Sizes

| Platform | Type | Size (px) | Aspect |
|----------|------|-----------|--------|
| Instagram | Post | 1080 x 1080 | 1:1 |
| Instagram | Story/Reel | 1080 x 1920 | 9:16 |
| Instagram | Carousel | 1080 x 1350 | 4:5 |
| Facebook | Post | 1200 x 630 | ~1.9:1 |
| Facebook | Story | 1080 x 1920 | 9:16 |
| Twitter/X | Post | 1200 x 675 | 16:9 |
| Twitter/X | Card | 800 x 418 | ~1.91:1 |
| LinkedIn | Post | 1200 x 627 | ~1.91:1 |
| LinkedIn | Article | 1200 x 644 | ~1.86:1 |
| Pinterest | Pin | 1000 x 1500 | 2:3 |
| YouTube | Thumbnail | 1280 x 720 | 16:9 |
| TikTok | Cover | 1080 x 1920 | 9:16 |
| Threads | Post | 1080 x 1080 | 1:1 |

## Workflow

### Step 1: Activate Project Management

Invoke `project-management` skill to create persistent TODO tasks via Claude's native task orchestration. Break down into:
- Requirement analysis task
- Idea generation task(s)
- HTML design task(s) — can parallelize per size/variant
- Screenshot export task(s) — can parallelize per file
- Report generation task

Spawn parallel subagents for independent tasks (e.g., multiple HTML files for different sizes).

### Step 2: Analyze Requirements

Parse user input for:
- **Subject/topic** — what the social photo represents
- **Target platforms** — which sizes needed (default: Instagram Post 1:1 + Story 9:16)
- **Visual style** — minimalist, bold, gradient, photo-based, etc.
- **Brand context** — read from `docs/brand-guidelines.md` if exists
- **Content elements** — headline, subtext, CTA, images, icons
- **Quantity** — how many variations (default: 3)

### Step 3: Generate Ideas

Create 3-5 concept ideas that:
- Match the input prompt/requirements
- Consider platform-specific best practices
- Vary in composition, color, typography approach
- Align with brand guidelines if available

Show the ideas to the user and wait for a pick before designing. Say them in
chat, numbered, in one message — there is no question widget here, and
designing all of them first is how an hour goes into the three nobody wanted.

### Step 4: Design HTML Files

Read these first, in this order:

1. **`aetox-brand`** — the brand's colours, type and voice, from the user's own
   guidelines file.
2. **`aetox-design-system`** — the tokens: spacing, typography scale, palette.

Then lay it out yourself. Variety comes from the brief and the platform, not
from picking a different helper at random.

For each approved idea + each target size, create an HTML file:

```
output/social-photos/
├── idea-1-instagram-post-1080x1080.html
├── idea-1-instagram-story-1080x1920.html
├── idea-2-instagram-post-1080x1080.html
├── idea-2-instagram-story-1080x1920.html
└── ...
```

#### HTML Design Rules

- **Viewport** — Set exact pixel dimensions matching target size
- **Self-contained** — Inline all CSS, embed fonts via Google Fonts CDN
- **No scrolling** — Everything fits in one viewport
- **High contrast** — Text readable at thumbnail size
- **Brand-aligned** — Use extracted brand colors/fonts
- **Safe zones** — Critical content within central 80% area
- **Typography** — Min 24px for headlines, min 16px for body at 1080px width
- **Visual hierarchy** — One focal point, clear reading flow

#### HTML Template Structure

```html
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width={WIDTH}, initial-scale=1.0">
  <link href="https://fonts.googleapis.com/css2?family={FONT}&display=swap" rel="stylesheet">
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    html, body {
      width: {WIDTH}px;
      height: {HEIGHT}px;
      overflow: hidden;
      font-family: '{FONT}', sans-serif;
    }
    .canvas {
      width: {WIDTH}px;
      height: {HEIGHT}px;
      position: relative;
      /* Background: gradient, solid, or image */
    }
    /* Design tokens from brand/design-system */
  </style>
</head>
<body>
  <div class="canvas">
    <!-- Content layers -->
  </div>
</body>
</html>
```

### Step 5: Export the picture

Windows ships Edge, and Edge renders a page to a PNG at any size you ask for.
Measured on 2026-08-22: an 1080x1080 page came back as a 1080x1080 PNG.

```powershell
& "C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe" --headless `
  --disable-gpu --hide-scrollbars --window-size=1080,1080 `
  --virtual-time-budget=3000 `
  --screenshot="C:\full\path\to\out.png" "file:///C:/full/path/to/in.html"
```

Three things are not optional:

- **`--screenshot` must be a full path.** A relative one fails with
  "Access is denied" and writes nothing — the run looks like it worked.
- **`--window-size` must match the page's own size**, or the picture is a crop
  of it.
- **`--virtual-time-budget`** is the wait for web fonts and images. Without it
  the capture can land before they arrive, and the fallback font is what ships.

Chrome works with the same flags if it is installed; Edge is the one that is
always there. Playwright and Puppeteer are not installed and are not worth
installing for this.

For anything 16:9, the slides room is the other route: write it as a
single-slide deck and use the room's export bar. That renders at exactly
1280x720 (`aetox-slides`), which is a YouTube thumbnail and is not any other
size on the table above.


### Step 6: Verify & Fix Designs

`read` the exported PNG. It hands the picture to the model, so this step is
looking at it, not inferring from the HTML that produced it:

1. Open exported screenshots and check for layout/styling issues
2. Verify: fonts rendered correctly, colors match brand, text readable at thumbnail size
3. Check: no overflow, no cut-off content, safe zones respected, visual hierarchy clear
4. If issues found → fix HTML source → re-export screenshot → verify again
5. Repeat until all designs pass visual QA

**Common issues to check:**
- Fonts not loaded (fallback to system fonts)
- Text overflow or clipping
- Elements outside safe zone (central 80%)
- Low contrast text (below WCAG AA 4.5:1)
- Misaligned elements or broken layouts

### Step 7: Generate Summary Report

Save report to `plans/reports/` with naming pattern from session hooks.

Report structure:

```markdown
# Social Photos Design Report

## Overview
- Prompt/requirements: {original input}
- Platforms: {target platforms}
- Variations: {count}
- Style: {chosen style}

## Ideas Generated
1. **{Idea name}** — {brief description, rationale}
2. ...

## Design Decisions
- Color palette: {colors used, why}
- Typography: {fonts, sizes, why}
- Layout: {composition approach, why}
- Brand alignment: {how brand guidelines influenced design}

## Output Files
| File | Size | Platform | Preview |
|------|------|----------|---------|
| exports/{filename}.png | {WxH} | {platform} | {description} |

## Why This Works
- {Platform-specific reasoning}
- {Brand alignment reasoning}
- {Visual hierarchy reasoning}
- {Engagement potential reasoning}

## Recommendations
- {A/B test suggestions}
- {Platform-specific tips}
- {Iteration opportunities}
```

### Step 8: Organize Output

Invoke `assets-organizing` skill to organize all output files and reports:
- Move/copy exported PNGs to proper asset directories
- Ensure reports are in `plans/reports/` with correct naming
- Clean up intermediate HTML files if requested
- Tag outputs with metadata (platform, size, concept name)

## Design Best Practices

### Platform-Specific Tips

- **Instagram** — Visual-first, minimal text (<20%), strong colors, lifestyle feel
- **Facebook** — Informative, can have more text, eye-catching in feed
- **Twitter/X** — Bold headlines, contrast for dark/light mode, clear message
- **LinkedIn** — Professional, clean, data-driven visuals, thought leadership
- **Pinterest** — Vertical format, text overlay on images, how-to style
- **YouTube** — Face close-ups perform best, bright colors, readable at small size
- **TikTok** — Trendy, energetic, bold typography, youth-oriented

### Art Direction Styles (Reuse from Banner)

| Style | Best For | Key Elements |
|-------|----------|--------------|
| Minimalist | SaaS, tech, luxury | Whitespace, single accent color, clean type |
| Bold Typography | Announcements, quotes | Large type, high contrast, minimal imagery |
| Gradient Mesh | Modern brands, apps | Fluid color transitions, floating elements |
| Photo-Based | Lifestyle, e-commerce | Hero image, subtle overlay, text on image |
| Geometric | Tech, fintech | Shapes, patterns, structured layouts |
| Glassmorphism | SaaS, modern apps | Frosted glass, blur effects, transparency |
| Flat Illustration | Education, health | Custom illustrations, friendly, approachable |
| Duotone | Creative, editorial | Two-color treatment on photos |
| Collage | Fashion, culture | Mixed media, overlapping elements |
| 3D/Isometric | Tech, product | Depth, shadows, modern perspective |

### Color & Contrast

- Ensure WCAG AA contrast ratio (4.5:1 min) for all text
- Test designs at 50% size to verify readability
- Consider platform dark/light mode compatibility
- Use brand primary color as dominant, secondary as accent

### Typography Hierarchy

| Element | Min Size (at 1080px) | Weight |
|---------|---------------------|--------|
| Headline | 48px | Bold/Black |
| Subheadline | 32px | Semibold |
| Body | 24px | Regular |
| Caption | 18px | Regular/Light |
| CTA | 28px | Bold |

## Security & Scope

This sub-skill handles social media image design only. Does NOT handle:
- Video content creation
- Animation/motion graphics
- Print production files (CMYK, bleed)
- Direct social media posting/scheduling
- AI image generation (use `ai-artist` skill for that)
