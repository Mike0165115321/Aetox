package statereport

import (
	"errors"
	"fmt"
	"testing"
)

func TestTheMarkSurvivesWrapping(t *testing.T) {
	base := New("เซิร์ฟเวอร์ไม่ตอบ")
	if !Is(base) {
		t.Error("New did not mark")
	}
	// The normal journey: a skill layer adds context on the way up.
	if !Is(fmt.Errorf("n8n list: %w", base)) {
		t.Error("the mark was lost under one wrap")
	}
	if !Is(Mark(errors.New("key expired"))) {
		t.Error("Mark did not mark")
	}
	if !Is(Newf("ติดต่อ %s ไม่ได้", "n8n")) {
		t.Error("Newf did not mark")
	}
}

func TestOrdinaryErrorsStayUnmarked(t *testing.T) {
	if Is(errors.New("ต้องระบุ id ของ workflow")) {
		t.Error("an ordinary error read as a state report")
	}
	if Is(nil) {
		t.Error("nil read as a state report")
	}
	if Mark(nil) != nil {
		t.Error("Mark(nil) should stay nil")
	}
}

// %w inside Newf must keep the wrapped error reachable — callers use
// errors.Is/As on these like on any other error.
func TestNewfStillWraps(t *testing.T) {
	sentinel := errors.New("connection refused")
	err := Newf("ติดต่อ n8n ไม่ได้: %w", sentinel)
	if !errors.Is(err, sentinel) {
		t.Error("the underlying error is unreachable through the mark")
	}
}
