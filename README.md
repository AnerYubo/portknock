# 🔐 PortKnock —— 基于 nftables 的端口敲门服务

PortKnock 是一个基于 Linux `nftables` 的轻量级端口敲门服务。它允许你通过指定顺序访问一组“敲门端口”来临时放行某个 IP 访问目标端口（如 SSH），从而增强服务器的安全性。

---

## 📦 特性

- 支持多服务配置（每个服务可绑定不同网卡）
- 使用 `nftables` 实现规则管理，性能高、安全可靠
- 支持自动配置生成
- 日志记录详细行为，便于审计与调试
- 可通过一键脚本安装部署

---

## 🧰 安装方式（一键安装）

你可以通过以下命令快速安装并运行 PortKnock：

```bash
curl -fsSL https://github.com/AnerYubo/portknock//raw//main/install.sh | bash
```

该脚本会自动完成以下操作：

- 下载预编译二进制文件（或从源码构建）
- 创建系统服务文件（systemd）
- 创建默认配置目录 `/etc/portknock/`
- 启动并启用服务：`systemctl enable --now portknock`

> ⚠️ 注意：目前你需要手动编辑 `/etc/portknock/config.yaml` 来配置敲门规则。

---

## 🛠️ 配置说明

PortKnock 使用 YAML 格式的配置文件：`/etc/portknock/config.yaml`。

### 示例配置（已注释，需手动取消注释）：

```yaml
services:
  - name: "webadmin"
    interface: "eth0"
    knock_ports: [1111, 2222, 3333]
    allow_port: 80
    expire_seconds: 300
    step_timeout_seconds: 5
```

- `name`: 服务名称，用于日志标识
- `interface`: 绑定的网卡名（如 eth0）
- `knock_ports`: 敲门端口序列
- `allow_port`: 敲门成功后放行的目标端口
- `expire_seconds`: 授权持续时间（秒）
- `step_timeout_seconds`: 每步敲门最大间隔（秒）

---

## 📦 手动构建与运行

如果你希望从源码构建：

```bash
git clone https://github.com/AnerYubo/portknock.git
cd portknock
go build -o portknock main.go
sudo ./portknock
```

---

## 📁 默认路径说明

| 路径 | 用途 |
|------|------|
| `/etc/portknock/config.yaml` | 主配置文件 |
| `/var/log/portknock/app.log` | 默认日志输出路径 |
| `/usr/local/bin/portknock` | 二进制文件路径 |
| `/etc/systemd/system/portknock.service` | systemd 服务文件 |

---

## 📝 使用方式示例

客户端敲门流程（使用 `nc`）：

```bash
nc -zvw 1 yourserver 1111
nc -zvw 1 yourserver 2222
nc -zvw 1 yourserver 3333
```

如果敲门成功，服务端将临时放行目标端口（如 80），持续时间为配置中的 `expire_seconds`。

---

## 🧪 日志查看

服务运行后可通过如下命令查看日志：

```bash
journalctl -u portknock -f
```

或者查看日志文件：

```bash
cat /var/log/portknock/app.log
```

---

## 📌 已知问题 & 注意事项

- 当前版本只支持 TCP SYN 包识别，SYN 包重传可能导致多次触发敲门流程。
- 不同服务监听相同网卡时，需要避免端口冲突。
- 初始配置文件中服务是被注释的，请务必取消注释后再运行程序。

---

## 🤝 贡献代码

欢迎提交 PR 和 issue：

- 报告 bug
- 提出新特性建议（如支持 UDP 敲门、HTTP API 查询状态等）
- 编写文档和测试用例

---

## 📄 License

MIT License

---

## 💬 联系作者

GitHub: [@AnerYubo](https://github.com/AnerYubo)

---
