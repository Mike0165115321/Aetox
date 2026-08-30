---
name: aetox-contract-standards
description: ร่างสัญญาภาษาอังกฤษจากแบบมาตรฐานที่ใช้ฟรีได้ NDA, SLA, ข้อตกลงเรื่อง AI, สัญญาพาร์ตเนอร์ช่วงพัฒนา ใช้ตอนต้องส่งสัญญาให้คู่ค้าต่างประเทศ หรือตรวจฉบับที่เขาส่งมา
source: https://commonpaper.com/standards/
license: CC BY 4.0
copyright: Common Paper. Each template carries its own attribution line at the foot of the file
---

# Standard agreements you can start from

Most commercial agreements are 90% the same terms argued over again by people
who did not write them. A standard agreement is that 90% written once by a
committee of lawyers, published free, and left alone — so the only thing anyone
negotiates is the part that is actually about this deal.

The ones here are Common Paper's, released under CC BY 4.0, which means they can
be used, edited and republished as long as the attribution stays. Every file in
`templates/` carries its attribution line at the foot. **Do not delete that
line**, and do not delete it from a document you assemble out of one.

## The Cover Page pattern, which is the real idea here

These agreements come in two pieces:

- **Standard Terms** — the long part. Nobody edits it. It is published at a URL
  with a version number, and the contract points at it.
- **Cover Page** — one page holding only what is specific to this deal: the
  parties, the purpose, the dates, the numbers. It says it incorporates the
  Standard Terms, and it is the only page anyone signs.

That split is worth understanding even when the user is not using these
templates, because it is the answer to "why does a two-page deal need forty
pages of contract". Open `templates/mutual-nda-coverpage.md` with `skill_view`
to see how short the negotiated part actually is — every file below opens the
same way, and none of them is loaded until you ask for it.

## What is on the shelf

| File | Use it for |
|---|---|
| `templates/mutual-nda.md` + `templates/mutual-nda-coverpage.md` | Both sides about to share confidential information. The most-used document here |
| `templates/design-partner-agreement.md` | An early customer who gets the product cheap or free in exchange for working with you on it, and whose feedback you want to be able to use |
| `templates/service-level-agreement.md` | Uptime, response time, and the credits owed when they are missed. Attaches to a service agreement |
| `templates/ai-addendum.md` | Anything where a model is involved: who owns the input, who owns the output, and whether either may be used for training. Short, and increasingly the clause a customer asks about first |

Four more exist and are too long to carry here. Fetch the text when one is
genuinely needed, do not reconstruct it from memory:

- Cloud Service Agreement — <https://commonpaper.com/standards/cloud-service-agreement/>
- Professional Services Agreement — <https://commonpaper.com/standards/professional-services-agreement/>
- Data Processing Agreement — <https://commonpaper.com/standards/data-processing-agreement/>
- Pilot Agreement — <https://commonpaper.com/standards/pilot-agreement/>

## Governing law is not a blank to fill in

Every one of these says the agreement is governed by the laws of a **US state**
and that disputes go to that state's courts. That is not a placeholder you swap
for "Thailand" and carry on — it changes which rules the rest of the document is
written against, and the standard terms were drafted for the ones they name.

For a Thai party, this is the clause to raise rather than to quietly fill.
Say it plainly in your reply: the template is sound as a starting point and the
governing-law and jurisdiction clauses are a decision for a lawyer, not for the
person filling in the cover page. The same goes for any term the user changes in
the Standard Terms themselves — the moment those are edited, it stops being the
standard everyone recognises and becomes a bespoke contract that reads like one.

## Filling one in

Work the cover page, not the standard terms. Every bracketed field is either
known or a question, and there is no third state:

- Known — put it in.
- A question — leave the bracket and **list it in your reply**. Effective date,
  the exact purpose, the term of confidentiality, the notice addresses. A
  plausible value invented for a bracket is the failure mode of this whole job,
  because a signed contract is the one document nobody re-reads before it
  matters.

The legal entity names and addresses are the fields most often filled with
something close enough. They must be the registered names, exactly, on both
sides.

When you hand it back, say which template and version you used, what you filled,
what you left open, and which clauses the user should have someone look at
before signing. You are drafting a document from a standard, not advising on it,
and being clear about that line is part of doing the job well.

## Reading an inbound contract

When the user has been sent someone else's paper, these templates are still
useful: they are what a fair version of that agreement looks like. Read theirs
against the standard and report the differences that matter rather than every
difference — one-sided confidentiality where the standard is mutual, an
indefinite term where the standard has one, obligations that have nothing to do
with the document's title (a non-solicit inside an NDA, a licence grant inside a
mutual confidentiality agreement).

Say what each difference costs the user in practice. "ข้อ 7 ให้เขาใช้ข้อมูลของเรา
ไปเทรนโมเดลได้" is a finding; "clause 7 differs from the standard" is a diff.
