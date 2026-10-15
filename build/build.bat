@echo off
REM Build FreeWheel for Windows (no console window)
REM CI builds and releases use build/ci-build.sh; this is for local builds.
cd /d "%~dp0\.."
set PATH=C:\msys64\mingw64\bin;C:\Program Files\Go\bin;%PATH%
set CGO_ENABLED=1

REM Embed the RELEASE APKs from the pinned velo-platform release
REM (VELO_PLATFORM_VERSION). Never embed a local debug launcher build.
set /p VELO_TAG=<VELO_PLATFORM_VERSION
where gh >nul 2>nul
if %ERRORLEVEL% equ 0 (
    echo Downloading APKs from velo-platform %VELO_TAG%...
    gh release download %VELO_TAG% -R ril3y/velo-platform -p velolauncher.apk -p serialbridge.apk -D assets --clobber
    if errorlevel 1 echo WARNING: download failed -- using existing assets\*.apk
) else (
    echo WARNING: GitHub CLI ^(gh^) not found -- using existing assets\*.apk
    echo   Get them from https://github.com/ril3y/velo-platform/releases/tag/%VELO_TAG%
)
if not exist assets\velolauncher.apk (
    echo ERROR: assets\velolauncher.apk missing
    goto :end
)
if not exist assets\serialbridge.apk (
    echo ERROR: assets\serialbridge.apk missing
    goto :end
)

set VERSION=dev
for /f %%i in ('git describe --tags --always --dirty 2^>nul') do set VERSION=%%i
if "%VERSION:~0,1%"=="v" set VERSION=%VERSION:~1%
set HASH=unknown
for /f %%i in ('git rev-parse --short^=8 HEAD 2^>nul') do set HASH=%%i

go build -ldflags "-H windowsgui -X main.version=%VERSION% -X main.gitHash=%HASH%" -o freewheel.exe ./cmd/freewheel
if %ERRORLEVEL% equ 0 (
    echo Built freewheel.exe %VERSION% successfully
) else (
    echo Build failed!
)
:end
pause
