@echo off
title IDMM Uninstaller
echo Cleaning IDMM Native Messaging Registry keys...
reg delete "HKCU\Software\Google\Chrome\NativeMessagingHosts\com.idmm.downloader" /f >nul 2>&1
reg delete "HKCU\Software\Microsoft\Edge\NativeMessagingHosts\com.idmm.downloader" /f >nul 2>&1
echo Done!
pause
