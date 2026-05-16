package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/FenkoHQ/vulpes-core/internal/capabilities"
	"github.com/FenkoHQ/vulpes-core/internal/config"
	"github.com/FenkoHQ/vulpes-core/internal/models"
	"github.com/FenkoHQ/vulpes-core/internal/observability"
	"github.com/FenkoHQ/vulpes-core/internal/plugins"
)

type Pipeline struct {
	cfg               config.Config
	registry          *plugins.Registry
	models            *models.Registry
	authn             capabilities.Authenticator
	authz             capabilities.Authorizer
	ratelimit         capabilities.RateLimiter
	router            capabilities.Router
	cache             capabilities.CacheProvider
	prompt            capabilities.PromptProvider
	cost              capabilities.CostProvider
	upstreams         map[string]capabilities.UpstreamProvider
	blockingObservers []capabilities.Observer
	events            *observability.Queue
	ready             bool
	missing           []capabilities.CapabilityType
}

type Result struct {
	RequestID     string
	Response      capabilities.ChatCompletionResponse
	Stream        <-chan capabilities.ResponseChunk
	Usage         capabilities.Usage
	Headers       map[string]string
	FallbackIndex int
}

func Compile(cfg config.Config, reg *plugins.Registry, modelReg *models.Registry) (*Pipeline, error) {
	return compile(cfg, reg, modelReg)
}

func compile(cfg config.Config, reg *plugins.Registry, modelReg *models.Registry) (*Pipeline, error) {
	p := &Pipeline{cfg: cfg, registry: reg, models: modelReg, upstreams: map[string]capabilities.UpstreamProvider{}}
	if !cfg.Auth.Anonymous {
		if cfg.Pipeline.Authenticator == "" {
			p.missing = append(p.missing, capabilities.AuthenticatorCap)
		} else {
			v, err := reg.Authenticator(cfg.Pipeline.Authenticator)
			if err != nil {
				p.missing = append(p.missing, capabilities.AuthenticatorCap)
			} else {
				p.authn = v
			}
		}
	}
	if cfg.Pipeline.Authorizer != "" {
		v, err := reg.Authorizer(cfg.Pipeline.Authorizer)
		if err != nil {
			return nil, err
		}
		p.authz = v
	}
	if cfg.Pipeline.RateLimiter != "" {
		v, err := reg.RateLimiter(cfg.Pipeline.RateLimiter)
		if err != nil {
			return nil, err
		}
		p.ratelimit = v
	}
	if cfg.Pipeline.CacheProvider != "" {
		v, err := reg.CacheProvider(cfg.Pipeline.CacheProvider)
		if err != nil {
			return nil, err
		}
		p.cache = v
	}
	if cfg.Pipeline.PromptProvider != "" {
		v, err := reg.PromptProvider(cfg.Pipeline.PromptProvider)
		if err != nil {
			return nil, err
		}
		p.prompt = v
	}
	if cfg.Pipeline.Router == "" {
		p.missing = append(p.missing, capabilities.RouterCap)
	} else {
		v, err := reg.Router(cfg.Pipeline.Router)
		if err != nil {
			p.missing = append(p.missing, capabilities.RouterCap)
		} else {
			p.router = v
		}
	}
	for _, name := range cfg.Pipeline.UpstreamProviders {
		v, err := reg.UpstreamProvider(name)
		if err != nil {
			return nil, err
		}
		p.upstreams[name] = v
	}
	if len(p.upstreams) == 0 {
		p.missing = append(p.missing, capabilities.UpstreamProviderCap)
	}
	var async []capabilities.Observer
	for _, name := range cfg.Pipeline.Observers {
		v, err := reg.Observer(name)
		if err != nil {
			return nil, err
		}
		async = append(async, v)
	}
	for _, name := range cfg.Pipeline.BlockingObservers {
		v, err := reg.Observer(name)
		if err != nil {
			return nil, err
		}
		p.blockingObservers = append(p.blockingObservers, v)
	}
	p.events = observability.NewQueue(cfg.Observability.QueueSize, cfg.Observability.BatchSize, cfg.Observability.FlushInterval.Duration(), cfg.Observability.OnOverflow, async)
	p.ready = len(p.missing) == 0
	return p, nil
}

