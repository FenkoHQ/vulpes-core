package capabilities

import (
	"context"
	"errors"
	"time"
)

// CapabilityType is a strict v1 capability identifier.
type CapabilityType string

const (
	AuthenticatorCap         CapabilityType = "authenticator"
	AuthorizerCap            CapabilityType = "authorizer"
	RateLimiterCap           CapabilityType = "rate_limiter"
	RouterCap                CapabilityType = "router"
	UpstreamProviderCap      CapabilityType = "upstream_provider"
	CacheProviderCap         CapabilityType = "cache_provider"
	ObserverCap              CapabilityType = "observer"
	PromptProviderCap        CapabilityType = "prompt_provider"
	CostProviderCap          CapabilityType = "cost_provider"
	ModelRegistryProviderCap CapabilityType = "model_registry_provider"
)

var ErrDenied = errors.New("denied")

type CallContext struct {
	RequestID      string
	TenantID       string
	Deadline       time.Time
	TraceContext   map[string]string
	GatewayVersion string
	PluginInstance string
}

type Diagnostic struct {
	Severity string            `json:"severity"`
	Code     string            `json:"code"`
	Message  string            `json:"message"`
	Details  map[string]string `json:"details,omitempty"`
}

type Identity struct {
	Subject    string            `json:"subject"`
	TenantID   string            `json:"tenant_id"`
	Groups     []string          `json:"groups,omitempty"`
	Claims     map[string]string `json:"claims,omitempty"`
	AuthMethod string            `json:"auth_method"`
}

type RequestSummary struct {
	Operation            string            `json:"operation"`
	RequestedModel       string            `json:"requested_model"`
	EstimatedInputTokens int64             `json:"estimated_input_tokens"`
	Properties           map[string]string `json:"properties,omitempty"`
}

type RequestConstraints struct {
	AllowedModels       []string `json:"allowed_models,omitempty"`
	MaxOutputTokens     int64    `json:"max_output_tokens,omitempty"`
	MaxEstimatedCostUSD float64  `json:"max_estimated_cost_usd,omitempty"`
	AllowedRegions      []string `json:"allowed_regions,omitempty"`
}

type Usage struct {
	ProviderInstance string  `json:"provider_instance,omitempty"`
	ProviderModel    string  `json:"provider_model,omitempty"`
	InputTokens      int64   `json:"input_tokens,omitempty"`
	OutputTokens     int64   `json:"output_tokens,omitempty"`
	TotalTokens      int64   `json:"total_tokens,omitempty"`
	CostUSD          float64 `json:"cost_usd,omitempty"`
}

type ChatMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall mirrors the OpenAI tool-call shape. Arguments is a JSON-encoded
// string of the function arguments — that is the upstream contract, not a
// parsed object. In streaming responses upstream emits partial deltas with
// matching Index; the gateway forwards them verbatim and clients aggregate.
type ToolCall struct {
	// Index must always be emitted: streaming clients key argument deltas by
	// it, and index 0 (a single tool call) is the common case — omitempty
	// would drop it and break client-side aggregation.
	Index    int              `json:"index"`
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function ToolCallFunction `json:"function,omitempty"`
}

type ToolCallFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type ChatCompletionRequest struct {
	Model       string         `json:"model"`
	Messages    []ChatMessage  `json:"messages"`
	Stream      bool           `json:"stream,omitempty"`
	Temperature *float64       `json:"temperature,omitempty"`
	MaxTokens   *int64         `json:"max_tokens,omitempty"`
	Tools       any            `json:"tools,omitempty"`
	ToolChoice  any            `json:"tool_choice,omitempty"`
	Metadata    any            `json:"metadata,omitempty"`
	Extra       map[string]any `json:"-"`
}

type ChatCompletionResponse struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []ChatChoice     `json:"choices"`
	Usage   map[string]int64 `json:"usage,omitempty"`
}

type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message,omitempty"`
	Delta        ChatMessage `json:"delta,omitempty"`
	FinishReason string      `json:"finish_reason,omitempty"`
}

type ChatCompletionChunk struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
}

type UpstreamError struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	HTTPStatus  int    `json:"http_status"`
	Retryable   bool   `json:"retryable"`
	RateLimited bool   `json:"rate_limited"`
}

func (e *UpstreamError) Error() string { return e.Message }

