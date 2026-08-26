
# Visual Design Foundations

Build cohesive, accessible visual systems using typography, color, spacing, and iconography fundamentals.

## When to Use This Skill

- Establishing design tokens for a new project
- Creating or refining a spacing and sizing system
- Selecting and pairing typefaces
- Building accessible color palettes
- Designing icon systems and visual assets
- Improving visual hierarchy and readability
- Auditing designs for visual consistency
- Implementing dark mode or theming

## Core Systems

### 1. Typography Scale

**Modular Scale** (ratio-based sizing):

```css
:root {
  --font-size-xs: 0.75rem; /* 12px */
  --font-size-sm: 0.875rem; /* 14px */
  --font-size-base: 1rem; /* 16px */
  --font-size-lg: 1.125rem; /* 18px */
  --font-size-xl: 1.25rem; /* 20px */
  --font-size-2xl: 1.5rem; /* 24px */
  --font-size-3xl: 1.875rem; /* 30px */
  --font-size-4xl: 2.25rem; /* 36px */
  --font-size-5xl: 3rem; /* 48px */
}
```

**Line Height Guidelines**:
| Text Type | Line Height |
|-----------|-------------|
| Headings | 1.1 - 1.3 |
| Body text | 1.5 - 1.7 |
| UI labels | 1.2 - 1.4 |

### 2. Spacing System

**8-point grid** (industry standard):

```css
:root {
  --space-1: 0.25rem; /* 4px */
  --space-2: 0.5rem; /* 8px */
  --space-3: 0.75rem; /* 12px */
  --space-4: 1rem; /* 16px */
  --space-5: 1.25rem; /* 20px */
  --space-6: 1.5rem; /* 24px */
  --space-8: 2rem; /* 32px */
  --space-10: 2.5rem; /* 40px */
  --space-12: 3rem; /* 48px */
  --space-16: 4rem; /* 64px */
}
```

### 3. Color System

**Semantic color tokens**:

```css
:root {
  /* Brand */
  --color-primary: #2563eb;
  --color-primary-hover: #1d4ed8;
  --color-primary-active: #1e40af;

  /* Semantic */
  --color-success: #16a34a;
  --color-warning: #ca8a04;
  --color-error: #dc2626;
  --color-info: #0891b2;

  /* Neutral */
  --color-gray-50: #f9fafb;
  --color-gray-100: #f3f4f6;
  --color-gray-200: #e5e7eb;
  --color-gray-300: #d1d5db;
  --color-gray-400: #9ca3af;
  --color-gray-500: #6b7280;
  --color-gray-600: #4b5563;
  --color-gray-700: #374151;
  --color-gray-800: #1f2937;
  --color-gray-900: #111827;
}
```

## Quick Start: Design Tokens in Tailwind

```js
// tailwind.config.js
module.exports = {
  theme: {
    extend: {
      fontFamily: {
        sans: ["Inter", "system-ui", "sans-serif"],
        mono: ["JetBrains Mono", "monospace"],
      },
      fontSize: {
        xs: ["0.75rem", { lineHeight: "1rem" }],
        sm: ["0.875rem", { lineHeight: "1.25rem" }],
        base: ["1rem", { lineHeight: "1.5rem" }],
        lg: ["1.125rem", { lineHeight: "1.75rem" }],
        xl: ["1.25rem", { lineHeight: "1.75rem" }],
        "2xl": ["1.5rem", { lineHeight: "2rem" }],
      },
      colors: {
        brand: {
          50: "#eff6ff",
          500: "#3b82f6",
          600: "#2563eb",
          700: "#1d4ed8",
        },
      },
      spacing: {
        // Extends default with custom values
        18: "4.5rem",
        88: "22rem",
      },
    },
  },
};
```

## Typography Best Practices

### Font Pairing

**Safe combinations**:

- Heading: **Inter** / Body: **Inter** (single family)
- Heading: **Playfair Display** / Body: **Source Sans Pro** (contrast)
- Heading: **Space Grotesk** / Body: **IBM Plex Sans** (geometric)

### Responsive Typography

```css
/* Fluid typography using clamp() */
h1 {
  font-size: clamp(2rem, 5vw + 1rem, 3.5rem);
  line-height: 1.1;
}

p {
  font-size: clamp(1rem, 2vw + 0.5rem, 1.125rem);
  line-height: 1.6;
  max-width: 65ch; /* Optimal reading width */
}
```

### Font Loading

```css
/* Prevent layout shift */
@font-face {
  font-family: "Inter";
  src: url("/fonts/Inter.woff2") format("woff2");
  font-display: swap;
  font-weight: 400 700;
}
```

## Color Theory

### Contrast Requirements (WCAG)

| Element            | Minimum Ratio |
| ------------------ | ------------- |
| Body text          | 4.5:1 (AA)    |
| Large text (18px+) | 3:1 (AA)      |
| UI components      | 3:1 (AA)      |
| Enhanced           | 7:1 (AAA)     |

### Dark Mode Strategy

```css
:root {
  --bg-primary: #ffffff;
  --bg-secondary: #f9fafb;
  --text-primary: #111827;
  --text-secondary: #6b7280;
  --border: #e5e7eb;
}

[data-theme="dark"] {
  --bg-primary: #111827;
  --bg-secondary: #1f2937;
  --text-primary: #f9fafb;
  --text-secondary: #9ca3af;
  --border: #374151;
}
```

### Color Accessibility

