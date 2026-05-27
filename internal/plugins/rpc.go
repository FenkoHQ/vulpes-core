package plugins

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/rpc"
	"sync"
	"time"

	"github.com/FenkoHQ/vulpes-core/internal/capabilities"
)

// net/rpc uses gob, which requires the concrete type of every value held in an
// interface field to be registered. ChatMessage.Content and ChatCompletionRequest's
// Tools/ToolChoice/Metadata are typed as `any` and frequently hold values
// decoded from JSON (map[string]any, []any, etc.). Without these registrations
// gob.Encode panics, which shuts down the rpc.Client and surfaces as
// "connection is shut down" on every subsequent call against that plugin.
// The plugin SDK already registers these on the server side; this is the
// gateway-side counterpart.
func init() {
	gob.Register(map[string]any{})
	gob.Register([]any{})
	gob.Register("")
	gob.Register(float64(0))
	gob.Register(int64(0))
	gob.Register(bool(false))
}

type HandshakeRequest struct {
	GatewayVersion            string
	SupportedProtocolVersions []int
}
type HandshakeResponse struct {
	SelectedProtocolVersion int
	PluginName              string
	PluginVersion           string
	Diagnostics             []capabilities.Diagnostic
}
type ConfigureRequest struct {
	Context         capabilities.CallContext
	ConfigJSON      string
	ResolvedSecrets map[string]string
}
type ConfigureResponse struct{ Diagnostics []capabilities.Diagnostic }
type HealthRequest struct{}
type HealthResponse struct {
	State       string
	Diagnostics []capabilities.Diagnostic
}
type GetConfigSchemaRequest struct{}
type GetConfigSchemaResponse struct{ SchemaJSON string }
type GetMetadataRequest struct{}
type ShutdownRequest struct{}
type ShutdownResponse struct{}

type RPCPluginClient struct {
	instance string
	network  string
	address  string

	mu     sync.Mutex
	client *rpc.Client

	metadata Metadata
}

func DialRPCPlugin(ctx context.Context, network, address, instance string) (*RPCPluginClient, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	return &RPCPluginClient{instance: instance, network: network, address: address, client: rpc.NewClient(conn)}, nil
}

func (c *RPCPluginClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		return nil
	}
	return c.client.Close()
}

func (c *RPCPluginClient) getClient() *rpc.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client
}

// reconnect redials the plugin socket and atomically swaps in the new client
// if the current one matches `stale`. This prevents a thundering herd of
// reconnects when many concurrent callers all see ErrShutdown at once: only
// the first reconnect wins; the rest pick up the replacement client.
func (c *RPCPluginClient) reconnect(ctx context.Context, stale *rpc.Client) (*rpc.Client, error) {
	c.mu.Lock()
	if c.client != stale {
		client := c.client
		c.mu.Unlock()
		return client, nil
	}
	c.mu.Unlock()
	var d net.Dialer
	conn, err := d.DialContext(ctx, c.network, c.address)
	if err != nil {
		return nil, err
	}
	fresh := rpc.NewClient(conn)
	c.mu.Lock()
	if c.client == stale {
		_ = stale.Close()
		c.client = fresh
		c.mu.Unlock()
		return fresh, nil
	}
	current := c.client
	c.mu.Unlock()
	_ = fresh.Close()
	return current, nil
}

func (c *RPCPluginClient) call(ctx context.Context, method string, req, resp any) error {
	client := c.getClient()
	err := callOn(ctx, client, method, req, resp)
	if !errors.Is(err, rpc.ErrShutdown) {
		return err
	}
	// Connection died (e.g. a previous call's encode panicked, or the plugin
	// closed its side). Plugin processes keep listening across connections,
	// so a fresh dial recovers without a process restart.
	fresh, dialErr := c.reconnect(ctx, client)
	if dialErr != nil {
		return fmt.Errorf("%w (reconnect failed: %v)", err, dialErr)
	}
	return callOn(ctx, fresh, method, req, resp)
}

