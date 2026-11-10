@echo off
chcp 65001 > nul
title NetMon ITOps 监控运维平台
echo ==============================================================================
echo   NetMon ITOps 智能网络监控与运维平台 - Windows 启动脚本
echo ==============================================================================
echo.

if not exist .env (
    if exist .env.example (
        echo [提示] 检测到首次运行，正在自动从 .env.example 生成 .env 配置文件...
        copy .env.example .env > nul
    )
)

echo [INFO] 正在启动 NetMon 服务端...
echo [INFO] 前端页面已内置集成，访问地址: http://localhost:8088
echo [INFO] 默认管理员账号: admin  默认密码: Admin@2026
echo.

itops-server-windows-amd64.exe

pause
