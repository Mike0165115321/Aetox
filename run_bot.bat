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

:: 2. Start the Bot
echo [*] Starting AetoxOS Interface (Pipe Mode)...
echo.
python -m aetox.interfaces.discord_bot

echo.
echo ------------------------------------------
echo System Stopped.
pause
