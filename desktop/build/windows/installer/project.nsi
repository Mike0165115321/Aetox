Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them with the values from ProjectInfo.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
##
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a ARM64 only installer:
## > makensis -DARG_WAILS_ARM64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
####
## The following information is taken from the ProjectInfo file, but they can be overwritten here.
####
## !define INFO_PROJECTNAME    "MyProject" # Default "{{.Name}}"
## !define INFO_COMPANYNAME    "MyCompany" # Default "{{.Info.CompanyName}}"
## !define INFO_PRODUCTNAME    "MyProduct" # Default "{{.Info.ProductName}}"
## !define INFO_PRODUCTVERSION "1.0.0"     # Default "{{.Info.ProductVersion}}"
## !define INFO_COPYRIGHT      "Copyright" # Default "{{.Info.Copyright}}"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
## !define UNINST_KEY_NAME     "UninstKeyInRegistry"  # Default "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
####
## !define REQUEST_EXECUTION_LEVEL "admin"            # Default "admin"  see also https://nsis.sourceforge.io/Docs/Chapter4.html
####
## Include the wails tools
####
!include "wails_tools.nsh"

####
## Tesseract OCR is a runtime prerequisite for the image_ocr skill (agent
## reads text out of attached images). Installed the same way Wails installs
## the WebView2 runtime above: download the official installer at install
## time (kept out of git — it's a ~48MB binary) and run it silently, rather
## than vendoring the binary in this repo. Pinned to one version + a SHA256
## check against tampering/corruption, since this fetches and executes a
## third-party installer during our own install.
####
!define TESSERACT_URL      "https://github.com/UB-Mannheim/tesseract/releases/download/v5.4.0.20240606/tesseract-ocr-w64-setup-5.4.0.20240606.exe"
!define TESSERACT_SHA256   "C885FFF6998E0608BA4BB8AB51436E1C6775C2BAFC2559A19B423E18678B60C9"
!define TESSDATA_THA_URL     "https://raw.githubusercontent.com/tesseract-ocr/tessdata/main/tha.traineddata"
!define TESSDATA_THA_SHA256  "88032A9F21ACCFF825EFAED29604EB8A534E265CF8058A95EA5417A6DF91C005"

!macro wails.tesseractocr
    IfFileExists "$PROGRAMFILES64\Tesseract-OCR\tesseract.exe" 0 tesseract_install
    SetDetailsPrint both
    DetailPrint "Skipping: Tesseract OCR is already installed"
    Goto tesseract_langdata

    tesseract_install:
    SetDetailsPrint both
    DetailPrint "Installing: Tesseract OCR (used by the agent to read text in attached images)"
    SetDetailsPrint listonly

    InitPluginsDir
    ; curl.exe ships in Windows System32 since 10 (1803) / all of 11 — no extra
    ; NSIS plugin needed, unlike most HTTPS-download recipes for NSIS.
    nsExec::ExecToLog 'curl.exe -L --max-time 180 -o "$PLUGINSDIR\tesseract-setup.exe" "${TESSERACT_URL}"'
    Pop $0
    ${If} $0 != 0
        DetailPrint "Tesseract download failed (curl exit $0) — skipping. image_ocr will report how to install it manually if used."
        Goto tesseract_done
    ${EndIf}

    ; [Console]::Write (not Write-Output) so the captured string has no
    ; trailing newline for the exact-string ${If} comparison below to match.
    nsExec::ExecToStack 'powershell -NoProfile -Command "[Console]::Write((Get-FileHash -Algorithm SHA256 $\'$PLUGINSDIR\tesseract-setup.exe$\').Hash)"'
    Pop $0
    Pop $1
    ${If} $1 != "${TESSERACT_SHA256}"
        DetailPrint "Tesseract installer checksum did not match the pinned release — skipping for safety."
        Goto tesseract_done
    ${EndIf}

    ExecWait '"$PLUGINSDIR\tesseract-setup.exe" /S'

    ; Thai isn't bundled by default (only English) — the installer's own docs
    ; point at dropping a .traineddata straight into tessdata\ as the silent-
    ; install-friendly way to add a language, so do that instead of trying to
    ; script its GUI component picker.
    ;
    ; Checked on its own file, not folded into the skip above: a machine that
    ; already has Tesseract (from any other app) has the English-only default,
    ; and jumping straight past this left image_ocr silently unable to read Thai.
    tesseract_langdata:
    IfFileExists "$PROGRAMFILES64\Tesseract-OCR\tessdata\tha.traineddata" tesseract_done 0

    SetDetailsPrint both
    DetailPrint "Installing: Thai language data for Tesseract OCR"
    SetDetailsPrint listonly

    InitPluginsDir
    nsExec::ExecToLog 'curl.exe -L --max-time 60 -o "$PLUGINSDIR\tha.traineddata" "${TESSDATA_THA_URL}"'
    Pop $0
    ${If} $0 == 0
        nsExec::ExecToStack 'powershell -NoProfile -Command "[Console]::Write((Get-FileHash -Algorithm SHA256 $\'$PLUGINSDIR\tha.traineddata$\').Hash)"'
        Pop $0
        Pop $1
        ${If} $1 == "${TESSDATA_THA_SHA256}"
            CopyFiles "$PLUGINSDIR\tha.traineddata" "$PROGRAMFILES64\Tesseract-OCR\tessdata\tha.traineddata"
        ${EndIf}
    ${EndIf}

    SetDetailsPrint both
    tesseract_done:
!macroend

####
## poppler's pdftotext backs the pdf_read skill (the agent reading an attached
## PDF — `read` refuses one, a PDF is a binary container). Same pin + SHA256
## rule as Tesseract above, but the delivery differs in two ways that matter:
##
##  - It ships as a plain zip with no installer, so nothing registers it on
##    PATH. It is unpacked INTO the install directory and
##    internal/skill/pdf_read.go looks for it next to Aetox's own exe. Editing
##    the machine's PATH is the other option and a much worse one to get wrong.
##  - There is no system-wide install to detect, so what is checked instead is
##    a marker file named after the pinned version inside our own tree:
##    reinstalling the same version downloads nothing, while bumping the version
##    changes the marker's name and so still forces a full replace. pdftotext.exe
##    is checked alongside it, so a half-deleted tree reinstalls rather than
##    trusting a marker that outlived what it vouched for.
##
## POPPLER_INNER is the archive's own top folder and carries the version, so
## all three defines are bumped together.
####
!define POPPLER_URL     "https://github.com/oschwartz10612/poppler-windows/releases/download/v26.02.0-0/Release-26.02.0-0.zip"
!define POPPLER_SHA256  "993E4A94376ED712FAFC7058D724EA0B943D118BBD2305CD9ED55174EB85CDA5"
!define POPPLER_INNER   "poppler-26.02.0"

!macro wails.poppler
    SetDetailsPrint both
    IfFileExists "$INSTDIR\poppler\bin\pdftotext.exe" 0 poppler_install
    IfFileExists "$INSTDIR\poppler\${POPPLER_INNER}.ok" 0 poppler_install
    DetailPrint "Skipping: poppler ${POPPLER_INNER} is already installed"
    Goto poppler_done

    poppler_install:
    DetailPrint "Installing: poppler (used by the agent to read attached PDFs)"
    SetDetailsPrint listonly

    InitPluginsDir
    nsExec::ExecToLog 'curl.exe -L --max-time 300 -o "$PLUGINSDIR\poppler.zip" "${POPPLER_URL}"'
    Pop $0
    ${If} $0 != 0
        DetailPrint "poppler download failed (curl exit $0) — skipping. pdf_read will report how to install it manually if used."
        Goto poppler_done
    ${EndIf}

    nsExec::ExecToStack 'powershell -NoProfile -Command "[Console]::Write((Get-FileHash -Algorithm SHA256 $\'$PLUGINSDIR\poppler.zip$\').Hash)"'
    Pop $0
    Pop $1
    ${If} $1 != "${POPPLER_SHA256}"
        DetailPrint "poppler archive checksum did not match the pinned release — skipping for safety."
        Goto poppler_done
    ${EndIf}

    ; tar.exe ships in System32 since Windows 10 (1803) and on all of 11 — the
    ; same assumption curl.exe above already makes, so no NSIS unzip plugin is
    ; needed. --strip-components drops the archive's poppler-<version>/Library/
    ; prefix, so what lands is exactly $INSTDIR\poppler\bin\pdftotext.exe: one
    ; fixed layout for the runtime lookup, with no version in the path.
    RMDir /r "$INSTDIR\poppler"
    CreateDirectory "$INSTDIR\poppler"
    nsExec::ExecToLog 'tar.exe -xf "$PLUGINSDIR\poppler.zip" -C "$INSTDIR\poppler" --strip-components=2 "${POPPLER_INNER}/Library"'
    Pop $0

    IfFileExists "$INSTDIR\poppler\bin\pdftotext.exe" 0 poppler_failed
    FileOpen $2 "$INSTDIR\poppler\${POPPLER_INNER}.ok" w
    FileClose $2
    Goto poppler_done

    poppler_failed:
    DetailPrint "poppler did not unpack as expected (tar exit $0) — pdf_read will fall back to any pdftotext already on PATH."

    poppler_done:
    SetDetailsPrint both
!macroend