```tsx
// Check contrast programmatically
function getContrastRatio(foreground: string, background: string): number {
  const getLuminance = (hex: string) => {
    const rgb = hexToRgb(hex);
    const [r, g, b] = rgb.map((c) => {
      c = c / 255;
      return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
    });
    return 0.2126 * r + 0.7152 * g + 0.0722 * b;
  };

  const l1 = getLuminance(foreground);
  const l2 = getLuminance(background);
  const lighter = Math.max(l1, l2);
  const darker = Math.min(l1, l2);

  return (lighter + 0.05) / (darker + 0.05);
}
```

## Spacing Guidelines

### Component Spacing

```
Card padding:      16-24px (--space-4 to --space-6)
Section gap:       32-64px (--space-8 to --space-16)
Form field gap:    16-24px (--space-4 to --space-6)
Button padding:    8-16px vertical, 16-24px horizontal
Icon-text gap:     8px (--space-2)
```

### Visual Rhythm

```css
/* Consistent vertical rhythm */
.prose > * + * {
  margin-top: var(--space-4);
}

.prose > h2 + * {
  margin-top: var(--space-2);
}

.prose > * + h2 {
  margin-top: var(--space-8);
}
```

## Iconography

### Icon Sizing System

```css
:root {
  --icon-xs: 12px;
  --icon-sm: 16px;
  --icon-md: 20px;
  --icon-lg: 24px;
  --icon-xl: 32px;
}
```

### Icon Component

```tsx
interface IconProps {
  name: string;
  size?: "xs" | "sm" | "md" | "lg" | "xl";
  className?: string;
}

const sizeMap = {
  xs: 12,
  sm: 16,
  md: 20,
  lg: 24,
  xl: 32,
};

export function Icon({ name, size = "md", className }: IconProps) {
  return (
    <svg
      width={sizeMap[size]}
      height={sizeMap[size]}
      className={cn("inline-block flex-shrink-0", className)}
      aria-hidden="true"
    >
      <use href={`/icons.svg#${name}`} />
    </svg>
  );
}
```

## Best Practices

1. **Establish Constraints**: Limit choices to maintain consistency
2. **Document Decisions**: Create a living style guide
3. **Test Accessibility**: Verify contrast, sizing, touch targets
4. **Use Semantic Tokens**: Name by purpose, not appearance
5. **Design Mobile-First**: Start with constraints, add complexity
6. **Maintain Vertical Rhythm**: Consistent spacing creates harmony
7. **Limit Font Weights**: 2-3 weights per family is sufficient

## Common Issues

- **Inconsistent Spacing**: Not using a defined scale
- **Poor Contrast**: Failing WCAG requirements
- **Font Overload**: Too many families or weights
- **Magic Numbers**: Arbitrary values instead of tokens
- **Missing States**: Forgetting hover, focus, disabled
- **No Dark Mode Plan**: Retrofitting is harder than planning


---

## อ้างอิง: color-systems

# Color Systems Reference

## Color Palette Generation

### Perceptually Uniform Scales

Using OKLCH for perceptually uniform color scales:

```css
/* OKLCH: Lightness, Chroma, Hue */
:root {
  /* Generate a blue scale with consistent perceived lightness steps */
  --blue-50: oklch(97% 0.02 250);
  --blue-100: oklch(93% 0.04 250);
  --blue-200: oklch(86% 0.08 250);
  --blue-300: oklch(75% 0.12 250);
  --blue-400: oklch(65% 0.16 250);
  --blue-500: oklch(55% 0.2 250); /* Primary */
  --blue-600: oklch(48% 0.18 250);
  --blue-700: oklch(40% 0.16 250);
  --blue-800: oklch(32% 0.12 250);
  --blue-900: oklch(25% 0.08 250);
  --blue-950: oklch(18% 0.05 250);
}
```

### Programmatic Scale Generation

```tsx
function generateColorScale(
  hue: number,
  saturation: number = 100,
): Record<string, string> {
  const lightnessStops = [
    { name: "50", l: 97 },
    { name: "100", l: 93 },
    { name: "200", l: 85 },
    { name: "300", l: 75 },
    { name: "400", l: 65 },
    { name: "500", l: 55 },
    { name: "600", l: 45 },
    { name: "700", l: 35 },
    { name: "800", l: 25 },
    { name: "900", l: 18 },
    { name: "950", l: 12 },
  ];

  return Object.fromEntries(
    lightnessStops.map(({ name, l }) => [
      name,
      `hsl(${hue}, ${saturation}%, ${l}%)`,
    ]),
  );
}

// Generate semantic colors
const brand = generateColorScale(220); // Blue
const success = generateColorScale(142); // Green
const warning = generateColorScale(38); // Amber
const error = generateColorScale(0); // Red
```

## Semantic Color Tokens

### Two-Tier Token System

```css
/* Tier 1: Primitive colors (raw values) */
:root {
  --primitive-blue-500: #3b82f6;
  --primitive-blue-600: #2563eb;
  --primitive-green-500: #22c55e;
  --primitive-red-500: #ef4444;
  --primitive-gray-50: #f9fafb;
  --primitive-gray-900: #111827;
}

