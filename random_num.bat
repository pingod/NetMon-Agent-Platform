@echo off
title 随机数生成器 (1-60)
color 0B

:loop
cls
set /a num=%random% %% 60 + 1
echo.
echo ==============================================
echo           随机数抽取工具 (范围: 1 - 60)
echo ==============================================
echo.
echo                   【 %num% 】
echo.
echo ==============================================
echo.
echo [按回车/任意键再次抽取，直接关闭窗口退出]
pause >nul
goto loop
