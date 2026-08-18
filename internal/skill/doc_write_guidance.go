package skill

// What the document writer's block used to say, moved (§132).
//
// The schema above carries existence and signature: the block kinds, the field
// names, and the units where a name alone would be ambiguous. What is here is
// the third layer — the things that are true about documents rather than about
// this JSON, and that a caller needs once rather than on every message.
//
// The test of whether a line belongs here rather than in the schema is whether
// getting it wrong produces a *bad document* or a *failed call*. A failed call
// teaches itself: parseBlocks names what was missing and the next attempt is
// right. A bad document does not — a priced table whose total was typed rather
// than computed opens perfectly, prints perfectly, and is found weeks later by
// somebody's accountant.

func (*docWriteSkill) Guidance(map[string]any) string {
	return "" +
		"One block per heading, paragraph, list, table or picture, in reading order.\n" +
		"lineitems is the block for anything priced — a quotation, an invoice, a receipt. " +
		"Send the lines and the rates; every amount, the subtotal and the total are worked " +
		"out here and never sent, which is the whole reason it exists. A rate is a fraction " +
		"of the subtotal: 0.07 is VAT at 7%, -0.03 is withholding tax deducted.\n" +
		"A picture is embedded rather than linked, so the .docx can be mailed on its own, " +
		"and it is drawn at its own size up to the width of the text column. Its `text` is " +
		"the caption, in Word's own Caption style — which is what lets a reader insert a " +
		"Table of Figures, and what a figure needs to keep its number when the document is " +
		"edited later.\n" +
		"`widths` are relative weights, so {4,1,1,1} gives the first column four times the " +
		"room of each of the others. Right-align a column of figures; a column of amounts " +
		"that reads down its left edge is the loudest sign a document was generated rather " +
		"than written."
}
