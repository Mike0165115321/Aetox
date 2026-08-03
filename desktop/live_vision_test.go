package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mike0165115321/Aetox/internal/bootstrap"
	"github.com/Mike0165115321/Aetox/internal/model"
)

// The two changes unit tests cannot prove.
//
// §51 and §53.2 changed what goes on the wire — content parts, image blocks,
// Ollama's sibling images field. A unit test proves the JSON has the right
// shape; only a real provider proves the provider agrees. §52's web_fetch
// digest is the same story: it is one more completion, and the only question
// that matters is whether a real model answers the question instead of
// summarising the page.
//
// Run with:
//
//	AETOX_LIVE=1 go test ./desktop/ -run TestLiveVision -v -count=1
//
// Skips without a key rather than failing: CI has none, and a red suite there
// would say nothing about the code.

// writeVisionFixture draws a shape no OCR could read as text — three coloured
// bars — so a model describing it correctly can only have seen the picture.
func writeVisionFixture(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 240, 120))
	bars := []color.RGBA{{220, 40, 40, 255}, {40, 180, 60, 255}, {50, 90, 230, 255}}
	for x := 0; x < 240; x++ {
		for y := 0; y < 120; y++ {
			img.Set(x, y, bars[x/80])
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("fixture: %v", err)
	}
}

