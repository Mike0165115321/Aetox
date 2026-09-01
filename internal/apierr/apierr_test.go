package apierr

import (
	"strings"
	"testing"
)

// The case the package was born from: OpenAI's 401 echoes the redacted key —
// hundreds of asterisks — and the settings page used to show all of them. A
// bad key must come back as one calm sentence that says where to fix it, with
// no quote of the body at all.
func TestBadKeyIsOneCalmSentence(t *testing.T) {
	body := []byte(`{ "error": { "message": "Incorrect API key provided: sk-proj-` + strings.Repeat("*", 300) + `. You can find your API key at https://platform.openai.com/account/api-keys." } }`)
	err := HTTP("OpenAI", 401, body)
	if strings.Contains(err.Error(), "*") {
		t.Errorf("401 message quotes the body: %q", err)
	}
	if !strings.Contains(err.Error(), "ตั้งค่า") {
		t.Errorf("401 message does not say where to fix the key: %q", err)
	}
}

// Unknown statuses keep the vendor's words — they may name the real problem —
// but extracted from the envelope, not the raw JSON, and with redaction runs
// collapsed so they cannot stripe the page.
func TestUnknownStatusKeepsTheVendorsWordsCleanly(t *testing.T) {
	body := []byte(`{"error": {"message": "The model 'tts-9' does not exist: ` + strings.Repeat("*", 50) + `"}}`)
	err := HTTP("OpenAI", 404, body)
	msg := err.Error()
	if !strings.Contains(msg, "tts-9") {
		t.Errorf("404 dropped the vendor's explanation: %q", msg)
	}
	if strings.Contains(msg, `{"error"`) {
		t.Errorf("404 shows the raw envelope: %q", msg)
	}
	if strings.Contains(msg, "****") {
		t.Errorf("redaction run survived: %q", msg)
	}
}

// ElevenLabs wraps as {"detail":{"message":...}}; a body that is not JSON at
// all is still shown, first line only.
func TestOtherEnvelopesAndPlainText(t *testing.T) {
	if err := HTTP("ElevenLabs", 400, []byte(`{"detail":{"message":"voice not found"}}`)); !strings.Contains(err.Error(), "voice not found") {
		t.Errorf("detail.message not extracted: %q", err)
	}
	if err := HTTP("Groq", 418, []byte("first line\nsecond line")); strings.Contains(err.Error(), "second") {
		t.Errorf("kept more than the first line: %q", err)
	}
}