/* Tier 2: Semantic tokens (purpose-based) */
:root {
  /* Background */
  --color-bg-primary: var(--primitive-gray-50);
  --color-bg-secondary: white;
  --color-bg-tertiary: var(--primitive-gray-100);
  --color-bg-inverse: var(--primitive-gray-900);

  /* Text */
  --color-text-primary: var(--primitive-gray-900);
  --color-text-secondary: var(--primitive-gray-600);
  --color-text-tertiary: var(--primitive-gray-400);
  --color-text-inverse: white;
  --color-text-link: var(--primitive-blue-600);

  /* Border */
  --color-border-default: var(--primitive-gray-200);
  --color-border-strong: var(--primitive-gray-300);
  --color-border-focus: var(--primitive-blue-500);

  /* Interactive */
  --color-interactive-primary: var(--primitive-blue-600);
  --color-interactive-primary-hover: var(--primitive-blue-700);
  --color-interactive-primary-active: var(--primitive-blue-800);

  /* Status */
  --color-status-success: var(--primitive-green-500);
  --color-status-warning: var(--primitive-amber-500);
  --color-status-error: var(--primitive-red-500);
  --color-status-info: var(--primitive-blue-500);
}
```

### Component Tokens

```css
/* Tier 3: Component-specific tokens */
:root {
  /* Button */
  --button-bg: var(--color-interactive-primary);
  --button-bg-hover: var(--color-interactive-primary-hover);
  --button-text: white;
  --button-border-radius: 0.375rem;

  /* Input */
  --input-bg: var(--color-bg-secondary);
  --input-border: var(--color-border-default);
  --input-border-focus: var(--color-border-focus);
  --input-text: var(--color-text-primary);
  --input-placeholder: var(--color-text-tertiary);

  /* Card */
  --card-bg: var(--color-bg-secondary);
  --card-border: var(--color-border-default);
  --card-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
}
```

## Dark Mode Implementation

### CSS Custom Properties Approach

```css
/* Light theme (default) */
:root {
  --color-bg-primary: #ffffff;
  --color-bg-secondary: #f9fafb;
  --color-bg-tertiary: #f3f4f6;
  --color-text-primary: #111827;
  --color-text-secondary: #4b5563;
  --color-border-default: #e5e7eb;
}

/* Dark theme */
[data-theme="dark"] {
  --color-bg-primary: #111827;
  --color-bg-secondary: #1f2937;
  --color-bg-tertiary: #374151;
  --color-text-primary: #f9fafb;
  --color-text-secondary: #9ca3af;
  --color-border-default: #374151;
}

/* System preference */
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --color-bg-primary: #111827;
    /* ... dark theme values */
  }
}
```

### React Theme Context

```tsx
import { createContext, useContext, useEffect, useState } from "react";

type Theme = "light" | "dark" | "system";

interface ThemeContextValue {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  resolvedTheme: "light" | "dark";
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [theme, setTheme] = useState<Theme>("system");
  const [resolvedTheme, setResolvedTheme] = useState<"light" | "dark">("light");

  useEffect(() => {
    const root = document.documentElement;

    if (theme === "system") {
      const systemTheme = window.matchMedia("(prefers-color-scheme: dark)")
        .matches
        ? "dark"
        : "light";
      setResolvedTheme(systemTheme);
      root.setAttribute("data-theme", systemTheme);
    } else {
      setResolvedTheme(theme);
      root.setAttribute("data-theme", theme);
    }
  }, [theme]);

  useEffect(() => {
    if (theme !== "system") return;

    const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
    const handler = (e: MediaQueryListEvent) => {
      const newTheme = e.matches ? "dark" : "light";
      setResolvedTheme(newTheme);
      document.documentElement.setAttribute("data-theme", newTheme);
    };

    mediaQuery.addEventListener("change", handler);
    return () => mediaQuery.removeEventListener("change", handler);
  }, [theme]);

  return (
    <ThemeContext.Provider value={{ theme, setTheme, resolvedTheme }}>
      {children}
    </ThemeContext.Provider>
  );
}

export function useTheme() {
  const context = useContext(ThemeContext);
  if (!context) throw new Error("useTheme must be within ThemeProvider");
  return context;
}
```

## Contrast and Accessibility

### WCAG Contrast Checker

```tsx
function hexToRgb(hex: string): [number, number, number] {
  const result = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(hex);
  if (!result) throw new Error("Invalid hex color");
  return [
    parseInt(result[1], 16),
    parseInt(result[2], 16),
    parseInt(result[3], 16),
  ];
}

function getLuminance(r: number, g: number, b: number): number {
  const [rs, gs, bs] = [r, g, b].map((c) => {
    c = c / 255;
    return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
  });
  return 0.2126 * rs + 0.7152 * gs + 0.0722 * bs;
}

function getContrastRatio(hex1: string, hex2: string): number {
  const [r1, g1, b1] = hexToRgb(hex1);
  const [r2, g2, b2] = hexToRgb(hex2);

  const l1 = getLuminance(r1, g1, b1);
  const l2 = getLuminance(r2, g2, b2);

  const lighter = Math.max(l1, l2);
  const darker = Math.min(l1, l2);

  return (lighter + 0.05) / (darker + 0.05);
}

function meetsWCAG(
  foreground: string,
  background: string,
  size: "normal" | "large" = "normal",
  level: "AA" | "AAA" = "AA",
): boolean {
  const ratio = getContrastRatio(foreground, background);

  const requirements = {
    normal: { AA: 4.5, AAA: 7 },
    large: { AA: 3, AAA: 4.5 },
  };

  return ratio >= requirements[size][level];
}

