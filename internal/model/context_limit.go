package model

import "strings"

// contextLimitPhrases are what providers say when the prompt itself is too big
// for the model, as opposed to too big for the minute (a rate limit) or too big
// for the account (a quota). Lowercase; matched as substrings.
//
// Every entry is a real message from a provider Aetox ships with, kept in the
// vendor's own words rather than normalized, because the words are the API here.
var contextLimitPhrases = []string{
	"context_length_exceeded",              // OpenAI error code, chat and responses
	"maximum context length",               // "This model's maximum context length is 400000 tokens"
	"exceeds the context window",           // Responses API's phrasing of the same thing
	"prompt is too long",                   // Anthropic: "prompt is too long: 219398 tokens > 200000 maximum"
	"exceeds the maximum number of tokens", // Gemini
	"reduce the length of the messages",    // OpenAI's suggested remedy, present when the code is not
	"too many tokens",                      // several OpenAI-compatible resellers
}

// IsContextLengthError reports that a request failed because the prompt did not
// fit the model's context window, and that a shorter history is the fix.
//
// String matching, and it is not a shortcut taken for speed: providers return
// this as a 400 with prose, and Aetox's own provider layer folds that prose into
// a plain fmt.Errorf (anthropic.go, openai_compatible.go, responses.go, and
// ollama.go all build "... request failed with status %d: %s"). There is no
// typed error to switch on without changing five providers first, and the
// phrases below are stable in a way the numeric status is not: 400 is also what
// a malformed tool schema returns.
//
// Deliberately conservative. A false positive costs one summarization round the
// conversation did not need; a false negative costs nothing that is not already
// being lost today, because before this the answer was always "no". So the
// phrases are specific to the size of the PROMPT, and rate limits, quota
// exhaustion and output-token truncation are all left out on purpose. Those are
// three different failures with three different fixes, and a compaction helps
// none of them.
func IsContextLengthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, phrase := range contextLimitPhrases {
		if strings.Contains(msg, phrase) {
			return true
		}
	}
	return false
}
