---
name: aetox-variance-commentary
description: อธิบายว่าตัวเลขงวดนี้ต่างจากงวดก่อนและจากงบประมาณเพราะอะไร ทีละบรรทัดที่เกินเกณฑ์ ใช้ตอนทำรายงานปิดงวด สรุปให้ผู้บริหาร หรือหาว่าทำไมเดือนนี้ค่าใช้จ่ายพุ่ง
source: https://github.com/anthropics/financial-services-plugins
license: Apache-2.0
copyright: Copyright Anthropic, PBC. Full terms in the source repository
---

# Variance commentary

Three numbers per line — this period, last period, budget — and one sentence
saying why they differ. The three numbers are arithmetic. The sentence is the
entire value of the exercise, and it is the part that cannot be produced from
the three numbers alone.

## What gets commentary

Not every line. Flag one if **either** is true:

- The variance clears the materiality threshold. Use the user's own figure if
  they have one. If they do not, use 5% of the line or a fixed floor, whichever
  is larger, and **say which you used** — a threshold you chose silently makes
  your list of "significant" movements unreproducible.
- It is on the always-comment list: รายได้, ค่าใช้จ่ายพนักงาน, เงินสด. These get
  a sentence even when they barely moved, because a flat month for revenue is
  itself the finding.

A percentage on a small base is the trap here. A line that went from 800 to
2,400 is up 200% and is worth two words; a line that went from 4.2 ล้าน to 4.6
ล้าน is up 9.5% and may be the largest real movement on the page. Rank by the
amount, and let the percentage be a column rather than the sort order.

## The table

| บรรทัด | งวดนี้ | งวดก่อน | งบประมาณ | ต่างจากงวดก่อน | ต่างจากงบ | สาเหตุ |

Both variance columns carry the amount and the percentage. Both are formulas
against the three value cells, never numbers you worked out — the user will
correct one figure and expect the row to follow.

## What counts as a driver

A driver explains **why**, from the activity underneath the number. It does not
restate the number in words.

- ใช่: "ค่าคลาวด์เพิ่ม 1.2 ล้าน จากการจอง GPU เพิ่มสำหรับการเปิดตัวเดือน พ.ค."
- ไม่ใช่: "ค่าคลาวด์เพิ่มขึ้น 1.2 ล้านบาท (18%)"

The second sentence is the number with a font change. Anybody reading the table
already has it, and a commentary column full of them is the report equivalent of
saying nothing at length.

To find the real driver, go under the line: the breakdown by source, the vendor
mix, the change in headcount, volume times rate. That material is in whatever
the user gave you — the ledger export, the payroll file, last month's package.
Read it. A driver assembled from the account name is a guess wearing a
sentence.

## When you cannot find it

Write **"หาสาเหตุไม่ได้จากข้อมูลที่มี"**, name what you would need to answer it,
and say who would know — the person who owns that account is the one this line
is being escalated to, and a commentary that leaves them unnamed escalates to
nobody. Do not reason backwards from the account name to a plausible cause: a
sentence that sounds like a driver and was inferred from nothing is worse than a
blank, because the blank is the only thing that makes anyone go and look.

The same rule the numbers side of this job runs on applies to the prose: never
invent a figure, and never invent a reason either.

## Output

The table, plus 3 to 5 sentences on the period's biggest movers — what moved,
what caused it, and what a reader should do about it if anything. If any line
went uncommented because you could not source the driver, that belongs in those
sentences too, not only in a cell somebody has to scroll to.
