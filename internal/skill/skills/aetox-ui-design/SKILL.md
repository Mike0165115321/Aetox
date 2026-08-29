---
name: aetox-ui-design
description: ลงมือ implement UI ระดับ production, design tokens/theming, responsive (container queries, fluid type, CSS grid), interaction/microinteraction/motion, คอมโพเนนต์เว็บ (React/Vue/Svelte), การเข้าถึง WCAG 2.2 และ native (iOS HIG/SwiftUI, Material 3/Compose, React Native) รวมเป็นประตูเดียว ใช้ตอนลงมือเขียนจริง ไม่ใช่แค่ตัดสินทิศทาง (ทิศทางดู aetox-frontend-design)
source: https://github.com/wshobson/agents (plugins/ui-design)
license: MIT
copyright: Copyright (c) 2024 Seth Hobson. Full terms in LICENSE
---

# UI Design (implementation)

Nine implementation guides, one per problem, the how-to-build layer.
`aetox-frontend-design` decides the look and `aetox-design-system` gives the
tokens; these turn that into code. Open the one the task is actually about;
each file carries its source skill's own reference material inline.

**For the skeleton of a page, do not start from memory.** `aetox-web-templates`
ships twenty-one sections as real markup, nav and hero through pricing and FAQ
to a dashboard shell, a data table, a form, a 404, an article and a docs page.
Every guide here describes how a thing should behave; that skill is the thing
itself, ready to paste, and starting from it leaves this reading for the part
that is actually particular to the job.

**On the web**

- `references/design-system-patterns.md`, design tokens, theming and
  theme-switching, component architecture. Read first when starting a system.
- `references/visual-design-foundations.md`, typography, colour theory, spacing
  systems and iconography as concrete tokens.
- `references/responsive-design.md`, container queries, fluid typography, CSS
  Grid, mobile-first breakpoints, component-level responsiveness.
- `references/interaction-design.md`, microinteractions, motion, transitions,
  loading states and feedback patterns.
- `references/web-component-design.md`, React/Vue/Svelte component patterns,
  CSS-in-JS, composition, reusable component APIs.
- `references/accessibility-compliance.md`, WCAG 2.2 in code: ARIA patterns,
  screen-reader support, mobile accessibility, inclusive patterns.

**Native**

- `references/mobile-ios-design.md`, iOS Human Interface Guidelines and SwiftUI.
- `references/mobile-android-design.md`, Material Design 3 and Jetpack Compose.
- `references/react-native-design.md`, React Native styling, navigation and
  Reanimated animations, cross-platform.

Adapted from wshobson/agents' ui-design plugin (MIT), nine skills folded into
one door, each kept whole with its references appended.