func (p *Pipeline) Ready() bool { return p.ready }
func (p *Pipeline) MissingCapabilities() []capabilities.CapabilityType {
	out := make([]capabilities.CapabilityType, len(p.missing))
	copy(out, p.missing)
	return out
}

func (p *Pipeline) ExecuteChat(ctx context.Context, req capabilities.ChatCompletionRequest, headers map[string]string, sourceIP string) (Result, error) {
	requestID := uuid.NewString()
	result := Result{RequestID: requestID, Headers: map[string]string{"X-Gateway-Request-Id": requestID, "X-Gateway-Cache": "BYPASS"}}
	if !p.ready {
		return result, MissingCapabilitiesError{Missing: p.missing}
	}
	started := time.Now()
	callCtx := capabilities.CallContext{RequestID: requestID, GatewayVersion: "dev", TraceContext: map[string]string{}}
	props := extractProperties(headers)
	props["requested_model"] = req.Model
	summary := capabilities.RequestSummary{Operation: "chat.completions", RequestedModel: req.Model, EstimatedInputTokens: estimateTokens(req), Properties: props}
	identity := capabilities.Identity{Subject: "anonymous", TenantID: "anonymous", AuthMethod: "anonymous"}
	if !p.cfg.Auth.Anonymous {
		auth, err := p.authn.Authenticate(ctx, capabilities.AuthenticateRequest{Context: callCtx, Headers: headers, SourceIP: sourceIP, Method: "POST", Path: "/v1/chat/completions"})
		if err != nil {
			return result, GatewayError{Type: "gateway_error", Code: "authentication_failed", Message: err.Error(), Status: 401}
		}
		if !auth.Allow {
			return result, GatewayError{Type: "gateway_error", Code: "authentication_failed", Message: auth.DenyReason, Status: 401}
		}
		identity = auth.Identity
		callCtx.TenantID = identity.TenantID
	}
	p.emit("request.started", requestID, identity.TenantID, props, nil)
	if p.authz != nil {
		az, err := p.authz.Authorize(ctx, capabilities.AuthorizeRequest{Context: callCtx, Identity: identity, Request: summary})
		if err != nil {
			return result, err
		}
		if !az.Allow {
			return result, GatewayError{Type: "gateway_error", Code: "authorization_denied", Message: az.DenyReason, Status: 403}
		}
	}
	if p.prompt != nil {
		ref := props["context-key"]
		if ref == "" {
			ref = props["prompt-ref"]
		}
		if ref == "" {
			ref = req.Model
		}
		vars := promptVariables(props, req, identity)
		pr, err := p.prompt.ResolvePrompt(ctx, capabilities.PromptResolveRequest{Context: callCtx, Identity: identity, PromptRef: ref, Variables: vars})
		if err != nil {
			return result, err
		}
		if len(pr.Messages) > 0 {
			req.Messages = pr.Messages
			for k, v := range pr.Properties {
				props[k] = v
			}
		}
	}
	if p.cache != nil && !req.Stream {
		key := cacheKey(req, identity, props)
		lk, err := p.cache.Lookup(ctx, capabilities.CacheLookupRequest{Context: callCtx, Identity: identity, CacheKey: key, Policy: capabilities.CachePolicy{TTLMillis: 60000}})
		if err == nil && lk.Hit {
			result.Response = lk.Response.Response
			result.Usage = lk.Response.Usage
			result.Headers["X-Gateway-Cache"] = "HIT"
			p.emit("cache.hit", requestID, identity.TenantID, props, &result.Usage)
			return result, nil
		}
		result.Headers["X-Gateway-Cache"] = "MISS"
	}
	if p.ratelimit != nil {
		maxOut := int64(0)
		if req.MaxTokens != nil {
			maxOut = *req.MaxTokens
		}
		rl, err := p.ratelimit.Check(ctx, capabilities.RateLimitCheckRequest{Context: callCtx, Identity: identity, Model: req.Model, EstimatedInputTokens: summary.EstimatedInputTokens, RequestedOutputTokens: maxOut, Properties: props})
		if err != nil {
			return result, err
		}
		if rl.State.RequestLimit > 0 {
			result.Headers["X-Gateway-RateLimit-Limit"] = fmt.Sprint(rl.State.RequestLimit)
			result.Headers["X-Gateway-RateLimit-Remaining"] = fmt.Sprint(rl.State.RequestRemaining)
			result.Headers["X-Gateway-RateLimit-Reset"] = fmt.Sprint(rl.State.ResetUnixNano)
		}
		if rl.Decision == "deny" {
			return result, GatewayError{Type: "gateway_error", Code: "rate_limit_exceeded", Message: rl.DenyReason, Status: 429, Details: map[string]any{"retry_after_ms": rl.RetryAfterMS}}
		}
	}
	candidates, err := p.models.Candidates(req.Model)
	if err != nil {
		return result, GatewayError{Type: "gateway_error", Code: "model_not_found", Message: err.Error(), Status: 404}
	}
	routeResp, err := p.router.Route(ctx, capabilities.RouteRequest{Context: callCtx, Identity: identity, RequestedModel: req.Model, Request: summary, Candidates: candidates})
	if err != nil {
		return result, err
	}
	if len(routeResp.Routes) == 0 {
		return result, GatewayError{Type: "gateway_error", Code: "no_route_available", Message: "router returned no routes", Status: 503}
	}
	p.emit("route.selected", requestID, identity.TenantID, routeEventProps(props, routeResp.Routes[0], 0, started), nil)
	if req.Stream {
		return p.executeStream(ctx, req, callCtx, identity, props, routeResp.Routes, result, started)
	}
	return p.executeNonStream(ctx, req, callCtx, identity, props, routeResp.Routes, result, started)
}

