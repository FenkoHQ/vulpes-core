package testutil

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/FenkoHQ/vulpes-core/internal/capabilities"
)

type Authenticator struct {
	Allow bool
	Calls atomic.Int64
}

func (a *Authenticator) Authenticate(ctx context.Context, req capabilities.AuthenticateRequest) (capabilities.AuthenticateResponse, error) {
	a.Calls.Add(1)
	if !a.Allow {
		return capabilities.AuthenticateResponse{Allow: false, DenyReason: "invalid credentials"}, nil
	}
	return capabilities.AuthenticateResponse{Allow: true, Identity: capabilities.Identity{Subject: "test", TenantID: "test", AuthMethod: "fake"}}, nil
}

type Router struct{ Calls atomic.Int64 }

func (r *Router) Route(ctx context.Context, req capabilities.RouteRequest) (capabilities.RouteResponse, error) {
	r.Calls.Add(1)
	routes := make([]capabilities.SelectedRoute, 0, len(req.Candidates))
	for i, c := range req.Candidates {
		routes = append(routes, capabilities.SelectedRoute{ProviderInstance: c.ProviderInstance, ProviderModel: c.ProviderModel, Priority: i})
	}
	return capabilities.RouteResponse{Routes: routes, Reason: "fake"}, nil
}

type Upstream struct {
	Calls                 atomic.Int64
	FailFirst             atomic.Bool
	StreamErrorAfterFirst bool
	Text                  string
}

func (u *Upstream) Invoke(ctx context.Context, req capabilities.InvokeRequest) (<-chan capabilities.ResponseChunk, error) {
	u.Calls.Add(1)
	if u.FailFirst.CompareAndSwap(true, false) {
		ch := make(chan capabilities.ResponseChunk, 1)
		ch <- capabilities.ResponseChunk{Error: &capabilities.UpstreamError{Code: "upstream_5xx", Message: "temporary", HTTPStatus: 503, Retryable: true}}
		close(ch)
		return ch, nil
	}
	text := u.Text
	if text == "" {
		text = "hello"
	}
	ch := make(chan capabilities.ResponseChunk)
	go func() {
		defer close(ch)
		id := "chatcmpl_" + uuid.NewString()
		if req.Request.Stream {
			for _, part := range []string{text[:min(1, len(text))], text[min(1, len(text)):]} {
				if part == "" {
					continue
				}
				ch <- capabilities.ResponseChunk{Chunk: &capabilities.ChatCompletionChunk{ID: id, Object: "chat.completion.chunk", Created: time.Now().Unix(), Model: req.ProviderModel, Choices: []capabilities.ChatChoice{{Index: 0, Delta: capabilities.ChatMessage{Role: "assistant", Content: part}}}}}
				if u.StreamErrorAfterFirst {
					ch <- capabilities.ResponseChunk{Error: &capabilities.UpstreamError{Code: "stream_interrupted", Message: "boom", HTTPStatus: 502, Retryable: true}}
					return
				}
			}
		} else {
			ch <- capabilities.ResponseChunk{Chunk: &capabilities.ChatCompletionChunk{ID: id, Object: "chat.completion.chunk", Created: time.Now().Unix(), Model: req.ProviderModel, Choices: []capabilities.ChatChoice{{Index: 0, Message: capabilities.ChatMessage{Role: "assistant", Content: text}}}}}
		}
		ch <- capabilities.ResponseChunk{Usage: &capabilities.Usage{ProviderInstance: req.Context.PluginInstance, ProviderModel: req.ProviderModel, InputTokens: 1, OutputTokens: 1, TotalTokens: 2}}
	}()
	return ch, nil
}
func (u *Upstream) ListModels(ctx context.Context) ([]capabilities.ModelInfo, error) {
	return []capabilities.ModelInfo{{ID: "fake", Object: "model", OwnedBy: "fake", Healthy: true}}, nil
}

type RateLimiter struct {
	Deny    bool
	Calls   atomic.Int64
	Commits atomic.Int64
}

func (r *RateLimiter) Check(ctx context.Context, req capabilities.RateLimitCheckRequest) (capabilities.RateLimitCheckResponse, error) {
	r.Calls.Add(1)
	if r.Deny {
		return capabilities.RateLimitCheckResponse{Decision: "deny", DenyReason: "limit exceeded", RetryAfterMS: 1000}, nil
	}
	return capabilities.RateLimitCheckResponse{Decision: "allow", State: capabilities.RateLimitState{RequestLimit: 10, RequestRemaining: 9}}, nil
}
func (r *RateLimiter) Commit(ctx context.Context, req capabilities.CommitUsageRequest) error {
	r.Commits.Add(1)
	return nil
}

type Cache struct {
	Hit      bool
	Calls    atomic.Int64
	Stores   atomic.Int64
	Response capabilities.ChatCompletionResponse
}

func (c *Cache) Lookup(ctx context.Context, req capabilities.CacheLookupRequest) (capabilities.CacheLookupResponse, error) {
	c.Calls.Add(1)
	if c.Hit {
		resp := c.Response
		if resp.ID == "" {
			resp = capabilities.ChatCompletionResponse{ID: "cached", Object: "chat.completion", Created: time.Now().Unix(), Model: "cached", Choices: []capabilities.ChatChoice{{Index: 0, Message: capabilities.ChatMessage{Role: "assistant", Content: "cached"}, FinishReason: "stop"}}}
		}
		return capabilities.CacheLookupResponse{Hit: true, Response: capabilities.CachedResponse{Response: resp}}, nil
	}
	return capabilities.CacheLookupResponse{}, nil
}
func (c *Cache) Store(ctx context.Context, req capabilities.CacheStoreRequest) error {
	c.Stores.Add(1)
	return nil
}

type Observer struct {
	Block bool
	Fail  bool
	Calls atomic.Int64
}

func (o *Observer) Emit(ctx context.Context, events []capabilities.GatewayEvent) error {
	o.Calls.Add(1)
	if o.Block {
		<-ctx.Done()
		return ctx.Err()
	}
	if o.Fail {
		return fmt.Errorf("observer failed")
	}
	return nil
}
