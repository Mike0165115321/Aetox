package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mikedev115/Aetox/internal/skill"
)

// The approval door, extended to skills. A skill-kind proposal (what the
// generator will queue in a later stage; here inserted directly) applies only
// when the human approves — and applying it rewrites the SKILL.md on disk,
// copying the bundled skill out whole so its references travel with it.
func TestApprovingASkillEditRewritesTheSkillOnDisk(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	a := newJobApp(t)

	db, err := a.database()
	if err != nil {
		t.Fatalf("database: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO pending_changes(kind, scope, target, op, before, body, reason, evidence, source, state, created_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		kindSkill, "aetox-ui-design", "", "add", "", "OPTIMIZER-ADDED-LINE",
		"flagged by 3 bad ratings", "jobs:1,2,3", "optimizer", statePending, "2026-08-26T00:00:00Z"); err != nil {
		t.Fatalf("seed skill proposal: %v", err)
	}
	var id int64
	if err := db.QueryRow(`SELECT id FROM pending_changes WHERE kind = ?`, kindSkill).Scan(&id); err != nil {
		t.Fatalf("find proposal: %v", err)
	}

	if err := a.ApprovePendingChange(id); err != nil {
		t.Fatalf("approve: %v", err)
	}

	dir := filepath.Join(skill.DefaultSkillsDir(), "aetox-ui-design")
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read edited skill: %v", err)
	}
	if !strings.Contains(string(data), "OPTIMIZER-ADDED-LINE") {
		t.Error("approving the skill edit did not rewrite the skill")
	}
	if _, err := os.Stat(filepath.Join(dir, "references", "responsive-design.md")); err != nil {
		t.Errorf("the skill was copied out without its references: %v", err)
	}
	if got := a.PendingChangeByID(id).State; got != stateApproved {
		t.Errorf("state = %q, want approved", got)
	}
}
