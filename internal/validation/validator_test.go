package validation

import (
	"strings"
	"testing"
	"time"

	"github.com/yxorp/internal/config"
)

func TestValidateConfig_Valid(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:         "8080",
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
		Proxy: config.ProxyConfig{
			Targets:        []string{"http://localhost:3001", "https://localhost:3002"},
			MaxRequestSize: 1024 * 1024,
		},
		Security: config.SecurityConfig{
			MaxBodySize:         2 * 1024 * 1024,
			MaxDecompressedSize: 5 * 1024 * 1024,
			BlockUserAgents:     []string{"curl"},
			RateLimit: config.RateLimitConfig{
				Enabled:           true,
				RequestsPerMinute: 100,
			},
			Rules: []config.SecurityRule{
				{Name: "sql", Pattern: "union select", Location: "query_params"},
				{Name: "xss", Pattern: "<script", Location: "body"},
			},
		},
	}

	err := ValidateConfig(cfg)
	if err != nil {
		t.Errorf("Expected configuration to be valid, got errors: %v", err)
	}
}

func TestValidateConfig_Invalid(t *testing.T) {
	tests := []struct {
		name          string
		modify        func(*config.Config)
		expectedError string
	}{
		{
			name: "Missing Port",
			modify: func(c *config.Config) {
				c.Server.Port = ""
			},
			expectedError: "server.port: port is required",
		},
		{
			name: "Port Not a Number",
			modify: func(c *config.Config) {
				c.Server.Port = "abc"
			},
			expectedError: "server.port: port must be a number",
		},
		{
			name: "Port Out of Range",
			modify: func(c *config.Config) {
				c.Server.Port = "70000"
			},
			expectedError: "server.port: port must be between 1 and 65535",
		},
		{
			name: "Negative Read Timeout",
			modify: func(c *config.Config) {
				c.Server.ReadTimeout = -10 * time.Second
			},
			expectedError: "server.read_timeout: read timeout cannot be negative",
		},
		{
			name: "Incomplete TLS Configuration",
			modify: func(c *config.Config) {
				c.Server.CertFile = "cert.pem"
				c.Server.KeyFile = ""
			},
			expectedError: "server.tls: both cert_file and key_file must be provided for TLS",
		},
		{
			name: "Empty Proxy Targets",
			modify: func(c *config.Config) {
				c.Proxy.Targets = []string{}
			},
			expectedError: "proxy.targets: at least one proxy target is required",
		},
		{
			name: "Negative Max Request Size",
			modify: func(c *config.Config) {
				c.Proxy.MaxRequestSize = -100
			},
			expectedError: "proxy.max_request_size: max request size cannot be negative",
		},
		{
			name: "Unreasonably Small Max Request Size",
			modify: func(c *config.Config) {
				c.Proxy.MaxRequestSize = 500
			},
			expectedError: "proxy.max_request_size: max request size should be at least 1024 bytes (1KB)",
		},
		{
			name: "Negative Max Body Size",
			modify: func(c *config.Config) {
				c.Security.MaxBodySize = -1
			},
			expectedError: "security.max_body_size: max body size cannot be negative",
		},
		{
			name: "Negative Max Decompressed Size",
			modify: func(c *config.Config) {
				c.Security.MaxDecompressedSize = -5
			},
			expectedError: "security.max_decompressed_size: max decompressed size cannot be negative",
		},
		{
			name: "Empty Blocked User Agent",
			modify: func(c *config.Config) {
				c.Security.BlockUserAgents = []string{"curl", "  "}
			},
			expectedError: "security.block_user_agents[1]: user agent cannot be empty",
		},
		{
			name: "Rate Limiting with Invalid RPM",
			modify: func(c *config.Config) {
				c.Security.RateLimit.Enabled = true
				c.Security.RateLimit.RequestsPerMinute = 0
			},
			expectedError: "security.rate_limit.requests_per_minute: requests per minute must be greater than 0 when rate limiting is enabled",
		},
		{
			name: "Duplicate Security Rule Names",
			modify: func(c *config.Config) {
				c.Security.Rules = []config.SecurityRule{
					{Name: "rule1", Pattern: "abc", Location: "body"},
					{Name: "rule1", Pattern: "xyz", Location: "body"},
				}
			},
			expectedError: "security.rules[1].name: duplicate rule name: rule1",
		},
		{
			name: "Rule with Invalid Regex",
			modify: func(c *config.Config) {
				c.Security.Rules = []config.SecurityRule{
					{Name: "bad-rule", Pattern: "[a-z", Location: "body"},
				}
			},
			expectedError: "security.rules[0].pattern: invalid regex",
		},
		{
			name: "Rule with Invalid Location",
			modify: func(c *config.Config) {
				c.Security.Rules = []config.SecurityRule{
					{Name: "bad-loc", Pattern: "abc", Location: "invalid_loc"},
				}
			},
			expectedError: "security.rules[0].location: invalid location",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Start with a valid config base
			cfg := &config.Config{
				Server: config.ServerConfig{
					Port: "8080",
				},
				Proxy: config.ProxyConfig{
					Targets: []string{"http://localhost:3001"},
				},
				Security: config.SecurityConfig{
					RateLimit: config.RateLimitConfig{
						Enabled: false,
					},
				},
			}

			tt.modify(cfg)
			err := ValidateConfig(cfg)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}

			if !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("expected error containing %q, got %q", tt.expectedError, err.Error())
			}
		})
	}
}

func TestValidateAndFix(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: "8080",
		},
		Proxy: config.ProxyConfig{
			Targets: []string{"localhost:3001"}, // missing scheme
		},
	}

	fixedCfg, warnings := ValidateAndFix(cfg)
	
	if len(warnings) != 3 { // scheme fixed + read_timeout set + write_timeout set
		t.Errorf("expected 3 warnings, got %d: %v", len(warnings), warnings)
	}

	if fixedCfg.Proxy.Targets[0] != "https://localhost:3001" {
		t.Errorf("expected target scheme to be auto-fixed to https, got %s", fixedCfg.Proxy.Targets[0])
	}

	if fixedCfg.Server.ReadTimeout != 30 {
		t.Errorf("expected default read timeout 30, got %v", fixedCfg.Server.ReadTimeout)
	}
}

func TestQuickValidate(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: "8080",
		},
		Proxy: config.ProxyConfig{
			Targets: []string{"http://localhost"},
		},
	}

	if err := QuickValidate(cfg); err != nil {
		t.Errorf("unexpected quick validation error: %v", err)
	}

	cfg.Server.Port = ""
	if err := QuickValidate(cfg); err == nil {
		t.Error("expected quick validation error for empty port")
	}
}
