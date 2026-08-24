package model

import (
	"strings"
	"testing"
)

// The body below is verbatim from a real turn on 2026-08-20, with the account
// id shortened. Two different models produced two different function ids and
// the same refusal, which is what established that it is the account and not
// the model — the fact the raw message hides.
const nvidiaEntitlement404 = `{"status":404,"title":"Not Found","detail":"Function 'ee47df99-c92b-4dc9-b3a7-f3fb0f087b73': Not found for account 'LYikgj4…'"}`

func TestAccountAccessErrorNamesTheRealFix(t *testing.T) {
	err := accountAccessError("nvidia", 404, []byte(nvidiaEntitlement404), nvidiaEntitlement404)
	if err == nil {
		t.Fatal("a refusal-by-entitlement was read as an ordinary 404 — the user is sent back to the model picker for a problem no model can solve")
	}
	msg := err.Error()
	// Try another model first, support second. The order matters: on the key
	// this was measured with, a priced model answered fine minutes after two
	// free ones were refused, so "your account is locked" would have been a
	// wrong instruction for a wall the user could walk around.
	for _, want := range []string{"try another model", "help@build.nvidia.com", "nvidia"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message does not mention %q, so it does not say what to do: %s", want, msg)
		}
	}
	if strings.Index(msg, "try another model") > strings.Index(msg, "help@build.nvidia.com") {
		t.Errorf("support is offered before the fix that usually works: %s", msg)
	}
	// The raw body survives in the message. A user forwarding this to NVIDIA
	// support needs the function id, and support asks for it by name.
	if !strings.Contains(msg, "ee47df99") {
		t.Errorf("the function id was dropped; support asks for it: %s", msg)
	}
}

// The common 404 must stay the common 404. Rewriting every not-found into an
// account lecture would be worse than the raw message it replaced.
func TestAnOrdinaryNotFoundIsLeftAlone(t *testing.T) {
	body := `{"error":{"message":"The model 'gpt-9' does not exist","code":"model_not_found"}}`
	if err := accountAccessError("openai", 404, []byte(body), body); err != nil {
		t.Fatalf("a missing model was rewritten as an account problem: %v", err)
	}
}

// Verbatim from dashscope-intl.aliyuncs.com on 2026-08-24, junk key. The same
// body came back from dashscope, dashscope-us, and a made-up workspace id on
// ap-southeast-1 — five hosts, one sentence, and nothing in it about regions.
const modelStudioBadKey401 = `{"error":{"message":"Incorrect API key provided. For details, see: https://www.alibabacloud.com/help/en/model-studio/error-code#apikey-error","type":"invalid_request_error","param":null,"code":"invalid_api_key"},"request_id":"6ce1ad79-c7e2-9afb-acf8-e68191913351"}`

// And from cn-beijing the same day: identical except the help link, which is
// the Chinese doc site. The marker has to survive that difference or the hint
// appears for half the regions it is about.
const modelStudioBadKey401CN = `{"error":{"message":"Incorrect API key provided. For details, see: https://help.aliyun.com/zh/model-studio/error-code#apikey-error","type":"invalid_request_error","param":null,"code":"invalid_api_key"},"request_id":"a38e3d77-cff0-9152-bfe0-52c7e5d6fd4e"}`

// And from a workspace host on ap-southeast-1, same day: a THIRD wording for
// the same rejection — "Invalid API-key provided", with the request id under
// "id" instead of "request_id". Three spellings across five hosts is the whole
// argument for matching the doc link rather than the sentence: a marker written
// against the prose would have gone quiet on the maas hosts, which are the five
// regions out of six.
const modelStudioBadKey401MAAS = `{"error":{"message":"Invalid API-key provided. For details, see: https://www.alibabacloud.com/help/en/model-studio/error-code#apikey-error","id":"12105deb-efba-9660-a304-2edaa46de207","type":"invalid_request_error","code":"invalid_api_key"}}`

func TestModelStudio401SaysTheRegionCouldBeTheCause(t *testing.T) {
	for name, body := range map[string]string{
		"international":  modelStudioBadKey401,
		"china":          modelStudioBadKey401CN,
		"maas workspace": modelStudioBadKey401MAAS,
	} {
		hint := credentialHint([]byte(body))
		if hint == "" {
			t.Fatalf("%s: a Model Studio 401 got no hint — the user is told the key is wrong and nothing about the six endpoints it could belong to", name)
		}
		for _, want := range []string{"region", "base URL", "maas.aliyuncs.com"} {
			if !strings.Contains(hint, want) {
				t.Errorf("%s: hint does not mention %q: %s", name, want, hint)
			}
		}
	}
}

// Every other provider's 401 stays exactly as it was. A key really can just be
// wrong, and appending a lecture about Alibaba regions to OpenAI's rejection
// would be noise on the far more common case.
func TestAnOrdinaryBadKeyGetsNoHint(t *testing.T) {
	for _, body := range []string{
		`{"error":{"message":"Incorrect API key provided: sk-abc***","type":"invalid_request_error","code":"invalid_api_key"}}`,
		`{"error":{"message":"No credentials presented"}}`,
	} {
		if hint := credentialHint([]byte(body)); hint != "" {
			t.Errorf("an ordinary 401 was given the Model Studio hint: %s", hint)
		}
	}
}
