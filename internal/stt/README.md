# internal/stt — one language for every speech engine

> Module map: [ARCHITECTURE.md §4.1](../../ARCHITECTURE.md) · Decision: [§33](../../ARCHITECTURE.md) · The tool that uses it: [internal/skill/audio_transcribe.go](../skill/audio_transcribe.go)

**What it is:** the translation layer between "some speech recognition runtime" and the rest of Aetox. Engines disagree about everything — whisper.cpp is a C++ binary printing `[HH:MM:SS.mmm --> ...]` on stdout, faster-whisper is a Python CLI, Vosk and sherpa-onnx are different runtimes with different model formats. Nothing above this package is allowed to care.

Same shape as [internal/model](../model) does for LLM providers: a catalog describes what exists, `New()` switches on it, callers hold one interface.

## Key seams

| Seam | What hangs off it |
|---|---|
| `Segment` ([stt.go](stt.go)) | `StartMs`, `EndMs`, `Text` — **the one language**. Every engine is translated into this. Milliseconds, not `[m:ss]`: formatting is the caller's choice. |
| `Engine` ([stt.go](stt.go)) | `Transcribe(ctx, wavPath) ([]Segment, error)`. Input is always a 16kHz mono WAV; the caller produces it and owns the file. |
| `Descriptor` + `catalog` ([stt.go](stt.go)) | Data-only description of an engine: binary candidates, model glob, install hint. The settings UI renders straight from this. |
| `New(Options)` ([stt.go](stt.go)) | The one switch from descriptor to runtime. Errors are Thai and actionable — unknown engine, missing binary, missing model — and go straight to the user. |
| `Stores` / `InstalledModels` ([stores.go](stores.go)) | Managed store (`<DataRoot>/models`, Aetox may delete) vs external stores (Ollama, LM Studio, HF cache, user folders — **read-only forever**). `InstalledModel.Managed` carries that all the way to the delete button. |

## Adding an engine

1. Add a `Descriptor` to `catalog` in [stt.go](stt.go).
2. Add a case in `newEngine` and a file implementing `Engine`.
3. Translate its output into `[]Segment` inside that file. Nothing leaks out.

No caller changes. No UI changes — the settings picker is built from `Catalog()`.

## Rules of thumb

- **No cloud engines.** Not a technical limit, a product one (ARCHITECTURE.md §31): audio does not leave the machine. The interface would happily accept one; the catalog will not ship one.
- **Never auto-download a model.** A missing model is an error that tells the user what to fetch, how big it is, and where to put it (§32). Bandwidth is theirs to spend.
- **Never write outside the managed store.** A model found in someone else's folder is used where it lies.
