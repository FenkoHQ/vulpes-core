package plugins

import (
	"fmt"
	"sync"

	"github.com/FenkoHQ/vulpes-core/internal/capabilities"
)

type Metadata struct {
	Name         string                 `json:"name"`
	Version      string                 `json:"version"`
	Homepage     string                 `json:"homepage,omitempty"`
	Capabilities []CapabilityDescriptor `json:"capabilities"`
	Permissions  Permissions            `json:"permissions"`
}

type CapabilityDescriptor struct {
	Type    capabilities.CapabilityType `json:"type"`
	Name    string                      `json:"name"`
	Version string                      `json:"version"`
}
type Permissions struct {
	OutboundHosts []string        `json:"outbound_hosts"`
	SecretNames   []string        `json:"secret_names"`
	Data          DataPermissions `json:"data"`
}
type DataPermissions struct {
	ReadPrompt     bool `json:"read_prompt"`
	ReadResponse   bool `json:"read_response"`
	ModifyRequest  bool `json:"modify_request"`
	ModifyResponse bool `json:"modify_response"`
	ReadHeaders    bool `json:"read_headers"`
}

type Registry struct {
	mu            sync.RWMutex
	authn         map[string]capabilities.Authenticator
	authz         map[string]capabilities.Authorizer
	ratelimit     map[string]capabilities.RateLimiter
	router        map[string]capabilities.Router
	upstream      map[string]capabilities.UpstreamProvider
	cache         map[string]capabilities.CacheProvider
	observer      map[string]capabilities.Observer
	prompt        map[string]capabilities.PromptProvider
	cost          map[string]capabilities.CostProvider
	modelregistry map[string]capabilities.ModelRegistryProvider
	metadata      map[string]Metadata
}

func NewRegistry() *Registry {
	return &Registry{authn: map[string]capabilities.Authenticator{}, authz: map[string]capabilities.Authorizer{}, ratelimit: map[string]capabilities.RateLimiter{}, router: map[string]capabilities.Router{}, upstream: map[string]capabilities.UpstreamProvider{}, cache: map[string]capabilities.CacheProvider{}, observer: map[string]capabilities.Observer{}, prompt: map[string]capabilities.PromptProvider{}, cost: map[string]capabilities.CostProvider{}, modelregistry: map[string]capabilities.ModelRegistryProvider{}, metadata: map[string]Metadata{}}
}

func (r *Registry) AddMetadata(instance string, md Metadata) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metadata[instance] = md
}
func (r *Registry) Metadata(instance string) (Metadata, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.metadata[instance]
	return v, ok
}

func (r *Registry) AddAuthenticator(name string, v capabilities.Authenticator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.authn[name] = v
}
func (r *Registry) AddAuthorizer(name string, v capabilities.Authorizer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.authz[name] = v
}
func (r *Registry) AddRateLimiter(name string, v capabilities.RateLimiter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ratelimit[name] = v
}
func (r *Registry) AddRouter(name string, v capabilities.Router) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.router[name] = v
}
func (r *Registry) AddUpstreamProvider(name string, v capabilities.UpstreamProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upstream[name] = v
}
func (r *Registry) AddCacheProvider(name string, v capabilities.CacheProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[name] = v
}
func (r *Registry) AddObserver(name string, v capabilities.Observer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observer[name] = v
}
func (r *Registry) AddPromptProvider(name string, v capabilities.PromptProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prompt[name] = v
}
func (r *Registry) AddCostProvider(name string, v capabilities.CostProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cost[name] = v
}
func (r *Registry) AddModelRegistryProvider(name string, v capabilities.ModelRegistryProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modelregistry[name] = v
}

func (r *Registry) Authenticator(name string) (capabilities.Authenticator, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v := r.authn[name]
	if v == nil {
		return nil, fmt.Errorf("authenticator %q not found", name)
	}
	return v, nil
}
func (r *Registry) Authorizer(name string) (capabilities.Authorizer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v := r.authz[name]
	if v == nil {
		return nil, fmt.Errorf("authorizer %q not found", name)
	}
	return v, nil
}
func (r *Registry) RateLimiter(name string) (capabilities.RateLimiter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v := r.ratelimit[name]
	if v == nil {
		return nil, fmt.Errorf("rate limiter %q not found", name)
	}
	return v, nil
}
func (r *Registry) Router(name string) (capabilities.Router, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v := r.router[name]
	if v == nil {
		return nil, fmt.Errorf("router %q not found", name)
	}
	return v, nil
}
func (r *Registry) UpstreamProvider(name string) (capabilities.UpstreamProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v := r.upstream[name]
	if v == nil {
		return nil, fmt.Errorf("upstream provider %q not found", name)
	}
	return v, nil
}
func (r *Registry) CacheProvider(name string) (capabilities.CacheProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v := r.cache[name]
	if v == nil {
		return nil, fmt.Errorf("cache provider %q not found", name)
	}
	return v, nil
}
func (r *Registry) Observer(name string) (capabilities.Observer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v := r.observer[name]
	if v == nil {
		return nil, fmt.Errorf("observer %q not found", name)
	}
	return v, nil
}
func (r *Registry) PromptProvider(name string) (capabilities.PromptProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v := r.prompt[name]
	if v == nil {
		return nil, fmt.Errorf("prompt provider %q not found", name)
	}
	return v, nil
}
func (r *Registry) CostProvider(name string) (capabilities.CostProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v := r.cost[name]
	if v == nil {
		return nil, fmt.Errorf("cost provider %q not found", name)
	}
	return v, nil
}
func (r *Registry) ModelRegistryProvider(name string) (capabilities.ModelRegistryProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v := r.modelregistry[name]
	if v == nil {
		return nil, fmt.Errorf("model registry provider %q not found", name)
	}
	return v, nil
}

func (r *Registry) Has(name string, cap capabilities.CapabilityType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	switch cap {
	case capabilities.AuthenticatorCap:
		return r.authn[name] != nil
	case capabilities.AuthorizerCap:
		return r.authz[name] != nil
	case capabilities.RateLimiterCap:
		return r.ratelimit[name] != nil
	case capabilities.RouterCap:
		return r.router[name] != nil
	case capabilities.UpstreamProviderCap:
		return r.upstream[name] != nil
	case capabilities.CacheProviderCap:
		return r.cache[name] != nil
	case capabilities.ObserverCap:
		return r.observer[name] != nil
	case capabilities.PromptProviderCap:
		return r.prompt[name] != nil
	case capabilities.CostProviderCap:
		return r.cost[name] != nil
	case capabilities.ModelRegistryProviderCap:
		return r.modelregistry[name] != nil
	default:
		return false
	}
}