####
## ffmpeg backs video_ocr (frame sampling) and audio_transcribe (any input →
## the 16kHz mono WAV every speech engine wants). Without it BOTH tools are
## dead on a fresh install, while README advertises them — the gap this closes.
##
## Same delivery as poppler, and for the same reason: a plain zip with no
## installer, nothing on PATH, so it is unpacked into the install directory and
## internal/skill/bundled.go looks for it next to Aetox's own exe.
##
## Pinned to a dated BtbN autobuild rather than their "latest" tag: the assets
## under `latest` are rebuilt in place, so its name is stable while its bytes
## are not — a pinned SHA256 against it would start failing on their next
## build, not on ours. The dated tag is immutable.
##
## LGPL, not GPL: the LGPL build is the safer one to put on someone else's
## machine, and nothing here needs the GPL-only encoders. Shared, not static:
## 59.5MB to download instead of 133MB for the same ffmpeg.
####
!define FFMPEG_URL     "https://github.com/BtbN/FFmpeg-Builds/releases/download/autobuild-2026-07-27-14-00/ffmpeg-n7.1.5-10-g2aefd64d48-win64-lgpl-shared-7.1.zip"
!define FFMPEG_SHA256  "D2A6DF844A674C04780478F33224134A29D1B54152F8D8314B82E02ECCB02EDD"
!define FFMPEG_INNER   "ffmpeg-n7.1.5-10-g2aefd64d48-win64-lgpl-shared-7.1"

!macro wails.ffmpeg
    SetDetailsPrint both
    IfFileExists "$INSTDIR\ffmpeg\bin\ffmpeg.exe" 0 ffmpeg_install
    IfFileExists "$INSTDIR\ffmpeg\${FFMPEG_INNER}.ok" 0 ffmpeg_install
    DetailPrint "Skipping: ffmpeg is already installed at the pinned build"
    Goto ffmpeg_done

    ffmpeg_install:
    DetailPrint "Installing: ffmpeg (used by the agent to read video and audio)"
    SetDetailsPrint listonly

    InitPluginsDir
    nsExec::ExecToLog 'curl.exe -L --max-time 600 -o "$PLUGINSDIR\ffmpeg.zip" "${FFMPEG_URL}"'
    Pop $0
    ${If} $0 != 0
        DetailPrint "ffmpeg download failed (curl exit $0) — skipping. video_ocr and audio_transcribe will report how to install it manually if used."
        Goto ffmpeg_done
    ${EndIf}

    nsExec::ExecToStack 'powershell -NoProfile -Command "[Console]::Write((Get-FileHash -Algorithm SHA256 $\'$PLUGINSDIR\ffmpeg.zip$\').Hash)"'
    Pop $0
    Pop $1
    ${If} $1 != "${FFMPEG_SHA256}"
        DetailPrint "ffmpeg archive checksum did not match the pinned release — skipping for safety."
        Goto ffmpeg_done
    ${EndIf}

    ; Only bin\ is taken — ffmpeg.exe plus the DLLs it loads. The rest of the
    ; archive is headers and import libraries for building against ffmpeg,
    ; which nothing here does. --strip-components=1 (not 2, as for poppler)
    ; because the wanted directory sits one level deeper in that archive.
    RMDir /r "$INSTDIR\ffmpeg"
    CreateDirectory "$INSTDIR\ffmpeg"
    nsExec::ExecToLog 'tar.exe -xf "$PLUGINSDIR\ffmpeg.zip" -C "$INSTDIR\ffmpeg" --strip-components=1 "${FFMPEG_INNER}/bin"'
    Pop $0

    IfFileExists "$INSTDIR\ffmpeg\bin\ffmpeg.exe" 0 ffmpeg_failed
    FileOpen $2 "$INSTDIR\ffmpeg\${FFMPEG_INNER}.ok" w
    FileClose $2
    Goto ffmpeg_done

    ffmpeg_failed:
    DetailPrint "ffmpeg did not unpack as expected (tar exit $0) — the tools will fall back to any ffmpeg already on PATH."

    ffmpeg_done:
    SetDetailsPrint both
!macroend

####
## A starter speech model, so audio_transcribe works the moment Aetox is
## installed instead of failing until the user goes and finds a 141MB file.
##
## tiny, not base: 31MB against 141MB, and it is the difference between a
## default that costs almost nothing and one that doubles the install. It is
## also the least accurate model whisper ships, so internal/skill/
## audio_transcribe.go appends a line to every transcript produced from it
## saying exactly that and where to pick a bigger one. A quiet wrong transcript
## would be the worse trade.
##
## It lands in $INSTDIR\models, NOT %APPDATA%\aetox\models: with a per-machine
## install $APPDATA resolves to the ALL USERS profile, which is not where any
## user's DataRoot points. internal/stt/stores.go scans the install directory
## for exactly this reason.
##
## Pinned to a commit, not to "main": HuggingFace's resolve/main is a moving
## pointer, so a hash against it would start failing on the repo's next push
## rather than on any change of ours.
####
!define WHISPER_MODEL_URL     "https://huggingface.co/ggerganov/whisper.cpp/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/ggml-tiny-q5_1.bin"
!define WHISPER_MODEL_SHA256  "818710568DA3CA15689E31A743197B520007872FF9576237BDA97BD1B469C3D7"
!define WHISPER_MODEL_NAME    "ggml-tiny-q5_1.bin"

