#!/bin/bash
set -e

REPO="AnerYubo/portknock"
BINARY_NAME="portknock"
CONFIG_DIR="/etc/portknock"
LOG_DIR="/var/log/portknock"
BINARY_PATH="/usr/local/bin/$BINARY_NAME"
SERVICE_FILE="/etc/systemd/system/portknock.service"

echo "🔍 获取最新 Release 版本号..."
LATEST_TAG=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep -Po '"tag_name": "\K.*?(?=")')
echo "最新版本：$LATEST_TAG"

function get_local_version() {
    if [ -x "$BINARY_PATH" ]; then
        # 尝试获取本地版本
        local ver
        ver=$("$BINARY_PATH" --version 2>/dev/null || true)
        echo "$ver"
    else
        echo ""
    fi
}

LOCAL_VERSION=$(get_local_version)
echo "本地版本：${LOCAL_VERSION:-无}"

if [ "$LOCAL_VERSION" = "$LATEST_TAG" ]; then
    echo "🎉 当前已是最新版本，无需更新二进制文件。"
else
    echo "⬇️ 需要更新二进制文件..."
    BINARY_URL="https://github.com/$REPO/releases/download/$LATEST_TAG/$BINARY_NAME.$LATEST_TAG"
    TMP_PATH="/usr/local/bin/${BINARY_NAME}.tmp"

    echo "🔧 创建目录..."
    sudo mkdir -p "$CONFIG_DIR" "$LOG_DIR"

    echo "📥 下载最新版本二进制文件..."
    sudo curl -L --fail "$BINARY_URL" -o "$TMP_PATH"
    sudo chmod +x "$TMP_PATH"
    
    # 如果程序正在运行，先停止服务
    if systemctl is-active --quiet portknock; then
        echo "⏸️  服务正在运行，先停止服务..."
        sudo systemctl stop portknock
    fi

    echo "🔄 替换二进制文件..."
    sudo mv "$TMP_PATH" "$BINARY_PATH"
fi

echo "⚙️ 检查配置文件..."
if [ ! -f "$CONFIG_DIR/config.yaml" ]; then
    echo "📄 未检测到配置文件，正在写入默认配置..."
    sudo tee "$CONFIG_DIR/config.yaml" > /dev/null << 'EOL'
# 示例配置，请根据你的需求修改或添加多个服务。
# 每个服务必须包含以下字段：
# - name: 服务名称（自定义）
# - interface: 绑定网卡名（如 eth0）
# - knock_ports: 敲门端口序列
# - allow_port: 放行的目标端口（如 SSH 22）
# - expire_seconds: 授权持续时间（秒）
# - step_timeout_seconds: 每步敲门最大间隔（秒）
# - whitelist: 白名单列表 [ 如果没有白名单则将值变为 "[]"]
# 注意：127.0.0.1 默认不放行，有需要则需要添加至白名单

services:
  # 示例服务：
  #
  #- name: webadmin
  #  interface: eth0
  #  knock_ports: [1111, 2222, 3333]
  #  allow_port: 80
  #  expire_seconds: 300
  #  step_timeout_seconds: 5
  #  whitelist:
  #    - 127.0.0.1
EOL
else
    echo "⚠️ 检测到已有配置文件：$CONFIG_DIR/config.yaml"
    read -p "是否要覆盖现有配置并使用默认模板？[y/N]: " confirm_config
    if [[ "$confirm_config" =~ ^[Yy]$ ]]; then
        echo "📄 正在覆盖旧配置..."
        sudo tee "$CONFIG_DIR/config.yaml" > /dev/null << 'EOL'
# 示例配置，请根据你的需求修改或添加多个服务。
# 每个服务必须包含以下字段：
# - name: 服务名称（自定义）
# - interface: 绑定网卡名（如 eth0）
# - knock_ports: 敲门端口序列
# - allow_port: 放行的目标端口（如 SSH 22）
# - expire_seconds: 授权持续时间（秒）
# - step_timeout_seconds: 每步敲门最大间隔（秒）
# - whitelist: 白名单列表 [ 如果没有白名单则将值变为 "[]"]
# 注意：127.0.0.1 默认不放行，有需要则需要添加至白名单

services:
  # 示例服务：
  #
  #- name: webadmin
  #  interface: eth0
  #  knock_ports: [1111, 2222, 3333]
  #  allow_port: 80
  #  expire_seconds: 300
  #  step_timeout_seconds: 5
  #  whitelist:
  #    - 127.0.0.1
EOL
    else
        echo "✅ 保留现有配置文件"
    fi
fi

echo "🧾 检查 systemd 服务文件..."
if [ ! -f "$SERVICE_FILE" ]; then
    echo "📄 未检测到服务文件，正在写入默认服务定义..."
    sudo tee "$SERVICE_FILE" > /dev/null << EOL
[Unit]
Description=PortKnock Service
After=network.target

[Service]
ExecStart=$BINARY_PATH
Restart=always
User=root

[Install]
WantedBy=multi-user.target
EOL
else
    echo "⚠️ 检测到已有服务文件：$SERVICE_FILE"
    read -p "是否要覆盖现有服务定义并使用默认模板？[y/N]: " confirm_service
    if [[ "$confirm_service" =~ ^[Yy]$ ]]; then
        echo "📄 正在覆盖服务文件..."
        sudo tee "$SERVICE_FILE" > /dev/null << EOL
[Unit]
Description=PortKnock Service
After=network.target

[Service]
ExecStart=$BINARY_PATH
Restart=always
User=root

[Install]
WantedBy=multi-user.target
EOL
    else
        echo "✅ 保留现有服务定义"
    fi
fi

echo "🔄 启动服务..."
sudo systemctl daemon-reload
sudo systemctl enable portknock
sudo systemctl restart portknock

echo "✅ 安装完成！如有需要，请编辑 $CONFIG_DIR/config.yaml 并重启服务"