type ResponseChunk struct {
	Chunk *ChatCompletionChunk `json:"chunk,omitempty"`
	Usage *Usage               `json:"usage,omitempty"`
	Error *UpstreamError       `json:"error,omitempty"`
}

type AuthenticateRequest struct {
	Context  CallContext       `json:"context"`
	Headers  map[string]string `json:"headers"`
	SourceIP string            `json:"source_ip"`
	Method   string            `json:"method"`
	Path     string            `json:"path"`
}

type AuthenticateResponse struct {
	Allow       bool         `json:"allow"`
	Identity    Identity     `json:"identity"`
	DenyReason  string       `json:"deny_reason,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

type AuthorizeRequest struct {
	Context  CallContext    `json:"context"`
	Identity Identity       `json:"identity"`
	Request  RequestSummary `json:"request"`
}

type AuthorizeResponse struct {
	Allow       bool               `json:"allow"`
	DenyReason  string             `json:"deny_reason,omitempty"`
	Constraints RequestConstraints `json:"constraints,omitempty"`
	Diagnostics []Diagnostic       `json:"diagnostics,omitempty"`
}

type RateLimitCheckRequest struct {
	Context               CallContext       `json:"context"`
	Identity              Identity          `json:"identity"`
	Model                 string            `json:"model"`
	EstimatedInputTokens  int64             `json:"estimated_input_tokens"`
	RequestedOutputTokens int64             `json:"requested_output_tokens"`
	Properties            map[string]string `json:"properties,omitempty"`
}

type RateLimitState struct {
	RequestLimit       int64   `json:"request_limit,omitempty"`
	RequestRemaining   int64   `json:"request_remaining,omitempty"`
	TokenLimit         int64   `json:"token_limit,omitempty"`
	TokenRemaining     int64   `json:"token_remaining,omitempty"`
	BudgetLimitUSD     float64 `json:"budget_limit_usd,omitempty"`
	BudgetRemainingUSD float64 `json:"budget_remaining_usd,omitempty"`
	ResetUnixNano      int64   `json:"reset_unix_nano,omitempty"`
}

type RateLimitCheckResponse struct {
	Decision     string         `json:"decision"` // allow, deny, delay
	DenyReason   string         `json:"deny_reason,omitempty"`
	RetryAfterMS int64          `json:"retry_after_ms,omitempty"`
	State        RateLimitState `json:"state,omitempty"`
}

type CommitUsageRequest struct {
	Context  CallContext `json:"context"`
	Identity Identity    `json:"identity"`
	Usage    Usage       `json:"usage"`
}

type RouteCandidate struct {
	ProviderInstance         string            `json:"provider_instance"`
	ProviderModel            string            `json:"provider_model"`
	LogicalModel             string            `json:"logical_model"`
	Weight                   int               `json:"weight"`
	Region                   string            `json:"region,omitempty"`
	Healthy                  bool              `json:"healthy"`
	EstimatedCostPer1KInput  float64           `json:"estimated_cost_per_1k_input,omitempty"`
	EstimatedCostPer1KOutput float64           `json:"estimated_cost_per_1k_output,omitempty"`
	Properties               map[string]string `json:"properties,omitempty"`
}

type RouteRequest struct {
	Context        CallContext        `json:"context"`
	Identity       Identity           `json:"identity"`
	RequestedModel string             `json:"requested_model"`
	Request        RequestSummary     `json:"request"`
	Candidates     []RouteCandidate   `json:"candidates"`
	Constraints    RequestConstraints `json:"constraints,omitempty"`
}

type SelectedRoute struct {
	ProviderInstance string            `json:"provider_instance"`
	ProviderModel    string            `json:"provider_model"`
	Priority         int               `json:"priority"`
	Properties       map[string]string `json:"properties,omitempty"`
}

type RouteResponse struct {
	Routes      []SelectedRoute `json:"routes"`
	Reason      string          `json:"reason,omitempty"`
	Diagnostics []Diagnostic    `json:"diagnostics,omitempty"`
}

type InvokeRequest struct {
	Context       CallContext           `json:"context"`
	Identity      Identity              `json:"identity"`
	ProviderModel string                `json:"provider_model"`
	Request       ChatCompletionRequest `json:"request"`
	Properties    map[string]string     `json:"properties,omitempty"`
}

type ModelInfo struct {
	ID                string            `json:"id"`
	Object            string            `json:"object,omitempty"`
	OwnedBy           string            `json:"owned_by,omitempty"`
	ProviderInstance  string            `json:"provider_instance,omitempty"`
	ProviderModel     string            `json:"provider_model,omitempty"`
	ContextWindow     int64             `json:"context_window,omitempty"`
	SupportsStreaming bool              `json:"supports_streaming,omitempty"`
	SupportsTools     bool              `json:"supports_tools,omitempty"`
	SupportsJSONMode  bool              `json:"supports_json_mode,omitempty"`
	SupportsVision    bool              `json:"supports_vision,omitempty"`
	Region            string            `json:"region,omitempty"`
	Healthy           bool              `json:"healthy"`
	Properties        map[string]string `json:"properties,omitempty"`
}

type CachePolicy struct {
	TTLMillis int64             `json:"ttl_ms,omitempty"`
	Semantic  bool              `json:"semantic,omitempty"`
	Vary      map[string]string `json:"vary,omitempty"`
}

type CachedResponse struct {
	Response ChatCompletionResponse `json:"response"`
	Usage    Usage                  `json:"usage,omitempty"`
}

type CacheLookupRequest struct {
	Context  CallContext `json:"context"`
	Identity Identity    `json:"identity"`
	CacheKey string      `json:"cache_key"`
	Policy   CachePolicy `json:"policy"`
}

type CacheLookupResponse struct {
	Hit      bool           `json:"hit"`
	Response CachedResponse `json:"response,omitempty"`
	CacheID  string         `json:"cache_id,omitempty"`
}

type CacheStoreRequest struct {
	Context  CallContext    `json:"context"`
	Identity Identity       `json:"identity"`
	CacheKey string         `json:"cache_key"`
	Response CachedResponse `json:"response"`
	Policy   CachePolicy    `json:"policy"`
}

type GatewayEvent struct {
	EventID           string            `json:"event_id"`
	RequestID         string            `json:"request_id"`
	TenantID          string            `json:"tenant_id,omitempty"`
	EventType         string            `json:"event_type"`
	TimestampUnixNano int64             `json:"timestamp_unix_nano"`
	Properties        map[string]string `json:"properties,omitempty"`
	Usage             Usage             `json:"usage,omitempty"`
	Error             map[string]string `json:"error,omitempty"`
}

type PromptResolveRequest struct {
	Context   CallContext       `json:"context"`
	Identity  Identity          `json:"identity"`
	PromptRef string            `json:"prompt_ref"`
	Version   string            `json:"version,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
}

