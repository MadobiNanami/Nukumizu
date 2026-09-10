@echo off
setlocal enabledelayedexpansion

:: Build from the repository root, however the script was invoked.
cd /d "%~dp0"

:: Optional first argument: --frontend also builds the Vue frontend, which web/
:: serves at runtime from frontend/dist. Omitted, only the backend is compiled.
set BUILD_FRONTEND=0
if /i "%~1"=="--frontend" set BUILD_FRONTEND=1

if not "%BUILD_FRONTEND%"=="1" goto :backend

echo Building frontend...
cd frontend
call npm ci
if errorlevel 1 goto :fail
call npm run build
if errorlevel 1 goto :fail
cd ..

:backend
echo Building for Linux (amd64)...

:: Get git commit hash (shortened to 7 characters, can also use full)
for /f %%i in ('git rev-parse --short HEAD') do set COMMIT=%%i

:: Get UTC time
for /f %%i in ('powershell -Command "(Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')"') do set BUILD_DATE=%%i

:: Set environment variables for Linux build
set GOOS=linux
set GOARCH=amd64
set CGO_ENABLED=0

:: Build -ldflags
set LDFLAGS=-X main.BuildTime=%BUILD_DATE% -X main.CommitHash=%COMMIT%

:: Set output file name
set OUTPUT=nukumizu-linux-amd64

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
