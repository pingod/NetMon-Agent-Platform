# NetMon ITOps 智能网络监控与运维平台 — 生产发布包

本目录包含适用于 **Linux** 与 **Windows** 平台的独立二进制执行文件。
每个二进制文件内部均已**完整内置打包了全套前端静态资源、REST API、SNMP 引擎与 MySQL / SQLite 驱动**，无须额外安装 Node.js、Nginx 或 Python 环境，开箱即用。

---

## 📦 文件清单

| 文件名 | 适用平台 / 架构 | 说明 |
| :--- | :--- | :--- |
| `itops-server-windows-amd64.exe` | **Windows 64位 (x86_64)** | Windows 10/11/Server 2016/2019/2022 |
| `itops-server-windows-arm64.exe` | **Windows ARM64** | Windows on ARM / Surface Pro X 等 |
| `itops-server-linux-amd64` | **Linux 64位 (x86_64)** | Ubuntu / Debian / CentOS / RHEL / Rocky / EulerOS |
| `itops-server-linux-arm64` | **Linux ARM64 (aarch64)** | 鲲鹏 / 飞腾 / 树莓派 / 银河麒麟 / 统信 UOS |
| `start-windows.bat` | Windows | Windows 一键双击启动脚本 |
| `install-service-linux.sh` | Linux | Linux 一键注册 Systemd 守护进程与开机自启脚本 |
| `.env.example` | 通用 | 生产环境配置模板（包含 MySQL 与 InfluxDB 配置项） |

---

## 🚀 快速运行指南

### 一、Windows 平台运行
1. 将 `release/` 目录复制到目标机器。
2. 双击运行 `start-windows.bat`（或在 PowerShell 中执行 `.\itops-server-windows-amd64.exe`）。
3. 打开浏览器访问：`http://localhost:8088`
4. 初始管理员账号：`admin`，初始密码：`Admin@2026`。

---

### 二、Linux 平台运行

#### 方式 1：直接命令行运行
```bash
# 给予可执行权限
chmod +x itops-server-linux-amd64

# (可选) 复制配置文件并修改 MySQL / 端口
cp .env.example .env

# 启动服务
./itops-server-linux-amd64
```

#### 方式 2：注册为 Systemd 系统服务（推荐，自动开机自启）
```bash
# 执行一键安装脚本（自动拷贝到 /opt/netmon 并注册 netmon.service）
chmod +x install-service-linux.sh
sudo ./install-service-linux.sh

# 常用服务管理命令
sudo systemctl status netmon    # 查看状态
sudo journalctl -u netmon -f   # 查看实时日志
sudo systemctl restart netmon   # 重启服务
sudo systemctl stop netmon      # 停止服务
```

---

## ⚙️ 生产环境配置 (`.env`)

在二进制程序同级目录下创建 `.env` 文件即可覆盖默认配置：

```env
# ── 服务端基础设置 ──
PORT=8088
LOG_LEVEL=info
JWT_SECRET=your_super_secure_random_jwt_secret_32_chars_long

# ── MySQL 业务持久化数据库 ──
MYSQL_ENABLED=true
MYSQL_DSN=root:yourpassword@tcp(127.0.0.1:3306)/wangguan?charset=utf8mb4&parseTime=True&loc=Local

# ── InfluxDB 时序指标库 ──
INFLUXDB_ENABLED=true
INFLUXDB_VERSION=v2
INFLUXDB_HOST=127.0.0.1
INFLUXDB_PORT=8086
INFLUXDB_TOKEN=your_influx_token
INFLUXDB_ORG=itops
INFLUXDB_BUCKET=netmon_metrics
```
