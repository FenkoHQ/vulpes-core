package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/rpc"
	"time"

	"github.com/FenkoHQ/vulpes-core/internal/capabilities"
)

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
	client   *rpc.Client
	metadata Metadata
}

func DialRPCPlugin(ctx context.Context, network, address, instance string) (*RPCPluginClient, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	return &RPCPluginClient{instance: instance, client: rpc.NewClient(conn)}, nil
}

func (c *RPCPluginClient) Close() error { return c.client.Close() }
func (c *RPCPluginClient) call(ctx context.Context, method string, req, resp any) error {
	done := make(chan error, 1)
	go func() { done <- c.client.Call(method, req, resp) }()
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
