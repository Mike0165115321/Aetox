# Third-party notices

Aetox itself is proprietary (see [LICENSE](LICENSE)). The components listed
here are **not**: each one is the work of its own authors, licensed under its
own terms, and nothing in the Aetox licence restricts your rights in any of
them. They are listed so that the notices travel with the software, which is
what those licences ask for.

Every component below is under a permissive licence (MIT, BSD, Apache-2.0,
SIL OFL, or MPL-2.0 with an Apache-2.0 alternative). There is no GPL or AGPL
code in Aetox.

The list is what is actually linked into the shipped binary and bundled into
the shipped frontend — not everything `go.mod` or `package.json` mentions.
Regenerate it after a dependency change; the versions below are those of
v1.3.0.

---

## Go modules

Full licence texts are in each module's own repository, and in the local Go
module cache under `$(go env GOMODCACHE)`.

| Module | Version | Licence |
|:---|:---|:---|
| github.com/UserExistsError/conpty | v0.1.4 | MIT |
| github.com/dlclark/regexp2/v2 | v2.5.2 | MIT |
| github.com/dop251/goja | v0.0.0-20260806115107 | MIT |
| github.com/dustin/go-humanize | v1.0.1 | MIT |
| github.com/go-sourcemap/sourcemap | v2.1.3+incompatible | BSD-2-Clause |
| github.com/google/jsonschema-go | v0.4.3 | MIT |
| github.com/google/pprof | v0.0.0-20250317173921 | Apache-2.0 |
| github.com/leaanthony/go-ansi-parser | v1.6.1 | MIT |
| github.com/leaanthony/slicer | v1.6.0 | MIT |
| github.com/leaanthony/u | v1.1.1 | MIT |
| github.com/mattn/go-isatty | v0.0.20 | MIT |
| github.com/modelcontextprotocol/go-sdk | v1.6.1 | Apache-2.0 |
| github.com/ncruces/go-strftime | v1.0.0 | MIT |
| github.com/pkg/errors | v0.9.1 | BSD-2-Clause |
| github.com/remyoudompheng/bigfft | v0.0.0-20230129092748 | BSD-3-Clause |
| github.com/rivo/uniseg | v0.4.7 | MIT |
| github.com/segmentio/asm | v1.1.3 | MIT |
| github.com/segmentio/encoding | v0.5.4 | MIT |
| github.com/skip2/go-qrcode | v0.0.0-20200617195104 | MIT |
| github.com/wailsapp/go-webview2 | v1.0.22 | MIT |
| github.com/wailsapp/wails/v2 | v2.13.0 | MIT |
| github.com/yosida95/uritemplate/v3 | v3.0.2 | BSD-3-Clause |
| golang.org/x/net | v0.54.0 | BSD-3-Clause |
| golang.org/x/oauth2 | v0.35.0 | BSD-3-Clause |
| golang.org/x/sys | v0.46.0 | BSD-3-Clause |
| golang.org/x/term | v0.43.0 | BSD-3-Clause |
| golang.org/x/text | v0.37.0 | BSD-3-Clause |
| modernc.org/libc | v1.74.1 | BSD-2-Clause |
| modernc.org/mathutil | v1.7.1 | BSD-2-Clause |
| modernc.org/memory | v1.11.0 | BSD-2-Clause |
| modernc.org/sqlite | v1.54.0 | BSD-3-Clause |

### Vendored copies

Two of the above are vendored into this repository under `replace` directives
in [go.mod](go.mod), because Aetox carries patches against them. Their
unmodified licence files ship with the source:

| Path | Upstream | Licence file | Patches |
|:---|:---|:---|:---|
| `third_party/conpty/` | github.com/UserExistsError/conpty | [LICENSE](third_party/conpty/LICENSE) (MIT, © 2020 UserExistsError) | — |
| `third_party/go-webview2/` | github.com/wailsapp/go-webview2 | [LICENSE](third_party/go-webview2/LICENSE) (MIT, © 2020 John Chadwick) | [AETOX-PATCH.md](third_party/go-webview2/AETOX-PATCH.md) |

The MIT licence permits these modifications and requires the copyright notice
and permission notice to be kept, which they are. The patches are documented
rather than silent so that the difference from upstream can be read.

---

## Frontend packages

Bundled into the application by Vite and shipped inside the binary.

| Package | Version | Licence |
|:---|:---|:---|
| @xterm/xterm | 6.0.0 | MIT |
| @xterm/addon-fit | 0.11.0 | MIT |
| monaco-editor | 0.56.0 | MIT |
| marked | 14.0.0 | MIT |
| katex | 0.18.4 | MIT |
| highlight.js | 11.11.1 | BSD-3-Clause |
| dompurify | 3.4.8 | MPL-2.0 OR Apache-2.0 |
| svelte (runtime) | 5.x | MIT |

`dompurify` is dual-licensed; Aetox includes it under the **Apache-2.0**
alternative, which carries no file-level copyleft obligation.

### Fonts

| Family | Package | Licence |
|:---|:---|:---|
| Inter | @fontsource-variable/inter 5.3.0 | SIL Open Font License 1.1 |
| Noto Sans Thai | @fontsource-variable/noto-sans-thai 5.3.0 | SIL Open Font License 1.1 |
| Noto Sans SC | @fontsource-variable/noto-sans-sc 5.3.0 | SIL Open Font License 1.1 |

The OFL permits bundling these fonts in software of any licence, including
proprietary software. It requires that the fonts themselves stay under the
OFL and not be sold on their own, neither of which Aetox does.

---

## External programs Aetox can use but does not ship

These are separate programs on your machine. Aetox runs them if they are
present and tells you how to install them if they are not. They are not
distributed with Aetox and are not covered by anything here — they carry
their own licences from their own authors.

| Program | Used by | Licence |
|:---|:---|:---|
| Tesseract OCR | `image_ocr`, `video_ocr` | Apache-2.0 |
| poppler (`pdftotext`, `pdftoppm`) | `pdf_read` | GPL-2.0-or-later |
| ffmpeg | `video_ocr`, `audio_transcribe` | LGPL-2.1-or-later / GPL-2.0-or-later |
| whisper.cpp | `audio_transcribe` | MIT |
| git | `git` | GPL-2.0 |

Calling a separate program is not linking it, so their terms — including the
copyleft ones — do not reach Aetox's own code. The Windows installer offers
to fetch some of them from their official sources; each arrives under its own
licence, and nothing is downloaded without asking.
