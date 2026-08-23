package model

import (
	"strings"

	"github.com/Mike0165115321/Aetox/internal/provider"
)

// Which models can be handed a whole document to read.
//
// The sibling of ResolveVision, and the same gate for the same reason: a model
// that cannot take a file gets a 400 from some backends and silently ignores
// the part on others, and silent is the worse one — the model then answers
// about a document it never received. The fallback is pdf_read, which works
// everywhere and loses the layout.
//
// Two questions, and keeping them apart is the standard this package now runs
// on. Can this ENDPOINT carry a document part at all? That is a wire dialect,
// so it is answered by runtime, below. Does this MODEL read one? That is the
// catalog's, per model, and it is why the answer stopped being a list.
//
// The trade against pdf_read runs the opposite way to the usual one and is
// worth restating: sending the document costs more tokens, not fewer — the
// model ingests the whole file instead of 220 extracted lines. It buys layout,
// tables, charts and scanned pages. desktop/app.go caps the size at which that
// stops being worth it.

// documentRuntimes are the wire dialects that can carry a document, each with
// the shape it carries one in:
//
//	responses           input_file  {filename, file_data}
//	anthropic           document    {source: {type: base64, media_type, data}}
//	openai-compatible   file        {filename, file_data}
//
// Keyed by runtime rather than by provider name, and that is the change. The
// list used to read `{"codex": true}` — one name, with a note that anthropic and
// openai "are the obvious next two" and would be added as each was proven. That
// is a list that has to be edited every time a provider is added, which is
// exactly what adding a provider is supposed to stop requiring. There are three
// dialects and twenty-two rows; the rows do not each need an opinion.
//
// Ollama is absent on purpose: its native API has no file part, and a model
// pulled onto a local GPU is not described by any catalog either.
var documentRuntimes = map[provider.Runtime]bool{
	provider.RuntimeResponses:        true,
	provider.RuntimeAnthropic:        true,
	provider.RuntimeOpenAICompatible: true,
}

// documentMediaTypes is what Aetox will hand over. PDF alone: it is the format
// the attachment picker has always taken, the one `read` cannot open, and the
// only one every dialect above documents.
var documentMediaTypes = map[string]bool{
	"application/pdf": true,
}

// ResolveDocuments reports whether this provider and model accept a document
// part.
//
// Where the catalog knows the model, its answer is used outright — 103 of
// OpenRouter's models take a pdf, 35 of opencode's, 19 of Gemini's, and every
// one of them was being handed extracted text instead.
//
// Where it does not, the rule that has been serving is kept: a model that can
// look at a page can read a document. That is what keeps codex working, which
// is the one path verified against a real backend (a PDF in, the number drawn
// inside it back out) and which no catalog describes, because "codex" is a
// subscription rather than a published model list.
func ResolveDocuments(providerName, modelName string) bool {
	if !documentRuntimes[provider.RuntimeFor(NormalizeProvider(providerName))] {
		return false
	}
	caps := resolveModalities(providerName, modelName)
	if caps.Source == "models.dev" {
		return caps.Accepts("pdf")
	}
	return caps.Accepts("image")
}

// SupportsDocumentType reports whether Aetox will send this media type as a
// document at all.
func SupportsDocumentType(mediaType string) bool {
	return documentMediaTypes[strings.ToLower(strings.TrimSpace(mediaType))]
}
