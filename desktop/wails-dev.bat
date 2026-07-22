@echo off
cd /d "%~dp0"
rem Keeps WebView2 profile data (cache/cookies/IndexedDB) off the C: drive
rem during dev — see desktop/main.go:webviewUserDataDir. Production builds
rem never set this and keep the normal %AppData% behavior.
set AETOX_WEBVIEW_DATA_DIR=%~dp0.webview2-data
wails dev
pause
