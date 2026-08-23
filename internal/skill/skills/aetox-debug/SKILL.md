---
name: aetox-debug
description: วินัยแก้บั๊กที่ห้ามแก้จนกว่าจะรู้ต้นเหตุจริง - อ่าน error เต็ม ยืนยันซ้ำได้ ตั้งสมมติฐานเป็นประโยค ทดสอบทีละตัวแปร พลาดครบ 3 ครั้งแล้วให้หยุดถามสถาปัตยกรรมแทนแก้ต่อ อ่านตัวนี้ก่อนแก้บั๊ก/test ล้ม/พฤติกรรมที่ไม่คาดคิด ก่อนเสนอทางแก้ใดๆ
source: https://github.com/obra/superpowers
license: MIT
copyright: Copyright (c) 2026 Aetox Skills
---

# Debugging Discipline

## The rule everything else follows

No fix gets proposed until the root cause is found. Not "probably found" —
found, with evidence. A fix that makes the symptom go away without that is a
failure here, even if the symptom is gone. Technically working through the
steps below while still reaching for a fix on a hunch is the same violation
as skipping them outright — the steps are what they check for, not a ritual
to complete around a guess.

## Four phases, in order

**1. Find the root cause, not the symptom.** Read every error, warning and
stack trace in full — the exact line, the exact message, not a skim. Confirm
the bug reproduces and note the exact steps; if it won't reproduce, gather
more data rather than start guessing. Check what changed recently: diffs,
commits, dependencies, config. For anything that crosses a boundary (a build
step, an API call, a signing pipeline), instrument every boundary before
theorizing — log what goes in and out of each layer, run once, read the
evidence, and only then decide which layer is actually broken.

**2. Find the working pattern.** Locate a working example elsewhere in the
same codebase that resembles the broken thing, and read all of it — not a
skim of a reference that assumes a difference "can't matter." List what the
broken piece actually depends on (config, environment, assumptions) before
touching it.

**3. One hypothesis, one test.** Write the hypothesis as a sentence — "I
think X is the cause because Y" — vague theories don't count. Test it with
the smallest change that isolates one variable. If it worked, move to phase
4. If not, the first hypothesis is discarded and a new one is written; never
stack a second guess on top of an unconfirmed first one. Not understanding
something is said out loud — "I don't understand X" — rather than papered
over with fake confidence.

**4. Fix it once, verified.** Write a test that reproduces the bug before
touching the fix — automated where a framework exists, a throwaway script
otherwise. Make exactly one change that addresses the root cause found in
phase 1 — no drive-by refactor riding along. Then verify for real: the new
test passes, nothing else broke, and the original report is actually gone —
checked, not assumed.

## Three strikes

Count fix attempts. Under three, a failed attempt sends you back to phase 1
with whatever new information surfaced. At three, do not attempt a fourth —
stop instead, on any of these signs: each fix uncovers a new problem in a
different place than the last one; each fix would need a change out of
proportion to the bug to actually hold; each fix creates a new symptom
instead of closing the one you started with. That is not a hypothesis
problem anymore — it is the design underneath being wrong, and no amount of
further looping through phases 1-3 fixes a wrong design. Put the question to
the user before a fourth attempt: is the current approach actually sound, or
is it standing on inertia. Catching yourself think "just one more attempt"
after two failures already is itself the signal to stop.

The one legitimate exception — genuinely environmental, timing, or external,
with no fixable root cause in the code — only counts after the full process
above was actually run, not skipped to reach that conclusion faster. Most
"there's no root cause" conclusions are an investigation that stopped early,
not a real dead end.

## What "just try it" sounds like from the inside

Each of these, heard in your own reasoning, means stop and go back to phase
1 — not push through:

- "Quick fix now, investigate properly later"
- "Just try changing X and see"
- "A few changes at once, then run the tests"
- "Skip the test, I'll check it by hand"
- "It's probably X" (without having traced to X)
- "I don't fully understand this but it might work"
- "The reference does it differently, I'll adapt it" (without reading all
  of the reference first)
- Naming several possible fixes before tracing what the data actually does
- "One more attempt" — after two have already failed

And from the other side of the conversation, these usually mean the same
thing about you: "is that not happening?" (an assumption went unchecked),
"stop guessing", a flat "we're stuck?" — all of them mean the same return
to phase 1, not a harder push on the current guess.

## When it's small

A one-line typo with an obvious cause does not need four labeled phases
performed out loud — the discipline is for when the cause is not obvious
yet, which is the case this document exists for. Skipping it because a bug
looks simple is itself the first rationalization on the list above.
