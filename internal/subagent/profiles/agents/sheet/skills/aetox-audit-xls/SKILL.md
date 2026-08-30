---
name: aetox-audit-xls
description: ตรวจเวิร์กบุ๊กที่คนอื่นทำมา ก่อนเชื่อตัวเลขในนั้นหรือส่งต่อให้ใครใช้ หา error, ค่าคงที่ฝังในสูตร, สูตรที่หลุดแพตเทิร์น, ช่วง SUM ที่ขาดแถว, สูตรที่ถูกวางทับด้วยค่า และงบที่ไม่บาลานซ์
source: https://github.com/anthropics/financial-services-plugins
license: Apache-2.0
copyright: Copyright Anthropic, PBC. Full terms in the source repository
---

# Audit a workbook

Somebody built this file and somebody else is about to act on it. Your job is to
find the places where what it displays and what it computes have come apart.

This job has exactly one failure mode, and it is not missing a bug. It is
reporting **"ตรวจแล้วไม่พบปัญหา"** on a workbook whose formulas you never saw.
That answer is indistinguishable from a real all-clear, it is the answer the
person wanted to hear, and it is the reason they stop checking. Read the section
below before you form any opinion about a file.

## You do not see formulas by reading the file

`read` on an `.xlsx` gives you the **cached values Excel last wrote**, not the
expressions behind them. That is deliberate and right for every other job: a
workbook from Excel carries the computed result in every cell, and showing
`SUM(C2:C6)` where the user expects `18,420` would be worse than useless.

For this job it is fatal. A column that reads `100 / 200 / 300 / 600` tells you
nothing about whether the 600 is `=SUM(C2:C4)`, a hardcoded 600 somebody pasted
over the formula, or `=SUM(C2:C3)+100`. All three look identical. The check that
matters most here is the one `read` cannot make.

An `.xlsx` is a zip. Unpack it and the formulas are plain XML:

```powershell
Copy-Item "D:\work\model.xlsx" "$env:TEMP\model.zip"
Expand-Archive "$env:TEMP\model.zip" -DestinationPath "$env:TEMP\model_x" -Force
```

Then read and grep what came out, with the tools you already hold:

| File | What it answers |
|---|---|
| `xl/workbook.xml` | The tab names, in order, and which are `state="hidden"` |
| `xl/_rels/workbook.xml.rels` | Which `sheetN.xml` is which tab. `sheet1.xml` is **not** reliably the first tab |
| `xl/worksheets/sheetN.xml` | Every cell: `<f>` is the expression, `<v>` is the cached result |
| `xl/sharedStrings.xml` | The text the cells point at by index |

Four things in that XML are worth knowing before you read it, because each one
produces a wrong conclusion if you do not:

- A shared formula is written **once**, as `<f t="shared" si="3" ref="D2:D40"/>`,
  and the rest of the column carries only the `si` back-reference. A column of
  live formulas therefore looks, at a glance, like one formula and 38 empty
  cells. It is not.
- A cell with `<v>` and no `<f>`, sitting in a run of cells that all have `<f>`,
  is a **pasted-over formula**. This is the single highest-value finding in the
  whole audit, and the XML is where it becomes mechanical instead of a hunch.
- `calcMode="manual"` in `xl/workbook.xml` means the cached values may be
  **stale** — the numbers on screen are not what the formulas currently say.
  Everything anyone has read off that file is suspect until it is recalculated.
- Iterative calculation switched on in the same element means circular
  references are there **on purpose**. In a model with a debt-interest loop that
  is correct; anywhere else it is somebody silencing an error rather than
  fixing it.

Two limits of `read` to keep in mind when you use it for the data: it stops at
500 rows, 64 columns and 20 sheets. A conclusion drawn from row 500 of a
2,000-row sheet is a conclusion about the part you were shown.

## Step 1: agree the scope

Use the scope the user gave. If they gave none, ask which of the three:

- **ช่วงที่เลือก** — one range they name
- **ทั้งชีต** — one sheet, end to end
- **ทั้งเวิร์กบุ๊ก** — every sheet, plus the model-integrity checks in step 3

Whole-workbook is the deep one. It is what a file is worth before it goes to a
client, a bank, a board, or anyone who will make a decision on it.

## Step 2: formula-level checks — every scope

| Check | What you are looking for |
|---|---|
| Error cells | `#REF!` `#VALUE!` `#N/A` `#DIV/0!` `#NAME?` |
| Hardcodes inside formulas | `=A1*1.05` — the 1.05 belongs in a cell somebody can find and change |
| Inconsistent formulas | One cell in a row or column that breaks its neighbours' pattern |
| Off-by-one ranges | `SUM`/`AVERAGE` that misses the first or last row of what it means to cover |
| Pasted-over formulas | A number sitting where the pattern says a formula should be |
| Circular references | And whether this workbook meant them |
| Broken cross-sheet links | References to a sheet or cell that has since moved or gone |
| Unit and scale mismatches | Thousands mixed with millions; a percent column holding 7 instead of 0.07 |
| Hidden rows and tabs | Where overrides and stale working live |

