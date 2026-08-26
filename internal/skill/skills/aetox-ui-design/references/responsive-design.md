
# Responsive Design

Master modern responsive design techniques to create interfaces that adapt seamlessly across all screen sizes and device contexts.

## When to Use This Skill

- Implementing mobile-first responsive layouts
- Using container queries for component-based responsiveness
- Creating fluid typography and spacing scales
- Building complex layouts with CSS Grid and Flexbox
- Designing breakpoint strategies for design systems
- Implementing responsive images and media
- Creating adaptive navigation patterns
- Building responsive tables and data displays

## Detailed patterns and worked examples

The detailed patterns follow below in this file.

## Best Practices

1. **Mobile-First**: Start with mobile styles, enhance for larger screens
2. **Content Breakpoints**: Set breakpoints based on content, not devices
3. **Fluid Over Fixed**: Use fluid values for typography and spacing
4. **Container Queries**: Use for component-level responsiveness
5. **Test Real Devices**: Simulators don't catch all issues
6. **Performance**: Optimize images, lazy load off-screen content
7. **Touch Targets**: Maintain 44x44px minimum on mobile
8. **Logical Properties**: Use inline/block for internationalization

## Common Issues

- **Horizontal Overflow**: Content breaking out of viewport
- **Fixed Widths**: Using px instead of relative units
- **Viewport Height**: 100vh issues on mobile browsers
- **Font Size**: Text too small on mobile
- **Touch Targets**: Buttons too small to tap accurately
- **Aspect Ratio**: Images squishing or stretching
- **Z-Index Stacking**: Overlays breaking on different screens


---

## อ้างอิง: breakpoint-strategies

# Breakpoint Strategies

## Overview

Effective breakpoint strategies focus on content needs rather than device sizes. Modern responsive design uses fewer, content-driven breakpoints combined with fluid techniques.

## Mobile-First Approach

### Core Philosophy

Start with the smallest screen, then progressively enhance for larger screens.

```css
/* Base styles (mobile first) */
.component {
  display: flex;
  flex-direction: column;
  padding: 1rem;
}

/* Enhance for larger screens */
@media (min-width: 640px) {
  .component {
    flex-direction: row;
    padding: 1.5rem;
  }
}

@media (min-width: 1024px) {
  .component {
    padding: 2rem;
  }
}
```

### Benefits

1. **Performance**: Mobile devices load only necessary CSS
2. **Progressive Enhancement**: Features add rather than subtract
3. **Content Priority**: Forces focus on essential content first
4. **Simplicity**: Easier to reason about cascading styles

## Common Breakpoint Scales

### Tailwind CSS Default

```css
/* Tailwind breakpoints */
/* sm: 640px  - Landscape phones */
/* md: 768px  - Tablets */
/* lg: 1024px - Laptops */
/* xl: 1280px - Desktops */
/* 2xl: 1536px - Large desktops */

@media (min-width: 640px) {
  /* sm */
}
@media (min-width: 768px) {
  /* md */
}
@media (min-width: 1024px) {
  /* lg */
}
@media (min-width: 1280px) {
  /* xl */
}
@media (min-width: 1536px) {
  /* 2xl */
}
```

### Bootstrap 5

```css
/* Bootstrap breakpoints */
/* sm: 576px */
/* md: 768px */
/* lg: 992px */
/* xl: 1200px */
/* xxl: 1400px */

@media (min-width: 576px) {
  /* sm */
}
@media (min-width: 768px) {
  /* md */
}
@media (min-width: 992px) {
  /* lg */
}
@media (min-width: 1200px) {
  /* xl */
}
@media (min-width: 1400px) {
  /* xxl */
}
```

### Minimalist Scale

```css
/* Simplified 3-breakpoint system */
/* Base: Mobile (< 600px) */
/* Medium: Tablets and small laptops (600px - 1024px) */
/* Large: Desktops (> 1024px) */

:root {
  --bp-md: 600px;
  --bp-lg: 1024px;
}

@media (min-width: 600px) {
  /* Medium */
}
@media (min-width: 1024px) {
  /* Large */
}
```

## Content-Based Breakpoints

### Finding Natural Breakpoints

Instead of using device-based breakpoints, identify where your content naturally needs to change.

```css
/* Bad: Device-based thinking */
@media (min-width: 768px) {
  /* iPad breakpoint */
}

/* Good: Content-based thinking */
/* Breakpoint where sidebar fits comfortably next to content */
@media (min-width: 50rem) {
  .layout {
    display: grid;
    grid-template-columns: 1fr 300px;
  }
}

/* Breakpoint where cards can show 3 across without crowding */
@media (min-width: 65rem) {
  .card-grid {
    grid-template-columns: repeat(3, 1fr);
  }
}
```

### Testing Content Breakpoints

```javascript
// Find where content breaks
function findBreakpoints(selector) {
  const element = document.querySelector(selector);
  const breakpoints = [];

  for (let width = 320; width <= 1920; width += 10) {
    element.style.width = `${width}px`;

    // Check for overflow, wrapping, or layout issues
    if (element.scrollWidth > element.clientWidth) {
      breakpoints.push({ width, issue: "overflow" });
    }
  }

  return breakpoints;
}
```

## Design Token Integration

### Breakpoint Tokens

```css
:root {
  /* Breakpoint values */
  --breakpoint-sm: 640px;
  --breakpoint-md: 768px;
  --breakpoint-lg: 1024px;
  --breakpoint-xl: 1280px;
  --breakpoint-2xl: 1536px;

  /* Container widths for each breakpoint */
  --container-sm: 640px;
  --container-md: 768px;
  --container-lg: 1024px;
  --container-xl: 1280px;
  --container-2xl: 1536px;
}

.container {
  width: 100%;
  max-width: var(--container-lg);
  margin-inline: auto;
  padding-inline: var(--space-4);
}
```

### JavaScript Integration

