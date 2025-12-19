package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Proxy    ProxyConfig    `yaml:"proxy"`
	Security SecurityConfig `yaml:"security"`
}

type ServerConfig struct {
	Port         string        `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	CertFile     string        `yaml:"cert_file"`
	KeyFile      string        `yaml:"key_file"`
}

type ProxyConfig struct {
	Targets []string `yaml:"targets"`
}

type SecurityConfig struct {
	BlockUserAgents []string        `yaml:"block_user_agents"`
	RateLimit       RateLimitConfig `yaml:"rate_limit"`
	Rules           []SecurityRule  `yaml:"rules"`
}

type RateLimitConfig struct {
	Enabled           bool `yaml:"enabled"`
	RequestsPerMinute int  `yaml:"requests_per_minute"`
}

type SecurityRule struct {
	Name     string `yaml:"name"`
	Pattern  string `yaml:"pattern"`
	Location string `yaml:"location"`
}

func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cfg Config
	decoder := yaml.NewDecoder(f)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
