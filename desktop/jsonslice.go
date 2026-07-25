package main

// jsonSlice is the fix for a whole class of frontend crashes.
//
// A nil Go slice marshals to JSON `null`, not `[]`. The frontend then assigns
// that null into `$state<Row[]>` and the first `rows.length` throws — which in
// Svelte 5 aborts the update mid-flush. The nav button that was already
// updated stays highlighted while the panel never re-renders, so the page
// looks unresponsive rather than broken: "ทำไมกดไม่ได้".
//
// Found 2026-07-25 in Settings → Skills on a machine with no ~/.aetox/skills
// (ARCHITECTURE.md §34). Idiomatic Go returns nil for "nothing"; every Go
// caller is fine with it. The boundary is what cannot be, so the conversion
// belongs here, at the last Go frame before the frontend.
//
// Rule: a binding that returns a slice to the frontend returns it through
// this. binding_slices_test.go enforces it for the ones a test can call.
func jsonSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