// Usage
meetsWCAG("#ffffff", "#3b82f6"); // true (4.5:1 for AA normal)
meetsWCAG("#ffffff", "#60a5fa"); // false (below 4.5:1)
```

### Accessible Color Pairs

```tsx
// Generate accessible text color for any background
function getAccessibleTextColor(backgroundColor: string): string {
  const [r, g, b] = hexToRgb(backgroundColor);
  const luminance = getLuminance(r, g, b);

  // Use white text on dark backgrounds, black on light
  return luminance > 0.179 ? "#111827" : "#ffffff";
}

// Find the nearest accessible shade
function findAccessibleShade(
  textColor: string,
  backgroundScale: string[],
  minContrast: number = 4.5,
): string | null {
  for (const shade of backgroundScale) {
    if (getContrastRatio(textColor, shade) >= minContrast) {
      return shade;
    }
  }
  return null;
}
```

## Color Harmony

### Harmony Functions

```tsx
type HarmonyType =
  | "complementary"
  | "triadic"
  | "analogous"
  | "split-complementary";

function generateHarmony(baseHue: number, type: HarmonyType): number[] {
  switch (type) {
    case "complementary":
      return [baseHue, (baseHue + 180) % 360];
    case "triadic":
      return [baseHue, (baseHue + 120) % 360, (baseHue + 240) % 360];
    case "analogous":
      return [(baseHue - 30 + 360) % 360, baseHue, (baseHue + 30) % 360];
    case "split-complementary":
      return [baseHue, (baseHue + 150) % 360, (baseHue + 210) % 360];
    default:
      return [baseHue];
  }
}

// Generate palette from harmony
function generateHarmoniousPalette(
  baseHue: number,
  type: HarmonyType,
): Record<string, string> {
  const hues = generateHarmony(baseHue, type);
  const names = ["primary", "secondary", "tertiary"];

  return Object.fromEntries(
    hues.map((hue, i) => [names[i] || `color-${i}`, `hsl(${hue}, 70%, 50%)`]),
  );
}
```

## Color Blindness Considerations

```tsx
// Simulate color blindness
type ColorBlindnessType = "protanopia" | "deuteranopia" | "tritanopia";

// Matrix transforms for common types
const colorBlindnessMatrices: Record<ColorBlindnessType, number[][]> = {
  protanopia: [
    [0.567, 0.433, 0],
    [0.558, 0.442, 0],
    [0, 0.242, 0.758],
  ],
  deuteranopia: [
    [0.625, 0.375, 0],
    [0.7, 0.3, 0],
    [0, 0.3, 0.7],
  ],
  tritanopia: [
    [0.95, 0.05, 0],
    [0, 0.433, 0.567],
    [0, 0.475, 0.525],
  ],
};

// Best practices for color-blind accessibility:
// 1. Do not rely solely on color to convey information
// 2. Use patterns or icons alongside color
// 3. Ensure sufficient contrast between colors
// 4. Test with color blindness simulators
// 5. Use blue-orange instead of red-green for contrast
```

## CSS Color Functions

```css
/* Modern CSS color functions */
.modern-colors {
  /* Relative color syntax */
  --lighter: hsl(from var(--base-color) h s calc(l + 20%));
  --darker: hsl(from var(--base-color) h s calc(l - 20%));

  /* Color mixing */
  --mixed: color-mix(in srgb, var(--color-1), var(--color-2) 30%);

  /* Transparency */
  --semi-transparent: rgb(from var(--base-color) r g b / 50%);

  /* OKLCH for perceptual uniformity */
  --vibrant-blue: oklch(60% 0.2 250);
}

