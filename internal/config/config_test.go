package config

import (
	"testing"
	"time"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
	}{
		{
			name: "valid config",
			config: &Config{
				Server: ServerConfig{
					Port:         "8080",
					ReadTimeout:  30 * time.Second,
					WriteTimeout: 30 * time.Second,
				},
				Proxy: ProxyConfig{
					Targets: []string{"http://backend1.com", "http://backend2.com"},
				},
			},
			expectError: false,
		},
		{
			name: "missing port",
			config: &Config{
				Server: ServerConfig{
					Port: "",
				},
				Proxy: ProxyConfig{
					Targets: []string{"http://backend1.com"},
				},
			},
			expectError: true,
		},
		{
			name: "missing proxy targets",
			config: &Config{
				Server: ServerConfig{
					Port: "8080",
				},
				Proxy: ProxyConfig{
					Targets: []string{},
				},
			},
			expectError: true,
		},
		{
			name: "nil proxy targets",
			config: &Config{
				Server: ServerConfig{
					Port: "8080",
				},
				Proxy: ProxyConfig{
					Targets: nil,
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestNewManager(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Port: "8080",
		},
	}

	mgr := NewManager(cfg)
	if mgr == nil {
		t.Fatal("expected manager but got nil")
	}

	retrieved := mgr.Get()
	if retrieved.Server.Port != "8080" {
		t.Errorf("expected port 8080, got %s", retrieved.Server.Port)
	}
}

func TestManagerSetAndGet(t *testing.T) {
	cfg1 := &Config{
		Server: ServerConfig{
			Port: "8080",
		},
	}

	mgr := NewManager(cfg1)

	cfg2 := &Config{
		Server: ServerConfig{
			Port: "9090",
		},
	}

	mgr.Set(cfg2)

	retrieved := mgr.Get()
	if retrieved.Server.Port != "9090" {
		t.Errorf("expected port 9090, got %s", retrieved.Server.Port)
	}
}

func TestManagerConcurrency(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Port: "8080",
		},
	}

	mgr := NewManager(cfg)

	// Test concurrent reads and writes
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			newCfg := &Config{
				Server: ServerConfig{
					Port: "8080",
				},
			}
			mgr.Set(newCfg)
		}
		done <- true
	}()

	// Reader goroutines
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = mgr.Get()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 11; i++ {
		<-done
	}
}
