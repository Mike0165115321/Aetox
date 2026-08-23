# Tailwind Integration

Map design system tokens to Tailwind CSS configuration — but check which of two
contexts applies first, because they take different routes entirely.

## Which route applies

| Context | Route |
|---|---|
| A deck, banner, or artifact written as one self-contained `.html` file | No Tailwind, no build step, no `npx`. Write the CSS variables straight into a `<style>` block and consume them with `var(--token)`. Nothing below is reachable — there is no `node_modules`, no config file gets read, and a CDN-fetched `<script>` can render blank on export if the CDN is not reachable at export time. See this skill's own Slide System section: a deck ships as one file with nothing linked in beside it. |
| A real project on the user's machine that already has Tailwind installed | Everything below applies as written. |

Reading this file while writing a self-contained artifact and reaching for
`@tailwind base` is the single most common way this reference gets misapplied.

## CSS Variables Setup

### Base Layer

```css
/* globals.css */
@tailwind base;
@tailwind components;
@tailwind utilities;

@layer base {
  :root {
    /* Primitives */
    --color-blue-600: 37 99 235;  /* HSL: 217 91% 60% */

    /* Semantic */
    --background: 0 0% 100%;
    --foreground: 222 47% 11%;
    --primary: 217 91% 60%;
    --primary-foreground: 0 0% 100%;
    --secondary: 220 14% 96%;
    --secondary-foreground: 222 47% 11%;
    --muted: 220 14% 96%;
    --muted-foreground: 220 9% 46%;
    --accent: 220 14% 96%;
    --accent-foreground: 222 47% 11%;
    --destructive: 0 84% 60%;
    --destructive-foreground: 0 0% 100%;
    --border: 220 13% 91%;
    --input: 220 13% 91%;
    --ring: 217 91% 60%;
    --radius: 0.5rem;
  }

  .dark {
    --background: 222 47% 4%;
    --foreground: 210 40% 98%;
    --primary: 217 91% 60%;
    --primary-foreground: 0 0% 100%;
    --secondary: 217 33% 17%;
    --secondary-foreground: 210 40% 98%;
    --muted: 217 33% 17%;
    --muted-foreground: 215 20% 65%;
    --accent: 217 33% 17%;
    --accent-foreground: 210 40% 98%;
    --destructive: 0 62% 30%;
    --destructive-foreground: 0 0% 100%;
    --border: 217 33% 17%;
    --input: 217 33% 17%;
    --ring: 217 91% 60%;
  }
}
```

## Tailwind Config

### tailwind.config.ts

```typescript
import type { Config } from 'tailwindcss'

const config: Config = {
  darkMode: ['class'],
  content: ['./src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        background: 'hsl(var(--background))',
        foreground: 'hsl(var(--foreground))',
        primary: {
          DEFAULT: 'hsl(var(--primary))',
          foreground: 'hsl(var(--primary-foreground))',
        },
        secondary: {
          DEFAULT: 'hsl(var(--secondary))',
          foreground: 'hsl(var(--secondary-foreground))',
        },
        muted: {
          DEFAULT: 'hsl(var(--muted))',
          foreground: 'hsl(var(--muted-foreground))',
        },
        accent: {
          DEFAULT: 'hsl(var(--accent))',
          foreground: 'hsl(var(--accent-foreground))',
        },
        destructive: {
          DEFAULT: 'hsl(var(--destructive))',
          foreground: 'hsl(var(--destructive-foreground))',
        },
        border: 'hsl(var(--border))',
        input: 'hsl(var(--input))',
        ring: 'hsl(var(--ring))',
        card: {
          DEFAULT: 'hsl(var(--card))',
          foreground: 'hsl(var(--card-foreground))',
        },
      },
      borderRadius: {
        lg: 'var(--radius)',
        md: 'calc(var(--radius) - 2px)',
        sm: 'calc(var(--radius) - 4px)',
      },
    },
  },
  plugins: [],
}

export default config
```

## HSL Format Benefits

Using HSL without function allows opacity modifiers:

