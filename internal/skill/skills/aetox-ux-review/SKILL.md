---
name: aetox-ux-review
description: ตรวจและวิจารณ์ UX ของ UI ที่ทำไปแล้ว ด้วยกรอบมาตรฐาน, Nielsen 10 heuristics, Don Norman 7 หลัก, WCAG POUR, UX audit (IxDF 7 ปัจจัย), รีวิวงานภาพ และ cognitive walkthrough รายทาสก์ ใช้ตอนอยากประเมินของที่มีอยู่ ไม่ใช่ตอนสร้างใหม่ (สร้างใหม่ดู aetox-frontend-design)
source: https://github.com/mastepanoski/claude-skills
license: MIT
copyright: Copyright (c) 2026 mastepanoski. Full terms in LICENSE
---

# UX Review

Evaluating a UI that already exists, not designing one. Each method below is a
different lens; open the one the question calls for and read it whole before
judging. They are audits, so they end in findings, not opinions.

Pick the lens by what you are asking:

- **Usability, broadly**, `references/nielsen.md`. Jakob Nielsen's 10 heuristics:
  visibility of status, match to the real world, user control, consistency,
  error prevention, recognition over recall, flexibility, minimalist aesthetics,
  error recovery, help. The first reach for a general "is this usable" pass.
- **Why an interaction confuses**, `references/don-norman.md`. Don Norman's 7
  principles: discoverability, affordances, signifiers, feedback, mapping,
  constraints, conceptual model. Reach here when something is technically fine
  but people still do not understand it.
- **Accessibility**, `references/wcag-pour.md`. WCAG 2.1/2.2 across the four
  POUR principles at A/AA/AAA. The audit to run before calling anything done.
- **A whole experience, ending in a redesign**, `references/ux-audit.md`. IxDF's
  7 factors, 5 usability characteristics and 5 interaction dimensions, closing on
  redesign proposals rather than a bare list.
- **The visual layer**, `references/visual-review.md`. Typography, colour,
  spacing, hierarchy, consistency and branding against current category norms.
- **One task, step by step**, `references/cognitive-walkthrough.md`. Simulates a
  first-time user through a specific task to find where learnability breaks.

Run the narrowest lens that answers the question; reach for a second only when
the first surfaces something it cannot judge. Adapted from mastepanoski's
claude-skills (MIT), the six audits kept whole, one file each.