/* Alpha variations */
.alpha-scale {
  --color-10: rgb(59 130 246 / 0.1);
  --color-20: rgb(59 130 246 / 0.2);
  --color-30: rgb(59 130 246 / 0.3);
  --color-40: rgb(59 130 246 / 0.4);
  --color-50: rgb(59 130 246 / 0.5);
}
```


---

## อ้างอิง: spacing-iconography

# Spacing and Iconography Reference

## Spacing Systems

### 8-Point Grid System

The 8-point grid is the industry standard for consistent spacing.

```css
:root {
  /* Base spacing unit */
  --space-unit: 0.25rem; /* 4px */

  /* Spacing scale */
  --space-0: 0;
  --space-px: 1px;
  --space-0-5: calc(var(--space-unit) * 0.5); /* 2px */
  --space-1: var(--space-unit); /* 4px */
  --space-1-5: calc(var(--space-unit) * 1.5); /* 6px */
  --space-2: calc(var(--space-unit) * 2); /* 8px */
  --space-2-5: calc(var(--space-unit) * 2.5); /* 10px */
  --space-3: calc(var(--space-unit) * 3); /* 12px */
  --space-3-5: calc(var(--space-unit) * 3.5); /* 14px */
  --space-4: calc(var(--space-unit) * 4); /* 16px */
  --space-5: calc(var(--space-unit) * 5); /* 20px */
  --space-6: calc(var(--space-unit) * 6); /* 24px */
  --space-7: calc(var(--space-unit) * 7); /* 28px */
  --space-8: calc(var(--space-unit) * 8); /* 32px */
  --space-9: calc(var(--space-unit) * 9); /* 36px */
  --space-10: calc(var(--space-unit) * 10); /* 40px */
  --space-11: calc(var(--space-unit) * 11); /* 44px */
  --space-12: calc(var(--space-unit) * 12); /* 48px */
  --space-14: calc(var(--space-unit) * 14); /* 56px */
  --space-16: calc(var(--space-unit) * 16); /* 64px */
  --space-20: calc(var(--space-unit) * 20); /* 80px */
  --space-24: calc(var(--space-unit) * 24); /* 96px */
  --space-28: calc(var(--space-unit) * 28); /* 112px */
  --space-32: calc(var(--space-unit) * 32); /* 128px */
}
```

### Semantic Spacing Tokens

```css
:root {
  /* Component-level spacing */
  --spacing-xs: var(--space-1); /* 4px - tight spacing */
  --spacing-sm: var(--space-2); /* 8px - compact spacing */
  --spacing-md: var(--space-4); /* 16px - default spacing */
  --spacing-lg: var(--space-6); /* 24px - comfortable spacing */
  --spacing-xl: var(--space-8); /* 32px - loose spacing */
  --spacing-2xl: var(--space-12); /* 48px - generous spacing */
  --spacing-3xl: var(--space-16); /* 64px - section spacing */

  /* Specific use cases */
  --spacing-inline: var(--space-2); /* Between inline elements */
  --spacing-stack: var(--space-4); /* Between stacked elements */
  --spacing-inset: var(--space-4); /* Padding inside containers */
  --spacing-section: var(--space-16); /* Between major sections */
  --spacing-page: var(--space-24); /* Page margins */
}
```

### Spacing Utility Functions

```tsx
// Tailwind-like spacing scale generator
function createSpacingScale(baseUnit: number = 4): Record<string, string> {
  const scale: Record<string, string> = {
    "0": "0",
    px: "1px",
  };

  const multipliers = [
    0.5, 1, 1.5, 2, 2.5, 3, 3.5, 4, 5, 6, 7, 8, 9, 10, 11, 12, 14, 16, 20, 24,
    28, 32, 36, 40, 44, 48, 52, 56, 60, 64, 72, 80, 96,
  ];

  for (const m of multipliers) {
    const key = m % 1 === 0 ? String(m) : String(m).replace(".", "-");
    scale[key] = `${baseUnit * m}px`;
  }

  return scale;
}
```

## Layout Spacing Patterns

### Container Queries for Spacing

```css
/* Responsive spacing based on container size */
.card {
  container-type: inline-size;
  padding: var(--space-4);
}

@container (min-width: 400px) {
  .card {
    padding: var(--space-6);
  }
}

@container (min-width: 600px) {
  .card {
    padding: var(--space-8);
  }
}
```

### Negative Space Patterns

```css
/* Asymmetric spacing for visual hierarchy */
.hero-section {
  padding-top: var(--space-24);
  padding-bottom: var(--space-16);
}

/* Content breathing room */
.prose > * + * {
  margin-top: var(--space-4);
}

.prose > h2 + * {
  margin-top: var(--space-2);
}

.prose > * + h2 {
  margin-top: var(--space-8);
}
```

## Icon Systems

### Icon Size Scale

```css
:root {
  /* Icon sizes aligned to spacing grid */
  --icon-xs: 12px; /* Inline decorators */
  --icon-sm: 16px; /* Small UI elements */
  --icon-md: 20px; /* Default size */
  --icon-lg: 24px; /* Emphasis */
  --icon-xl: 32px; /* Large displays */
  --icon-2xl: 48px; /* Hero icons */

  /* Touch target sizes */
  --touch-target-min: 44px; /* WCAG minimum */
  --touch-target-comfortable: 48px;
}
```

### SVG Icon Component

```tsx
import { forwardRef, type SVGProps } from "react";

interface IconProps extends SVGProps<SVGSVGElement> {
  name: string;
  size?: "xs" | "sm" | "md" | "lg" | "xl" | "2xl";
  label?: string;
}

const sizeMap = {
  xs: 12,
  sm: 16,
  md: 20,
  lg: 24,
  xl: 32,
  "2xl": 48,
};

export const Icon = forwardRef<SVGSVGElement, IconProps>(
  ({ name, size = "md", label, className, ...props }, ref) => {
    const pixelSize = sizeMap[size];

    return (
      <svg
        ref={ref}
        width={pixelSize}
        height={pixelSize}
        className={`inline-block flex-shrink-0 ${className}`}
        aria-hidden={!label}
        aria-label={label}
        role={label ? "img" : undefined}
        {...props}
      >
        <use href={`/icons.svg#${name}`} />
      </svg>
    );
  },
);

Icon.displayName = "Icon";
```

### Icon Button Patterns

```tsx
interface IconButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  icon: string;
  label: string;
  size?: "sm" | "md" | "lg";
  variant?: "solid" | "ghost" | "outline";
}

const sizeClasses = {
  sm: "p-1.5" /* 32px total with 16px icon */,
  md: "p-2" /* 40px total with 20px icon */,
  lg: "p-2.5" /* 48px total with 24px icon */,
};

const iconSizes = {
  sm: "sm" as const,
  md: "md" as const,
  lg: "lg" as const,
};

