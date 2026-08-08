package ooxml

// The arithmetic under a priced document, worked out here rather than sent in.
//
// This exists because of one failure that no reader of the finished file can
// see: a total that was worked out in a model's head. It arrives in the same
// confident shape as a correct one, on a document whose whole purpose is that
// the figure is right, and the person who finds out is an accountant three
// weeks later.
//
// Two rules govern everything below, and both are about the page rather than
// the maths:
//
//   - Every figure is rounded to two decimals before it is used again, so the
//     lines printed on the document add up to the total printed on the
//     document. A total computed from unrounded parts is off by a satang
//     often enough to be noticed, and "the computer is right, the page is
//     wrong" is not a defence anyone accepts.
//   - Rounding is half away from zero, which is what every invoice, till and
//     accounting system in this part of the world does. Go's default is
//     half-to-even, which would round 2.625 down and disagree with the vendor's
//     own system on a figure the two are supposed to reconcile.

import (
	"math"
	"strconv"
	"strings"
)

// round2 rounds to two decimals, half away from zero.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// computedTotal is one summary row after the arithmetic: what to print, and
// what it came to.
type computedTotal struct {
	Label  string
	Amount float64
}

// computeLines returns each line's amount and the summary rows under them.
//
// The order of the summary rows is the caller's, and it matters: a `total` row
// adds up the rate rows *above* it, so a document can show subtotal → VAT →
// grand total, or subtotal → VAT → withholding → net payable, without this
// function knowing what any of those words mean.
func computeLines(lines []LineItem, totals []TotalRow) ([]float64, []computedTotal) {
	amounts := make([]float64, len(lines))
	subtotal := 0.0
	for i, line := range lines {
		qty := line.Qty
		if qty == 0 {
			// A line with no quantity is one of the thing, which is how a flat
			// fee is written. Zero would silently price it at nothing.
			qty = 1
		}
		amounts[i] = round2(qty * line.Price)
		subtotal += amounts[i]
	}
	subtotal = round2(subtotal)

	running := subtotal
	out := make([]computedTotal, 0, len(totals))
	for _, row := range totals {
		var amount float64
		switch row.Kind {
		case TotalRate:
			// Of the subtotal, never of the running figure. Thai withholding
			// tax and VAT are both assessed on the value of the goods, so
			// charging the second against a figure that already contains the
			// first is the classic way to be 0.21% wrong.
			amount = round2(subtotal * row.Rate)
			running = round2(running + amount)
		case TotalTotal:
			amount = running
		default: // subtotal
			amount = subtotal
			running = subtotal
		}
		out = append(out, computedTotal{Label: row.Label, Amount: amount})
	}
	return amounts, out
}

// formatMoney writes a figure the way a document reads it: thousands separated,
// two decimals, always both — "1,234.50" and "0.00", never "1234.5".
func formatMoney(v float64) string {
	s := strconv.FormatFloat(round2(v), 'f', 2, 64)
	negative := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	whole, fraction, _ := strings.Cut(s, ".")

	var b strings.Builder
	if negative {
		b.WriteString("-")
	}
	for i, digit := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteString(",")
		}
		b.WriteRune(digit)
	}
	b.WriteString(".")
	b.WriteString(fraction)
	return b.String()
}

// formatQuantity keeps a count looking like a count. Quantities are whole far
// more often than not, and "2.00 ชิ้น" reads as a measurement rather than a
// number of things.
func formatQuantity(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}
