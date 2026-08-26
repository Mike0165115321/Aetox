---
name: aetox-brand
description: อัตลักษณ์แบรนด์ฝั่งถ้อยคำและการกำกับ - กรอบเสียงและน้ำเสียง โครงข้อความที่ใช้ซ้ำได้ กฎการใช้โลโก้ สเปกตัวอักษร การจัดระเบียบและตั้งชื่อไฟล์งาน กับเช็กลิสต์ก่อนปล่อยงานออก ใช้ตอนเขียนงานในนามแบรนด์ ตรวจว่างานที่ทำมาตรงแนวทางไหม หรือร่างคู่มือแบรนด์ให้ลูกค้า
source: https://github.com/claudekit (brand)
license: MIT
copyright: Copyright (c) claudekit contributors
---

# Brand

Brand identity, voice, messaging, asset management, and consistency frameworks.

## When to Use

- Brand voice definition and content tone guidance
- Visual identity standards and style guide development
- Messaging framework creation
- Brand consistency review and audit
- Asset organization, naming, and approval
- Color palette management and typography specs

## Quick Start

There is nothing to run. The brand lives in one file in the user's project
usually `docs/brand-guidelines.md`, and the work is reading it before writing
anything in the brand's name, and editing it when the brand changes.

**Before writing anything branded:** read the guidelines file whole. Voice is
not a rule you can apply from a summary of itself.

**Checking a piece of work:** `references/consistency-checklist.md` and
`references/approval-checklist.md` are the two passes, done by reading. A file
name or an image size is `glob` and a look; a tone is a judgement, and the
checklist is what keeps it from being only taste.

**Colours:** the palette is whatever the guidelines file says it is. Reading a
colour *out of an image* is not something this app can do, say so rather than
guessing at a hex from a description of a picture.

## Brand Sync Workflow

The guidelines file is the source of truth and the tokens follow it, by hand and
in that order:

1. Edit the guidelines file, the brand changed there first.
2. Carry the change into the project's design tokens. `aetox-design-system` is
   the skill that says how the three token layers fit together.
3. Read back what you wrote. A token file that disagrees with the guidelines is
   the failure this order exists to prevent, and it is silent.

## Subcommands

| Subcommand | Description | Reference |
|------------|-------------|-----------|
| `update` | Update brand identity and sync to all design systems | `references/update.md` |

## References

| Topic | File |
|-------|------|
| Voice Framework | `references/voice-framework.md` |
| Visual Identity | `references/visual-identity.md` |
| Messaging | `references/messaging-framework.md` |
| Consistency | `references/consistency-checklist.md` |
| Guidelines Template | `references/brand-guideline-template.md` |
| Asset Organization | `references/asset-organization.md` |
| Color Management | `references/color-palette-management.md` |
| Typography | `references/typography-specifications.md` |
| Logo Usage | `references/logo-usage-rules.md` |
| Approval Checklist | `references/approval-checklist.md` |

## Templates

| Template | Purpose |
|----------|---------|
| `templates/brand-guidelines-starter.md` | Complete starter template for new brands |

## Routing

1. Parse subcommand from `$ARGUMENTS` (first word)
2. Load corresponding `references/{subcommand}.md`
3. Execute with remaining arguments
