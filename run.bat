@echo off
setlocal enabledelayedexpansion

:: The console is embedded in the binary (web\embed.go), so compiling without
:: web\dist fails. Say that plainly rather than leaving go:embed's error.
if not exist "web\dist\index.html" (
    echo The web console is not built: web\dist is missing.
    echo Run build-win-x86_64.bat once, or "npm run build" in frontend\.
    exit /b 1
)

:: Get git commit hash (shortened to 7 characters, can also use full)
for /f %%i in ('git rev-parse --short HEAD') do set COMMIT=%%i

:: Get UTC time
for /f %%i in ('powershell -Command "Get-Date -Format 'yyyy-MM-ddTHH:mm:ssZ'"') do set BUILD_DATE=%%i

:: Build -ldflags
set LDFLAGS=-X main.BuildTime=%BUILD_DATE% -X main.CommitHash=%COMMIT%
go run -ldflags "%LDFLAGS%" .