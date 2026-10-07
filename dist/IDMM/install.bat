@echo off
title IDMM Installer
echo ========================================================
echo   IDMM - Next-Gen Download Accelerator Installer
echo ========================================================
echo.
echo [1/2] Registering Browser Native Messaging Host...
"%~dp0idmm-host.exe" -register
echo.
echo [2/2] Creating Desktop Shortcut...
powershell -NoProfile -Command "$s=(New-Object -COM WScript.Shell).CreateShortcut([System.IO.Path]::Combine([Environment]::GetFolderPath('Desktop'), 'IDMM.lnk')); $s.TargetPath='%~dp0idmm.exe'; $s.WorkingDirectory='%~dp0'; $s.Save()"
echo.
echo ========================================================
echo  SUCCESS! IDMM is now ready to use!
echo  Double-click IDMM on your desktop to launch.
echo ========================================================
pause
