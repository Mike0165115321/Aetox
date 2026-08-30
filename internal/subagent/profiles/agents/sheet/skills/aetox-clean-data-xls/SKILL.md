---
name: aetox-clean-data-xls
description: ล้างข้อมูลที่เละก่อนเอาไปคำนวณ รวมยอด หรือ pivot ช่องว่างหัวท้าย ตัวพิมพ์ไม่ตรงกัน ตัวเลขที่ถูกเก็บเป็นข้อความ วันที่คนละฟอร์แมตในคอลัมน์เดียว แถวซ้ำ และตัวอักษรเพี้ยน
source: https://github.com/anthropics/financial-services-plugins
license: Apache-2.0
copyright: Copyright Anthropic, PBC. Full terms in the source repository
---

# Clean up messy data

Data arrives from an export, a form, a system somebody else runs, or fifty rows
somebody typed by hand. Your job is to get it to where every column has one
type, every value is what it looks like, and nothing has to be retyped before it
can be summed.

The failure to avoid is a cleaned file that is *tidier* than the original and no
more trustworthy: rows quietly dropped, a value silently changed, a date read as
month-day where the source meant day-month. The user is not going to diff your
output against their input, so anything you change without saying is a change
nobody will ever find.

## Step 1: scope

Take the range the user named, or the full extent of the sheet. Then profile
each column before touching it: what is the dominant type, and which cells
disagree with it. That profile is the whole basis for everything below, and it
is also the most useful thing you can say back if the user only wanted an
opinion.

## Step 2: what to look for

| Issue | What it looks like |
|---|---|
| Whitespace | Leading and trailing spaces, double spaces inside a value |
| Casing | `usa` / `USA` / `Usa` in one categorical column |
| Number as text | Numerics stored as text; stray `฿` `$` `,` `%` living inside a number cell |
| Dates | `3/8/26`, `2026-03-08`, `8 มี.ค. 69` in the same column |
| Duplicates | Exact duplicate rows, and near-duplicates that differ only by case or spacing |
| Blanks | Empty cells in a column that is otherwise fully populated |
| Mixed types | A column that is 98% numbers with three text entries in it |
| Encoding | Mojibake (`Ã©`, `â€™`), non-printing characters |
| Errors | `#REF!` `#N/A` `#VALUE!` `#DIV/0!` carried in from a formula that broke |

Two of those are worse than they look in a Thai file. `3/8/26` is ambiguous
between 3 August and 8 March and the source usually does not say which — do not
guess, ask, or keep both readings visible. And a year may arrive as พ.ศ.: 2569
is 2026, and a column where 2569 and 2026 both appear has two calendars in it,
not one bad row.

## Step 3: propose before you change

Show this table and stop:

| คอลัมน์ | ปัญหา | จำนวนแถว | จะแก้เป็น |

Anything destructive — removing duplicate rows, filling blanks, collapsing two
columns into one — is asked about here, not reported afterwards. A duplicate row
is sometimes two real transactions of the same amount on the same day.

## Step 4: clean it

You do not edit a workbook in place: `sheet_write` produces a **new file**, and
the original stays exactly as the user left it. That is the safety net for this
whole job, and it is worth saying in your reply so the user knows their source is
untouched.

**Prefer a formula over a value you computed.** Where the repair can be written
as one, put it in an adjacent helper column — `=TRIM(A2)`,
`=VALUE(SUBSTITUTE(B2,"฿",""))`, `=UPPER(C2)`, `=DATEVALUE(D2)` — rather than
working the answer out yourself and writing the result. The user can then see
what was done to every row, and undo it by deleting a column. A cleaned value
with no formula behind it is a number they have to take on faith.

Write computed values in place of formulas only when the user asks for it, or
when no formula expresses the fix — mojibake repair is the honest example.

The repair that matters most is not a formula at all: send the value with its
real JSON type. A cleaned amount goes out as `1234.5`, a cleaned date as
`"2026-03-08"`. Handing back `"1,234.50"` with the spaces trimmed is a column
that still sums to zero.

Work in the order the categories are listed above — whitespace, casing, numbers,
dates, duplicates — and show a sample after each one. A user who sees the casing
pass go wrong stops you before the dedupe pass eats the evidence.

## Step 5: hand it back

Say what changed, per column, with counts. Say what you did **not** change and
why: the ambiguous dates you could not resolve, the three text entries in a
number column that turned out to be `"n/a"`, the near-duplicates you left alone
because they might be real.

Never fill a blank with something plausible. An empty cell the user can see is a
question they can answer in a second; a filled one is a fact they will never
check. If the data is too broken to clean without deciding something only they
can decide, say which decision it is and stop there.

If what they actually needed was to know whether an existing workbook computes
correctly rather than whether its data is tidy, that is `aetox-audit-xls`.
