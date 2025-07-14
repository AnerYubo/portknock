package config

import (
	"io/ioutil"
	"time"

	"gopkg.in/yaml.v2"
)

type ServiceConfig struct {
	Name          string  `yaml:"name"`
	KnockPorts    []int   `yaml:"knock_ports"`
	AllowPort     uint16  `yaml:"allow_port"`
	ExpireSeconds int     `yaml:"expire_seconds"`
	Interface     string  `yaml:"interface"` // 👈 添加这一行
	StepTimeoutSeconds int `yaml:"step_timeout_seconds"`
}


type Config struct {
	Services []ServiceConfig `yaml:"services"`
}

// LoadConfig 从指定路径读取并解析配置文件
func LoadConfig(path string) (*Config, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// 兼容处理：如果某个服务未配置过期时间，默认 300 秒
	for i := range cfg.Services {
		if cfg.Services[i].ExpireSeconds <= 0 {
			cfg.Services[i].ExpireSeconds = 300
		}
	}

	return &cfg, nil
}

// ExpireDuration 返回服务的过期时间（time.Duration）
func (s *ServiceConfig) ExpireDuration() time.Duration {
	return time.Duration(s.ExpireSeconds) * time.Second
}
// ParseYAML 将 YAML 数据解析为 Config
func (c *Config) ParseYAML(data []byte) error {
    return yaml.Unmarshal(data, c)
}