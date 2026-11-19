@echo off
setlocal
TITLE Cazy Lyre - Video Engine Setup
set "ENV=%LOCALAPPDATA%\Cazy Lyre\video_engine"
echo.
echo  Cazy Lyre Visual Piano Engine Setup
py -3.11 -V
if errorlevel 1 (
 echo Python 3.11 is missing.
 pause
 exit /b 1
)
if not exist "%ENV%\Scripts\python.exe" (
 py -3.11 -m venv "%ENV%"
 if errorlevel 1 goto FAIL
)
"%ENV%\Scripts\python.exe" -m pip install --upgrade pip
if errorlevel 1 goto FAIL
"%ENV%\Scripts\python.exe" -m pip install "numpy==1.26.4" "opencv-python-headless==4.10.0.84" "imageio-ffmpeg>=0.5,<1"
if errorlevel 1 goto FAIL
"%ENV%\Scripts\python.exe" -c "import cv2,numpy,imageio_ffmpeg; print('OpenCV',cv2.__version__)"
if errorlevel 1 goto FAIL
echo SUCCESS. Open Cazy Lyre, import MP4, choose VISUAL.
pause
exit /b 0
:FAIL
echo Setup failed. Review messages above.
pause
exit /b 1
