package config

import "sync"

type Manager struct {
	mu  sync.RWMutex
	cfg *Config
}

func NewManager(initial *Config) *Manager {
	return &Manager{
		cfg: initial,
	}
}

func (m *Manager) Set(cfg *Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
}

func (m *Manager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

func (m *Manager) Update(path string, newCfg *Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := SaveConfig(path, newCfg); err != nil {
		return err
	}
	m.cfg = newCfg
	return nil
}
