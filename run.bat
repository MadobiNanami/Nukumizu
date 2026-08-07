@echo off
setlocal enabledelayedexpansion

:: Get git commit hash (shortened to 7 characters, can also use full)
for /f %%i in ('git rev-parse --short HEAD') do set COMMIT=%%i

:: Get UTC time
for /f %%i in ('powershell -Command "Get-Date -Format 'yyyy-MM-ddTHH:mm:ssZ'"') do set BUILD_DATE=%%i

:: Build -ldflags
set LDFLAGS=-X main.BuildTime=%BUILD_DATE% -X main.CommitHash=%COMMIT%
go run -ldflags "%LDFLAGS%" .