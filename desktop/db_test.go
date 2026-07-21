package main

import "testing"

func TestDatabaseUsesOverrideDir(t *testing.T) {
	a := &App{dbDir: t.TempDir()}
	closeDBOnCleanup(t, a)
	db, err := a.database()
	if err != nil {
		t.Fatalf("database(): unexpected error: %v", err)
	}
	if db == nil {
		t.Fatal("database() returned nil *sql.DB with no error")
	}
	// Schema must have applied (sessions table exists) against the override dir.
	if _, err := db.Exec("SELECT 1 FROM sessions LIMIT 1"); err != nil {
		t.Errorf("sessions table not created in override dir: %v", err)
	}
}

func TestDatabaseSingleton(t *testing.T) {
	a := &App{dbDir: t.TempDir()}
	closeDBOnCleanup(t, a)
	db1, err := a.database()
	if err != nil {
		t.Fatalf("database(): unexpected error: %v", err)
	}
	db2, _ := a.database()
	if db1 != db2 {
		t.Error("database() returned a different *sql.DB on second call, want the same singleton (sync.Once)")
	}
}
