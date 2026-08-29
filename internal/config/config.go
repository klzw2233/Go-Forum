package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Listen   string  `toml:"listen"`
	Database string  `toml:"database"`
	Founder  Founder `toml:"founder"`
}

type Founder struct {
	LoginName   string `toml:"login_name"`
	DisplayName string `toml:"display_name"`
	Password    string `toml:"password"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := toml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if c.Listen == "" {
		c.Listen = "127.0.0.1:8080"
	}
	if c.Database == "" {
		c.Database = "forum.db"
	}
	if c.Founder.LoginName == "" || c.Founder.Password == "" || c.Founder.DisplayName == "" {
		return nil, fmt.Errorf("founder.login_name, founder.display_name and founder.password are required")
	}
	return &c, nil
}
