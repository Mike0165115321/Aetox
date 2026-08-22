package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResolveVision(t *testing.T) {
	cases := []struct {
		provider string
		model    string
		want     bool
	}{
		{"openai", "gpt-4o", true},
		{"anthropic", "claude-sonnet-5", true},
		{"gemini", "gemini-2.5-pro", true},
		{"ollama", "llava:13b", true},
		{"ollama", "llama3.2-vision:11b", true},
		{"openrouter", "qwen/qwen2.5-vl-72b-instruct:free", true},
		// codex is a ChatGPT plan, so the models it serves are OpenAI's and the
		// gpt-5 marker already covers them. Pinned because the provider key is
		// new to this table and the Responses runtime is the only one whose
		// image part nothing else in this file exercises.
		{"codex", "gpt-5.5", true},
		{"codex", "gpt-5.1-codex", true},
		// A family name is not evidence either way. These two carry no vision
		// marker of their own and fall through to the blind default, which is
		// the same answer the old "deepseek" family marker gave and the reason
		// that marker was never doing any work.
		{"deepseek", "deepseek-v4", false},
		{"deepseek", "deepseek-v4-flash", false},
		// The regression. The family marker used to win, so the model the owner
		// had actually selected in the composer was called blind and his
		// screenshot went to image_ocr (22 ส.ค.). The member's own name is the
		// more specific evidence and has to beat the family it belongs to.
		{"deepseek", "deepseek-v4-flash-vision-exp", true},
		{"deepseek", "deepseek-vl", true},
		// Role markers still win over an explicit vision marker, which is the
		// half of the old rule worth keeping: this one really does take images
		// and is still not something to hold a turn with.
		{"ollama", "nomic-embed-vision", false},
		{"ollama", "nomic-embed-text", false},
		{"openai", "whisper-1", false},
		// Unknown is blind on purpose: OCR still works, a silently dropped
		// image does not.
		{"acme", "totally-new-model", false},
		{"openai", "", false},
	}
	for _, tc := range cases {
		if got := ResolveVision(tc.provider, tc.model); got != tc.want {
			t.Errorf("ResolveVision(%q, %q) = %v, want %v", tc.provider, tc.model, got, tc.want)
		}
	}
}