The percent one is worth its own sentence: a column of 7s where the formulas
expect 0.07 is wrong by a factor of a hundred, and it displays perfectly.

## Step 3: model integrity — whole-workbook scope only

Name the model type first (DCF, LBO, 3-statement, merger, comps, or something
the author invented), then run what applies.

**Structure.** Are inputs visually separated from calculations, is the colour
convention applied consistently, do the tabs run in a readable order, are the
date headers and the units the same on every tab?

**Balance sheet.** Assets = Liabilities + Equity **in every period**. Retained
earnings roll forward: prior RE + net income − dividends = current RE. On a deal
model, goodwill and intangibles flow from the acquisition assumptions rather
than sitting there as typed figures. If the balance sheet does not balance,
quantify the gap per period and trace where it starts, then stop auditing
anything downstream — nothing below it can be trusted until that is closed.

**Cash flow.** Ending cash on the CF equals cash on the BS, every period.
CFO + CFI + CFF = the change in cash. D&A matches the income statement. CapEx
matches the PP&E rollforward. Working-capital movements carry the right sign.

**Income statement.** The revenue build ties to the segment detail underneath
it. Tax expense equals pre-tax income times the rate, allowing for deferred tax.
Share count ties to the dilution schedule — options, converts, buybacks.

**Circular references.** Interest to debt balance to cash and back to interest
is a normal, deliberate circularity in an LBO or a 3-statement model. If it is
meant, check the iteration setting is actually on and actually working. If it is
not meant, trace the loop and say where to break it — a circular reference the
author has been ignoring is a number that changes depending on when it was last
calculated.

**Reasonableness.** Revenue growth over 100% with nothing explaining it, margins
outside anything the industry does, a terminal value that is more than about
three quarters of enterprise value, projections that hockey-stick in the out
years, EBITDA compounding to an absurd figure by year 10. Then push the edges:
does the model survive 0% growth, negative growth, negative EBITDA?

**Bugs that are specific to the type.** DCF: the discount rate applied to the
wrong period, terminal value never discounted back, WACC on book rather than
market values, interest expense left inside an unlevered FCF, a tax shield
counted twice. LBO: debt paydown that does not match the cash sweep, PIK
interest not accruing to principal, management rollover missing from the returns,
an exit multiple on the wrong EBITDA (LTM where it should be NTM), fees and
expenses missing from day-one equity. Merger: accretion on the pre-deal share
count, synergies not phased in, purchase price allocation that does not balance,
foregone interest on the cash used left out, transaction fees missing from
sources and uses. 3-statement: working-capital signs reversed, depreciation that
does not match the PP&E schedule, a debt maturity schedule that does not match
the principal payments, dividends exceeding net income with nothing explaining
it.

## Step 4: report

One findings table, worst first:

| # | ชีต | เซลล์/ช่วง | ความรุนแรง | ประเภท | ปัญหา | ควรแก้เป็น |

- **Critical** — the output is wrong: the balance sheet does not balance, cash
  does not tie, a formula is broken
- **Warning** — it is right today and fragile: hardcodes, inconsistent formulas,
  edge cases that break
- **Info** — convention and layout

For a whole-workbook audit, open with one line: model type, overall verdict
(สะอาด / มีจุดต้องดู / มีปัญหาใหญ่), and the count at each severity.

**Fix nothing without being asked.** You are reporting on somebody else's file
and they may have reasons you cannot see from the cells. Report, then fix on
request.

## Three things experience says before you start

**The balance sheet comes first.** If it does not balance, everything downstream
is suspect and auditing it is time spent on numbers that are going to move.

**Hardcoded overrides are the number one source of silent bugs.** Search for
them harder than the list above suggests: a figure somebody typed over a formula
during a late night is the defect that survives every review, because it looks
like an answer.

**Sign-convention errors are extremely common.** Cash out as a positive, a
deduction added, a working-capital movement carrying the sign of the balance
rather than the change. They are easy to miss precisely because the number is
the right size.

## What is worth saying at the end

If the file uses VBA macros, say so: calculation that happens in a macro cannot
be audited from the formulas, so your all-clear covers less than it appears to.

And if you could not get at the formulas at all — the file would not unpack, or
it is `.xls` rather than `.xlsx` — say **that**, in place of a verdict. An audit
of the displayed numbers is a different and much smaller claim than an audit of
the workbook, and the person reading your answer cannot tell the two apart
unless you tell them.