func callOn(ctx context.Context, client *rpc.Client, method string, req, resp any) error {
	done := make(chan error, 1)
	go func() { done <- client.Call(method, req, resp) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (c *RPCPluginClient) Handshake(ctx context.Context) error {
	var resp HandshakeResponse
	if err := c.call(ctx, "Plugin.Handshake", HandshakeRequest{GatewayVersion: "dev", SupportedProtocolVersions: []int{1}}, &resp); err != nil {
		return err
	}
	if resp.SelectedProtocolVersion != 1 {
		return fmt.Errorf("unsupported plugin protocol version %d", resp.SelectedProtocolVersion)
	}
	return nil
}
func (c *RPCPluginClient) GetMetadata(ctx context.Context) (Metadata, error) {
	var resp Metadata
	err := c.call(ctx, "Plugin.GetMetadata", GetMetadataRequest{}, &resp)
	c.metadata = resp
	return resp, err
}
func (c *RPCPluginClient) GetConfigSchema(ctx context.Context) (string, error) {
	var resp GetConfigSchemaResponse
	err := c.call(ctx, "Plugin.GetConfigSchema", GetConfigSchemaRequest{}, &resp)
	return resp.SchemaJSON, err
}
func (c *RPCPluginClient) Configure(ctx context.Context, config map[string]any, secrets map[string]string) error {
	b, _ := json.Marshal(config)
	var resp ConfigureResponse
	return c.call(ctx, "Plugin.Configure", ConfigureRequest{Context: capabilities.CallContext{PluginInstance: c.instance}, ConfigJSON: string(b), ResolvedSecrets: secrets}, &resp)
}
func (c *RPCPluginClient) Health(ctx context.Context) (HealthResponse, error) {
	var resp HealthResponse
	err := c.call(ctx, "Plugin.Health", HealthRequest{}, &resp)
	return resp, err
}

func (c *RPCPluginClient) Authenticate(ctx context.Context, req capabilities.AuthenticateRequest) (capabilities.AuthenticateResponse, error) {
	var resp capabilities.AuthenticateResponse
	err := c.call(ctx, "Plugin.Authenticate", req, &resp)
	return resp, err
}
func (c *RPCPluginClient) Authorize(ctx context.Context, req capabilities.AuthorizeRequest) (capabilities.AuthorizeResponse, error) {
	var resp capabilities.AuthorizeResponse
	err := c.call(ctx, "Plugin.Authorize", req, &resp)
	return resp, err
}
func (c *RPCPluginClient) Check(ctx context.Context, req capabilities.RateLimitCheckRequest) (capabilities.RateLimitCheckResponse, error) {
	var resp capabilities.RateLimitCheckResponse
	err := c.call(ctx, "Plugin.CheckRateLimit", req, &resp)
	return resp, err
}
func (c *RPCPluginClient) Commit(ctx context.Context, req capabilities.CommitUsageRequest) error {
	var resp struct{}
	return c.call(ctx, "Plugin.CommitUsage", req, &resp)
}
func (c *RPCPluginClient) Route(ctx context.Context, req capabilities.RouteRequest) (capabilities.RouteResponse, error) {
	var resp capabilities.RouteResponse
	err := c.call(ctx, "Plugin.Route", req, &resp)
	return resp, err
}
func (c *RPCPluginClient) Invoke(ctx context.Context, req capabilities.InvokeRequest) (<-chan capabilities.ResponseChunk, error) {
	var resp []capabilities.ResponseChunk
	if err := c.call(ctx, "Plugin.Invoke", req, &resp); err != nil {
		return nil, err
	}
	ch := make(chan capabilities.ResponseChunk)
	go func() {
		defer close(ch)
		for _, chunk := range resp {
			select {
			case ch <- chunk:
				if chunk.Chunk != nil {
					time.Sleep(1 * time.Millisecond)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}
func (c *RPCPluginClient) ListModels(ctx context.Context) ([]capabilities.ModelInfo, error) {
	var resp []capabilities.ModelInfo
	err := c.call(ctx, "Plugin.ListModels", struct{}{}, &resp)
	return resp, err
}
func (c *RPCPluginClient) Lookup(ctx context.Context, req capabilities.CacheLookupRequest) (capabilities.CacheLookupResponse, error) {
	var resp capabilities.CacheLookupResponse
	err := c.call(ctx, "Plugin.CacheLookup", req, &resp)
	return resp, err
}
func (c *RPCPluginClient) Store(ctx context.Context, req capabilities.CacheStoreRequest) error {
	var resp struct{}
	return c.call(ctx, "Plugin.CacheStore", req, &resp)
}
func (c *RPCPluginClient) Emit(ctx context.Context, events []capabilities.GatewayEvent) error {
	var resp struct{}
	return c.call(ctx, "Plugin.Emit", events, &resp)
}
func (c *RPCPluginClient) ResolvePrompt(ctx context.Context, req capabilities.PromptResolveRequest) (capabilities.PromptResolveResponse, error) {
	var resp capabilities.PromptResolveResponse
	err := c.call(ctx, "Plugin.ResolvePrompt", req, &resp)
	return resp, err
}
func (c *RPCPluginClient) Estimate(ctx context.Context, req capabilities.CostEstimateRequest) (capabilities.CostEstimateResponse, error) {
	var resp capabilities.CostEstimateResponse
	err := c.call(ctx, "Plugin.EstimateCost", req, &resp)
	return resp, err
}