export function IconButton({
  icon,
  label,
  size = "md",
  variant = "ghost",
  className,
  ...props
}: IconButtonProps) {
  return (
    <button
      className={`
        inline-flex items-center justify-center rounded-lg
        transition-colors focus-visible:outline-none focus-visible:ring-2
        ${sizeClasses[size]}
        ${variant === "solid" && "bg-blue-600 text-white hover:bg-blue-700"}
        ${variant === "ghost" && "hover:bg-gray-100"}
        ${variant === "outline" && "border border-gray-300 hover:bg-gray-50"}
        ${className}
      `}
      aria-label={label}
      {...props}
    >
      <Icon name={icon} size={iconSizes[size]} />
    </button>
  );
}
```

### Icon Sprite Generation

```tsx
// Build script for SVG sprite
import { readdir, readFile, writeFile } from "fs/promises";
import { optimize } from "svgo";

async function buildIconSprite(iconDir: string, outputPath: string) {
  const files = await readdir(iconDir);
  const svgFiles = files.filter((f) => f.endsWith(".svg"));

  const symbols = await Promise.all(
    svgFiles.map(async (file) => {
      const content = await readFile(`${iconDir}/${file}`, "utf-8");
      const name = file.replace(".svg", "");

      // Optimize SVG
      const result = optimize(content, {
        plugins: [
          "removeDoctype",
          "removeXMLProcInst",
          "removeComments",
          "removeMetadata",
          "removeTitle",
          "removeDesc",
          "removeUselessDefs",
          "removeEditorsNSData",
          "removeEmptyAttrs",
          "removeHiddenElems",
          "removeEmptyText",
          "removeEmptyContainers",
          "convertStyleToAttrs",
          "convertColors",
          "convertPathData",
          "convertTransform",
          "removeUnknownsAndDefaults",
          "removeNonInheritableGroupAttrs",
          "removeUselessStrokeAndFill",
          "removeUnusedNS",
          "cleanupNumericValues",
          "cleanupListOfValues",
          "moveElemsAttrsToGroup",
          "moveGroupAttrsToElems",
          "collapseGroups",
          "mergePaths",
        ],
      });

      // Extract viewBox and content
      const viewBoxMatch = result.data.match(/viewBox="([^"]+)"/);
      const viewBox = viewBoxMatch ? viewBoxMatch[1] : "0 0 24 24";
      const innerContent = result.data
        .replace(/<svg[^>]*>/, "")
        .replace(/<\/svg>/, "");

      return `<symbol id="${name}" viewBox="${viewBox}">${innerContent}</symbol>`;
    }),
  );

  const sprite = `<svg xmlns="http://www.w3.org/2000/svg" style="display:none">${symbols.join("")}</svg>`;

  await writeFile(outputPath, sprite);
  console.log(`Generated sprite with ${symbols.length} icons`);
}
```

### Icon Libraries Integration

```tsx
// Lucide React
import { Home, Settings, User, Search } from "lucide-react";

function Navigation() {
  return (
    <nav className="flex gap-4">
      <NavItem icon={Home} label="Home" />
      <NavItem icon={Search} label="Search" />
      <NavItem icon={Settings} label="Settings" />
      <NavItem icon={User} label="Profile" />
    </nav>
  );
}

// Heroicons
import { HomeIcon, Cog6ToothIcon } from "@heroicons/react/24/outline";
import { HomeIcon as HomeIconSolid } from "@heroicons/react/24/solid";

function ToggleIcon({ active }: { active: boolean }) {
  const Icon = active ? HomeIconSolid : HomeIcon;
  return <Icon className="w-6 h-6" />;
}

// Radix Icons
import { HomeIcon, GearIcon } from "@radix-ui/react-icons";
```

## Sizing Systems

### Element Sizing Scale

```css
:root {
  /* Fixed sizes */
  --size-4: 1rem; /* 16px */
  --size-5: 1.25rem; /* 20px */
  --size-6: 1.5rem; /* 24px */
  --size-8: 2rem; /* 32px */
  --size-10: 2.5rem; /* 40px */
  --size-12: 3rem; /* 48px */
  --size-14: 3.5rem; /* 56px */
  --size-16: 4rem; /* 64px */
  --size-20: 5rem; /* 80px */
  --size-24: 6rem; /* 96px */
  --size-32: 8rem; /* 128px */

  /* Component heights */
  --height-input-sm: var(--size-8); /* 32px */
  --height-input-md: var(--size-10); /* 40px */
  --height-input-lg: var(--size-12); /* 48px */

  /* Avatar sizes */
  --avatar-xs: var(--size-6); /* 24px */
  --avatar-sm: var(--size-8); /* 32px */
  --avatar-md: var(--size-10); /* 40px */
  --avatar-lg: var(--size-12); /* 48px */
  --avatar-xl: var(--size-16); /* 64px */
  --avatar-2xl: var(--size-24); /* 96px */
}
```

### Aspect Ratios

```css
.aspect-ratios {
  /* Standard ratios */
  --aspect-square: 1 / 1;
  --aspect-video: 16 / 9;
  --aspect-photo: 4 / 3;
  --aspect-portrait: 3 / 4;
  --aspect-cinema: 21 / 9;
  --aspect-golden: 1.618 / 1;
}

/* Usage */
.thumbnail {
  aspect-ratio: var(--aspect-video);
  object-fit: cover;
}

