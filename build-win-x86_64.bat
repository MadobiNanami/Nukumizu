@echo off
setlocal enabledelayedexpansion

echo Building for Windows (amd64)...

:: Get git commit hash (shortened to 7 characters, can also use full)
for /f %%i in ('git rev-parse --short HEAD') do set COMMIT=%%i

:: Get UTC time
for /f %%i in ('powershell -Command "Get-Date -Format 'yyyy-MM-ddTHH:mm:ssZ'"') do set BUILD_DATE=%%i

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

if %errorlevel% equ 0 (
    echo Build succeeded: %OUTPUT%
) else (
    echo Build failed.
)