```typescript
// Breakpoint constants
export const breakpoints = {
  sm: 640,
  md: 768,
  lg: 1024,
  xl: 1280,
  "2xl": 1536,
} as const;

// Media query hook
function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState(false);

  useEffect(() => {
    const media = window.matchMedia(query);
    setMatches(media.matches);

    const listener = () => setMatches(media.matches);
    media.addEventListener("change", listener);
    return () => media.removeEventListener("change", listener);
  }, [query]);

  return matches;
}

// Breakpoint hook
function useBreakpoint() {
  const isSmall = useMediaQuery(`(min-width: ${breakpoints.sm}px)`);
  const isMedium = useMediaQuery(`(min-width: ${breakpoints.md}px)`);
  const isLarge = useMediaQuery(`(min-width: ${breakpoints.lg}px)`);
  const isXLarge = useMediaQuery(`(min-width: ${breakpoints.xl}px)`);

  return {
    isMobile: !isSmall,
    isTablet: isSmall && !isLarge,
    isDesktop: isLarge,
    current: isXLarge
      ? "xl"
      : isLarge
        ? "lg"
        : isMedium
          ? "md"
          : isSmall
            ? "sm"
            : "base",
  };
}
```

## Feature Queries

### @supports for Progressive Enhancement

```css
/* Feature detection instead of browser detection */
@supports (display: grid) {
  .layout {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
  }
}

@supports (container-type: inline-size) {
  .card-container {
    container-type: inline-size;
  }

  @container (min-width: 400px) {
    .card {
      flex-direction: row;
    }
  }
}

@supports (aspect-ratio: 16/9) {
  .video-container {
    aspect-ratio: 16/9;
  }
}

/* Fallback for older browsers */
@supports not (gap: 1rem) {
  .flex-container > * + * {
    margin-left: 1rem;
  }
}
```

### Combining Feature and Size Queries

```css
/* Only apply grid layout if supported and screen is large enough */
@supports (display: grid) {
  @media (min-width: 768px) {
    .layout {
      display: grid;
      grid-template-columns: 250px 1fr;
    }
  }
}
```

## Responsive Patterns by Component

### Navigation

```css
.nav {
  /* Mobile: vertical stack */
  display: flex;
  flex-direction: column;
}

@media (min-width: 768px) {
  .nav {
    /* Tablet+: horizontal */
    flex-direction: row;
    align-items: center;
  }
}

/* Or with container queries */
.nav-container {
  container-type: inline-size;
}

@container (min-width: 600px) {
  .nav {
    flex-direction: row;
  }
}
```

### Cards Grid

```css
.cards {
  display: grid;
  gap: 1.5rem;
  grid-template-columns: 1fr;
}

@media (min-width: 640px) {
  .cards {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (min-width: 1024px) {
  .cards {
    grid-template-columns: repeat(3, 1fr);
  }
}

@media (min-width: 1280px) {
  .cards {
    grid-template-columns: repeat(4, 1fr);
  }
}

/* Better: auto-fit with minimum size */
.cards-auto {
  display: grid;
  gap: 1.5rem;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 280px), 1fr));
}
```

### Hero Section

```css
.hero {
  min-height: 50vh;
  padding: var(--space-lg) var(--space-md);
  text-align: center;
}

.hero-title {
  font-size: clamp(2rem, 5vw + 1rem, 4rem);
}

.hero-subtitle {
  font-size: clamp(1rem, 2vw + 0.5rem, 1.5rem);
}

@media (min-width: 768px) {
  .hero {
    min-height: 70vh;
    display: flex;
    align-items: center;
    text-align: left;
  }

  .hero-content {
    max-width: 60%;
  }
}

@media (min-width: 1024px) {
  .hero {
    min-height: 80vh;
  }

  .hero-content {
    max-width: 50%;
  }
}
```

### Tables

```css
/* Mobile: cards or horizontal scroll */
.table-container {
  overflow-x: auto;
}

.responsive-table {
  min-width: 600px;
}

/* Alternative: transform to cards on mobile */
@media (max-width: 639px) {
  .responsive-table {
    min-width: 0;
  }

  .responsive-table thead {
    display: none;
  }

  .responsive-table tr {
    display: block;
    margin-bottom: 1rem;
    border: 1px solid var(--border);
    border-radius: 0.5rem;
    padding: 1rem;
  }

  .responsive-table td {
    display: flex;
    justify-content: space-between;
    padding: 0.5rem 0;
    border: none;
  }

  .responsive-table td::before {
    content: attr(data-label);
    font-weight: 600;
  }
}
```

## Print Styles

```css
@media print {
  /* Remove non-essential elements */
  .nav,
  .sidebar,
  .footer,
  .ads {
    display: none;
  }

  /* Reset colors and backgrounds */
  * {
    background: white !important;
    color: black !important;
    box-shadow: none !important;
  }

  /* Ensure content fits on page */
  .container {
    max-width: 100%;
    padding: 0;
  }

  /* Handle page breaks */
  h1,
  h2,
  h3 {
    page-break-after: avoid;
  }

  img,
  table {
    page-break-inside: avoid;
  }

  /* Show URLs for links */
  a[href^="http"]::after {
    content: " (" attr(href) ")";
    font-size: 0.8em;
  }
}
```

## Preference Queries

```css
/* Dark mode preference */
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #1a1a1a;
    --text: #f0f0f0;
  }
}

/* Reduced motion preference */
@media (prefers-reduced-motion: reduce) {
  *,
  *::before,
  *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
  }
}

/* High contrast preference */
@media (prefers-contrast: high) {
  :root {
    --text: #000;
    --bg: #fff;
    --border: #000;
  }

  .button {
    border: 2px solid currentColor;
  }
}

/* Reduced data preference */
@media (prefers-reduced-data: reduce) {
  .hero-video {
    display: none;
  }

  .hero-image {
    display: block;
  }
}
```

## Testing Breakpoints

