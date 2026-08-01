package model

// The document channel: which endpoints take a file part, and the wire shape
// the one verified backend accepts. Sibling of vision_test.go, split from it
// because a document is gated on the endpoint where a picture is gated on the
// model — different question, different table.

import "testing"

// A document is the picture's sibling part on this runtime, with one shape
// detail that is not guessable and cost a 400 to learn: file_data must be a
// data: URL. Bare base64 is rejected with "Invalid 'input[0].content[1]
// .file_data'" — a message that names the field and not the reason, which is
// exactly the kind of thing a test should hold rather than a person remember.
func TestResponsesMessagesCarryDocuments(t *testing.T) {
	_, input := convertMessagesToResponses([]Message{{
		Role:      RoleUser,
		Content:   "summarise this",
		Documents: []Document{{Name: "statement.pdf", MediaType: "application/pdf", Data: []byte{1, 2, 3}}},
	}})
	if len(input) != 1 || len(input[0].Content) != 2 {
		t.Fatalf("want one message of two parts, got %+v", input)
	}
	doc := input[0].Content[1]
	if doc.Type != "input_file" {
		t.Errorf("part type = %q, want input_file", doc.Type)
	}
	if doc.FileData != "data:application/pdf;base64,AQID" {
		t.Errorf("file_data = %q, want a data URL — bare base64 is a 400", doc.FileData)
	}
	if doc.Filename != "statement.pdf" {
		t.Errorf("filename = %q, want the document's own name", doc.Filename)
	}
}

// A document with no name still has to carry one: the backend rejects the part
// without it, and a default beats a 400.
func TestResponsesDocumentAlwaysHasAFilename(t *testing.T) {
	_, input := convertMessagesToResponses([]Message{{
		Role:      RoleUser,
		Documents: []Document{{MediaType: "application/pdf", Data: []byte{9}}},
	}})
	if len(input) != 1 || len(input[0].Content) != 1 {
		t.Fatalf("want one message of one part, got %+v", input)
	}
	if input[0].Content[0].Filename == "" {
		t.Error("filename is empty — the backend refuses the part")
	}
}

// The gate itself. Keyed on the provider first, because taking a file upload is
// a property of the endpoint rather than of the model — the same model behind a
// third-party gateway has no file part to arrive through.
func TestResolveDocuments(t *testing.T) {
	cases := []struct {
		provider string
		model    string
		want     bool
	}{
		{"codex", "gpt-5.5", true},
		{"chatgpt-codex", "gpt-5.1-codex", true}, // resolves through the alias
		// Verified endpoints only. These two document a file part and are the
		// obvious next candidates, but an unverified wire shape is a 400 on a
		// turn that works today — they stay on pdf_read until proven.
		{"anthropic", "claude-sonnet-5", false},
		{"openai", "gpt-4o", false},
		// A document-capable endpoint still needs a model that reads.
		{"codex", "codex-auto-review", false},
		{"ollama", "llava:13b", false},
		{"acme", "totally-new-model", false},
	}
	for _, tc := range cases {
		if got := ResolveDocuments(tc.provider, tc.model); got != tc.want {
			t.Errorf("ResolveDocuments(%q, %q) = %v, want %v", tc.provider, tc.model, got, tc.want)
		}
	}
	if !SupportsDocumentType("application/pdf") || SupportsDocumentType("text/csv") {
		t.Error("SupportsDocumentType should take PDF and nothing else yet")
	}
}