!macro wails.whispermodel
    SetDetailsPrint both
    ; The file name carries the model, so its presence is the whole check — no
    ; version marker as for poppler/ffmpeg. Changing the pinned commit does not
    ; change these weights; picking a different model would change the name.
    IfFileExists "$INSTDIR\models\${WHISPER_MODEL_NAME}" 0 whispermodel_install
    DetailPrint "Skipping: the starter speech model is already installed"
    Goto whispermodel_done

    whispermodel_install:
    DetailPrint "Installing: starter speech model (used by the agent to transcribe audio)"
    SetDetailsPrint listonly

    InitPluginsDir
    nsExec::ExecToLog 'curl.exe -L --max-time 300 -o "$PLUGINSDIR\${WHISPER_MODEL_NAME}" "${WHISPER_MODEL_URL}"'
    Pop $0
    ${If} $0 != 0
        DetailPrint "Speech model download failed (curl exit $0) — skipping. audio_transcribe will say how to get one if it is used."
        Goto whispermodel_done
    ${EndIf}

    nsExec::ExecToStack 'powershell -NoProfile -Command "[Console]::Write((Get-FileHash -Algorithm SHA256 $\'$PLUGINSDIR\${WHISPER_MODEL_NAME}$\').Hash)"'
    Pop $0
    Pop $1
    ${If} $1 != "${WHISPER_MODEL_SHA256}"
        DetailPrint "Speech model checksum did not match the pinned release — skipping for safety."
        Goto whispermodel_done
    ${EndIf}

    CreateDirectory "$INSTDIR\models"
    CopyFiles /SILENT "$PLUGINSDIR\${WHISPER_MODEL_NAME}" "$INSTDIR\models\${WHISPER_MODEL_NAME}"

    IfFileExists "$INSTDIR\models\${WHISPER_MODEL_NAME}" whispermodel_done 0
    DetailPrint "Speech model could not be copied into the install folder — audio_transcribe will fall back to any model already on this machine."

    whispermodel_done:
    SetDetailsPrint both
!macroend

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
# !define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp" #Include this to add a bitmap on the left side of the Welcome Page. Must be a size of 164x314
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_INSTFILES # Uinstalling page

# English first, because the first table inserted is what a system whose
# language matches none of them falls back to. NSIS then picks by the machine's
# UI language on its own — no selection dialog, and nothing to click through.
!insertmacro MUI_LANGUAGE "English"

# Thai, for the app's first audience. Guarded on the language file actually
# being in the NSIS the runner has: the installer is built only by the release
# workflow (.github/workflows/release.yml), so a missing .nlf would not surface
# on any PR — it would surface as a failed release. An installer that is English
# on one machine is a small loss; an installer that does not build is the whole
# release.
!if /FileExists "${NSISDIR}\Contrib\Language files\Thai.nlf"
  !insertmacro MUI_LANGUAGE "Thai"
!else
  !warning "Thai.nlf not found in this NSIS install — building an English-only installer."
!endif

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file.
!ifdef WAILS_INSTALL_SCOPE
  !if "${WAILS_INSTALL_SCOPE}" == "user"
    InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
  !else
    InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
  !endif
!else
  InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
!endif # Default installing folder ($PROGRAMFILES is Program Files folder).
ShowInstDetails show # This will always show the installation details.

Function .onInit
   !insertmacro wails.checkArchitecture
FunctionEnd

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime
    !insertmacro wails.tesseractocr

    ; A running instance locks its own exe, so a reinstall/update over top of
    ; it fails with "Can't write: ...\aetox-desktop.exe" and the installer
    ; just aborts, blaming the user for not closing the app first. taskkill
    ; it ourselves instead — /F is synchronous (returns only once the process
    ; is actually gone), so the file handle is released by the time this
    ; macro returns.
    DetailPrint "Closing any running Aetox instance…"
    nsExec::ExecToLog 'taskkill /IM "${PRODUCT_EXECUTABLE}" /F'
    Pop $0

    SetOutPath $INSTDIR

    !insertmacro wails.files

    ; After wails.files, not alongside the Tesseract macro above: this one
    ; unpacks into $INSTDIR, which only exists once the app's own files land.
    !insertmacro wails.poppler
    !insertmacro wails.ffmpeg
    !insertmacro wails.whispermodel

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd
