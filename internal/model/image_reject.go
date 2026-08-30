package model

import "strings"

// imageRejectionPhrases are what providers say when the request was refused
// because of a PICTURE in it, rather than because of its length, its tools, its
// account or its rate. Lowercase; matched as substrings, in the vendor's own
// words — the same contract contextLimitPhrases keeps, and for the same reason:
// every provider folds this into a plain "request failed with status 400: ..."
// on the way out of the client, so the prose is the only thing left to read.
var imageRejectionPhrases = []string{
	"unsupported image",       // DeepSeek: "You have uploaded an unsupported image"
	"could not process image", // Anthropic
	"invalid_image",           // OpenAI error code (invalid_image_format, invalid_image_url)
	"invalid image",           // the same thing spelt as prose by several resellers
	"image exceeds",           // "image exceeds 8000 pixels"
	"image dimensions",        // Gemini, and Anthropic's over-size wording
	"failed to process image",
	"image is too large",
}

// IsImageRejection reports that a request failed because of a picture it
// carried, and that sending it again without the pictures is the fix.
//
// This is the fourth failure in the tool loop with a mechanical answer, beside
// context length, a refused tool block and an empty completion, and it is the
// one that does not heal on its own. The others fail the turn; this one poisons
// the conversation. The picture that caused it is in the history, so the next
// turn the user types is refused before it reaches the model — measured on
// 30 ส.ค., where "ต่อ" died in 2.3 seconds on the identical 400 the previous
// turn had spent six minutes earning (FitForWire carries the full account).
//
// Conservative on purpose, and deliberately narrower than it could be: a
// picture the provider merely resized, a model without eyes, and a quota wall
// are all failures with other fixes. A false positive costs one round without
// the pictures; a false negative leaves the conversation where it already was,
// which is dead.
func IsImageRejection(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, phrase := range imageRejectionPhrases {
		if strings.Contains(msg, phrase) {
			return true
		}
	}
	return false
}