func (p *Pipeline) executeNonStream(ctx context.Context, req capabilities.ChatCompletionRequest, callCtx capabilities.CallContext, identity capabilities.Identity, props map[string]string, routes []capabilities.SelectedRoute, result Result, started time.Time) (Result, error) {
	var lastErr error
	maxAttempts := min(p.cfg.Fallbacks.MaxAttempts, len(routes))
	if maxAttempts <= 0 {
		maxAttempts = len(routes)
	}
	for i, r := range routes[:maxAttempts] {
		up := p.upstreams[r.ProviderInstance]
		if up == nil {
			lastErr = fmt.Errorf("provider %s not configured", r.ProviderInstance)
			continue
		}
		result.Headers["X-Gateway-Route-Provider"] = r.ProviderInstance
		result.Headers["X-Gateway-Route-Model"] = r.ProviderModel
		result.Headers["X-Gateway-Fallback-Index"] = fmt.Sprint(i)
		result.FallbackIndex = i
		routeProps := routeEventProps(props, r, i, started)
		invokeCtx := callCtx
		invokeCtx.PluginInstance = r.ProviderInstance
		ch, err := up.Invoke(ctx, capabilities.InvokeRequest{Context: invokeCtx, Identity: identity, ProviderModel: r.ProviderModel, Request: req, Properties: routeProps})
		if err != nil {
			lastErr = err
			p.emitError("upstream.error", result.RequestID, identity.TenantID, routeProps, err)
			if retryable(err) {
				continue
			}
			break
		}
		resp, usage, err := collectResponse(ch, req.Model)
		if err != nil {
			lastErr = err
			p.emitError("upstream.error", result.RequestID, identity.TenantID, routeProps, err)
			if retryable(err) {
				continue
			}
			break
		}
		result.Response = resp
		result.Usage = usage
		if err := p.afterSuccess(ctx, callCtx, identity, req, routeProps, result); err != nil {
			return result, err
		}
		return result, nil
	}
	err := GatewayError{Type: "gateway_error", Code: "upstream_unavailable", Message: fmt.Sprintf("all configured upstream providers failed before response streaming started: %v", lastErr), Status: 503}
	p.emitError("request.failed", result.RequestID, identity.TenantID, p.withTranscriptProps(addTiming(props, started), req, capabilities.ChatCompletionResponse{}), err)
	return result, err
}

