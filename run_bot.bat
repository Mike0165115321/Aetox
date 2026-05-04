@echo off
title AetoxOS - Discord Bot Runner
cls

echo ==========================================
echo       AetoxOS - Discord Bot System
echo ==========================================
echo.

:: 1. Precise Cleanup using PowerShell
echo [*] Checking for ghost instances...
powershell -Command "Get-Process python* -ErrorAction SilentlyContinue | Where-Object { $_.CommandLine -like '*aetox.interfaces.discord_bot*' } | Stop-Process -Force"

:: 1.5 Dynamic Model Pre-loading (Read from config/models.yaml)
for /f "tokens=2 delims=: " %%a in ('findstr "executor:" config\models.yaml') do set MODEL_RAW=%%a
set MODEL_NAME=%MODEL_RAW:"=%

echo [*] Pre-loading model (%MODEL_NAME%) into VRAM...
curl -s -X POST http://localhost:11434/api/generate -d "{\"model\": \"%MODEL_NAME%\", \"keep_alive\": \"1h\"}" > nul

:: 2. Start the Bot
echo [*] Starting AetoxOS Interface (Pipe Mode)...
echo.
python -m aetox.interfaces.discord_bot

echo.
echo ------------------------------------------
echo System Stopped.
pause
