package subagent

import (
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/mode"
)

// Two things in this repository hand back a plan: the วางแผน stance, where the
// main agent writes to the person who will decide, and this profile, where a
// delegate writes to the main agent that will act. Their wording differs on
// purpose — this one is a coding helper and can say "the file and line you read
// it in", while a stance runs at every desk and cannot assume the work has files
// at all — but the headings are the shape itself, and a user handed two
// different shapes for the same request has learned neither.
//
// mode.PlanHeadings is the one place the shape is written down. This is the
// thread between it and the markdown, because a Go constant cannot reach inside
// a .md file: editing one side and not the other fails here rather than being
// noticed months later by somebody wondering why plans stopped matching.
func TestThePlanProfileKeepsTheSharedPlanShape(t *testing.T) {
	raw, err := bundledProfiles.ReadFile(bundledHelperDir + "/plan.md")
	if err != nil {
		t.Fatalf("the bundled plan profile must be readable: %v", err)
	}
	body := string(raw)
	for _, heading := range mode.PlanHeadings() {
		if !strings.Contains(body, heading) {
			t.Errorf("plan.md no longer asks for %q — it and the วางแผน stance must hand back one shape, "+
				"so change mode.planShape and both together or neither", heading)
		}
	}
}

// The other half of the same thread: the stance has to actually state the shape
// it claims to own. Direction() is prose assembled by hand, and prose is exactly
// where a heading gets reworded in passing.
func TestThePlanStanceStatesEveryHeading(t *testing.T) {
	direction := mode.StancePlan.Direction()
	for _, heading := range mode.PlanHeadings() {
		if !strings.Contains(direction, heading) {
			t.Errorf("วางแผน's direction never names %q, so nothing asks the model to produce it", heading)
		}
	}
}
