@echo off
setlocal

set "APP_DATA_DIR=%APPDATA%\com.tushu.cadpdm"
set "DATA_FILE=%APP_DATA_DIR%\data-document.json"
set "BACKUP_FILE=%APP_DATA_DIR%\data-document.json.bak"
set "LEGACY_DATA_FILE=%APP_DATA_DIR%\data-document.json.v1"

echo.
echo TuShu JSON test data reset tool
echo.
echo The following runtime files will be deleted:
echo   %DATA_FILE%
echo   %BACKUP_FILE%
echo   %LEGACY_DATA_FILE%
echo.
echo data.seed.json and the attachments directory will be kept.
echo.
set /p "CONFIRM=Type Y to continue: "
if /I "%CONFIRM%"=="Y" goto DELETE_FILES

echo Cancelled.
goto END

:DELETE_FILES
set "DELETED=0"

if exist "%DATA_FILE%" del /f /q "%DATA_FILE%" >nul 2>&1
if not exist "%DATA_FILE%" set "DELETED=1"

if exist "%BACKUP_FILE%" del /f /q "%BACKUP_FILE%" >nul 2>&1
if not exist "%BACKUP_FILE%" set "DELETED=1"

if exist "%LEGACY_DATA_FILE%" del /f /q "%LEGACY_DATA_FILE%" >nul 2>&1
if not exist "%LEGACY_DATA_FILE%" set "DELETED=1"

echo.
if "%DELETED%"=="1" echo JSON test data was deleted. The seed data will be restored on next launch.
if "%DELETED%"=="0" echo No JSON test data was found.

:END
echo.
pause
endlocal
