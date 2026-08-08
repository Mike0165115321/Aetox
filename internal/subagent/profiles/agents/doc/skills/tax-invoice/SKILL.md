---
name: tax-invoice
description: The fields a Thai full-form tax invoice (ใบกำกับภาษีแบบเต็มรูป) must legally carry, and how its VAT is shown. Use when the brief asks for ใบกำกับภาษี, a tax invoice, or an invoice that a Thai buyer will claim input VAT against.
---

# ใบกำกับภาษีแบบเต็มรูป

A tax invoice is not a document about a sale — it is the buyer's evidence for
claiming input VAT back. A missing required field does not make it look
unfinished; it makes the buyer's claim fail. That is the only failure mode of
this job worth designing around, and it is invisible on the page.

## Required — the document is invalid without these

Eight from ประมวลรัษฎากร มาตรา 86/4, and two more that ประกาศอธิบดีกรมสรรพากร
เกี่ยวกับภาษีมูลค่าเพิ่ม (ฉบับที่ 199) added for full-form invoices:

| # | Field | Note |
|---|---|---|
| 1 | The words **ใบกำกับภาษี** | Prominent. Not a subtitle, not "Invoice / ใบกำกับภาษี" in small type |
| 2 | Seller's ชื่อ, ที่อยู่, เลขประจำตัวผู้เสียภาษีอากร | 13 digits |
| 3 | Buyer's ชื่อ, ที่อยู่ | |
| 4 | เลขที่ (and เล่มที่, if the seller books them) | Sequential, and the seller's, never invented |
| 5 | ชื่อ ชนิด ประเภท ปริมาณ และมูลค่า of each line | |
| 6 | จำนวนภาษีมูลค่าเพิ่ม, **แยกออกจากมูลค่าอย่างชัดแจ้ง** | Its own line, never folded into a total |
| 7 | วัน เดือน ปี ที่ออกใบกำกับภาษี | |
| 8 | Anything else the Revenue Department has prescribed | Which is what the next two are |
| 9 | Buyer's เลขประจำตัวผู้เสียภาษีอากร | ฉบับที่ 199 — the field most often left off |
| 10 | **สำนักงานใหญ่** or **สาขาที่ NNNNN** for both parties | ฉบับที่ 199 — names the establishment that sold and bought |

Fields 9 and 10 are why a tax invoice that looks complete is often rejected. If
the brief does not supply them, ask for them or leave them blank and say so —
never fill them with something plausible.

## The arithmetic

Prices are per line: `ปริมาณ × ราคาต่อหน่วย`. The invoice shows
`มูลค่าสินค้า/บริการ`, then `ภาษีมูลค่าเพิ่ม 7%` on its own line, then
`จำนวนเงินรวมทั้งสิ้น`. Three numbers, in that order, never two.

If the brief gives VAT-inclusive prices, the base is `total × 100/107` and the
VAT is `total × 7/107`. Say in your reply which way you read the prices — the
same figures mean two different invoices depending on the answer, and the
person who finds out later is the accountant.

Withholding tax does not belong on a tax invoice. It is deducted at payment and
evidenced by a separate หนังสือรับรองการหักภาษี ณ ที่จ่าย. Putting it here
produces a document that reconciles against nothing.

## Never infer

The tax ID, the invoice number, the branch code, and any price. Each of these
is a fact the seller holds; a plausible one is worse than a blank, because a
blank is a question the user answers in a second and a wrong tax ID is a claim
that fails months later at the buyer's end.

## The skeleton, and what is not the skeleton

Required is the field list above and the three-line money block. Everything else
is yours: how the header reads, whether the lines carry a description column,
what the payment terms paragraph says, whether there is a note about delivery.

Write the document the brief actually asks for. A form that dictated its own
sentences would hand the same invoice to a freelancer billing one line and to a
supplier shipping forty, which is the reason this is a field list and not a
template.

## Building it

The parties go in a `plain` two-column table — seller left, buyer right, each
cell holding the name, address, tax ID and branch on their own lines. Plain
because this is a layout, not data, and a grid of boxes around a company address
reads as a mistake.

The document metadata — เลขที่, วันที่ — goes in a second `plain` table, or in
the right-hand cell above. Wherever it sits, the words **ใบกำกับภาษี** are a
heading and unmistakable, which is requirement 1 and the one thing on the page
that is not negotiable.

The goods go in a `lineitems` block with four column labels
(รายการ · จำนวน · ราคาต่อหน่วย · จำนวนเงิน), `align: ["","right","right","right"]`
and `widths: [4,1,2,2]`. Its totals are the three the law requires:

```
{"label": "มูลค่าสินค้า/บริการ",  "kind": "subtotal"}
{"label": "ภาษีมูลค่าเพิ่ม 7%",   "kind": "rate", "rate": 0.07}
{"label": "จำนวนเงินรวมทั้งสิ้น", "kind": "total"}
```

Every one of those figures is calculated for you. Do not compute any of them,
and do not send an `amount` — there is no field for one, deliberately.

A logo and a signature image are still not in the tool. Do not approximate them,
and do not line anything up by padding with spaces: a column spaced by hand
comes apart in the reader's own copy of Word.

---

Sources checked 2026-08-08: [มาตรา 86/4](https://www.rd.go.th/5208.html),
[ประกาศอธิบดีฯ (ฉบับที่ 199)](https://www.rd.go.th/27982.html). This file covers
the full-form tax invoice only — ใบกำกับภาษีอย่างย่อ (ม.86/6), ใบลดหนี้ (ม.86/10)
and ใบเพิ่มหนี้ (ม.86/9) each carry a different required list and are not
described here.
