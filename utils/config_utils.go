// utils/config_utils.go

package utils

import (
    "fmt"
    "io/ioutil"
    "os"
    "path/filepath"
    "portknock/config"
)

const DefaultConfigPath = "/etc/portknock/config.yaml"

// 示例配置模板（带注释提示）
var commentedExampleConfig = `
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
  #- name: SSHGuard
  #  interface: eth0
  #  knock_ports: [1000, 2000, 3000]
  #  allow_port: 22
  #  expire_seconds: 60
  #  step_timeout_seconds: 5
`

// EnsureConfigFileExists 检查配置文件是否存在，若无则创建示例配置
func EnsureConfigFileExists() error {
    dir := filepath.Dir(DefaultConfigPath)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return err
    }

    if _, err := os.Stat(DefaultConfigPath); os.IsNotExist(err) {
        LogWarn("未找到配置文件，正在创建示例配置: %s\n", DefaultConfigPath)

        err := ioutil.WriteFile(DefaultConfigPath, []byte(commentedExampleConfig), 0644)
        if err != nil {
            return err
        }

        LogWarn("示例配置已写入（默认禁用），请编辑 %s 并取消注释服务配置后重新运行程序", DefaultConfigPath)
        return fmt.Errorf("示例配置已生成但被注释，请编辑后再运行")
    }
    return nil
}

// LoadAndValidateConfig 加载并验证配置文件是否有效
func LoadAndValidateConfig() (*config.Config, error) {
    data, err := ioutil.ReadFile(DefaultConfigPath)
    if err != nil {
        return nil, err
    }

    var cfg config.Config
    if err := cfg.ParseYAML(data); err != nil {
        LogError("解析配置文件失败: %v", err)
        return nil, err
    }

    if len(cfg.Services) == 0 {
        LogWarn("配置文件中 services 列表为空，请在 %s 中添加服务配置", DefaultConfigPath)
        return nil, fmt.Errorf("配置文件中 services 列表为空")
    }

    return &cfg, nil
}