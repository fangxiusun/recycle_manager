@echo off
rem rbm build script: produces two exes from the same source (see README section 2 and
rem docs/gui section 10 for the measured details).
rem   rbm.exe      console subsystem (PE subsystem=3): terminal keeps waiting for subcommands.
rem   rbm-gui.exe  GUI subsystem (PE subsystem=2): no console is ever created, so double-click
rem                never flashes a black window.
setlocal
cd /d "%~dp0"

set CGO_ENABLED=0
set GOTOOLCHAIN=local

rem rsrc.syso is required: go build succeeds without it, but the GUI then fails on TTM_ADDTOOL.
windres --input=rbm.rc --input-format=rc --output=rsrc.syso --output-format=coff
if errorlevel 1 goto fail

go build -o rbm.exe .
if errorlevel 1 goto fail

go build -ldflags "-H windowsgui" -o rbm-gui.exe .
if errorlevel 1 goto fail

for %%F in (rbm.exe rbm-gui.exe) do echo built %%~zF bytes  %%F
goto eof

:fail
echo build failed: windres not in PATH, or go build reported an error above.
exit /b 1

:eof