.avatar {
  aspect-ratio: var(--aspect-square);
  border-radius: 50%;
}
```

### Border Radius Scale

```css
:root {
  --radius-none: 0;
  --radius-sm: 0.125rem; /* 2px */
  --radius-default: 0.25rem; /* 4px */
  --radius-md: 0.375rem; /* 6px */
  --radius-lg: 0.5rem; /* 8px */
  --radius-xl: 0.75rem; /* 12px */
  --radius-2xl: 1rem; /* 16px */
  --radius-3xl: 1.5rem; /* 24px */
  --radius-full: 9999px;

  /* Component-specific */
  --radius-button: var(--radius-md);
  --radius-input: var(--radius-md);
  --radius-card: var(--radius-lg);
  --radius-modal: var(--radius-xl);
  --radius-badge: var(--radius-full);
}
```


---

## อ้างอิง: typography-systems

# Typography Systems Reference

## Type Scale Construction

### Modular Scale

A modular scale creates harmonious relationships between font sizes using a mathematical ratio.

```tsx
// Common ratios
const RATIOS = {
  minorSecond: 1.067, // 16:15
  majorSecond: 1.125, // 9:8
  minorThird: 1.2, // 6:5
  majorThird: 1.25, // 5:4
  perfectFourth: 1.333, // 4:3
  augmentedFourth: 1.414, // √2
  perfectFifth: 1.5, // 3:2
  goldenRatio: 1.618, // φ
};

function generateScale(
  baseSize: number,
  ratio: number,
  steps: number,
): number[] {
  const scale: number[] = [];
  for (let i = -2; i <= steps; i++) {
    scale.push(Math.round(baseSize * Math.pow(ratio, i) * 100) / 100);
  }
  return scale;
}

// Generate a scale with 16px base and perfect fourth ratio
const typeScale = generateScale(16, RATIOS.perfectFourth, 6);
// Result: [9, 12, 16, 21.33, 28.43, 37.9, 50.52, 67.34, 89.76]
```

### CSS Custom Properties

```css
:root {
  /* Base scale using perfect fourth (1.333) */
  --font-size-2xs: 0.563rem; /* ~9px */
  --font-size-xs: 0.75rem; /* 12px */
  --font-size-sm: 0.875rem; /* 14px */
  --font-size-base: 1rem; /* 16px */
  --font-size-md: 1.125rem; /* 18px */
  --font-size-lg: 1.333rem; /* ~21px */
  --font-size-xl: 1.5rem; /* 24px */
  --font-size-2xl: 1.777rem; /* ~28px */
  --font-size-3xl: 2.369rem; /* ~38px */
  --font-size-4xl: 3.157rem; /* ~50px */
  --font-size-5xl: 4.209rem; /* ~67px */

  /* Font weights */
  --font-weight-normal: 400;
  --font-weight-medium: 500;
  --font-weight-semibold: 600;
  --font-weight-bold: 700;

  /* Line heights */
  --line-height-tight: 1.1;
  --line-height-snug: 1.25;
  --line-height-normal: 1.5;
  --line-height-relaxed: 1.625;
  --line-height-loose: 2;

  /* Letter spacing */
  --letter-spacing-tighter: -0.05em;
  --letter-spacing-tight: -0.025em;
  --letter-spacing-normal: 0;
  --letter-spacing-wide: 0.025em;
  --letter-spacing-wider: 0.05em;
  --letter-spacing-widest: 0.1em;
}
```

## Font Loading Strategies

### FOUT Prevention

```css
/* Use font-display to control loading behavior */
@font-face {
  font-family: "Inter";
  src: url("/fonts/Inter-Variable.woff2") format("woff2-variations");
  font-weight: 100 900;
  font-style: normal;
  font-display: swap; /* Show fallback immediately, swap when loaded */
}

/* Optional: size-adjust for better fallback matching */
@font-face {
  font-family: "Inter Fallback";
  src: local("Arial");
  size-adjust: 107%; /* Adjust to match Inter metrics */
  ascent-override: 90%;
  descent-override: 22%;
  line-gap-override: 0%;
}

body {
  font-family: "Inter", "Inter Fallback", system-ui, sans-serif;
}
```

### Preloading Critical Fonts

```html
<head>
  <!-- Preload critical fonts -->
  <link
    rel="preload"
    href="/fonts/Inter-Variable.woff2"
    as="font"
    type="font/woff2"
    crossorigin
  />
</head>
```

### Variable Fonts

```css
/* Variable font with weight and width axes */
@font-face {
  font-family: "Inter";
  src: url("/fonts/Inter-Variable.woff2") format("woff2");
  font-weight: 100 900;
  font-stretch: 75% 125%;
}

/* Use font-variation-settings for fine control */
.custom-weight {
  font-variation-settings:
    "wght" 450,
    "wdth" 95;
}

/* Or use standard properties */
.semi-expanded {
  font-weight: 550;
  font-stretch: 110%;
}
```

## Responsive Typography

### Fluid Type Scale

```css
/* Using clamp() for responsive sizing */
h1 {
  /* min: 32px, preferred: 5vw + 16px, max: 64px */
  font-size: clamp(2rem, 5vw + 1rem, 4rem);
  line-height: 1.1;
}

h2 {
  font-size: clamp(1.5rem, 3vw + 0.5rem, 2.5rem);
  line-height: 1.2;
}

p {
  font-size: clamp(1rem, 1vw + 0.75rem, 1.25rem);
  line-height: 1.6;
}

