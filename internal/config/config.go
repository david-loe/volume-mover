package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/david-loe/volume-mover/internal/model"
	"gopkg.in/yaml.v3"
)

type AppConfig struct {
	Hosts []model.HostConfig `yaml:"hosts"`
}

func DefaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	return filepath.Join(dir, "volume-mover", "hosts.yaml"), nil
}

func Load(path string) (*AppConfig, error) {
	cfg := &AppConfig{}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	if len(data) == 0 {
		return cfg, nil
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	cfg.SortHosts()
	return cfg, nil
}

func Save(path string, cfg *AppConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	cfg.SortHosts()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func (c *AppConfig) SortHosts() {
	sort.Slice(c.Hosts, func(i, j int) bool {
		return c.Hosts[i].Name < c.Hosts[j].Name
	})
}

func (c *AppConfig) UpsertHost(host model.HostConfig) {
	for i := range c.Hosts {
		if c.Hosts[i].Name == host.Name {
			c.Hosts[i] = host
			return
		}
	}
	c.Hosts = append(c.Hosts, host)
}

func (c *AppConfig) DeleteHost(name string) {
	filtered := c.Hosts[:0]
	for _, host := range c.Hosts {
		if host.Name != name {
			filtered = append(filtered, host)
		}
	}
	c.Hosts = filtered
}

func (c *AppConfig) FindHost(name string) (model.HostConfig, bool) {
	for _, host := range c.Hosts {
		if host.Name == name {
			return host, true
		}
	}
	return model.HostConfig{}, false
}
