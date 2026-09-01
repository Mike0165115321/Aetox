// Package apierr turns a cloud vendor's HTTP refusal into one calm Thai
// sentence.
//
// Born on the voice page, 1 ก.ย. 2026: OpenAI's 401 body embeds the whole
// redacted key — "sk-proj-" followed by hundreds of asterisks — and the
// engines showed that body verbatim, painting a red siren across the settings
// page. The statuses whose meaning is known (a bad key, a rate limit, a vendor
// outage) get a sentence that says what happened and what to do, and never
// quote the body at all. Anything else keeps the vendor's own words, because
// sometimes they are the only thing that names the real problem (a model that
// does not exist) — but extracted from the JSON envelope, collapsed to one
// line, and capped, never the raw dump.
//
// One home on purpose: internal/stt and internal/tts each had their own copy
// of the raw-body format, three sites in all, which is how the same ugly
// message got three chances to appear.
package apierr

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// HTTP is the one sentence for a non-2xx vendor reply. vendor is a display
// name ("OpenAI"), body the response body, read in full by the caller.
func HTTP(vendor string, status int, body []byte) error {
	switch {
	case status == 401 || status == 403:
		return fmt.Errorf("%s ไม่รับ API key (%d) ตรวจคีย์ที่ ตั้งค่า > โมเดล แล้วลองใหม่", vendor, status)
	case status == 429:
		return fmt.Errorf("%s ให้รอก่อน (429) เรียกถี่เกินไปหรือเครดิตหมด อีกสักครู่ค่อยลองใหม่", vendor)
	case status >= 500:
		return fmt.Errorf("%s ขัดข้องฝั่งเซิร์ฟเวอร์เขาเอง (%d) ลองใหม่อีกครั้ง", vendor, status)
	}
	if msg := message(body); msg != "" {
		return fmt.Errorf("%s ตอบ %d: %s", vendor, status, msg)
	}
	return fmt.Errorf("%s ตอบ %d", vendor, status)
}

// redactedRun is a vendor echoing a key back with most of it starred out.
// Three stars carry the meaning; three hundred carry a page-wide stripe.
var redactedRun = regexp.MustCompile(`\*{4,}`)

// message digs the human sentence out of an error body. Vendors wrap it
// differently — OpenAI and Google as {"error":{"message":...}}, ElevenLabs as
// {"detail":{"message":...}}, some as a bare {"message":...} — and a body
// that is not JSON at all is used as-is. Whatever is found is trimmed to its
// first line, has redaction runs collapsed, and is capped.
func message(body []byte) string {
	text := strings.TrimSpace(string(body))
	var envelope map[string]any
	if json.Unmarshal(body, &envelope) == nil {
		for _, path := range [][]string{
			{"error", "message"},
			{"detail", "message"},
			{"message"},
			{"error"},
			{"detail"},
		} {
			if found := dig(envelope, path); found != "" {
				text = found
				break
			}
		}
	}
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = strings.TrimSpace(text[:i])
	}
	text = redactedRun.ReplaceAllString(text, "***")
	if runes := []rune(text); len(runes) > 200 {
		text = string(runes[:200]) + "…"
	}
	return text
}

// dig walks nested maps and returns the string at the end of path, or "".
func dig(m map[string]any, path []string) string {
	var current any = m
	for _, key := range path {
		node, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = node[key]
	}
	s, _ := current.(string)
	return strings.TrimSpace(s)
}
