@echo off
setlocal

set "VS_ROOT=D:\exe\vs"
set "CAXA_ROOT=C:\Program Files\CAXA\CAXA CAD\2022"
set "SOURCE_DIR=%~dp0"
set "OUTPUT=%~dp0..\exb2dwg.crx"

call "%VS_ROOT%\VC\Auxiliary\Build\vcvars64.bat"
if errorlevel 1 exit /b %errorlevel%

cl.exe /nologo /c /std:c++17 /EHsc /W3 /O2 /DUNICODE /D_UNICODE /D_WIN64 /D_WINDOWS /D_CCRXAPP /Zc:wchar_t /Yc"StdAfx.h" /Fp"%SOURCE_DIR%StdAfx.pch" /I"%CAXA_ROOT%\CRX\Inc" "%SOURCE_DIR%StdAfx.cpp" /Fo"%SOURCE_DIR%StdAfx.obj"
if errorlevel 1 exit /b %errorlevel%

cl.exe /nologo /LD /std:c++17 /EHsc /W3 /O2 /DUNICODE /D_UNICODE /D_WIN64 /D_WINDOWS /D_CCRXAPP /Zc:wchar_t /Yu"StdAfx.h" /Fp"%SOURCE_DIR%StdAfx.pch" /I"%CAXA_ROOT%\CRX\Inc" "%SOURCE_DIR%Exb2Dwg.cpp" "%SOURCE_DIR%CrxEntryPoint.cpp" "%SOURCE_DIR%StdAfx.obj" /link /SUBSYSTEM:WINDOWS /MACHINE:X64 /LIBPATH:"%CAXA_ROOT%\CRX\Lib" Crx.lib CrxDb.lib CrxEdApi.lib CrxGe.lib CrxGi.lib User32.lib /OUT:"%OUTPUT%"
if errorlevel 1 exit /b %errorlevel%

echo Built "%OUTPUT%"
