#!/usr/bin/env bash
# ==============================================================================
# NetMon ITOps 智能网络监控与运维平台 - Linux 一键安装与 Systemd 服务注册脚本
# ==============================================================================

set -e

INSTALL_DIR="/opt/netmon"
SERVICE_NAME="netmon"

echo "=============================================================================="
echo "  NetMon ITOps 平台 - Linux 自动化安装与 Systemd 守护进程注册"
echo "=============================================================================="

# 1. 检测架构
ARCH=$(uname -m)
BIN_NAME="itops-server-linux-amd64"
if [[ "$ARCH" == "aarch64" || "$ARCH" == "arm64" ]]; then
    BIN_NAME="itops-server-linux-arm64"
fi

if [[ ! -f "$BIN_NAME" ]]; then
    echo "[错误] 未找到适合当前架构 ($ARCH) 的二进制文件: $BIN_NAME"
    exit 1
fi

# 2. 创建安装目录
echo "[1/4] 创建安装目录 $INSTALL_DIR..."
sudo mkdir -p "$INSTALL_DIR"
sudo cp "$BIN_NAME" "$INSTALL_DIR/netmon-server"
sudo chmod +x "$INSTALL_DIR/netmon-server"

# 可选：IDC 上架布线引擎（存在则一并安装；缺失时主平台仍可运行）
if [[ -d "planning" ]]; then
    sudo rm -rf "$INSTALL_DIR/planning"
    sudo cp -r planning "$INSTALL_DIR/planning"
    echo "[提示] 已安装 planning/ 引擎；如使用系统 Python，请先在 $INSTALL_DIR/planning/backend 建 venv。"
fi

# 3. 配置文件准备
if [[ -f ".env" ]]; then
    sudo cp .env "$INSTALL_DIR/.env"
elif [[ -f ".env.example" ]]; then
    sudo cp .env.example "$INSTALL_DIR/.env"
fi

# 4. 创建 systemd 服务
echo "[2/4] 配置 systemd 服务 /etc/systemd/system/${SERVICE_NAME}.service..."
sudo tee /etc/systemd/system/${SERVICE_NAME}.service > /dev/null <<EOF
[Unit]
Description=NetMon ITOps Intelligent Network Management Platform
After=network.target mysql.service influxdb.service
Wants=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/netmon-server
Restart=always
RestartSec=5s
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

# 5. 启动与设置开机自启
echo "[3/4] 重新加载 systemd 并启动服务..."
sudo systemctl daemon-reload
sudo systemctl enable ${SERVICE_NAME}
sudo systemctl restart ${SERVICE_NAME}

echo "[4/4] 验证服务运行状态..."
sleep 2
sudo systemctl status ${SERVICE_NAME} --no-pager

echo ""
echo "=============================================================================="
echo "  ✅ NetMon ITOps 服务已成功安装并启动！"
echo "  🌐 访问地址: http://<你的服务器IP>:8088"
echo "  👤 默认账号: admin   🔑 默认密码: Admin@2026"
echo "  📋 常用运维命令:"
echo "     - 查看状态: sudo systemctl status ${SERVICE_NAME}"
echo "     - 查看日志: sudo journalctl -u ${SERVICE_NAME} -f"
echo "     - 重启服务: sudo systemctl restart ${SERVICE_NAME}"
echo "     - 停止服务: sudo systemctl stop ${SERVICE_NAME}"
echo "=============================================================================="
