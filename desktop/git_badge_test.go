package main

import "testing"

func TestBadgeFromPorcelain(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},                     // clean: no badge, on purpose
		{" M desktop/app.go\n", "M"}, // unstaged modify
		{"M  desktop/app.go\n", "M"}, // staged modify
		{"?? new.go\n", "U"},         // untracked
		{"A  new.go\n", "A"},         // staged add
		{" D gone.go\n", "D"},        // deleted
		{"MM both.go\n", "M"},        // both columns: still one honest letter
	}
	for _, c := range cases {
		if got := badgeFromPorcelain(c.in); got != c.want {
			t.Errorf("badgeFromPorcelain(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
