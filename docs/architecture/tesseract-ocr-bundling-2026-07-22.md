# Tesseract OCR Bundling — How It's Installed, and What Other OSes Would Need

> **Date:** 2026-07-22
> **Status:** Windows has a real install-time mechanism ([project.nsi](../../desktop/build/windows/installer/project.nsi)). macOS/Linux got a deliberately lightweight runtime fallback in `internal/skill/image_ocr.go` instead — see §3. Neither mac nor Linux has an actual Aetox packaging pipeline yet (Aetox only really targets Windows today), so "bundling at install time" isn't available there to begin with.
> **Scope:** how the `image_ocr` skill's runtime dependency (the Tesseract OCR engine) gets onto the user's machine, so it doesn't require a manual install step for most users.

## 1. Why this exists

`internal/skill/image_ocr.go` shells out to a `tesseract` binary on `PATH` — it does not embed an OCR engine (see that file's header comment for why: the only real Go options are CGo-bound to a system Tesseract install anyway, or an abandoned pure-Go WASM port `github.com/danlock/gogosseract`, broken by a wazero 1.8.0 API change with no fix planned). So Tesseract has to actually be present on the machine for `image_ocr` to work.

Requiring the user to separately hunt down and install Tesseract before the "attach an image" chat feature works at all is a bad first-run experience. This doc covers how installation is automated per platform.

## 2. Windows (implemented)

### Mechanism: install-time download, not vendored in git

The NSIS installer (`project.nsi`, macro `wails.tesseractocr`, inserted into the install `Section` right after Wails' own `wails.webview2runtime`) downloads and silently runs the official Tesseract Windows installer **during Aetox's own install**, the same pattern Wails already uses to install the WebView2 runtime. The binary is **not** committed to this repo — it's a ~48MB third-party installer; committing it would bloat git history permanently. The trade-off accepted here: installing Aetox now requires internet access (previously true anyway, since WebView2's bootstrapper already needs it).

```
project.nsi
  !define TESSERACT_URL      "https://github.com/UB-Mannheim/tesseract/releases/download/v5.4.0.20240606/tesseract-ocr-w64-setup-5.4.0.20240606.exe"
  !define TESSERACT_SHA256   "C885FFF6998E0608BA4BB8AB51436E1C6775C2BAFC2559A19B423E18678B60C9"
  !define TESSDATA_THA_URL     ".../tesseract-ocr/tessdata/main/tha.traineddata"
  !define TESSDATA_THA_SHA256  "88032A9F21ACCFF825EFAED29604EB8A534E265CF8058A95EA5417A6DF91C005"
```

Steps, in `!macro wails.tesseractocr`:
1. Skip entirely if `$PROGRAMFILES64\Tesseract-OCR\tesseract.exe` already exists (don't reinstall on every Aetox update).
2. `curl.exe` (ships in Windows 10 1803+ / all of 11 — no NSIS plugin dependency, unlike most HTTPS-download recipes for NSIS) downloads the pinned installer to `$PLUGINSDIR`.
3. **Verify SHA256** via `powershell -Command "[Console]::Write((Get-FileHash ...).Hash)"` before executing anything — this fetches and *runs* a third-party installer during our own install, so integrity-checking it isn't optional. `[Console]::Write` (not `Write-Output`) avoids a trailing newline that would break the exact-string `${If}` comparison.
4. Run it silently: `"$PLUGINSDIR\tesseract-setup.exe" /S`.
5. Separately download `tha.traineddata` (checksummed the same way) into the installed `tessdata\` folder — the base installer only bundles English, and its GUI language-picker isn't scriptable in silent mode. Dropping a `.traineddata` file straight into `tessdata\` is Tesseract's own documented way to add a language without reinstalling.
6. Every failure path (`curl` non-zero exit, hash mismatch) **skips, not aborts** — a broken network or a bad download must never block installing Aetox itself. `image_ocr`'s own error message ("ไม่พบโปรแกรม Tesseract...") is the fallback if this step didn't leave a working Tesseract behind.

### Where the pinned values came from

`TESSERACT_URL` is the `browser_download_url` from `https://api.github.com/repos/UB-Mannheim/tesseract/releases/latest` at the time this was written. The SHA256 values were computed by downloading both files and running `sha256sum` — there was no published digest to cross-check against (GitHub's release API `digest` field was `null` for this asset). **To bump the pinned Tesseract version:** download the new installer, `sha256sum` it, update both `!define`s. Don't skip the hash update — a stale hash means the install-time check always fails closed (skips install) rather than silently accepting an unverified binary, so this fails safe, but it does mean OCR silently stops auto-installing until fixed.

### Untested caveat

This was written and reviewed without a working NSIS compiler available in the dev environment (`makensis` isn't installed) — it was checked by hand against the existing, working `wails.webview2runtime` macro's syntax and nsExec/LogicLib conventions, but **has not been build-and-run tested end to end**. Before relying on it: `wails build --nsis`, install on a clean machine (or VM) without Tesseract, confirm `tesseract --version` works afterward.

## 3. macOS / Linux — lightweight runtime fallback, not real packaging

Aetox has no macOS or Linux packaging pipeline at all today (it's a Windows-targeted app — `desktop/browser.go` is raw Win32). So there's no install-time hook to mirror `project.nsi` with, the way there would be if a `.pkg`/`.deb` pipeline already existed. Given that, and given the explicit ask to keep this part lightweight rather than build out real multi-OS packaging, the mac/Linux handling lives **entirely inside `internal/skill/image_ocr.go`**, triggered lazily the first time OCR actually runs into a missing binary — not at any install step, because there is no install step to hook yet.

### macOS: one automatic attempt, no sudo needed

Homebrew doesn't require root, so `tryAutoInstallTesseract` runs `brew install tesseract tesseract-lang` automatically (once, on the first `exec.ErrNotFound`) if `brew` is on `PATH`. This is a genuine "just works" case unique to macOS — nothing equivalent is safe to auto-run on Linux (see below) or Windows (UAC). If Homebrew isn't installed, or the install fails, the error message falls back to telling the user to run that same command themselves.

### Linux: tell, don't auto-run

`apt`/`dnf`/`pacman` all need `sudo`, and silently invoking a privileged install without the user watching is the same problem as bypassing Windows' UAC prompt — not something to script around. `linuxInstallHint` detects which package manager is present (`apt-get`/`dnf`/`pacman`, in that order) and returns the exact one-line command (e.g. `sudo apt-get install -y tesseract-ocr tesseract-ocr-tha`) in the error message for the user to run themselves. No distro-specific installer, no auto-elevation.

### What this deliberately does *not* cover

- No `.pkg`/DMG postinstall step, no `.deb`/`.rpm`/AppImage/Flatpak packaging — those still don't exist for Aetox at all, on any platform. If a real macOS/Linux distribution pipeline gets built later, whoever does it should decide then whether to keep this runtime fallback, replace it with an install-time step (mirroring `project.nsi`), or do both — this doc's original vendoring/Flatpak/AppImage tradeoffs discussion still applies at that point, just deferred rather than answered now.
- No notarized/vendored Tesseract binary for macOS, no static Linux binary for AppImage — both remain real options for later if Homebrew-presence or sudo-prompting turn out to be a bad first-run experience in practice.
