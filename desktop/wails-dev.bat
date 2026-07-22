@echo off
cd /d "%~dp0"
rem Keeps ALL of Aetox's own data (preferences, sessions, WebView2 profiles,
rem the downloaded rtk binary, ...) off the C: drive during dev — see
rem internal/config.DataRoot. Production builds never set this and use the
rem normal %AppData%\aetox default.
set AETOX_DATA_ROOT=%~dp0.aetox-data
wails dev
pause
