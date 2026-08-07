@echo off
setlocal enabledelayedexpansion

echo Building for Linux (amd64)...

:: Get git commit hash (shortened to 7 characters, can also use full)
for /f %%i in ('git rev-parse --short HEAD') do set COMMIT=%%i

:: Get UTC time
for /f %%i in ('powershell -Command "Get-Date -Format 'yyyy-MM-ddTHH:mm:ssZ'"') do set BUILD_DATE=%%i

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

if %errorlevel% equ 0 (
    echo Build succeeded: %OUTPUT%
) else (
    echo Build failed.
)