type PromptResolveResponse struct {
	Messages        []ChatMessage     `json:"messages"`
	ResolvedVersion string            `json:"resolved_version,omitempty"`
	Properties      map[string]string `json:"properties,omitempty"`
}

type CostEstimateRequest struct {
	Context      CallContext `json:"context"`
	Provider     string      `json:"provider"`
	Model        string      `json:"model"`
	InputTokens  int64       `json:"input_tokens"`
	OutputTokens int64       `json:"output_tokens"`
}

type CostEstimateResponse struct {
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

type Authenticator interface {
	Authenticate(context.Context, AuthenticateRequest) (AuthenticateResponse, error)
}
type Authorizer interface {
	Authorize(context.Context, AuthorizeRequest) (AuthorizeResponse, error)
}
type RateLimiter interface {
	Check(context.Context, RateLimitCheckRequest) (RateLimitCheckResponse, error)
	Commit(context.Context, CommitUsageRequest) error
}
type Router interface {
	Route(context.Context, RouteRequest) (RouteResponse, error)
}
type UpstreamProvider interface {
	Invoke(context.Context, InvokeRequest) (<-chan ResponseChunk, error)
	ListModels(context.Context) ([]ModelInfo, error)
}
type CacheProvider interface {
	Lookup(context.Context, CacheLookupRequest) (CacheLookupResponse, error)
	Store(context.Context, CacheStoreRequest) error
}
type Observer interface {
	Emit(context.Context, []GatewayEvent) error
}
type PromptProvider interface {
	ResolvePrompt(context.Context, PromptResolveRequest) (PromptResolveResponse, error)
}
type CostProvider interface {
	Estimate(context.Context, CostEstimateRequest) (CostEstimateResponse, error)
}
type ModelRegistryProvider interface {
	ListModels(context.Context) ([]ModelInfo, error)
}