```tsx
// With HSL format (space-separated)
<div className="bg-primary/50">   // 50% opacity
<div className="text-primary/80"> // 80% opacity

// CSS output
background-color: hsl(217 91% 60% / 0.5);
```

## Component Classes

### Button Example

```css
@layer components {
  .btn {
    @apply inline-flex items-center justify-center
           rounded-md font-medium
           transition-colors
           focus-visible:outline-none focus-visible:ring-2
           focus-visible:ring-ring focus-visible:ring-offset-2
           disabled:pointer-events-none disabled:opacity-50;
  }

  .btn-default {
    @apply bg-primary text-primary-foreground
           hover:bg-primary/90;
  }

  .btn-secondary {
    @apply bg-secondary text-secondary-foreground
           hover:bg-secondary/80;
  }

  .btn-outline {
    @apply border border-input bg-background
           hover:bg-accent hover:text-accent-foreground;
  }

  .btn-ghost {
    @apply hover:bg-accent hover:text-accent-foreground;
  }

  .btn-destructive {
    @apply bg-destructive text-destructive-foreground
           hover:bg-destructive/90;
  }

  /* Sizes */
  .btn-sm { @apply h-8 px-3 text-xs; }
  .btn-md { @apply h-10 px-4 text-sm; }
  .btn-lg { @apply h-12 px-6 text-base; }
}
```

## Spacing Integration

```typescript
// tailwind.config.ts
theme: {
  extend: {
    spacing: {
      // Map to CSS variables if needed
      'section': 'var(--spacing-section)',
      'component': 'var(--spacing-component)',
    }
  }
}
```

## Animation Tokens

```typescript
// tailwind.config.ts
theme: {
  extend: {
    transitionDuration: {
      fast: '150ms',
      normal: '200ms',
      slow: '300ms',
    },
    keyframes: {
      'accordion-down': {
        from: { height: '0' },
        to: { height: 'var(--radix-accordion-content-height)' },
      },
      'accordion-up': {
        from: { height: 'var(--radix-accordion-content-height)' },
        to: { height: '0' },
      },
    },
    animation: {
      'accordion-down': 'accordion-down 0.2s ease-out',
      'accordion-up': 'accordion-up 0.2s ease-out',
    },
  }
}
```

## Dark Mode Toggle

```typescript
// Toggle dark mode
function toggleDarkMode() {
  document.documentElement.classList.toggle('dark')
}

// System preference
if (window.matchMedia('(prefers-color-scheme: dark)').matches) {
  document.documentElement.classList.add('dark')
}
```

Set the `.dark` class in an inline `<script>` in `<head>`, before first paint
— not after a framework mounts — or the page flashes the wrong theme for one
frame. See `color-and-contrast.md` for how to decide light vs. dark vs.
system-synced as the *default* in the first place.

**Never hand-write a `dark:` override on a component that already consumes a
semantic token** (`bg-primary`, not `bg-blue-600 dark:bg-blue-400`) — the
semantic layer is what should flip when the mode changes, not the component.

### Concrete numbers for a dark surface

| Rule | Value | Why |
|---|---|---|
| Primary text contrast | 12:1–16:1, not 21:1 | Pure white on pure black reads as visually harsh; slightly reduced contrast is more comfortable for long text |
| Dark base color | `#0c1222`, not `#000000` | Pure black crushes shadow/elevation cues to nothing |
| Elevation | A lighter tint of the surface color (tonal), not a drop shadow | Shadows are close to invisible on a dark surface |
| Brand saturation | Reduce 10–15% versus the light-mode value | Full-saturation brand color on a dark ground reads harsher than intended |

## shadcn/ui Alignment

This configuration aligns with shadcn/ui conventions:

- Same CSS variable naming
- Same HSL format
- Same color scale structure
- Compatible with `npx shadcn@latest add` commands

### Using with shadcn/ui

```bash
# Initialize (uses same token structure)
npx shadcn@latest init

# Add components (styled with these tokens)
npx shadcn@latest add button card input
```

Components will automatically use your design system tokens.
