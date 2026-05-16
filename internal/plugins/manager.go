package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/FenkoHQ/vulpes-core/internal/capabilities"
	"github.com/FenkoHQ/vulpes-core/internal/config"
	"github.com/FenkoHQ/vulpes-core/internal/secrets"
)

type Manager struct {
	cfg            []config.PluginConfig
	registry       *Registry
	secretProvider secrets.Provider
	logger         *slog.Logger
	processes      map[string]*Process
	clients        map[string]*RPCPluginClient
	mu             sync.Mutex
}

func NewManager(cfg []config.PluginConfig, secretProvider secrets.Provider, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{cfg: cfg, registry: NewRegistry(), secretProvider: secretProvider, logger: logger, processes: map[string]*Process{}, clients: map[string]*RPCPluginClient{}}
}

func (m *Manager) Registry() *Registry            { return m.registry }
func (m *Manager) Processes() map[string]*Process { return m.processes }

func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, pc := range m.cfg {
		if pc.Trust == "untrusted" && !pc.Sandbox.Enabled {
			return fmt.Errorf("plugin %s refused: untrusted plugins require sandbox", pc.Name)
		}
		resolved, err := m.resolveSource(ctx, pc)
		if err != nil {
			return err
		}
		proc := NewProcess(pc.Name, resolved.Path, m.logger)
		startCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = proc.Start(startCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("plugin_start_failed %s: %w", pc.Name, err)
		}
		client, err := DialRPCPlugin(ctx, "unix", proc.Socket, pc.Name)
		if err != nil {
			return fmt.Errorf("dial plugin %s: %w", pc.Name, err)
		}
		if err := client.Handshake(ctx); err != nil {
			return fmt.Errorf("plugin_handshake_failed %s: %w", pc.Name, err)
		}
		md, err := client.GetMetadata(ctx)
		if err != nil {
			return fmt.Errorf("plugin metadata %s: %w", pc.Name, err)
		}
		schemaJSON, err := client.GetConfigSchema(ctx)
		if err != nil {
			return fmt.Errorf("plugin config schema %s: %w", pc.Name, err)
		}
		resolvedConfig, resolvedSecrets, err := secrets.ResolvePluginSecrets(pc.Config, md.Permissions.SecretNames, m.secretProvider)
		if err != nil {
			return fmt.Errorf("plugin %s secret scoping: %w", pc.Name, err)
		}
		if err := validateJSONSchema(schemaJSON, resolvedConfig); err != nil {
			return fmt.Errorf("plugin_config_invalid %s: %w", pc.Name, err)
		}
		if err := client.Configure(ctx, resolvedConfig, resolvedSecrets); err != nil {
			return fmt.Errorf("plugin configure %s: %w", pc.Name, err)
		}
		m.registerClient(pc, client, md)
		m.processes[pc.Name] = proc
		m.clients[pc.Name] = client
	}
	return nil
}

func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var errs []error
	for _, c := range m.clients {
		_ = c.Close()
	}
	for _, p := range m.processes {
		if err := p.Stop(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("stopping plugins: %v", errs)
	}
	return nil
}

func (m *Manager) resolveSource(ctx context.Context, pc config.PluginConfig) (ResolvedPlugin, error) {
	checksum := pc.Checksum
	if pc.Source.Checksum != "" {
		checksum = pc.Source.Checksum
	}
	switch pc.Source.Type {
	case "filesystem":
		return ResolveFilesystem(pc.Source.Path, checksum)
	case "github":
		home, err := os.UserHomeDir()
		if err != nil {
			home = os.TempDir()
		}
		return ResolveGitHub(ctx, GitHubSource{Repository: pc.Source.Repository, Release: pc.Source.Release, Asset: pc.Source.Asset, Binary: pc.Source.Binary, Checksum: checksum}, home, nil)
	default:
		return ResolvedPlugin{}, fmt.Errorf("unsupported plugin source %q", pc.Source.Type)
	}
}

func validateJSONSchema(schemaJSON string, cfg map[string]any) error {
	if strings.TrimSpace(schemaJSON) == "" {
		return nil
	}
	var schemaDoc any
	if err := json.Unmarshal([]byte(schemaJSON), &schemaDoc); err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource("schema.json", schemaDoc); err != nil {
		return err
	}
	sch, err := compiler.Compile("schema.json")
	if err != nil {
		return err
	}
	b, _ := json.Marshal(cfg)
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	return sch.Validate(v)
}

func (m *Manager) registerClient(pc config.PluginConfig, c *RPCPluginClient, md Metadata) {
	m.registry.AddMetadata(pc.Name, md)
	for _, cap := range pc.Capabilities {
		ct := capabilities.CapabilityType(cap)
		if !metadataHasCapability(md, ct) {
			continue
		}
		switch ct {
		case capabilities.AuthenticatorCap:
			m.registry.AddAuthenticator(pc.Name, c)
		case capabilities.AuthorizerCap:
			m.registry.AddAuthorizer(pc.Name, c)
		case capabilities.RateLimiterCap:
			m.registry.AddRateLimiter(pc.Name, c)
		case capabilities.RouterCap:
			m.registry.AddRouter(pc.Name, c)
		case capabilities.UpstreamProviderCap:
			m.registry.AddUpstreamProvider(pc.Name, c)
		case capabilities.CacheProviderCap:
			m.registry.AddCacheProvider(pc.Name, c)
		case capabilities.ObserverCap:
			m.registry.AddObserver(pc.Name, c)
		case capabilities.PromptProviderCap:
			m.registry.AddPromptProvider(pc.Name, c)
		case capabilities.CostProviderCap:
			m.registry.AddCostProvider(pc.Name, c)
		case capabilities.ModelRegistryProviderCap:
			m.registry.AddModelRegistryProvider(pc.Name, c)
		}
	}
}

func metadataHasCapability(md Metadata, ct capabilities.CapabilityType) bool {
	return slices.ContainsFunc(md.Capabilities, func(d CapabilityDescriptor) bool { return d.Type == ct })
}
