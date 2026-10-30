@echo off
rem Starts a local server and several Python chess clients, then stops the server when every client has closed.
rem Usage: scripts\chess.bat [count]
rem   scripts\chess.bat       2 clients
rem   scripts\chess.bat 4     4 clients (two matches)
rem The clients connect to 127.0.0.1:9000, so stop any other server on that port first (e.g. scripts\dev.sh down).
rem Server output goes to log files in %TEMP%; client output is printed here, prefixed with the client number.
setlocal EnableExtensions

rem Child mode: this script re-invokes itself once per client to prefix that client's output.
if /i "%~1"=="--client" goto client

cd /d "%~dp0.." || exit /b 1

set "count=2"
if not "%~1"=="" set "count=%~1"
echo %count%|findstr /r "^[0-9][0-9]*$" >nul || goto usage
if %count% lss 1 goto usage

set "addr_host=127.0.0.1"
set "addr_port=9000"
set "addr=%addr_host%:%addr_port%"
set "client_dir=clients\chess\interpreted_client"

call :is_listening
if %errorlevel%==0 (
    echo something is already listening on %addr%; stop it first ^(scripts\dev.sh down, or taskkill /F /IM ramserver-chess.exe^) 1>&2
    exit /b 1
)

if not exist "%client_dir%\gen\" (
    call buf generate
    if errorlevel 1 exit /b 1
)

rem Use the client's virtualenv when present (absolute path, so it still works after pushd).
set "python=python"
if exist "%client_dir%\.venv\Scripts\python.exe" set "python=%CD%\%client_dir%\.venv\Scripts\python.exe"

set "server_bin=%TEMP%\ramserver-chess.exe"
set "server_log=%TEMP%\ramserver-chess.log"
set "server_err=%TEMP%\ramserver-chess.err.log"

pushd server || exit /b 1
go build -o "%server_bin%" .\cmd\ramserver
set "build_rc=%errorlevel%"
popd
if not "%build_rc%"=="0" exit /b %build_rc%

rem One marker file per client; the wait loop below ends when every client has written its marker.
set "donedir=%TEMP%\ramserver-chess-done-%RANDOM%"
mkdir "%donedir%" || exit /b 1

rem Start the server hidden in the background. Start-Process cannot send stdout and stderr to the same file,
rem so the server's stderr (where Go's log package writes) goes to the .err.log file.
powershell -NoProfile -Command "$env:RAMSERVER_ADDR='%addr%'; Start-Process -FilePath '%server_bin%' -RedirectStandardOutput '%server_log%' -RedirectStandardError '%server_err%' -WindowStyle Hidden"
if errorlevel 1 (
    echo failed to start the server 1>&2
    call :cleanup
    exit /b 1
)

rem Wait up to 5s for the server to accept connections.
powershell -NoProfile -Command "$d=(Get-Date).AddSeconds(5); while ((Get-Date) -lt $d) { try { (New-Object Net.Sockets.TcpClient).Connect('%addr_host%',%addr_port%); exit 0 } catch { Start-Sleep -Milliseconds 100 } }; exit 1"
if errorlevel 1 (
    tasklist /FI "IMAGENAME eq ramserver-chess.exe" 2>nul | find /i "ramserver-chess.exe" >nul
    if errorlevel 1 (
        echo server exited; see %server_log% and %server_err% 1>&2
        call :cleanup
        exit /b 1
    )
)
echo server running on %addr% (logs: %server_log%, %server_err%)

for /l %%i in (1,1,%count%) do (
    start "" /b cmd /c ""%~f0" --client %%i"
    rem let each client join before the next, so they pair up in order
    timeout /t 1 /nobreak >nul
)

echo started %count% client(s); close their windows to stop
echo (Ctrl+C leaves the server running; stop it with: taskkill /F /IM ramserver-chess.exe)

:wait_clients
set "done=0"
for %%F in ("%donedir%\done-*") do set /a done+=1
if %done% geq %count% goto finish
timeout /t 1 /nobreak >nul
goto wait_clients

:finish
call :cleanup
exit /b 0

:cleanup
taskkill /F /T /IM ramserver-chess.exe >nul 2>&1
if defined donedir if exist "%donedir%" rmdir /s /q "%donedir%"
exit /b 0

:is_listening
powershell -NoProfile -Command "try { (New-Object Net.Sockets.TcpClient).Connect('%addr_host%',%addr_port%); exit 0 } catch { exit 1 }"
exit /b %errorlevel%

:usage
echo usage: %~nx0 [count] 1>&2
exit /b 1

:client
rem Runs one client, prefixing each output line with the client number, then writes its marker file.
set "n=%~2"
pushd "%client_dir%" || exit /b 1
for /f "usebackq delims=" %%L in (`"%python%" -u main.py 2^>^&1`) do echo [client %n%] %%L
popd
type nul > "%donedir%\done-%n%"
exit /b 0