func (p *Pipeline) executeStream(ctx context.Context, req capabilities.ChatCompletionRequest, callCtx capabilities.CallContext, identity capabilities.Identity, props map[string]string, routes []capabilities.SelectedRoute, result Result, started time.Time) (Result, error) {
	var lastErr error
	maxAttempts := min(p.cfg.Fallbacks.MaxAttempts, len(routes))
	if maxAttempts <= 0 {
		maxAttempts = len(routes)
	}
	for i, r := range routes[:maxAttempts] {
		up := p.upstreams[r.ProviderInstance]
		if up == nil {
			continue
		}
		result.Headers["X-Gateway-Route-Provider"] = r.ProviderInstance
		result.Headers["X-Gateway-Route-Model"] = r.ProviderModel
		result.Headers["X-Gateway-Fallback-Index"] = fmt.Sprint(i)
		result.FallbackIndex = i
		routeProps := routeEventProps(props, r, i, started)
		invokeCtx := callCtx
		invokeCtx.PluginInstance = r.ProviderInstance
		ch, err := up.Invoke(ctx, capabilities.InvokeRequest{Context: invokeCtx, Identity: identity, ProviderModel: r.ProviderModel, Request: req, Properties: routeProps})
		if err != nil {
			lastErr = err
			p.emitError("upstream.error", result.RequestID, identity.TenantID, routeProps, err)
			if retryable(err) {
				continue
			}
			break
		}
		out := make(chan capabilities.ResponseChunk)
		go func() {
			defer close(out)
			var usage capabilities.Usage
			failed := false
			var responseText strings.Builder
			for chunk := range ch {
				if chunk.Usage != nil {
					usage = *chunk.Usage
				}
				if chunk.Chunk != nil && p.cfg.Observability.CapturePayloads {
					for _, choice := range chunk.Chunk.Choices {
						if s, ok := choice.Delta.Content.(string); ok {
							responseText.WriteString(s)
						}
						if s, ok := choice.Message.Content.(string); ok {
							responseText.WriteString(s)
						}
					}
				}
				if chunk.Error != nil {
					failed = true
					p.emitError("upstream.error", result.RequestID, identity.TenantID, routeProps, chunk.Error)
				}
				out <- chunk
			}
			if p.ratelimit != nil && usage.TotalTokens > 0 {
				_ = p.ratelimit.Commit(context.Background(), capabilities.CommitUsageRequest{Context: callCtx, Identity: identity, Usage: usage})
			}
			completedProps := addTiming(routeProps, started)
			if p.cfg.Observability.CapturePayloads {
				completedProps = p.withStreamTranscriptProps(completedProps, req, responseText.String())
			}
			if failed {
				p.emitError("request.failed", result.RequestID, identity.TenantID, completedProps, fmt.Errorf("stream_interrupted"))
				return
			}
			p.emit("request.completed", result.RequestID, identity.TenantID, completedProps, &usage)
		}()
		result.Stream = out
		return result, nil
	}
	err := GatewayError{Type: "gateway_error", Code: "upstream_unavailable", Message: fmt.Sprintf("all configured upstream providers failed before response streaming started: %v", lastErr), Status: 503}
	p.emitError("request.failed", result.RequestID, identity.TenantID, p.withTranscriptProps(addTiming(props, started), req, capabilities.ChatCompletionResponse{}), err)
	return result, err
}

