@echo off
setlocal enabledelayedexpansion

:: Build from the repository root, however the script was invoked.
cd /d "%~dp0"

:: The Vue console is built first and embedded into the binary (web\dist, see
:: web\embed.go), so the executable serves the whole frontend on its own:
:: neither frontend\ nor web\dist\ is needed where it runs.
echo Building frontend...
cd frontend

:: node_modules is gitignored, so a fresh checkout (CI included) installs from
:: the lockfile; a warm tree only rebuilds.
if not exist "node_modules" (
    call npm ci
    if errorlevel 1 goto :frontend_failed
)

call npm run build
if errorlevel 1 goto :frontend_failed
cd ..

:: go:embed on web\dist fails anyway, but this names the real problem.
if exist "web\dist\index.html" goto :backend
echo Frontend build produced no web\dist\index.html.
goto :fail

:frontend_failed
cd ..
echo Frontend build failed.
goto :fail

:backend
echo Building for Windows (amd64)...

:: Get git commit hash (shortened to 7 characters, can also use full)
for /f %%i in ('git rev-parse --short HEAD') do set COMMIT=%%i

:: Get UTC time
for /f %%i in ('powershell -Command "(Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')"') do set BUILD_DATE=%%i

:: Set environment variables for Windows build
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0

:: Build -ldflags
set LDFLAGS=-X main.BuildTime=%BUILD_DATE% -X main.CommitHash=%COMMIT%

:: Set output file name
set OUTPUT=nukumizu-windows-amd64.exe

echo Commit: %COMMIT%
echo BuildDate: %BUILD_DATE%
echo Output: %OUTPUT%

go build -ldflags "%LDFLAGS%" -o "%OUTPUT%" .
if errorlevel 1 goto :fail

echo Build succeeded: %OUTPUT%
exit /b 0

:fail
echo Build failed.
exit /b 1
