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
    IfFileExists "$PROGRAMFILES64\Tesseract-OCR\tesseract.exe" tesseract_done tesseract_install

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

!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

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

    SetOutPath $INSTDIR

    !insertmacro wails.files

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
