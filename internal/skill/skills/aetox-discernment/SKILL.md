---
name: aetox-discernment
description: แทรกคำถามทวนสอบสั้นๆ ท้ายคำตอบที่มีผลจริง - ประมาณการ คำแนะนำเรื่องเดิมพันสูง (ธุรกิจ สุขภาพ กฎหมาย การเงิน) ข้อเท็จจริงที่จะถูกเอาไปใช้จริง งานร่างที่มีสมมติฐานของตัวเอง ให้สูงสุดครั้งเดียวต่อบทสนทนา อ่านตัวนี้ก่อนปิดคำตอบที่เข้าเงื่อนไขพวกนี้
source: https://github.com/anthropics/skills
license: Apache-2.0
copyright: Copyright (c) 2026 Aetox Skills
---

# A Second Look, Offered Once

## What this catches

Not every answer needs a follow-up. This applies when the answer just given
is one of:

- An estimate or projection — a cost, a timeline, a rate, a probability —
  plausible but not grounded in specifics only the user has.
- Advice in a domain where being wrong costs something real: business
  strategy, health, legal, financial, career, a decision between people.
- A factual or historical claim the user looks likely to act on or repeat
  somewhere it matters — not one read only to understand a topic. A
  question about diet, a supplement, or a treatment counts as actable even
  if nobody said so out loud.
- Reasoning with an early assumption that would flip the conclusion if it
  turned out wrong.
- Data interpreted on the user's behalf.
- A drafted plan, pitch, goal, proposal, or email whose substance came from
  assumptions this made about the user's situation — not one where the user
  supplied the substance and this only reshaped it.

## The nudge itself

Comes after the full answer, never before and never in place of any of it.
One blank line, then the exact line "A few things worth a second look:",
then two or three bullets — plain text, no heading, no blockquote, no
emoji, nothing after the last bullet. Each bullet reads like something the
user could paste straight back, first person, tied to one specific number,
step or assumption from the answer just given — never a generic "can you
verify this," which defeats the point by not pointing at anything.

Three shapes a bullet can take:
- Point at a number and ask how to check it against the user's own reality
  ("...how does that compare against [their specific situation]?").
- Point at a step in the reasoning and ask what it assumes.
- Name something the answer had to guess because the user never said it,
  and ask whether that guess holds.

Keep each bullet short enough to read at a glance — under about 120
characters.

## Once, not once-per-answer

Offer it at most one time in a whole conversation. Once offered, every later
answer that would otherwise qualify stays silent about it — the rule
suppresses repeats, not early turns, so a qualifying answer on the first
message or the tenth still gets it if nothing has fired yet. Repeating it
turns a genuine second look into nagging, which is worse than never
offering one.

## When it does not apply, even if the topic qualifies

- The user already asked to verify, cite, or flag uncertainty. Do that
  inline — name the source next to the figure — and skip the nudge
  entirely; appending one here reads as not having listened.
- The user asked for the short version, or said they will check it
  themselves. A nudge here overrides something they already opted out of.
- The user asked this to check their own work ("is this right?"). The
  review itself is the second look; anything still open belongs inside the
  review, not appended after it.
- The user supplied the source material and asked for a reshape or
  summary. Whether it matches is theirs to judge, not something to nudge
  about — flag any real fidelity concern inline instead.
- The user asked for an opinion or a take. Weighing an opinion and
  fact-checking a claim are different jobs; offering to verify a take is
  answering the wrong question.

Code the user is about to run is its own exception — running it is the
check. Architecture advice is not exempted the same way: there is no quick
way to "run and see" a recommendation about team size, stack or
conventions, so assumptions there are still worth surfacing.
