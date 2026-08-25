@echo off
setlocal

set "VS_ROOT=D:\exe\vs"
set "CAXA_ROOT=C:\Program Files\CAXA\CAXA CAD\2022"
set "PROJECT_ROOT=%~dp0..\.."
set "SOURCE=%~dp0main.cpp"
set "OUTPUT=%~dp0exb2dxf.exe"

call "%VS_ROOT%\VC\Auxiliary\Build\vcvars64.bat"
if errorlevel 1 exit /b %errorlevel%

cl.exe /nologo /std:c++17 /EHsc /W4 /O2 /DUNICODE /D_UNICODE /Zc:wchar_t /I"%CAXA_ROOT%\CRX\Inc" "%SOURCE%" /link /SUBSYSTEM:CONSOLE /MACHINE:X64 /LIBPATH:"%CAXA_ROOT%\CRX\Lib" CrxDb.lib Crx.lib CrxGe.lib CrxSpt.lib /OUT:"%OUTPUT%"
if errorlevel 1 exit /b %errorlevel%

echo Built "%OUTPUT%"
