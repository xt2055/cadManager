@echo off
rem ============================================================
rem  cadguanliq offline installer bootstrap
rem  1) self-elevate to administrator
rem  2) ensure python (silent install bundled 3.14.7 if missing)
rem  3) run install.py (PG + db + migrations + venv + app)
rem ============================================================
setlocal
cd /d "%~dp0"

rem ---- self-elevate ----
net session >nul 2>&1
if %errorlevel% neq 0 (
  echo Requesting administrator privileges...
  powershell -NoProfile -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
  exit /b
)

rem ---- locate or install python ----
set "PYEXE=%ProgramFiles%\Python314\python.exe"
if exist "%PYEXE%" goto :run

where python >nul 2>nul
if %errorlevel% equ 0 (
  for /f "delims=" %%P in ('python -c "import sys; print(sys.executable)"') do set "PYEXE=%%P"
  goto :run
)

echo [1/2] Python not found - installing bundled python-3.14.7-amd64.exe ...
"%~dp0python-3.14.7-amd64.exe" /quiet InstallAllUsers=1 PrependPath=1 TargetDir="%ProgramFiles%\Python314" Include_pip=1 Include_doc=0 Include_tcltk=0 Include_test=0
if errorlevel 1 (
  echo FAIL: python installer exited with error.
  pause
  exit /b 1
)
if not exist "%PYEXE%" (
  echo FAIL: python was installed but %PYEXE% not found.
  pause
  exit /b 1
)
echo OK: python installed to %PYEXE%

:run
echo [2/2] Running cadguanliq installer with %PYEXE% ...
"%PYEXE%" "%~dp0install.py"
pause