```javascript
// Automated breakpoint testing
async function testBreakpoints(page, breakpoints) {
  const results = [];

  for (const [name, width] of Object.entries(breakpoints)) {
    await page.setViewportSize({ width, height: 800 });

    // Check for horizontal overflow
    const hasOverflow = await page.evaluate(() => {
      return (
        document.documentElement.scrollWidth >
        document.documentElement.clientWidth
      );
    });

    // Check for elements going off-screen
    const offscreenElements = await page.evaluate(() => {
      const elements = document.querySelectorAll("*");
      return Array.from(elements).filter((el) => {
        const rect = el.getBoundingClientRect();
        return rect.right > window.innerWidth || rect.left < 0;
      }).length;
    });

    results.push({
      breakpoint: name,
      width,
      hasOverflow,
      offscreenElements,
    });
  }

  return results;
}
```

## Resources

- [Tailwind CSS Breakpoints](https://tailwindcss.com/docs/responsive-design)
- [The 100% Correct Way to Do CSS Breakpoints](https://www.freecodecamp.org/news/the-100-correct-way-to-do-css-breakpoints-88d6a5ba1862/)
- [Modern CSS Solutions](https://moderncss.dev/)
- [Defensive CSS](https://defensivecss.dev/)


---

## อ้างอิง: container-queries

# Container Queries Deep Dive

## Overview

Container queries enable component-based responsive design by allowing elements to respond to their container's size rather than the viewport. This paradigm shift makes truly reusable components possible.

## Browser Support

Container queries have excellent modern browser support (Chrome 105+, Firefox 110+, Safari 16+). For older browsers, provide graceful fallbacks.

## Containment Basics

### Container Types

```css
/* Size containment - queries based on inline and block size */
.container {
  container-type: size;
}

/* Inline-size containment - queries based on inline (width) size only */
/* Most common and recommended */
.container {
  container-type: inline-size;
}

/* Normal - style queries only, no size queries */
.container {
  container-type: normal;
}
```

### Named Containers

```css
/* Named container for targeted queries */
.card-wrapper {
  container-type: inline-size;
  container-name: card;
}

/* Shorthand */
.card-wrapper {
  container: card / inline-size;
}

/* Query specific container */
@container card (min-width: 400px) {
  .card-content {
    display: flex;
  }
}
```

## Container Query Syntax

### Width-Based Queries

```css
.container {
  container-type: inline-size;
}

/* Minimum width */
@container (min-width: 300px) {
  .element {
    /* styles */
  }
}

/* Maximum width */
@container (max-width: 500px) {
  .element {
    /* styles */
  }
}

/* Range syntax */
@container (300px <= width <= 600px) {
  .element {
    /* styles */
  }
}

/* Exact width */
@container (width: 400px) {
  .element {
    /* styles */
  }
}
```

### Combining Conditions

```css
/* AND condition */
@container (min-width: 400px) and (max-width: 800px) {
  .element {
    /* styles */
  }
}

/* OR condition */
@container (max-width: 300px) or (min-width: 800px) {
  .element {
    /* styles */
  }
}

/* NOT condition */
@container not (min-width: 400px) {
  .element {
    /* styles */
  }
}
```

### Named Container Queries

```css
/* Multiple named containers */
.page-wrapper {
  container: page / inline-size;
}

.sidebar-wrapper {
  container: sidebar / inline-size;
}

/* Target specific containers */
@container page (min-width: 1024px) {
  .main-content {
    max-width: 800px;
  }
}

@container sidebar (min-width: 300px) {
  .sidebar-widget {
    display: grid;
    grid-template-columns: 1fr 1fr;
  }
}
```

## Container Query Units

```css
/* Container query length units */
.element {
  /* Container query width - 1cqw = 1% of container width */
  width: 50cqw;

  /* Container query height - 1cqh = 1% of container height */
  height: 50cqh;

  /* Container query inline - 1cqi = 1% of container inline size */
  padding-inline: 5cqi;

  /* Container query block - 1cqb = 1% of container block size */
  padding-block: 3cqb;

  /* Container query min - smaller of cqi and cqb */
  font-size: 5cqmin;

  /* Container query max - larger of cqi and cqb */
  margin: 2cqmax;
}

/* Practical example: fluid typography based on container */
.card-title {
  font-size: clamp(1rem, 4cqi, 2rem);
}

.card-body {
  padding: clamp(0.75rem, 4cqi, 1.5rem);
}
```

## Style Queries

Style queries allow querying CSS custom property values. Currently limited support.

```css
/* Define a custom property */
.card {
  --layout: stack;
}

/* Query the property value */
@container style(--layout: stack) {
  .card-content {
    display: flex;
    flex-direction: column;
  }
}

@container style(--layout: inline) {
  .card-content {
    display: flex;
    flex-direction: row;
  }
}

/* Toggle layout via custom property */
.card.horizontal {
  --layout: inline;
}
```

## Practical Patterns

### Responsive Card Component

```css
.card-container {
  container: card / inline-size;
}

.card {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  padding: clamp(1rem, 4cqi, 2rem);
}

.card-image {
  aspect-ratio: 16/9;
  width: 100%;
  object-fit: cover;
  border-radius: 0.5rem;
}

.card-title {
  font-size: clamp(1rem, 4cqi, 1.5rem);
  font-weight: 600;
}

/* Medium container: side-by-side layout */
@container card (min-width: 400px) {
  .card {
    flex-direction: row;
    align-items: flex-start;
  }

  .card-image {
    width: 40%;
    aspect-ratio: 1;
  }

  .card-content {
    flex: 1;
  }
}

/* Large container: enhanced layout */
@container card (min-width: 600px) {
  .card-image {
    width: 250px;
  }

  .card-title {
    font-size: 1.5rem;
  }

  .card-actions {
    display: flex;
    gap: 0.5rem;
  }
}
```

### Responsive Grid Items

```css
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
  gap: 1.5rem;
}

.grid-item {
  container-type: inline-size;
}

.item-content {
  padding: 1rem;
}

/* Item adapts to its own size, not the viewport */
@container (min-width: 350px) {
  .item-content {
    padding: 1.5rem;
  }

  .item-title {
    font-size: 1.25rem;
  }
}

@container (min-width: 500px) {
  .item-content {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 1rem;
  }
}
```

### Dashboard Widget

```css
.widget-container {
  container: widget / inline-size;
}

.widget {
  --chart-height: 150px;
  padding: 1rem;
}

.widget-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.widget-chart {
  height: var(--chart-height);
}

.widget-stats {
  display: grid;
  grid-template-columns: 1fr;
  gap: 0.5rem;
}

@container widget (min-width: 300px) {
  .widget {
    --chart-height: 200px;
  }

  .widget-stats {
    grid-template-columns: repeat(2, 1fr);
  }
}

@container widget (min-width: 500px) {
  .widget {
    --chart-height: 250px;
    padding: 1.5rem;
  }

  .widget-stats {
    grid-template-columns: repeat(4, 1fr);
  }

  .widget-actions {
    display: flex;
    gap: 0.5rem;
  }
}
```

### Navigation Component

```css
.nav-container {
  container: nav / inline-size;
}

.nav {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.nav-link {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.75rem 1rem;
  border-radius: 0.5rem;
}

.nav-link-text {
  display: none;
}

.nav-link-icon {
  width: 1.5rem;
  height: 1.5rem;
}

/* Show text when container is wide enough */
@container nav (min-width: 200px) {
  .nav-link-text {
    display: block;
  }
}

/* Horizontal layout for wider containers */
@container nav (min-width: 600px) {
  .nav {
    flex-direction: row;
  }

  .nav-link {
    padding: 0.5rem 1rem;
  }
}
```

## Tailwind CSS Integration

```tsx
// Tailwind v3.2+ supports container queries
// tailwind.config.js
module.exports = {
  plugins: [require("@tailwindcss/container-queries")],
};

// Component usage
function Card({ title, image, description }) {
  return (
    // @container creates containment context
    <div className="@container">
      <article className="flex flex-col @md:flex-row @md:gap-4">
        <img
          src={image}
          alt=""
          className="w-full @md:w-48 @lg:w-64 aspect-video @md:aspect-square object-cover rounded-lg"
        />
        <div className="p-4 @md:p-0">
          <h2 className="text-lg @md:text-xl @lg:text-2xl font-semibold">
            {title}
          </h2>
          <p className="mt-2 text-muted-foreground @lg:text-lg">
            {description}
          </p>
        </div>
      </article>
    </div>
  );
}

// Named containers
function Dashboard() {
  return (
    <div className="@container/main">
      <aside className="@container/sidebar">
        <nav className="flex flex-col @lg/sidebar:flex-row">{/* ... */}</nav>
      </aside>
      <main className="@lg/main:grid @lg/main:grid-cols-2">{/* ... */}</main>
    </div>
  );
}
```

## Fallback Strategies

```css
/* Provide fallbacks for browsers without support */
.card {
  /* Default (fallback) styles */
  display: flex;
  flex-direction: column;
}

/* Feature query for container support */
@supports (container-type: inline-size) {
  .card-container {
    container-type: inline-size;
  }

  @container (min-width: 400px) {
    .card {
      flex-direction: row;
    }
  }
}

/* Alternative: media query fallback */
.card {
  display: flex;
  flex-direction: column;
}

/* Viewport-based fallback */
@media (min-width: 768px) {
  .card {
    flex-direction: row;
  }
}

/* Enhanced with container queries when supported */
@supports (container-type: inline-size) {
  @media (min-width: 768px) {
    .card {
      flex-direction: column; /* Reset */
    }
  }

  @container (min-width: 400px) {
    .card {
      flex-direction: row;
    }
  }
}
```

## Performance Considerations

```css
/* Avoid over-nesting containers */
/* Bad: Too many nested containers */
.level-1 {
  container-type: inline-size;
}
.level-2 {
  container-type: inline-size;
}
.level-3 {
  container-type: inline-size;
}
.level-4 {
  container-type: inline-size;
}

/* Good: Strategic container placement */
.component-wrapper {
  container-type: inline-size;
}

/* Use inline-size instead of size when possible */
/* size containment is more expensive */
.container {
  container-type: inline-size; /* Preferred */
  /* container-type: size; */ /* Only when needed */
}
```

## Testing Container Queries

```javascript
// Test container query support
const supportsContainerQueries = CSS.supports("container-type", "inline-size");

// Resize observer for testing
const observer = new ResizeObserver((entries) => {
  for (const entry of entries) {
    console.log("Container width:", entry.contentRect.width);
  }
});

observer.observe(document.querySelector(".container"));
```

## Resources

- [MDN Container Queries](https://developer.mozilla.org/en-US/docs/Web/CSS/CSS_container_queries)
- [CSS Container Queries Spec](https://www.w3.org/TR/css-contain-3/)
- [Una Kravets: Container Queries](https://web.dev/cq-stable/)
- [Ahmad Shadeed: Container Queries Guide](https://ishadeed.com/article/container-queries-are-finally-here/)


---

## อ้างอิง: details

# responsive-design, detailed patterns and worked examples

## Core Capabilities

### 1. Container Queries

- Component-level responsiveness independent of viewport
- Container query units (cqi, cqw, cqh)
- Style queries for conditional styling
- Fallbacks for browser support

### 2. Fluid Typography & Spacing

- CSS clamp() for fluid scaling
- Viewport-relative units (vw, vh, dvh)
- Fluid type scales with min/max bounds
- Responsive spacing systems

### 3. Layout Patterns

- CSS Grid for 2D layouts
- Flexbox for 1D distribution
- Intrinsic layouts (content-based sizing)
- Subgrid for nested grid alignment

### 4. Breakpoint Strategy

- Mobile-first media queries
- Content-based breakpoints
- Design token integration
- Feature queries (@supports)

## Quick Reference

### Modern Breakpoint Scale

```css
/* Mobile-first breakpoints */
/* Base: Mobile (< 640px) */
@media (min-width: 640px) {
  /* sm: Landscape phones, small tablets */
}
@media (min-width: 768px) {
  /* md: Tablets */
}
@media (min-width: 1024px) {
  /* lg: Laptops, small desktops */
}
@media (min-width: 1280px) {
  /* xl: Desktops */
}
@media (min-width: 1536px) {
  /* 2xl: Large desktops */
}

/* Tailwind CSS equivalent */
/* sm:  @media (min-width: 640px) */
/* md:  @media (min-width: 768px) */
/* lg:  @media (min-width: 1024px) */
/* xl:  @media (min-width: 1280px) */
/* 2xl: @media (min-width: 1536px) */
```

## Key Patterns

### Pattern 1: Container Queries

```css
/* Define a containment context */
.card-container {
  container-type: inline-size;
  container-name: card;
}

/* Query the container, not the viewport */
@container card (min-width: 400px) {
  .card {
    display: grid;
    grid-template-columns: 200px 1fr;
    gap: 1rem;
  }

  .card-image {
    aspect-ratio: 1;
  }
}

@container card (min-width: 600px) {
  .card {
    grid-template-columns: 250px 1fr;
  }

  .card-title {
    font-size: 1.5rem;
  }
}

/* Container query units */
.card-title {
  /* 5% of container width, clamped between 1rem and 2rem */
  font-size: clamp(1rem, 5cqi, 2rem);
}
```

```tsx
// React component with container queries
function ResponsiveCard({ title, image, description }) {
  return (
    <div className="@container">
      <article className="flex flex-col @md:flex-row @md:gap-4">
        <img
          src={image}
          alt=""
          className="w-full @md:w-48 @lg:w-64 aspect-video @md:aspect-square object-cover"
        />
        <div className="p-4 @md:p-0">
          <h2 className="text-lg @md:text-xl @lg:text-2xl font-semibold">
            {title}
          </h2>
          <p className="mt-2 text-muted-foreground @md:line-clamp-3">
            {description}
          </p>
        </div>
      </article>
    </div>
  );
}
```

### Pattern 2: Fluid Typography

```css
/* Fluid type scale using clamp() */
:root {
  /* Min size, preferred (fluid), max size */
  --text-xs: clamp(0.75rem, 0.7rem + 0.25vw, 0.875rem);
  --text-sm: clamp(0.875rem, 0.8rem + 0.375vw, 1rem);
  --text-base: clamp(1rem, 0.9rem + 0.5vw, 1.125rem);
  --text-lg: clamp(1.125rem, 1rem + 0.625vw, 1.25rem);
  --text-xl: clamp(1.25rem, 1rem + 1.25vw, 1.5rem);
  --text-2xl: clamp(1.5rem, 1.25rem + 1.25vw, 2rem);
  --text-3xl: clamp(1.875rem, 1.5rem + 1.875vw, 2.5rem);
  --text-4xl: clamp(2.25rem, 1.75rem + 2.5vw, 3.5rem);
}

/* Usage */
h1 {
  font-size: var(--text-4xl);
}
h2 {
  font-size: var(--text-3xl);
}
h3 {
  font-size: var(--text-2xl);
}
p {
  font-size: var(--text-base);
}

/* Fluid spacing scale */
:root {
  --space-xs: clamp(0.25rem, 0.2rem + 0.25vw, 0.5rem);
  --space-sm: clamp(0.5rem, 0.4rem + 0.5vw, 0.75rem);
  --space-md: clamp(1rem, 0.8rem + 1vw, 1.5rem);
  --space-lg: clamp(1.5rem, 1.2rem + 1.5vw, 2.5rem);
  --space-xl: clamp(2rem, 1.5rem + 2.5vw, 4rem);
}
```

```tsx
// Utility function for fluid values
function fluidValue(
  minSize: number,
  maxSize: number,
  minWidth = 320,
  maxWidth = 1280,
) {
  const slope = (maxSize - minSize) / (maxWidth - minWidth);
  const yAxisIntersection = -minWidth * slope + minSize;

  return `clamp(${minSize}rem, ${yAxisIntersection.toFixed(4)}rem + ${(slope * 100).toFixed(4)}vw, ${maxSize}rem)`;
}

// Generate fluid type scale
const fluidTypeScale = {
  sm: fluidValue(0.875, 1),
  base: fluidValue(1, 1.125),
  lg: fluidValue(1.25, 1.5),
  xl: fluidValue(1.5, 2),
  "2xl": fluidValue(2, 3),
};
```

### Pattern 3: CSS Grid Responsive Layout

```css
/* Auto-fit grid - items wrap automatically */
.grid-auto {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(300px, 100%), 1fr));
  gap: 1.5rem;
}

/* Auto-fill grid - maintains empty columns */
.grid-auto-fill {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
  gap: 1rem;
}

/* Responsive grid with named areas */
.page-layout {
  display: grid;
  grid-template-areas:
    "header"
    "main"
    "sidebar"
    "footer";
  gap: 1rem;
}

@media (min-width: 768px) {
  .page-layout {
    grid-template-columns: 1fr 300px;
    grid-template-areas:
      "header header"
      "main sidebar"
      "footer footer";
  }
}

@media (min-width: 1024px) {
  .page-layout {
    grid-template-columns: 250px 1fr 300px;
    grid-template-areas:
      "header header header"
      "nav main sidebar"
      "footer footer footer";
  }
}

.header {
  grid-area: header;
}
.main {
  grid-area: main;
}
.sidebar {
  grid-area: sidebar;
}
.footer {
  grid-area: footer;
}
```

```tsx
// Responsive grid component
function ResponsiveGrid({ children, minItemWidth = "250px", gap = "1.5rem" }) {
  return (
    <div
      className="grid"
      style={{
        gridTemplateColumns: `repeat(auto-fit, minmax(min(${minItemWidth}, 100%), 1fr))`,
        gap,
      }}
    >
      {children}
    </div>
  );
}

// Usage with Tailwind
function ProductGrid({ products }) {
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4 md:gap-6">
      {products.map((product) => (
        <ProductCard key={product.id} product={product} />
      ))}
    </div>
  );
}
```

### Pattern 4: Responsive Navigation

```tsx
function ResponsiveNav({ items }) {
  const [isOpen, setIsOpen] = useState(false);

  return (
    <nav className="relative">
      {/* Mobile menu button */}
      <button
        className="lg:hidden p-2"
        onClick={() => setIsOpen(!isOpen)}
        aria-expanded={isOpen}
        aria-controls="nav-menu"
      >
        <span className="sr-only">Toggle navigation</span>
        {isOpen ? <X /> : <Menu />}
      </button>

      {/* Navigation links */}
      <ul
        id="nav-menu"
        className={cn(
          // Base: hidden on mobile
          "absolute top-full left-0 right-0 bg-background border-b",
          "flex flex-col",
          // Mobile: slide down
          isOpen ? "flex" : "hidden",
          // Desktop: always visible, horizontal
          "lg:static lg:flex lg:flex-row lg:border-0 lg:bg-transparent",
        )}
      >
        {items.map((item) => (
          <li key={item.href}>
            <a
              href={item.href}
              className={cn(
                "block px-4 py-3",
                "lg:px-3 lg:py-2",
                "hover:bg-muted lg:hover:bg-transparent lg:hover:text-primary",
              )}
            >
              {item.label}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  );
}
```

### Pattern 5: Responsive Images

```tsx
// Responsive image with art direction
function ResponsiveHero() {
  return (
    <picture>
      {/* Art direction: different crops for different screens */}
      <source
        media="(min-width: 1024px)"
        srcSet="/hero-wide.webp"
        type="image/webp"
      />
      <source
        media="(min-width: 768px)"
        srcSet="/hero-medium.webp"
        type="image/webp"
      />
      <source srcSet="/hero-mobile.webp" type="image/webp" />

      {/* Fallback */}
      <img
        src="/hero-mobile.jpg"
        alt="Hero image description"
        className="w-full h-auto"
        loading="eager"
        fetchpriority="high"
      />
    </picture>
  );
}

// Responsive image with srcset for resolution switching
function ProductImage({ product }) {
  return (
    <img
      src={product.image}
      srcSet={`
        ${product.image}?w=400 400w,
        ${product.image}?w=800 800w,
        ${product.image}?w=1200 1200w
      `}
      sizes="(max-width: 640px) 100vw, (max-width: 1024px) 50vw, 33vw"
      alt={product.name}
      className="w-full h-auto object-cover"
      loading="lazy"
    />
  );
}
```

### Pattern 6: Responsive Tables

```tsx
// Responsive table with horizontal scroll
function ResponsiveTable({ data, columns }) {
  return (
    <div className="w-full overflow-x-auto">
      <table className="w-full min-w-[600px]">
        <thead>
          <tr>
            {columns.map((col) => (
              <th key={col.key} className="text-left p-3">
                {col.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data.map((row, i) => (
            <tr key={i} className="border-t">
              {columns.map((col) => (
                <td key={col.key} className="p-3">
                  {row[col.key]}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// Card-based table for mobile
function ResponsiveDataTable({ data, columns }) {
  return (
    <>
      {/* Desktop table */}
      <table className="hidden md:table w-full">
        {/* ... standard table */}
      </table>

      {/* Mobile cards */}
      <div className="md:hidden space-y-4">
        {data.map((row, i) => (
          <div key={i} className="border rounded-lg p-4 space-y-2">
            {columns.map((col) => (
              <div key={col.key} className="flex justify-between">
                <span className="font-medium text-muted-foreground">
                  {col.label}
                </span>
                <span>{row[col.key]}</span>
              </div>
            ))}
          </div>
        ))}
      </div>
    </>
  );
}
```

## Viewport Units

```css
/* Standard viewport units */
.full-height {
  height: 100vh; /* May cause issues on mobile */
}

/* Dynamic viewport units (recommended for mobile) */
.full-height-dynamic {
  height: 100dvh; /* Accounts for mobile browser UI */
}

/* Small viewport (minimum) */
.min-full-height {
  min-height: 100svh;
}

/* Large viewport (maximum) */
.max-full-height {
  max-height: 100lvh;
}

/* Viewport-relative font sizing */
.hero-title {
  /* 5vw with min/max bounds */
  font-size: clamp(2rem, 5vw, 4rem);
}
```


---

## อ้างอิง: fluid-layouts

# Fluid Layouts and Typography

## Overview

Fluid design creates smooth scaling experiences by using relative units and mathematical functions instead of fixed breakpoints. This approach reduces the need for media queries and creates more natural-feeling interfaces.

## Fluid Typography

### The clamp() Function

```css
/* clamp(minimum, preferred, maximum) */
.heading {
  /* Never smaller than 1.5rem, never larger than 3rem */
  /* Scales at 5vw between those values */
  font-size: clamp(1.5rem, 5vw, 3rem);
}
```

### Calculating Fluid Values

The preferred value in `clamp()` typically combines a base size with a viewport-relative portion:

```css
/* Formula: clamp(min, base + scale * vw, max) */

/* For text that scales from 16px (320px viewport) to 24px (1200px viewport): */
/* slope = (24 - 16) / (1200 - 320) = 8 / 880 = 0.00909 */
/* y-intercept = 16 - 0.00909 * 320 = 13.09px = 0.818rem */

.text {
  font-size: clamp(1rem, 0.818rem + 0.909vw, 1.5rem);
}
```

### Type Scale Generator

```javascript
// Generate a fluid type scale
function fluidType({
  minFontSize,
  maxFontSize,
  minViewport = 320,
  maxViewport = 1200,
}) {
  const minFontRem = minFontSize / 16;
  const maxFontRem = maxFontSize / 16;
  const minViewportRem = minViewport / 16;
  const maxViewportRem = maxViewport / 16;

  const slope = (maxFontRem - minFontRem) / (maxViewportRem - minViewportRem);
  const yAxisIntersection = minFontRem - slope * minViewportRem;

  return `clamp(${minFontRem}rem, ${yAxisIntersection.toFixed(4)}rem + ${(slope * 100).toFixed(4)}vw, ${maxFontRem}rem)`;
}

// Usage
const typeScale = {
  xs: fluidType({ minFontSize: 12, maxFontSize: 14 }),
  sm: fluidType({ minFontSize: 14, maxFontSize: 16 }),
  base: fluidType({ minFontSize: 16, maxFontSize: 18 }),
  lg: fluidType({ minFontSize: 18, maxFontSize: 20 }),
  xl: fluidType({ minFontSize: 20, maxFontSize: 24 }),
  "2xl": fluidType({ minFontSize: 24, maxFontSize: 32 }),
  "3xl": fluidType({ minFontSize: 30, maxFontSize: 48 }),
  "4xl": fluidType({ minFontSize: 36, maxFontSize: 60 }),
};
```

### Complete Type Scale

```css
:root {
  /* Base: 16-18px */
  --text-base: clamp(1rem, 0.9rem + 0.5vw, 1.125rem);

  /* Smaller sizes */
  --text-sm: clamp(0.875rem, 0.8rem + 0.375vw, 1rem);
  --text-xs: clamp(0.75rem, 0.7rem + 0.25vw, 0.875rem);

  /* Larger sizes */
  --text-lg: clamp(1.125rem, 1rem + 0.625vw, 1.25rem);
  --text-xl: clamp(1.25rem, 1.1rem + 0.75vw, 1.5rem);
  --text-2xl: clamp(1.5rem, 1.2rem + 1.5vw, 2rem);
  --text-3xl: clamp(1.875rem, 1.4rem + 2.375vw, 2.5rem);
  --text-4xl: clamp(2.25rem, 1.5rem + 3.75vw, 3.5rem);
  --text-5xl: clamp(3rem, 1.8rem + 6vw, 5rem);

  /* Line heights scale inversely */
  --leading-tight: 1.25;
  --leading-normal: 1.5;
  --leading-relaxed: 1.75;
}

/* Apply to elements */
body {
  font-size: var(--text-base);
  line-height: var(--leading-normal);
}

h1 {
  font-size: var(--text-4xl);
  line-height: var(--leading-tight);
}
h2 {
  font-size: var(--text-3xl);
  line-height: var(--leading-tight);
}
h3 {
  font-size: var(--text-2xl);
  line-height: var(--leading-tight);
}
h4 {
  font-size: var(--text-xl);
  line-height: var(--leading-normal);
}
h5 {
  font-size: var(--text-lg);
  line-height: var(--leading-normal);
}
h6 {
  font-size: var(--text-base);
  line-height: var(--leading-normal);
}

small {
  font-size: var(--text-sm);
}
```

## Fluid Spacing

### Spacing Scale

```css
:root {
  /* Spacing tokens that scale with viewport */
  --space-3xs: clamp(0.25rem, 0.2rem + 0.25vw, 0.375rem);
  --space-2xs: clamp(0.375rem, 0.3rem + 0.375vw, 0.5rem);
  --space-xs: clamp(0.5rem, 0.4rem + 0.5vw, 0.75rem);
  --space-sm: clamp(0.75rem, 0.6rem + 0.75vw, 1rem);
  --space-md: clamp(1rem, 0.8rem + 1vw, 1.5rem);
  --space-lg: clamp(1.5rem, 1.2rem + 1.5vw, 2rem);
  --space-xl: clamp(2rem, 1.5rem + 2.5vw, 3rem);
  --space-2xl: clamp(3rem, 2rem + 5vw, 5rem);
  --space-3xl: clamp(4rem, 2.5rem + 7.5vw, 8rem);

  /* One-up pairs (for asymmetric spacing) */
  --space-xs-sm: clamp(0.5rem, 0.3rem + 1vw, 1rem);
  --space-sm-md: clamp(0.75rem, 0.5rem + 1.25vw, 1.5rem);
  --space-md-lg: clamp(1rem, 0.6rem + 2vw, 2rem);
  --space-lg-xl: clamp(1.5rem, 1rem + 2.5vw, 3rem);
}

/* Usage examples */
.section {
  padding-block: var(--space-xl);
  padding-inline: var(--space-md);
}

.card {
  padding: var(--space-md);
  gap: var(--space-sm);
}

.stack > * + * {
  margin-top: var(--space-md);
}
```

### Container Widths

```css
:root {
  /* Fluid max-widths */
  --container-xs: min(100% - 2rem, 20rem);
  --container-sm: min(100% - 2rem, 30rem);
  --container-md: min(100% - 2rem, 45rem);
  --container-lg: min(100% - 2rem, 65rem);
  --container-xl: min(100% - 3rem, 80rem);
  --container-2xl: min(100% - 4rem, 96rem);
}

.container {
  width: var(--container-lg);
  margin-inline: auto;
}

.prose {
  max-width: var(--container-md);
}

.full-bleed {
  width: 100vw;
  margin-inline: calc(-50vw + 50%);
}
```

## CSS Grid Fluid Layouts

### Auto-fit Grid

```css
/* Grid that fills available space */
.auto-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 250px), 1fr));
  gap: var(--space-md);
}

/* With maximum columns */
.auto-grid-max-4 {
  display: grid;
  grid-template-columns: repeat(
    auto-fit,
    minmax(min(100%, max(200px, calc((100% - 3 * var(--space-md)) / 4))), 1fr)
  );
  gap: var(--space-md);
}
```

### Responsive Grid Areas

```css
.page-grid {
  display: grid;
  grid-template-columns:
    1fr
    min(var(--container-lg), 100%)
    1fr;
  grid-template-rows: auto 1fr auto;
}

.page-grid > * {
  grid-column: 2;
}

.full-width {
  grid-column: 1 / -1;
}

/* Content with sidebar */
.content-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: var(--space-lg);
}

@media (min-width: 768px) {
  .content-grid {
    grid-template-columns: 1fr min(300px, 30%);
  }
}
```

### Fluid Aspect Ratios

```css
/* Maintain aspect ratio fluidly */
.aspect-video {
  aspect-ratio: 16 / 9;
}

.aspect-square {
  aspect-ratio: 1;
}

/* Fluid aspect ratio that changes */
.hero-image {
  aspect-ratio: 1; /* Mobile: square */
}

@media (min-width: 640px) {
  .hero-image {
    aspect-ratio: 4 / 3;
  }
}

@media (min-width: 1024px) {
  .hero-image {
    aspect-ratio: 16 / 9;
  }
}
```

## Flexbox Fluid Patterns

### Flexible Sidebar

```css
/* Sidebar that collapses when too narrow */
.with-sidebar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-lg);
}

.with-sidebar > :first-child {
  flex-basis: 300px;
  flex-grow: 1;
}

.with-sidebar > :last-child {
  flex-basis: 0;
  flex-grow: 999;
  min-width: 60%;
}
```

### Cluster Layout

```css
/* Items cluster and wrap naturally */
.cluster {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-sm);
  justify-content: flex-start;
  align-items: center;
}

/* Center-aligned cluster */
.cluster-center {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-sm);
  justify-content: center;
  align-items: center;
}

/* Space-between cluster */
.cluster-spread {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-sm);
  justify-content: space-between;
  align-items: center;
}
```

### Switcher Layout

```css
/* Switches from horizontal to vertical based on container */
.switcher {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-md);
}

.switcher > * {
  /* Items go vertical when container is narrower than threshold */
  flex-grow: 1;
  flex-basis: calc((30rem - 100%) * 999);
}

/* Limit columns */
.switcher > :nth-last-child(n + 4),
.switcher > :nth-last-child(n + 4) ~ * {
  flex-basis: 100%;
}
```

## Intrinsic Sizing

### Content-Based Widths

```css
/* Size based on content */
.fit-content {
  width: fit-content;
  max-width: 100%;
}

/* Minimum content size */
.min-content {
  width: min-content;
}

/* Maximum content size */
.max-content {
  width: max-content;
}

/* Practical examples */
.button {
  width: fit-content;
  min-width: 8rem; /* Prevent too-narrow buttons */
  padding-inline: var(--space-md);
}

.tag {
  width: fit-content;
  padding: var(--space-2xs) var(--space-xs);
}

.modal {
  width: min(90vw, 600px);
  max-height: min(90vh, 800px);
}
```

### min() and max() Functions

```css
/* Responsive sizing without media queries */
.container {
  /* 90% of viewport or 1200px, whichever is smaller */
  width: min(90%, 1200px);
  margin-inline: auto;
}

.hero-text {
  /* At least 2rem, at most 4rem */
  font-size: max(2rem, min(5vw, 4rem));
}

.sidebar {
  /* At least 200px, at most 25% of parent */
  width: max(200px, min(300px, 25%));
}

.card-grid {
  /* Each card at least 200px, fill available space */
  grid-template-columns: repeat(auto-fit, minmax(max(200px, 100%/4), 1fr));
}
```

## Viewport Units

### Modern Viewport Units

```css
/* Dynamic viewport height - accounts for mobile browser UI */
.full-height {
  min-height: 100dvh;
}

/* Small viewport - minimum size when UI is visible */
.hero {
  min-height: 100svh;
}

/* Large viewport - maximum size when UI is hidden */
.backdrop {
  height: 100lvh;
}

/* Viewport-relative positioning */
.fixed-nav {
  position: fixed;
  inset-inline: 0;
  top: 0;
  height: max(60px, 8vh);
}

/* Safe area insets for notched devices */
.safe-area {
  padding-top: env(safe-area-inset-top);
  padding-right: env(safe-area-inset-right);
  padding-bottom: env(safe-area-inset-bottom);
  padding-left: env(safe-area-inset-left);
}
```

### Combining Viewport and Container Units

```css
/* Responsive based on both viewport and container */
.component {
  container-type: inline-size;
}

.component-text {
  /* Uses viewport when small, container when in container */
  font-size: clamp(1rem, 2vw + 0.5rem, 1.5rem);
}

@container (min-width: 400px) {
  .component-text {
    font-size: clamp(1rem, 4cqi, 1.5rem);
  }
}
```

## Utility Classes

```css
/* Tailwind-style fluid utilities */
.text-fluid-sm {
  font-size: var(--text-sm);
}
.text-fluid-base {
  font-size: var(--text-base);
}
.text-fluid-lg {
  font-size: var(--text-lg);
}
.text-fluid-xl {
  font-size: var(--text-xl);
}
.text-fluid-2xl {
  font-size: var(--text-2xl);
}
.text-fluid-3xl {
  font-size: var(--text-3xl);
}
.text-fluid-4xl {
  font-size: var(--text-4xl);
}

.p-fluid-sm {
  padding: var(--space-sm);
}
.p-fluid-md {
  padding: var(--space-md);
}
.p-fluid-lg {
  padding: var(--space-lg);
}

.gap-fluid-sm {
  gap: var(--space-sm);
}
.gap-fluid-md {
  gap: var(--space-md);
}
.gap-fluid-lg {
  gap: var(--space-lg);
}
```

## Resources

- [Utopia Fluid Type Calculator](https://utopia.fyi/)
- [Modern Fluid Typography](https://www.smashingmagazine.com/2022/01/modern-fluid-typography-css-clamp/)
- [Every Layout](https://every-layout.dev/)
- [CSS min(), max(), and clamp()](https://web.dev/min-max-clamp/)
