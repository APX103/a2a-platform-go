package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Name       string `yaml:"name"`
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	MySQL      MySQL  `yaml:"mysql"`
	AdminToken string `yaml:"admin_token"`
}

type MySQL struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
	MaxIdle  int    `yaml:"max_idle"`
	MaxOpen  int    `yaml:"max_open"`
}

func MustLoad(path string) *Config {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		panic(err)
	}
	if c.Port == 0 {
		c.Port = 18090
	}
	if c.MySQL.MaxIdle == 0 {
		c.MySQL.MaxIdle = 10
	}
	if c.MySQL.MaxOpen == 0 {
		c.MySQL.MaxOpen = 100
	}
	return &c
}

func (m MySQL) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		m.User, m.Password, m.Host, m.Port, m.Database)
}