/* Fluid line height */
.fluid-text {
  --min-line-height: 1.3;
  --max-line-height: 1.6;
  --min-vw: 320;
  --max-vw: 1200;

  line-height: calc(
    var(--min-line-height) + (var(--max-line-height) - var(--min-line-height)) *
      ((100vw - var(--min-vw) * 1px) / (var(--max-vw) - var(--min-vw)))
  );
}
```

### Viewport-Based Scaling

```tsx
// Tailwind config for responsive type
module.exports = {
  theme: {
    fontSize: {
      xs: ["0.75rem", { lineHeight: "1rem" }],
      sm: ["0.875rem", { lineHeight: "1.25rem" }],
      base: ["1rem", { lineHeight: "1.5rem" }],
      lg: ["1.125rem", { lineHeight: "1.75rem" }],
      xl: ["1.25rem", { lineHeight: "1.75rem" }],
      "2xl": ["1.5rem", { lineHeight: "2rem" }],
      "3xl": ["1.875rem", { lineHeight: "2.25rem" }],
      "4xl": ["2.25rem", { lineHeight: "2.5rem" }],
      "5xl": ["3rem", { lineHeight: "1" }],
    },
  },
};

// Component with responsive classes
function Heading({ children }) {
  return (
    <h1 className="text-3xl md:text-4xl lg:text-5xl font-bold leading-tight">
      {children}
    </h1>
  );
}
```

## Readability Guidelines

### Optimal Line Length

```css
/* Optimal reading width: 45-75 characters */
.prose {
  max-width: 65ch; /* ~65 characters */
}

/* Narrower for callouts */
.callout {
  max-width: 50ch;
}

/* Wider for code blocks */
pre {
  max-width: 80ch;
}
```

### Vertical Rhythm

```css
/* Establish baseline grid */
:root {
  --baseline: 1.5rem; /* 24px at 16px base */
}

/* All margins should be multiples of baseline */
h1 {
  font-size: 2.5rem;
  line-height: calc(var(--baseline) * 2);
  margin-top: calc(var(--baseline) * 2);
  margin-bottom: var(--baseline);
}

h2 {
  font-size: 2rem;
  line-height: calc(var(--baseline) * 1.5);
  margin-top: calc(var(--baseline) * 1.5);
  margin-bottom: calc(var(--baseline) * 0.5);
}

p {
  font-size: 1rem;
  line-height: var(--baseline);
  margin-bottom: var(--baseline);
}
```

### Text Wrapping

```css
/* Prevent orphans and widows */
p {
  text-wrap: pretty; /* Experimental: improves line breaks */
  widows: 3;
  orphans: 3;
}

/* Balance headings */
h1,
h2,
h3 {
  text-wrap: balance;
}

/* Prevent breaking in specific elements */
.no-wrap {
  white-space: nowrap;
}

/* Hyphenation for justified text */
.justified {
  text-align: justify;
  hyphens: auto;
  -webkit-hyphens: auto;
}
```

## Font Pairing Guidelines

### Contrast Pairings

```css
/* Serif heading + Sans body */
:root {
  --font-heading: "Playfair Display", Georgia, serif;
  --font-body: "Source Sans Pro", -apple-system, sans-serif;
}

/* Geometric heading + Humanist body */
:root {
  --font-heading: "Space Grotesk", sans-serif;
  --font-body: "IBM Plex Sans", sans-serif;
}

/* Modern sans heading + Classic serif body */
:root {
  --font-heading: "Inter", system-ui, sans-serif;
  --font-body: "Georgia", Times, serif;
}
```

### Superfamily Approach

```css
/* Single variable font family for all uses */
:root {
  --font-family: "Inter", system-ui, sans-serif;
}

h1 {
  font-family: var(--font-family);
  font-weight: 800;
  letter-spacing: -0.02em;
}

p {
  font-family: var(--font-family);
  font-weight: 400;
  letter-spacing: 0;
}

.caption {
  font-family: var(--font-family);
  font-weight: 500;
  font-size: 0.875rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
```

## Semantic Typography Classes

```css
/* Text styles by purpose, not appearance */
.text-display {
  font-size: var(--font-size-5xl);
  font-weight: var(--font-weight-bold);
  line-height: var(--line-height-tight);
  letter-spacing: var(--letter-spacing-tight);
}

.text-headline {
  font-size: var(--font-size-3xl);
  font-weight: var(--font-weight-semibold);
  line-height: var(--line-height-snug);
}

.text-title {
  font-size: var(--font-size-xl);
  font-weight: var(--font-weight-semibold);
  line-height: var(--line-height-snug);
}

.text-body {
  font-size: var(--font-size-base);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
}

.text-body-sm {
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
}

.text-caption {
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-medium);
  line-height: var(--line-height-normal);
  text-transform: uppercase;
  letter-spacing: var(--letter-spacing-wide);
}

.text-overline {
  font-size: var(--font-size-2xs);
  font-weight: var(--font-weight-semibold);
  line-height: var(--line-height-normal);
  text-transform: uppercase;
  letter-spacing: var(--letter-spacing-widest);
}
```

## OpenType Features

```css
/* Enable advanced typography features */
.fancy-text {
  /* Small caps */
  font-variant-caps: small-caps;

  /* Ligatures */
  font-variant-ligatures: common-ligatures;

  /* Numeric features */
  font-variant-numeric: tabular-nums lining-nums;

  /* Fractions */
  font-feature-settings: "frac" 1;
}

/* Tabular numbers for aligned columns */
.data-table td {
  font-variant-numeric: tabular-nums;
}

/* Old-style figures for body text */
.prose {
  font-variant-numeric: oldstyle-nums;
}

/* Discretionary ligatures for headings */
.fancy-heading {
  font-variant-ligatures: discretionary-ligatures;
}
```
