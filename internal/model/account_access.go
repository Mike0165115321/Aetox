package model

import (
	"bytes"
	"fmt"
)

// accountAccessMarkers are what a provider says when the credentials are fine
// and the account simply is not allowed to call the thing it just asked for.
//
// This is a third failure, and it was previously indistinguishable from the
// other two. A key that is wrong answers 401 and a wallet that is empty answers
// 402 or 429, and both of those already have a sentence a user can act on. This
// one arrives as a 404 — the same status a misspelled model id returns — so the
// only honest reading of the raw message is "Aetox asked for something that
// does not exist", which sends the user hunting through a model picker for a
// problem no model can solve.
//
// Lowercase, matched as substrings, in the vendor's own words. Same reasoning
// as contextLimitPhrases: the prose is the API here, and it is steadier than
// the status code beside it.
var accountAccessMarkers = []struct {
	marker []byte
	remedy string
}{
	{
		// NVIDIA: integrate.api.nvidia.com resolves a model id to an NVCF
		// function and then refuses to invoke it, echoing the account it
		// authenticated. So the key is good and the account is real — what is
		// missing is this account's access to this one function.
		//
		// Measured on one real key, 2026-08-20, and the second measurement is
		// what makes the wording careful. First reading was that the account
		// was locked out entirely: two free models, two function ids, the same
		// refusal, and NVIDIA's forums full of people asking for an entitlement
		// called "Public API Endpoints". Then nemotron-3-ultra-550b — a priced
		// model on the same key, the same account, minutes later — answered in
		// 7 seconds. So it is per model, not per account, and a message that
		// said "your account is locked" would have sent someone to email
		// support about a wall they could have walked around by picking a
		// different row in the list.
		//
		// Hence: the first sentence is the fix that works most of the time, and
		// support is named second, for the case where nothing in the list works.
		// GET /v1/models cannot predict this — it lists the shelf, not what the
		// account may call — which is why this is caught here and not filtered
		// out of the picker.
		marker: []byte("not found for account"),
		remedy: "the key and the account are fine; this particular model is not available to them. " +
			"NVIDIA turns each model id into a function and only some are deployed for a given account, " +
			"and its model list cannot tell you which — try another model from the list first. " +
			"If every one of them fails the same way, that is the account-wide case, and NVIDIA does not " +
			"self-serve it: email help@build.nvidia.com with your registration email, the phone number " +
			"you verified with, and this message.",
	},
}

// accountAccessError turns a provider's refusal-by-entitlement into a sentence
// that says what to do, or returns nil when this is an ordinary failure.
//
// Returning nil rather than a guess is the point: a 404 usually IS a model that
// does not exist, and rewriting every one of those into an account lecture would
// be worse than the raw message it replaced.
func accountAccessError(providerName string, status int, body []byte, detail string) error {
	lower := bytes.ToLower(body)
	for _, entry := range accountAccessMarkers {
		if !bytes.Contains(lower, entry.marker) {
			continue
		}
		return fmt.Errorf("%s accepted the key but refused the call: %s (%d: %s)",
			providerName, entry.remedy, status, detail)
	}
	return nil
}
