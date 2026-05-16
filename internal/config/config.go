package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server           ServerConfig        `yaml:"server" json:"server"`
	Auth             AuthConfig          `yaml:"auth" json:"auth"`
	Secrets          SecretsConfig       `yaml:"secrets" json:"secrets"`
	Plugins          []PluginConfig      `yaml:"plugins" json:"plugins"`
	Pipeline         PipelineConfig      `yaml:"pipeline" json:"pipeline"`
	Models           ModelsConfig        `yaml:"models" json:"models"`
	Fallbacks        FallbackConfig      `yaml:"fallbacks" json:"fallbacks"`
	Observability    ObservabilityConfig `yaml:"observability" json:"observability"`
	Restart          RestartConfig       `yaml:"restart" json:"restart"`
	AllowZeroPlugins bool                `yaml:"allow_zero_plugins" json:"allow_zero_plugins"`
}

type ServerConfig struct {
	Listen            string   `yaml:"listen" json:"listen"`
	RequestTimeout    Duration `yaml:"request_timeout" json:"request_timeout"`
	StreamIdleTimeout Duration `yaml:"stream_idle_timeout" json:"stream_idle_timeout"`
}

type AuthConfig struct {
	Anonymous bool `yaml:"anonymous" json:"anonymous"`
}

type SecretsConfig struct {
	Env EnvSecretsConfig `yaml:"env" json:"env"`
}
type EnvSecretsConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

type PluginConfig struct {
	Name         string         `yaml:"name" json:"name"`
	Source       PluginSource   `yaml:"source" json:"source"`
	Capabilities []string       `yaml:"capabilities" json:"capabilities"`
	FailMode     string         `yaml:"fail_mode" json:"fail_mode"`
	Config       map[string]any `yaml:"config" json:"config"`
	Checksum     string         `yaml:"checksum" json:"checksum"`
	Trust        string         `yaml:"trust" json:"trust"`
	Sandbox      SandboxConfig  `yaml:"sandbox" json:"sandbox"`
}

type PluginSource struct {
	Type       string `yaml:"type" json:"type"`
	Path       string `yaml:"path" json:"path"`
	Repository string `yaml:"repository" json:"repository"`
	Release    string `yaml:"release" json:"release"`
	Asset      string `yaml:"asset" json:"asset"`
	Binary     string `yaml:"binary" json:"binary"`
	Checksum   string `yaml:"checksum" json:"checksum"`
}

type SandboxConfig struct {
	Enabled             bool     `yaml:"enabled" json:"enabled"`
	MemoryMB            int      `yaml:"memory_mb" json:"memory_mb"`
	CPUQuota            float64  `yaml:"cpu_quota" json:"cpu_quota"`
	AllowNetwork        []string `yaml:"allow_network" json:"allow_network"`
	ReadOnlyFilesystem  bool     `yaml:"read_only_filesystem" json:"read_only_filesystem"`
	AllowChildProcesses bool     `yaml:"allow_child_processes" json:"allow_child_processes"`
}

type PipelineConfig struct {
	Authenticator         string   `yaml:"authenticator" json:"authenticator"`
	Authorizer            string   `yaml:"authorizer" json:"authorizer"`
	RateLimiter           string   `yaml:"rate_limiter" json:"rate_limiter"`
	CacheProvider         string   `yaml:"cache_provider" json:"cache_provider"`
	PromptProvider        string   `yaml:"prompt_provider" json:"prompt_provider"`
	Router                string   `yaml:"router" json:"router"`
	CostProvider          string   `yaml:"cost_provider" json:"cost_provider"`
	ModelRegistryProvider string   `yaml:"model_registry_provider" json:"model_registry_provider"`
	UpstreamProviders     []string `yaml:"upstream_providers" json:"upstream_providers"`
	Observers             []string `yaml:"observers" json:"observers"`
	BlockingObservers     []string `yaml:"blocking_observers" json:"blocking_observers"`
}

type ModelsConfig struct {
	Aliases map[string]ModelAlias `yaml:"aliases" json:"aliases"`
}
type ModelAlias struct {
	Candidates []ModelCandidate `yaml:"candidates" json:"candidates"`
}
type ModelCandidate struct {
	Provider   string            `yaml:"provider" json:"provider"`
	Model      string            `yaml:"model" json:"model"`
	Weight     int               `yaml:"weight" json:"weight"`
	Region     string            `yaml:"region" json:"region"`
	Properties map[string]string `yaml:"properties" json:"properties"`
}

type FallbackConfig struct {
	MaxAttempts  int      `yaml:"max_attempts" json:"max_attempts"`
	RetryOn      []string `yaml:"retry_on" json:"retry_on"`
	DoNotRetryOn []string `yaml:"do_not_retry_on" json:"do_not_retry_on"`
}

type ObservabilityConfig struct {
	QueueSize       int      `yaml:"queue_size" json:"queue_size"`
	BatchSize       int      `yaml:"batch_size" json:"batch_size"`
	FlushInterval   Duration `yaml:"flush_interval" json:"flush_interval"`
	OnOverflow      string   `yaml:"on_overflow" json:"on_overflow"`
	CapturePayloads bool     `yaml:"capture_payloads" json:"capture_payloads"`
}

type RestartConfig struct {
	MaxAttempts    int      `yaml:"max_attempts" json:"max_attempts"`
	InitialBackoff Duration `yaml:"initial_backoff" json:"initial_backoff"`
	MaxBackoff     Duration `yaml:"max_backoff" json:"max_backoff"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	ApplyDefaults(&cfg)
	if err := Validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func ApplyDefaults(c *Config) {
	if c.Server.Listen == "" {
		c.Server.Listen = "127.0.0.1:8080"
	}
	if c.Server.RequestTimeout == 0 {
		c.Server.RequestTimeout = Duration(120 * time.Second)
	}
	if c.Server.StreamIdleTimeout == 0 {
		c.Server.StreamIdleTimeout = Duration(60 * time.Second)
	}
	if c.Observability.QueueSize == 0 {
		c.Observability.QueueSize = 10000
	}
	if c.Observability.BatchSize == 0 {
		c.Observability.BatchSize = 100
	}
	if c.Observability.FlushInterval == 0 {
		c.Observability.FlushInterval = Duration(time.Second)
	}
	if c.Observability.OnOverflow == "" {
		c.Observability.OnOverflow = "drop_oldest"
	}
	if c.Fallbacks.MaxAttempts == 0 {
		c.Fallbacks.MaxAttempts = 3
	}
	if len(c.Fallbacks.RetryOn) == 0 {
		c.Fallbacks.RetryOn = []string{"upstream_timeout", "upstream_rate_limited", "upstream_5xx"}
	}
	if c.Restart.MaxAttempts == 0 {
		c.Restart.MaxAttempts = 5
	}
	if c.Restart.InitialBackoff == 0 {
		c.Restart.InitialBackoff = Duration(500 * time.Millisecond)
	}
	if c.Restart.MaxBackoff == 0 {
		c.Restart.MaxBackoff = Duration(30 * time.Second)
	}
	for i := range c.Plugins {
		if c.Plugins[i].FailMode == "" {
			c.Plugins[i].FailMode = "closed"
		}
		if c.Plugins[i].Trust == "" {
			c.Plugins[i].Trust = "community"
		}
	}
}
