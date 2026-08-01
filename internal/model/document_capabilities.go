package model

import "strings"

// Which models can be handed a whole document to read.
//
// The sibling of ResolveVision, and the same gate for the same reason: a model
// that cannot take a file gets a 400 from some backends and silently ignores
// the part on others, and silent is the worse one — the model then answers
// about a document it never received. The fallback is pdf_read, which works
// everywhere and loses the layout.
//
// It differs from ResolveVision in what it keys on, and the difference is the
// point. Vision is a property of the *model*, so that resolver matches on the
// model name across every provider. Accepting a file upload is a property of
// the *endpoint*: the same gpt-5.5 reached through a third-party
// OpenAI-compatible gateway has no file part to send it through. So this keys
// on the provider first, and only then asks whether the model is one that
// reads.
//
// Deliberately one provider today. codex is the path verified against the real
// backend (a PDF in, the number drawn inside it back out); anthropic and
// openai both document a file part and are the obvious next two, but an
// unverified wire shape here is a 400 on a turn that works fine today, and the
// fallback it would replace is not broken. Add each one as it is proven, not
// as it is read about.
var documentProviders = map[string]bool{
	"codex": true,
}

// documentMediaTypes is what Aetox will hand over. PDF alone: it is the format
// the attachment picker has always taken, the one `read` cannot open, and the
// only one the verified backend was checked with.
var documentMediaTypes = map[string]bool{
	"application/pdf": true,
}

// ResolveDocuments reports whether this provider and model accept a document
// part. An unknown pair is treated as "no" — that costs the layout and keeps a
// working pdf_read path, where the other way costs a failed turn.
func ResolveDocuments(provider, modelName string) bool {
	if !documentProviders[NormalizeProvider(provider)] {
		return false
	}
	// Same reading as ResolveVision: a model that cannot look at a page cannot
	// read a document either, and every model on a document-capable endpoint
	// that can see is one that reads.
	return ResolveVision(provider, modelName)
}

// SupportsDocumentType reports whether Aetox will send this media type as a
// document at all.
func SupportsDocumentType(mediaType string) bool {
	return documentMediaTypes[strings.ToLower(strings.TrimSpace(mediaType))]
}
