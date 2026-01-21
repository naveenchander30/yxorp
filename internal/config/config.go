package config

import (
	"errors"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	mutex = &sync.RWMutex{}
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
	APIKey       string        `yaml:"api_key"` // API key for admin endpoints
}

type ProxyConfig struct {
	Targets        []string `yaml:"targets"`
	MaxRequestSize int64    `yaml:"max_request_size"` // Maximum request size in bytes (0 = unlimited)
}

type SecurityConfig struct {
	BlockUserAgents     []string        `yaml:"block_user_agents"`
	RateLimit           RateLimitConfig `yaml:"rate_limit"`
	Rules               []SecurityRule  `yaml:"rules"`
	MaxBodySize         int64           `yaml:"max_body_size"`
	MaxDecompressedSize int64           `yaml:"max_decompressed_size"`
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

func (c *Config) Validate() error {
	if c.Server.Port == "" {
		return errors.New("server port is required")
	}
	if len(c.Proxy.Targets) == 0 {
		return errors.New("at least one proxy target is required")
	}
	return nil
}

func LoadConfig(path string) (*Config, error) {
	mutex.RLock()
	defer mutex.RUnlock()
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

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func SaveConfig(path string, cfg *Config) error {
	mutex.Lock()
	defer mutex.Unlock()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := yaml.NewEncoder(f)
	// Set default indentation to 2 spaces
	encoder.SetIndent(2)
	return encoder.Encode(cfg)
}
