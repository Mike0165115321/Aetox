package memory

import (
	"reflect"
	"testing"

	"github.com/Mikedev115/Aetox/internal/model"
)

// AddMessage used to rebuild the message field by field, which silently dropped
// anything the list did not name. Its own comment warned about that, and
// Documents — added later — fell into it anyway: every attached PDF was
// discarded one call after cognitive.addUserTurn put it there, so no provider
// ever received one and the model answered from the filename.
func TestAddMessageKeepsAttachments(t *testing.T) {
	c := NewContext("system", 0, 100000)
	c.AddMessage(model.Message{
		Role:      model.RoleUser,
		Content:   "  อ่าน PDF นี้ให้หน่อย  ",
		Images:    []model.Image{{MediaType: "image/png", Data: []byte("png")}},
		Documents: []model.Document{{Name: "invoice.pdf", MediaType: "application/pdf", Data: []byte("%PDF-1.7")}},
	})

	got := c.Messages()[1]
	if len(got.Documents) != 1 || got.Documents[0].Name != "invoice.pdf" {
		t.Fatalf("documents = %+v; want the attached PDF to reach the provider", got.Documents)
	}
	if len(got.Images) != 1 {
		t.Errorf("images = %+v; want the attached image kept too", got.Images)
	}
	// Normalisation still happens — that is the only reason this function
	// rewrites anything.
	if got.Content != "อ่าน PDF นี้ให้หน่อย" {
		t.Errorf("content = %q; want it trimmed", got.Content)
	}
}

// The real guard: whatever fields model.Message grows, AddMessage must carry
// them. A field-by-field rebuild passes the test above the day it is written
// and fails silently the next time somebody adds a field — which is exactly
// what happened. This fails immediately instead.
func TestAddMessageCarriesEveryField(t *testing.T) {
	full := model.Message{
		Role:             model.RoleAssistant,
		Name:             "read",
		ToolCallID:       "call_1",
		Content:          "text",
		ReasoningContent: "thinking",
		ToolCalls:        []model.ToolCall{{ID: "call_1", Type: "function"}},
		Images:           []model.Image{{MediaType: "image/png", Data: []byte("png")}},
		Documents:        []model.Document{{Name: "a.pdf", MediaType: "application/pdf", Data: []byte("pdf")}},
	}

	c := NewContext("system", 0, 100000)
	c.AddMessage(full)
	got := c.Messages()[1]

	v := reflect.ValueOf(full)
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		want := v.Field(i).Interface()
		have := reflect.ValueOf(got).Field(i).Interface()
		if !reflect.DeepEqual(want, have) {
			t.Errorf("model.Message.%s did not survive AddMessage: sent %v, stored %v", name, want, have)
		}
	}
}
