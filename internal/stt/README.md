# internal/stt — one language for every speech engine

> Module map: [ARCHITECTURE.md §4.1](../../ARCHITECTURE.md) · Decision: [§33](../../ARCHITECTURE.md) · Two callers since §215: [internal/skill/audio_transcribe.go](../skill/audio_transcribe.go) and the composer's mic ([desktop/voice.go](../../desktop/voice.go)) · The speaking mirror: [internal/tts](../tts)

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

- **Local is the default; cloud is a named choice** (§31 as amended by §216, 2026-09-01). The default engine runs on this machine and recordings never leave it — until the user picks a cloud row (OpenAI, Groq, Mistral Voxtral, Gemini, ElevenLabs Scribe) by name, whose Install text says outright that the audio goes out. What stays forbidden is Aetox sending audio anywhere on its own judgement. A vendor that differs only in field spellings is a spec entry on the shared multipart engine ([openai_api.go](openai_api.go)), not a new engine; Gemini rides its native API ([gemini.go](gemini.go)) and skips the base-URL override on purpose.
- **One row covers every OpenAI-compatible server.** The cloud rows honor the same per-provider base-URL override the LLM side uses, so a local Speaches/LocalAI box is the OpenAI row pointed at another address, not a new engine.
- **Never auto-download a model.** A missing model is an error that tells the user what to fetch, how big it is, and where to put it (§32). Bandwidth is theirs to spend.
- **Never write outside the managed store.** A model found in someone else's folder is used where it lies.
