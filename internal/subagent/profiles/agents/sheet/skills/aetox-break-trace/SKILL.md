---
name: aetox-break-trace
description: ไล่รายการที่กระทบยอดไม่ตรงกลับไปหาต้นทาง แล้วบอกเป็นประโยคเดียวว่าฝั่งไหนทำอะไรเพราะอะไร ใครต้องแก้ ใช้ต่อจาก aetox-gl-recon เมื่อรู้แล้วว่ารายการไหนไม่ตรง
source: https://github.com/anthropics/financial-services-plugins
license: Apache-2.0
copyright: Copyright Anthropic, PBC. Full terms in the source repository
---

# Root-cause one break

`aetox-gl-recon` produced a list of things that do not agree and a guess at why.
This is the other half: take one break and follow it back to the entry that
created it, on both sides, until you can say what actually happened.

One break at a time. A batch of forty traced at a glance is forty guesses with
the confidence of an investigation.

## The trace

1. **Pull the ledger side.** Find the entry behind that line: its document
   number, posting date, source, batch, and who prepared it.
2. **Pull the other side.** Find the transaction it should have matched: its
   own id, the transaction and settlement dates, the counterparty, the feed it
   came from, the rate it used.
3. **Line the attributes up side by side** — posting date, rate and rate date,
   account mapping, quantity, and the sign on both. The attribute that differs
   is almost always the cause, and seeing them in a column is what turns a
   suspicion into an answer.

You have no ledger to query. What you have is what the user can give you: the
journal export, the bank statement, the invoice, the delivery note, last
month's package. If a step of the trace needs a document you do not have, **name
the document** and stop there. Half a trace with the missing piece named is
useful; a completed trace with an inferred middle is not, and nobody downstream
can tell which one they are holding.

## Say it in one sentence

The form is **⟨ฝั่งไหน⟩ ⟨ทำอะไร⟩ เพราะ ⟨อะไร⟩**:

- "ฝั่งบัญชีลงวันที่ชำระ (T+2) ส่วนฝั่งรายละเอียดลงวันที่ทำรายการ เป็นผลต่างเชิงเวลา จะเคลียร์เองวันที่ 7 พ.ค."
- "ฝั่งรายละเอียดใช้อัตรา 4 โมงเย็น ฝั่งบัญชีใช้อัตราปิดตลาด ต่างกัน 12 bps บนยอดที่แปลงแล้ว"
- "สินค้า ABC123 ผูกกับบัญชี 11420 ในตารางผูกบัญชี แต่ฝั่งรายละเอียดส่งมาเป็น 11410 เป็นปัญหาการผูกบัญชี ต้องส่งให้ทีมข้อมูลหลัก"
- "ฝั่งรายละเอียดลงรายการซ้ำ (เลขที่ 88412 กับ 88419 เป็นรายการเดียวกัน) ต้องยกเลิกอันหลัง"

Each one names a side, an action and a reason, and every one of them is
actionable without reading anything else. "ผลต่างจากการลงบัญชี" names nothing and
is the sentence to avoid.

## What comes back per break

| ช่อง | เนื้อหา |
|---|---|
| รายการ | The key from the break report |
| สาเหตุที่แท้จริง | The one sentence above |
| ใครต้องแก้ | ฝ่ายปฏิบัติการ / ข้อมูลหลัก / บัญชี / ระบบต้นทาง |
| คาดว่าจะเคลียร์ | A date, or ว่างไว้ if it will not clear by itself |
| ต้องทำอะไร | เฝ้าดู / ปรับปรุง / เปิดเรื่อง / ยกเลิกรายการ |

**Diagnose, do not post.** You are not the one who books the correction, and a
break you quietly adjusted is a break nobody ever learns the cause of. Where the
fix is obvious, put it in ต้องทำอะไร and leave the doing to the person whose
name is on the ledger.

If the trace ends somewhere you did not expect — the two sides disagree about
something neither the recon nor the user thought was in scope — say that plainly
instead of forcing it into one of the buckets. A break that does not fit the
categories is usually the one worth the afternoon.
