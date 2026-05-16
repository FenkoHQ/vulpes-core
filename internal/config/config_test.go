package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDurationAndZeroPlugins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gateway.yaml")
	if err := os.WriteFile(path, []byte(`
server:
  listen: 127.0.0.1:9999
  request_timeout: 5s
plugins: []
pipeline: {}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.RequestTimeout.Duration() != 5*time.Second {
		t.Fatalf("duration = %s", cfg.Server.RequestTimeout.Duration())
	}
	if len(cfg.Plugins) != 0 {
		t.Fatalf("plugins = %d", len(cfg.Plugins))
	}
}

func TestValidatePluginSource(t *testing.T) {
	cfg := Config{Plugins: []PluginConfig{{Name: "bad", Source: PluginSource{Type: "http"}}}}
	ApplyDefaults(&cfg)
	if err := Validate(&cfg); err == nil {
		t.Fatal("expected error")
	}
}