func (p *Pipeline) afterSuccess(ctx context.Context, callCtx capabilities.CallContext, identity capabilities.Identity, req capabilities.ChatCompletionRequest, props map[string]string, result Result) error {
	props = p.withTranscriptProps(props, req, result.Response)
	if p.cache != nil {
		_ = p.cache.Store(ctx, capabilities.CacheStoreRequest{Context: callCtx, Identity: identity, CacheKey: cacheKey(req, identity, props), Response: capabilities.CachedResponse{Response: result.Response, Usage: result.Usage}, Policy: capabilities.CachePolicy{TTLMillis: 60000}})
	}
	if p.ratelimit != nil {
		_ = p.ratelimit.Commit(ctx, capabilities.CommitUsageRequest{Context: callCtx, Identity: identity, Usage: result.Usage})
	}
	p.emit("request.completed", result.RequestID, identity.TenantID, props, &result.Usage)
	for _, obs := range p.blockingObservers {
		if err := obs.Emit(ctx, []capabilities.GatewayEvent{{EventID: uuid.NewString(), RequestID: result.RequestID, TenantID: identity.TenantID, EventType: "request.completed", TimestampUnixNano: time.Now().UnixNano(), Properties: props, Usage: result.Usage}}); err != nil {
			return GatewayError{Type: "gateway_error", Code: "observer_failed", Message: err.Error(), Status: 500}
		}
	}
	return nil
}

func collectResponse(ch <-chan capabilities.ResponseChunk, model string) (capabilities.ChatCompletionResponse, capabilities.Usage, error) {
	var resp capabilities.ChatCompletionResponse
	var usage capabilities.Usage
	var content string
	for chunk := range ch {
		if chunk.Error != nil {
			return resp, usage, chunk.Error
		}
		if chunk.Usage != nil {
			usage = *chunk.Usage
		}
		if chunk.Chunk != nil {
			if resp.ID == "" {
				resp.ID = chunk.Chunk.ID
				resp.Object = "chat.completion"
				resp.Created = chunk.Chunk.Created
				resp.Model = model
			}
			for _, c := range chunk.Chunk.Choices {
				if s, ok := c.Delta.Content.(string); ok {
					content += s
				}
				if s, ok := c.Message.Content.(string); ok {
					content += s
				}
			}
		}
	}
	if resp.ID == "" {
		resp.ID = "chatcmpl_" + uuid.NewString()
		resp.Object = "chat.completion"
		resp.Created = time.Now().Unix()
		resp.Model = model
	}
	resp.Choices = []capabilities.ChatChoice{{Index: 0, Message: capabilities.ChatMessage{Role: "assistant", Content: content}, FinishReason: "stop"}}
	if usage.TotalTokens > 0 {
		resp.Usage = map[string]int64{"prompt_tokens": usage.InputTokens, "completion_tokens": usage.OutputTokens, "total_tokens": usage.TotalTokens}
	}
	return resp, usage, nil
}

func retryable(err error) bool {
	var ue *capabilities.UpstreamError
	if errors.As(err, &ue) {
		return ue.Retryable || ue.RateLimited || ue.HTTPStatus >= 500
	}
	return true
}
func estimateTokens(req capabilities.ChatCompletionRequest) int64 {
	b, _ := json.Marshal(req.Messages)
	return int64(len(b)/4 + 1)
}

func promptVariables(props map[string]string, req capabilities.ChatCompletionRequest, identity capabilities.Identity) map[string]string {
	vars := make(map[string]string, len(props)+5)
	for k, v := range props {
		vars[k] = v
	}
	vars["model"] = req.Model
	vars["tenant_id"] = identity.TenantID
	vars["subject"] = identity.Subject
	if b, err := json.Marshal(req.Messages); err == nil {
		vars["messages_json"] = string(b)
	}
	if b, err := json.Marshal(req); err == nil {
		vars["request_json"] = string(b)
	}
	return vars
}

