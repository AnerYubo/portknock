#!/bin/bash
set -e

BINARY_URL="https://github.com/AnerYubo/portknock/releases/download/app/portknock"
CONFIG_DIR="/etc/portknock"
LOG_DIR="/var/log/portknock"
BINARY_PATH="/usr/local/bin/portknock"
SERVICE_FILE="/etc/systemd/system/portknock.service"

echo "🔧 正在创建目录..."
sudo mkdir -p "$CONFIG_DIR" "$LOG_DIR"

echo "📥 正在下载二进制文件..."
sudo curl -sL "$BINARY_URL" -o "$BINARY_PATH"
sudo chmod +x "$BINARY_PATH"

echo "⚙️ 正在写入默认配置..."
if [ ! -f "$CONFIG_DIR/config.yaml" ]; then
    sudo tee "$CONFIG_DIR/config.yaml" > /dev/null << 'EOL'
# 示例配置，请根据你的需求修改或添加多个服务。
# 每个服务必须包含以下字段：
# - name: 服务名称（自定义）
# - interface: 绑定网卡名（如 eth0）
# - knock_ports: 敲门端口序列
# - allow_port: 放行的目标端口（如 SSH 22）
# - expire_seconds: 授权持续时间（秒）
# - step_timeout_seconds: 每步敲门最大间隔（秒）

services:
  # 下面是一个示例服务，你需要删除 '#' 取消注释，并修改参数
  #
  #- name: webadmin
  #  interface: eth0
  #  knock_ports: [1111, 2222, 3333]
  #  allow_port: 80
  #  expire_seconds: 300
  #  step_timeout_seconds: 5
EOL
fi

echo "📋 写入 systemd 服务文件..."
sudo tee "$SERVICE_FILE" > /dev/null << EOL
[Unit]
Description=PortKnock Service
After=network.target

[Service]
ExecStart=/usr/local/bin/portknock
Restart=always
User=root

[Install]
WantedBy=multi-user.target
EOL

echo "🔄 启动服务..."
sudo systemctl daemon-reload
sudo systemctl enable portknock
sudo systemctl start portknock

echo "✅ 安装完成！请编辑 $CONFIG_DIR/config.yaml 后重启服务"