// The guarantee the whole design rests on: a message with no image must
// serialize to the bytes it did before images existed, or every provider's
// prompt cache misses on every turn.
func TestOpenAIMessagesUnchangedWithoutImages(t *testing.T) {
	msgs := []Message{
		{Role: RoleSystem, Content: "you are a tool"},
		{Role: RoleUser, Content: "hello"},
		{Role: RoleAssistant, Content: "", ToolCalls: []ToolCall{{
			ID: "call_1", Type: "function",
			Function: FunctionCall{Name: "read", Arguments: `{"path":"a.go"}`},
		}}},
		{Role: RoleTool, ToolCallID: "call_1", Content: "package main"},
	}

	before, err := json.Marshal(msgs)
	if err != nil {
		t.Fatalf("marshal Message: %v", err)
	}
	after, err := json.Marshal(convertMessagesToOpenAI(msgs))
	if err != nil {
		t.Fatalf("marshal openAIRequestMessage: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("the wire bytes changed for image-free messages\n before: %s\n  after: %s", before, after)
	}
}

func TestOpenAIMessagesCarryImages(t *testing.T) {
	msgs := []Message{{
		Role:    RoleUser,
		Content: "why is this layout broken?",
		Images:  []Image{{MediaType: "image/png", Data: []byte{1, 2, 3}}},
	}}
	body, err := json.Marshal(convertMessagesToOpenAI(msgs))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(body)
	if !strings.Contains(got, `"type":"text"`) || !strings.Contains(got, "why is this layout broken?") {
		t.Errorf("the question did not survive the parts conversion: %s", got)
	}
	if !strings.Contains(got, `"image_url":{"url":"data:image/png;base64,AQID"}`) {
		t.Errorf("image part missing or malformed: %s", got)
	}
}

// The Responses runtime spells an image differently again: a typed input item
// whose parts are "input_text"/"input_image", with the picture as a flat
// image_url string rather than the nested object /chat/completions wants.
//
// This is the one wire format in this file with no other coverage of its image
// path — §69 restored the runtime from a snapshot, and a silently dropped part
// here reads exactly like a model that looked and saw nothing.
func TestResponsesMessagesCarryImages(t *testing.T) {
	_, input := convertMessagesToResponses([]Message{{
		Role:    RoleUser,
		Content: "why is this layout broken?",
		Images:  []Image{{MediaType: "image/png", Data: []byte{1, 2, 3}}},
	}})
	if len(input) != 1 || len(input[0].Content) != 2 {
		t.Fatalf("want one message of two parts, got %+v", input)
	}
	if text := input[0].Content[0]; text.Type != "input_text" || text.Text != "why is this layout broken?" {
		t.Errorf("first part = %+v, want the question as input_text", text)
	}
	image := input[0].Content[1]
	if image.Type != "input_image" {
		t.Errorf("second part type = %q, want input_image", image.Type)
	}
	if image.ImageURL != "data:image/png;base64,AQID" {
		t.Errorf("image_url = %q, want the data URL", image.ImageURL)
	}
}

// Same guarantee the OpenAI case above states: no image, no parts churn.
func TestResponsesImageWithoutTextSendsNoEmptyPart(t *testing.T) {
	_, input := convertMessagesToResponses([]Message{{
		Role:   RoleUser,
		Images: []Image{{MediaType: "image/webp", Data: []byte{7}}},
	}})
	if len(input) != 1 || len(input[0].Content) != 1 {
		t.Fatalf("want one message of one part, got %+v", input)
	}
	if input[0].Content[0].Type != "input_image" {
		t.Errorf("parts = %+v, want the image alone", input[0].Content)
	}
}

func TestAnthropicMessagesCarryImages(t *testing.T) {
	_, msgs := convertMessagesToAnthropic([]Message{{
		Role:    RoleUser,
		Content: "read this",
		Images:  []Image{{MediaType: "image/jpeg", Data: []byte{9, 9}}},
	}})
	if len(msgs) != 1 || len(msgs[0].Content) != 2 {
		t.Fatalf("want one message of two blocks, got %+v", msgs)
	}
	image := msgs[0].Content[1]
	if image.Type != "image" || image.Source == nil {
		t.Fatalf("second block is not an image: %+v", image)
	}
	if image.Source.Type != "base64" || image.Source.MediaType != "image/jpeg" || image.Source.Data != "CQk=" {
		t.Errorf("image source = %+v, want base64/image/jpeg/CQk=", image.Source)
	}
}

// An image with no caption must not drag an empty text block along: Anthropic
// rejects one, and the picture is the message.
func TestAnthropicImageWithoutTextSendsNoEmptyBlock(t *testing.T) {
	_, msgs := convertMessagesToAnthropic([]Message{{
		Role:   RoleUser,
		Images: []Image{{MediaType: "image/png", Data: []byte{1}}},
	}})
	if len(msgs) != 1 || len(msgs[0].Content) != 1 {
		t.Fatalf("want a single image block, got %+v", msgs)
	}
	if msgs[0].Content[0].Type != "image" {
		t.Errorf("block = %q, want image", msgs[0].Content[0].Type)
	}
}

func TestOllamaMessagesCarryImages(t *testing.T) {
	converted := convertMessagesToOllama([]Message{{
		Role:    RoleUser,
		Content: "what is this",
		Images:  []Image{{MediaType: "image/png", Data: []byte{7}}},
	}})
	if len(converted) != 1 {
		t.Fatalf("want one message, got %d", len(converted))
	}
	// Ollama takes bare base64 in a sibling field, with no media type and no
	// data: prefix — getting this wrong is a silently ignored image.
	if len(converted[0].Images) != 1 || converted[0].Images[0] != "Bw==" {
		t.Errorf("Images = %v, want one bare base64 string", converted[0].Images)
	}
	if converted[0].Content != "what is this" {
		t.Errorf("Content = %q, want the question kept beside the image", converted[0].Content)
	}
}