func cacheKey(req capabilities.ChatCompletionRequest, id capabilities.Identity, props map[string]string) string {
	h := sha256.New()
	_ = json.NewEncoder(h).Encode(req)
	_, _ = io.WriteString(h, id.TenantID)
	b, _ := json.Marshal(props)
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}
func extractProperties(headers map[string]string) map[string]string {
	props := map[string]string{}
	for k, v := range headers {
		lk := strings.ToLower(k)
		const p = "x-gateway-property-"
		if strings.HasPrefix(lk, p) {
			props[strings.TrimPrefix(lk, p)] = v
		}
	}
	return props
}
func (p *Pipeline) emit(event, requestID, tenant string, props map[string]string, usage *capabilities.Usage) {
	if p.events == nil {
		return
	}
	ev := capabilities.GatewayEvent{EventID: uuid.NewString(), RequestID: requestID, TenantID: tenant, EventType: event, TimestampUnixNano: time.Now().UnixNano(), Properties: props}
	if usage != nil {
		ev.Usage = *usage
	}
	_ = p.events.Enqueue(ev)
}

func (p *Pipeline) emitError(event, requestID, tenant string, props map[string]string, err error) {
	if p.events == nil {
		return
	}
	ev := capabilities.GatewayEvent{EventID: uuid.NewString(), RequestID: requestID, TenantID: tenant, EventType: event, TimestampUnixNano: time.Now().UnixNano(), Properties: props, Error: map[string]string{"message": err.Error()}}
	var up *capabilities.UpstreamError
	if errors.As(err, &up) {
		ev.Error["code"] = up.Code
		ev.Error["http_status"] = fmt.Sprint(up.HTTPStatus)
		ev.Error["retryable"] = fmt.Sprint(up.Retryable)
		ev.Error["rate_limited"] = fmt.Sprint(up.RateLimited)
	}
	_ = p.events.Enqueue(ev)
}

func routeEventProps(base map[string]string, route capabilities.SelectedRoute, fallbackIndex int, started time.Time) map[string]string {
	props := addTiming(base, started)
	props["route_provider"] = route.ProviderInstance
	props["route_model"] = route.ProviderModel
	props["fallback_index"] = fmt.Sprint(fallbackIndex)
	for k, v := range route.Properties {
		props[k] = v
	}
	return props
}

func addTiming(base map[string]string, started time.Time) map[string]string {
	props := make(map[string]string, len(base)+1)
	for k, v := range base {
		props[k] = v
	}
	props["duration_ms"] = fmt.Sprintf("%.3f", float64(time.Since(started).Microseconds())/1000.0)
	return props
}

func (p *Pipeline) withTranscriptProps(base map[string]string, req capabilities.ChatCompletionRequest, resp capabilities.ChatCompletionResponse) map[string]string {
	if !p.cfg.Observability.CapturePayloads {
		return base
	}
	props := make(map[string]string, len(base)+2)
	for k, v := range base {
		props[k] = v
	}
	if b, err := json.Marshal(req); err == nil {
		props["request_json"] = string(b)
	}
	if b, err := json.Marshal(resp); err == nil {
		props["response_json"] = string(b)
	}
	props["payloads_captured"] = "true"
	return props
}

func (p *Pipeline) withStreamTranscriptProps(base map[string]string, req capabilities.ChatCompletionRequest, responseText string) map[string]string {
	if !p.cfg.Observability.CapturePayloads {
		return base
	}
	resp := capabilities.ChatCompletionResponse{Model: req.Model, Choices: []capabilities.ChatChoice{{Index: 0, Message: capabilities.ChatMessage{Role: "assistant", Content: responseText}}}}
	return p.withTranscriptProps(base, req, resp)
}

func (p *Pipeline) ListModels(ctx context.Context) []capabilities.ModelInfo {
	out := p.models.List()
	for _, up := range p.upstreams {
		models, err := up.ListModels(ctx)
		if err == nil {
			out = append(out, models...)
		}
	}
	return out
}