// TestLiveVisionImageReachesTheModel sends a picture to a real vision model and
// asks it something only the picture answers.
func TestLiveVisionImageReachesTheModel(t *testing.T) {
	key := liveDeepSeekKey(t)
	modelName := strings.TrimSpace(os.Getenv("AETOX_LIVE_VISION_MODEL"))
	if modelName == "" {
		t.Skip("set AETOX_LIVE_VISION_MODEL to a model with vision (the default provider here has none)")
	}
	provider := strings.TrimSpace(os.Getenv("AETOX_LIVE_VISION_PROVIDER"))
	if provider == "" {
		provider = "openrouter"
	}
	if !model.ResolveVision(provider, modelName) {
		t.Fatalf("ResolveVision(%q, %q) = false — the gate would refuse to send the image at all", provider, modelName)
	}

	root := t.TempDir()
	shot := filepath.Join(root, "bars.png")
	writeVisionFixture(t, shot)
	data, err := os.ReadFile(shot)
	if err != nil {
		t.Fatal(err)
	}

	p, err := model.NewProvider(model.ProviderOptions{
		Provider: provider,
		Model:    modelName,
		APIKey:   liveVisionKey(t, provider, key),
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}

	resp, err := p.Complete(context.Background(), model.Request{
		Model:     modelName,
		MaxTokens: 200,
		Messages: []model.Message{{
			Role:    model.RoleUser,
			Content: "This image is three vertical bars. Name their colours, left to right, in three words.",
			Images:  []model.Image{{MediaType: "image/png", Data: data}},
		}},
	})
	if err != nil {
		t.Fatalf("the provider rejected a message carrying an image: %v", err)
	}
	answer := strings.ToLower(resp.Text)
	t.Logf("model said: %s", strings.TrimSpace(resp.Text))
	for _, colour := range []string{"red", "green", "blue"} {
		if !strings.Contains(answer, colour) {
			t.Errorf("answer is missing %q — the model did not see the image:\n%s", colour, resp.Text)
		}
	}
}

// TestLiveVisionOpenAICompatibleParts proves the *other* wire shape.
//
// The test above goes through convertMessagesToOllama, which uses Ollama's
// native `images: [base64]` sibling field. The content-parts form is a
// different code path entirely and it is the one that serves almost every
// provider in the catalog — OpenAI, DeepSeek, OpenRouter, Groq, LM Studio.
// Ollama also speaks OpenAI-compatible at /v1, so the same local vision model
// can prove it without a paid key.
//
//	AETOX_LIVE=1 AETOX_LIVE_OPENAI_VISION_URL=http://127.0.0.1:11434/v1 \
//	  AETOX_LIVE_VISION_MODEL=qwen3-vl:8b \
//	  go test ./desktop/ -run TestLiveVisionOpenAICompatible -v -count=1
func TestLiveVisionOpenAICompatibleParts(t *testing.T) {
	if os.Getenv("AETOX_LIVE") != "1" {
		t.Skip("set AETOX_LIVE=1 to run live tests")
	}
	baseURL := strings.TrimSpace(os.Getenv("AETOX_LIVE_OPENAI_VISION_URL"))
	modelName := strings.TrimSpace(os.Getenv("AETOX_LIVE_VISION_MODEL"))
	if baseURL == "" || modelName == "" {
		t.Skip("set AETOX_LIVE_OPENAI_VISION_URL and AETOX_LIVE_VISION_MODEL")
	}

	root := t.TempDir()
	shot := filepath.Join(root, "bars.png")
	writeVisionFixture(t, shot)
	data, err := os.ReadFile(shot)
	if err != nil {
		t.Fatal(err)
	}

	p, err := model.NewProvider(model.ProviderOptions{
		Provider: "openai",
		Model:    modelName,
		BaseURL:  baseURL,
		APIKey:   "none", // a local endpoint wants no key, but the factory wants a value
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}

	resp, err := p.Complete(context.Background(), model.Request{
		Model:     modelName,
		MaxTokens: 200,
		Messages: []model.Message{{
			Role:    model.RoleUser,
			Content: "This image is three vertical bars. Name their colours, left to right, in three words.",
			Images:  []model.Image{{MediaType: "image/png", Data: data}},
		}},
	})
	if err != nil {
		t.Fatalf("an OpenAI-compatible endpoint rejected the content-parts message: %v", err)
	}
	answer := strings.ToLower(resp.Text)
	t.Logf("model said: %s", strings.TrimSpace(resp.Text))
	for _, colour := range []string{"red", "green", "blue"} {
		if !strings.Contains(answer, colour) {
			t.Errorf("answer is missing %q — the image did not survive the parts conversion:\n%s", colour, resp.Text)
		}
	}
}

// TestLiveWebFetchDigestAnswersTheQuestion proves the digester returns an
// answer rather than a summary of the page, against a real model.
func TestLiveWebFetchDigestAnswersTheQuestion(t *testing.T) {
	key := liveDeepSeekKey(t)
	p, err := model.NewProvider(model.ProviderOptions{
		Provider: "deepseek", Model: "deepseek-v4-flash", APIKey: key,
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}

	digest := bootstrap.Digester(p, "deepseek-v4-flash")
	if digest == nil {
		t.Fatal("bootstrap.Digester returned nil for a real provider")
	}
	page := strings.Repeat("Assorted filler about unrelated topics. ", 400) +
		"\nThe maximum retry count is controlled by the MaxAttempts option, which defaults to 5.\n" +
		strings.Repeat("More filler that answers nothing. ", 200)

	answer, err := digest(context.Background(), "what option controls the retry count, and what is its default?", page)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	t.Logf("digest said: %s", answer)
	if !strings.Contains(answer, "MaxAttempts") {
		t.Errorf("the answer does not name the option:\n%s", answer)
	}
	if !strings.Contains(answer, "5") {
		t.Errorf("the answer does not give the default:\n%s", answer)
	}
	// The point of the feature: an answer, not the page back.
	if len(answer) > len(page)/4 {
		t.Errorf("the digest is %d chars against a %d char page — that is a summary, not an answer", len(answer), len(page))
	}
}

// liveVisionKey lets the vision test run against a provider other than the one
// whose key liveDeepSeekKey found, without inventing a second config format.
func liveVisionKey(t *testing.T, provider, fallback string) string {
	t.Helper()
	if v := strings.TrimSpace(os.Getenv("AETOX_LIVE_VISION_KEY")); v != "" {
		return v
	}
	if provider == "deepseek" {
		return fallback
	}
	t.Skipf("set AETOX_LIVE_VISION_KEY for provider %q", provider)
	return ""
}
