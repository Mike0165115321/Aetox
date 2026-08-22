package agentpkg

import "testing"

func TestParseAskReadsNameAndLabel(t *testing.T) {
	name, label, ok := ParseAsk("${ask:NOTION_TOKEN|โทเค็นของ Notion}")
	if !ok {
		t.Fatalf("a well-formed placeholder was not recognised")
	}
	if name != "NOTION_TOKEN" || label != "โทเค็นของ Notion" {
		t.Fatalf("name=%q label=%q", name, label)
	}
	if name, label, ok = ParseAsk("${ask:TOKEN}"); !ok || name != "TOKEN" || label != "" {
		t.Fatalf("bare placeholder: name=%q label=%q ok=%v", name, label, ok)
	}
}

// Half-understanding a value is worse than refusing it: the half that was
// understood would be written into a config file as though it were finished.
func TestParseAskRefusesAGluedToken(t *testing.T) {
	for _, v := range []string{
		"Bearer ${ask:TOKEN}",
		"${ask:TOKEN} extra",
		"${ask:}",
		"${ask:A}${ask:B}",
		"plain-value",
		"",
	} {
		if _, _, ok := ParseAsk(v); ok {
			t.Fatalf("%q was read as a placeholder", v)
		}
	}
}

func TestLooksSecret(t *testing.T) {
	secret := []string{"NOTION_TOKEN", "api_key", "GITHUB_PAT_SECRET", "Authorization", "DB_PASSWORD", "session_id"}
	plain := []string{"NOTION_LOCALE", "PORT", "NODE_ENV", "BASE_URL", "TIMEOUT_MS"}
	for _, k := range secret {
		if !LooksSecret(k) {
			t.Errorf("%q should be asked for, not shipped", k)
		}
	}
	for _, k := range plain {
		if LooksSecret(k) {
			t.Errorf("%q is an ordinary setting and should travel", k)
		}
	}
}
