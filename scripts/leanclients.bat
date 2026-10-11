@echo off
rem Usage: scripts\leanclients.bat [count] [leanclient flags...]
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0leanclients.ps1" %*
exit /b %errorlevel%