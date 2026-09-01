# internal/tts — one language for every speaking engine

> Module map: [ARCHITECTURE.md §215](../../ARCHITECTURE.md) · The mirror of [internal/stt](../stt) on the speaking side · The buttons that use it: ฟัง under every reply and ลองฟัง on ตั้งค่า > เสียง ([desktop/voice.go](../../desktop/voice.go))

**What it is:** the translation layer between "some text-to-speech runtime" and the rest of Aetox. Engines disagree about everything — Windows SAPI is a COM surface with two voice registries that cannot see each other, Piper is a binary whose voices are `.onnx` files, cloud vendors are HTTP APIs. Nothing above this package is allowed to care. An Engine takes text and a path, writes a WAV there, and that is the whole contract.

Same shape as [internal/stt](../stt) and [internal/model](../model): a catalog describes what exists, `New()` switches on it, callers hold one interface.

## Key seams

| Seam | What hangs off it |
|---|---|
| `Engine` ([tts.go](tts.go)) | `Voices(ctx)` + `Synthesize(ctx, text, wavPath)`. The caller owns the WAV — picks the path, plays it, deletes it. |
| `Voice` ([tts.go](tts.go)) | `ID` is the engine's own stable identity (SAPI: the token description; Piper: the file path) and is what config pins. `Lang`/`Gender` are display. |
| `Descriptor` + `catalog` ([tts.go](tts.go)) | Data-only description of a vendor. The settings picker renders straight from this — adding a vendor is a row plus a file. |
| `New(Options)` ([tts.go](tts.go)) | The one switch from descriptor to runtime. Errors are Thai and actionable, and go straight to the user. |

## The Windows engine's one hard-won fact

Windows keeps voices in **two registries that cannot see each other**. System.Speech (and plain SAPI enumeration) reads only the old Desktop voices; every modern OneCore voice — including the Thai voice a Thai Windows 11 actually has (Pattara) — lives under `Speech_OneCore` and is only reachable by pointing `SpObjectTokenCategory` at that key directly. Measured on the owner's machine 2026-09-01: System.Speech in powershell.exe 5.1 saw 2 voices and no Thai; the token-category route saw all 6. [windows.go](windows.go) enumerates both, OneCore first, and never goes through System.Speech.

## Adding an engine

1. Add a `Descriptor` to `catalog` in [tts.go](tts.go).
2. Add a case in `newEngine` and a file implementing `Engine`.
3. Keep every runtime quirk inside that file. Nothing leaks out.

No caller changes. No UI changes — the settings picker is built from `Catalog()`.

## Rules of thumb

- **No policy in here.** Which voice matches the UI language is [desktop/voice.go](../../desktop/voice.go)'s judgement — that is where the locale lives. An engine speaks with the voice it is told, or its own default.
- **Never auto-download a voice** (§32, same as speech models). A missing Piper voice is an error that names where voices come from and where to put the file.
- **Cloud rows are labeled cloud, and say what leaves.** The owner opened the catalog to cloud vendors on 2026-09-01 (§216): Edge and gTTS free with no key; OpenAI, Groq (PlayAI, English only), Gemini (multilingual incl. Thai, PCM wrapped to WAV in-engine) and ElevenLabs on the user's own key. Every cloud row's Label carries "คลาวด์" and its Install text says the text goes out — the local Windows engine stays the default. The Gemini engine skips the base-URL override on purpose: the provider row may point at the OpenAI-compatible chat endpoint, and this is a native-API call.
- **Keys come from the store the models page fills** (`config.ProviderAPIKey`), with the provider's usual env vars as fallback; base URLs honor the same per-provider override, so the OpenAI row pointed at a LocalAI/Speaches box speaks through it. A pip-installed CLI vendor is also found in `%APPDATA%\Python\*\Scripts`, which pip fills without putting on PATH.
