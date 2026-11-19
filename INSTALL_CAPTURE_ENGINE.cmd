@echo off
setlocal
TITLE Cazy Lyre - Windsong Lyre Capture Engine
set "ENV=%LOCALAPPDATA%\Cazy Lyre\capture_engine"
echo.
echo  CAZY LYRE v3.7 - INSTALL LIVE AUDIO CAPTURE
 echo  ===========================================
echo.
echo Requires Python 3.11 64-bit.
echo Installs numpy and soundcard in a private environment.
echo No OpenAI API credits are used for instrument capture.
echo.
py -3.11 -V
if errorlevel 1 goto :fail
if not exist "%ENV%\Scripts\python.exe" (
  py -3.11 -m venv "%ENV%"
  if errorlevel 1 goto :fail
)
"%ENV%\Scripts\python.exe" -m pip install --upgrade pip
if errorlevel 1 goto :fail
"%ENV%\Scripts\python.exe" -m pip install "numpy>=1.24,<3" "soundcard>=0.4,<1"
if errorlevel 1 goto :fail
"%ENV%\Scripts\python.exe" -c "import numpy, soundcard; print('Capture engine ready')"
if errorlevel 1 goto :fail
echo.
echo SUCCESS. Return to Cazy Lyre and click START CAPTURE.
pause
exit /b 0
:fail
echo.
echo Capture setup failed. Make sure Python 3.11 is installed.
pause
exit /b 1
