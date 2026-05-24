package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("expected missing config file to return an error")
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`name: test
mysql:
  host: 127.0.0.1
  user: root
  password: root_secret_2024
  database: a2a_platform
`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Port != 18090 {
		t.Fatalf("Port = %d, want 18090", cfg.Port)
	}
	if len(cfg.CorsOrigins) != 1 || cfg.CorsOrigins[0] != "*" {
		t.Fatalf("CorsOrigins = %#v, want [*]", cfg.CorsOrigins)
	}
	if cfg.RateLimitRPS != 100 {
		t.Fatalf("RateLimitRPS = %d, want 100", cfg.RateLimitRPS)
	}
	if cfg.MySQL.Port != 3306 {
		t.Fatalf("MySQL.Port = %d, want 3306", cfg.MySQL.Port)
	}
}

func TestLoadRequiresMySQL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("name: test\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("expected missing mysql config to fail")
	}
}
