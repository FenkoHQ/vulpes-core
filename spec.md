# Codex Implementation Spec: Strict Plugin-Based LLM Gateway Core in Idiomatic Go

## 0. Executive Summary

Build a small, high-performance LLM gateway core in Go with a strict capability-based plugin system inspired by Terraform's provider model, but adapted for a high-RPS data plane.

The core must:

* Run as an OpenAI-compatible HTTP gateway.
* Support zero configured plugins and fail clearly with actionable diagnostics when required runtime capabilities are missing.
* Import plugins only from:

  * local filesystem paths; and
  * GitHub repositories/releases.
* Launch plugins as long-lived subprocess workers, never per request.
* Communicate with plugins over versioned protobuf/gRPC using Unix domain sockets by default.
* Keep the hot request path minimal, deterministic, and precompiled from configuration at startup.
* Support strict typed capabilities rather than arbitrary middleware hooks.
* Be robust enough to replicate LiteLLM-style routing, provider abstraction, fallback, budget, and rate-limit behavior through plugins.
* Be robust enough to replicate Helicone-style observability, logging, request analytics, caching, cost tracking, custom properties, rate limits, and prompt management through plugins.
* Include thorough tests, examples, documentation, and a starter community plugins repository layout.

Do not implement a generic “on request” plugin system. Do not execute plugin binaries per request. Do not let plugins dynamically mutate the request pipeline.

The result should be a production-grade foundation that Ali can use internally first and release publicly later as a strict but extensible community ecosystem.

---

## 1. Core Design Principles

### 1.1 Small Core, Strict Boundaries

The core should own:

* HTTP server and OpenAI-compatible API surface.
* Request parsing and normalization.
* Streaming response coordination.
* Configuration loading and validation.
* Plugin discovery, installation, launch, health, supervision, restart.
* Capability discovery and schema validation.
* Request pipeline compilation.
* Deadlines, cancellation, tracing context, request IDs.
* Secret scoping and plugin permissions.
* In-memory model registry and route candidate registry.
* Core error format and compatibility behavior.

Plugins should own specialized behavior:

* Upstream provider integrations.
* Authentication sources.
* Authorization/policy engines.
* Routing strategies.
* Rate limiting and budgets.
* Caching backends.
* Observability/event sinks.
* Prompt template resolution.
* Cost/pricing metadata.

### 1.2 Strict Capability Model

A plugin binary is a package/runtime unit. A plugin may implement one or more typed capabilities.

Approved v1 capability types:

1. `Authenticator`
2. `Authorizer`
3. `RateLimiter`
4. `Router`
5. `UpstreamProvider`
6. `CacheProvider`
7. `Observer`
8. `PromptProvider`
9. `CostProvider`
10. `ModelRegistryProvider`

No generic hooks in v1.

Do not add arbitrary hooks such as:

* `BeforeRequest`
* `AfterRequest`
* `OnChunk`
* `Middleware`
* `TransformAnything`

If future content filtering is needed, add a strict `PromptGuard` or `ContentClassifier` capability later with clear input/output contracts and permission boundaries.

### 1.3 Data Plane vs Control Plane

The gateway must explicitly separate:

#### Data plane

Hot per-request path:

* authentication
* authorization
* cache lookup
* rate limit check
* routing
* upstream invocation
* response streaming
* usage accounting
* async event enqueue

#### Control plane

Background/plugin-heavy path:

* plugin installation
* plugin process launch
* plugin capability discovery
* config schema validation
* model catalog refresh
* pricing refresh
* provider health checks
* policy bundle refresh
* prompt template refresh
* event flushing
* plugin restarts

Never perform plugin installation or process startup on the data plane.

### 1.4 Performance Rules

* Plugins are started once at gateway boot/config reload.
* Plugins are kept warm.
* Plugin calls use persistent gRPC connections.
* Use Unix domain sockets by default for local plugin RPC.
* Compile the request pipeline at startup.
* Do not dynamically resolve capabilities per request.
* Do not call every plugin on every request.
* Do not send full prompts to plugins unless the capability requires it.
* Observer plugins are async and batched by default.
* Core must support common built-ins later, but v1 should keep code paths abstract through the same Go interfaces.

Target overhead excluding upstream LLM latency:

* p50 gateway overhead before upstream call: under 5ms in local plugin configuration.
* p95 gateway overhead before upstream call: under 20ms in local plugin configuration.
* No plugin process startup should occur per request.

These targets are test/benchmark goals, not hard functional requirements for the first commit.

---

## 2. Repository Layout

Use a Go module with clean packages and minimal magic.

Suggested repository:

```text
llm-gateway/
  go.mod
  go.sum
  README.md
  LICENSE
  Makefile
  cmd/
    gateway/
      main.go
    pluginctl/
      main.go
  internal/
    app/
      app.go
    config/
      config.go
      validate.go
      loader.go
    httpapi/
      server.go
      openai_chat.go
      openai_models.go
      errors.go
      streaming.go
    pipeline/
      pipeline.go
      compile.go
      execute.go
      context.go
    plugins/
      manager.go
      manifest.go
      installer.go
      github.go
      filesystem.go
      supervisor.go
      process.go
      sandbox.go
      registry.go
      health.go
      client.go
    capabilities/
      interfaces.go
      authn.go
      authz.go
      ratelimit.go
      router.go
      upstream.go
      cache.go
      observer.go
      prompt.go
      cost.go
      modelregistry.go
    secrets/
      provider.go
      env.go
      file.go
      scoped.go
    observability/
      events.go
      queue.go
      metrics.go
      tracing.go
    models/
      registry.go
      catalog.go
    usage/
      usage.go
      cost.go
    testutil/
      fake_plugin.go
      fake_upstream.go
      fixtures.go
  proto/
    gateway/
      v1/
        plugin.proto
        authn.proto
        authz.proto
        ratelimit.proto
        router.proto
        upstream.proto
        cache.proto
        observer.proto
        prompt.proto
        cost.proto
        modelregistry.proto
  gen/
    go/
      ... generated protobuf files ...
  examples/
    minimal-zero-plugins/
      gateway.yaml
    filesystem-plugin/
      gateway.yaml
    github-plugin/
      gateway.yaml
    openai-compatible/
      gateway.yaml
  docs/
    architecture.md
    plugin-authoring.md
    plugin-security.md
    configuration.md
    capability-contracts.md
    performance.md
    community-plugins.md
    testing.md
  tests/
    integration/
      ...
```

If using Buf for protobuf generation:

```text
  buf.yaml
  buf.gen.yaml
```

Use idiomatic Go package names. Avoid unnecessary framework complexity.

---

## 3. Community Plugins Repository Layout

Create a second repository later, but design the core so this works naturally.

Suggested community repo:

```text
llm-gateway-plugins/
  README.md
  go.work
  plugins/
    authn-static-api-key/
      go.mod
      README.md
      main.go
      gateway-plugin.yaml
      tests/
    authn-cloudflare-access/
      go.mod
      README.md
      main.go
      gateway-plugin.yaml
      tests/
    authz-opa/
      go.mod
      README.md
      main.go
      gateway-plugin.yaml
      tests/
    ratelimit-redis/
      go.mod
      README.md
      main.go
      gateway-plugin.yaml
      tests/
    router-weighted/
      go.mod
      README.md
      main.go
      gateway-plugin.yaml
      tests/
    upstream-openai/
      go.mod
      README.md
      main.go
      gateway-plugin.yaml
      tests/
    upstream-anthropic/
      go.mod
      README.md
      main.go
      gateway-plugin.yaml
      tests/
    cache-redis/
      go.mod
      README.md
      main.go
      gateway-plugin.yaml
      tests/
    observer-otel/
      go.mod
      README.md
      main.go
      gateway-plugin.yaml
      tests/
    observer-helicone-compatible/
      go.mod
      README.md
      main.go
      gateway-plugin.yaml
      tests/
    prompt-filesystem/
      go.mod
      README.md
      main.go
      gateway-plugin.yaml
      tests/
    cost-static-pricing/
      go.mod
      README.md
      main.go
      gateway-plugin.yaml
      tests/
```

Each plugin folder should be independently buildable.

Each plugin should expose:

* `README.md`
* `gateway-plugin.yaml`
* `main.go`
* tests
* example gateway config

---

## 4. Plugin Import Model

### 4.1 Supported Sources Only

The core must support importing plugins from only:

1. Filesystem
2. GitHub

No arbitrary URL downloads in v1.
No OCI registry in v1.
No npm/pip style registry in v1.

### 4.2 Filesystem Plugin Source

Config example:

```yaml
plugins:
  - name: local-openai
    source:
      type: filesystem
      path: ./plugins/upstream-openai/bin/upstream-openai
    capabilities:
      - upstream_provider
    config:
      base_url: https://api.openai.com/v1
      api_key: ${secret:OPENAI_API_KEY}
```

The filesystem source must:

* verify the file exists;
* verify it is executable;
* compute SHA256;
* optionally enforce configured checksum;
* refuse directories unless explicitly using a build mode later;
* fail with a helpful error.

Optional checksum:

```yaml
checksum: sha256:abc123...
```

### 4.3 GitHub Plugin Source

Support GitHub releases first.

Config example:

```yaml
plugins:
  - name: openai
    source:
      type: github
      repository: fenko/llm-gateway-plugins
      release: v0.1.0
      asset: upstream-openai_linux_amd64.tar.gz
      binary: upstream-openai
      checksum: sha256:abc123...
    capabilities:
      - upstream_provider
    config:
      base_url: https://api.openai.com/v1
      api_key: ${secret:OPENAI_API_KEY}
```

Implementation requirements:

* Download from GitHub Releases only.
* Cache downloaded plugins under a deterministic cache directory.
* Verify checksum if provided.
* Extract archive safely.
* Prevent path traversal during extraction.
* Refuse symlink tricks that escape extraction dir.
* Mark binary executable if needed.
* Do not download on every startup if cache hit and checksum matches.
* Support GitHub token via environment variable for private repos later, but do not require it.

Cache path suggestion:

```text
~/.llm-gateway/plugins/github/{owner}/{repo}/{release}/{asset}/{sha256}/
```

### 4.4 Plugin Manifest

A plugin binary must expose metadata at runtime via the plugin protocol.

A plugin package may also include a static `gateway-plugin.yaml` manifest for humans and tooling.

Example:

```yaml
api_version: gateway.fenko.dev/v1
kind: GatewayPlugin
name: upstream-openai
version: 0.1.0
entrypoint: upstream-openai
capabilities:
  - upstream_provider
config_schema:
  type: object
  required:
    - base_url
    - api_key
  properties:
    base_url:
      type: string
    api_key:
      type: string
      secret: true
permissions:
  network:
    outbound:
      - api.openai.com:443
  data:
    read_prompt: true
    read_response: true
  secrets:
    read:
      - OPENAI_API_KEY
```

The runtime protocol response is authoritative. The static file is for docs/tooling.

---

## 5. Configuration Model

### 5.1 Gateway Config

Example full config:

```yaml
server:
  listen: 127.0.0.1:8080
  request_timeout: 120s
  stream_idle_timeout: 60s

secrets:
  env:
    enabled: true

plugins:
  - name: authn-static
    source:
      type: filesystem
      path: ./bin/authn-static-api-key
    capabilities:
      - authenticator
    fail_mode: closed
    config:
      keys:
        - id: local-dev
          value: ${secret:DEV_API_KEY}
          tenant: dev

  - name: redis-limiter
    source:
      type: filesystem
      path: ./bin/ratelimit-redis
    capabilities:
      - rate_limiter
    fail_mode: closed
    config:
      redis_url: ${secret:REDIS_URL}
      default_rpm: 60

  - name: weighted-router
    source:
      type: filesystem
      path: ./bin/router-weighted
    capabilities:
      - router
    fail_mode: closed
    config:
      strategy: weighted

  - name: openai
    source:
      type: filesystem
      path: ./bin/upstream-openai
    capabilities:
      - upstream_provider
    fail_mode: closed
    config:
      base_url: https://api.openai.com/v1
      api_key: ${secret:OPENAI_API_KEY}

  - name: otel
    source:
      type: filesystem
      path: ./bin/observer-otel
    capabilities:
      - observer
    fail_mode: open
    config:
      endpoint: http://localhost:4318

pipeline:
  authenticator: authn-static
  authorizer: null
  rate_limiter: redis-limiter
  cache_provider: null
  prompt_provider: null
  router: weighted-router
  upstream_providers:
    - openai
  observers:
    - otel

models:
  aliases:
    gpt-4o-mini:
      candidates:
        - provider: openai
          model: gpt-4o-mini
          weight: 100
```

### 5.2 Zero Plugin Behavior

The core must boot with zero plugins only if explicitly allowed by config, but it must not serve inference unless required capabilities exist.

Example config:

```yaml
server:
  listen: 127.0.0.1:8080

plugins: []

pipeline: {}
```

Expected behavior:

* `/healthz` returns healthy process status.
* `/readyz` returns not ready.
* `/v1/chat/completions` returns a structured error explaining missing capabilities.
* Logs clearly say what is missing.

Example error:

```json
{
  "error": {
    "type": "gateway_configuration_error",
    "code": "missing_required_capabilities",
    "message": "Gateway cannot serve chat completions. Missing required capabilities: authenticator, router, upstream_provider.",
    "missing_capabilities": ["authenticator", "router", "upstream_provider"]
  }
}
```

Minimum required capabilities to serve `/v1/chat/completions`:

* `Authenticator`, unless anonymous mode is explicitly enabled.
* `Router`.
* At least one `UpstreamProvider`.

Optional capabilities:

* `Authorizer`
* `RateLimiter`
* `CacheProvider`
* `Observer`
* `PromptProvider`
* `CostProvider`
* `ModelRegistryProvider`

### 5.3 Anonymous Mode

For development, allow:

```yaml
auth:
  anonymous: true
```

Then `Authenticator` is not required.

Default must be `false`.

---

## 6. Plugin Process Lifecycle

### 6.1 Startup

At gateway startup:

1. Load config.
2. Resolve plugin sources.
3. Install/download plugins if needed.
4. Start plugin processes.
5. Establish gRPC connection over UDS.
6. Perform handshake.
7. Validate protocol version.
8. Fetch plugin metadata.
9. Fetch config schema.
10. Validate plugin config.
11. Send configure request.
12. Fetch capabilities.
13. Build capability registry.
14. Compile pipeline.
15. Mark ready if all required capabilities are satisfied.

### 6.2 Long-Lived Workers

Plugins are long-running server processes.

The core launches each plugin with:

* path to Unix socket or inherited file descriptor;
* plugin instance name;
* sandbox profile metadata;
* config path or config payload via protocol;
* no broad environment variable dump.

Possible env vars:

```text
GATEWAY_PLUGIN_SOCKET=/tmp/llm-gateway/plugins/openai.sock
GATEWAY_PLUGIN_INSTANCE=openai
GATEWAY_PLUGIN_PROTOCOL_VERSION=1
```

### 6.3 Health and Restart

Core must health-check plugins.

Health states:

* `starting`
* `ready`
* `degraded`
* `failed`
* `stopped`

If a critical plugin dies:

* mark gateway not ready;
* fail requests depending on capability fail mode;
* restart with exponential backoff;
* emit observer/system events.

Restart policy config:

```yaml
restart:
  max_attempts: 5
  initial_backoff: 500ms
  max_backoff: 30s
```

### 6.4 No Per-Request Startup

Add tests to prove plugin process launch count does not increase per request.

---

## 7. Plugin Protocol

Use protobuf/gRPC.

All RPCs must include:

* request ID;
* tenant ID if known;
* deadline;
* trace context;
* gateway version;
* plugin instance name.

### 7.1 Common Proto Types

Define in `plugin.proto`:

```protobuf
syntax = "proto3";

package gateway.v1;

option go_package = "github.com/fenko/llm-gateway/gen/go/gateway/v1;gatewayv1";

message CallContext {
  string request_id = 1;
  string tenant_id = 2;
  int64 deadline_unix_nano = 3;
  map<string, string> trace_context = 4;
  string gateway_version = 5;
  string plugin_instance = 6;
}

message Diagnostic {
  string severity = 1; // error, warning, info
  string code = 2;
  string message = 3;
  map<string, string> details = 4;
}

message SecretRef {
  string name = 1;
}

message JSONSchema {
  string schema_json = 1;
}

enum CapabilityType {
  CAPABILITY_TYPE_UNSPECIFIED = 0;
  CAPABILITY_TYPE_AUTHENTICATOR = 1;
  CAPABILITY_TYPE_AUTHORIZER = 2;
  CAPABILITY_TYPE_RATE_LIMITER = 3;
  CAPABILITY_TYPE_ROUTER = 4;
  CAPABILITY_TYPE_UPSTREAM_PROVIDER = 5;
  CAPABILITY_TYPE_CACHE_PROVIDER = 6;
  CAPABILITY_TYPE_OBSERVER = 7;
  CAPABILITY_TYPE_PROMPT_PROVIDER = 8;
  CAPABILITY_TYPE_COST_PROVIDER = 9;
  CAPABILITY_TYPE_MODEL_REGISTRY_PROVIDER = 10;
}

message CapabilityDescriptor {
  CapabilityType type = 1;
  string name = 2;
  string version = 3;
}

service PluginService {
  rpc Handshake(HandshakeRequest) returns (HandshakeResponse);
  rpc GetMetadata(GetMetadataRequest) returns (GetMetadataResponse);
  rpc GetConfigSchema(GetConfigSchemaRequest) returns (GetConfigSchemaResponse);
  rpc Configure(ConfigureRequest) returns (ConfigureResponse);
  rpc Health(HealthRequest) returns (HealthResponse);
  rpc Shutdown(ShutdownRequest) returns (ShutdownResponse);
}
```

### 7.2 Handshake

```protobuf
message HandshakeRequest {
  string gateway_version = 1;
  repeated int32 supported_protocol_versions = 2;
}

message HandshakeResponse {
  int32 selected_protocol_version = 1;
  string plugin_name = 2;
  string plugin_version = 3;
  repeated Diagnostic diagnostics = 4;
}
```

### 7.3 Metadata

```protobuf
message GetMetadataRequest {}

message GetMetadataResponse {
  string name = 1;
  string version = 2;
  string homepage = 3;
  repeated CapabilityDescriptor capabilities = 4;
  PluginPermissions permissions = 5;
}

message PluginPermissions {
  repeated string outbound_hosts = 1;
  repeated string secret_names = 2;
  DataPermissions data = 3;
}

message DataPermissions {
  bool read_prompt = 1;
  bool read_response = 2;
  bool modify_request = 3;
  bool modify_response = 4;
  bool read_headers = 5;
}
```

### 7.4 Configure

```protobuf
message ConfigureRequest {
  CallContext context = 1;
  string config_json = 2;
  map<string, string> resolved_secrets = 3;
}

message ConfigureResponse {
  repeated Diagnostic diagnostics = 1;
}
```

The core must validate config against schema before calling `Configure`.

Secrets should only be included if declared and permitted.

---

## 8. Capability Contracts

### 8.1 Authenticator

Purpose: establish identity.

Input must not include prompt body.

```protobuf
service AuthenticatorService {
  rpc Authenticate(AuthenticateRequest) returns (AuthenticateResponse);
}

message AuthenticateRequest {
  CallContext context = 1;
  map<string, string> headers = 2;
  string source_ip = 3;
  string method = 4;
  string path = 5;
}

message AuthenticateResponse {
  AuthDecision decision = 1;
  Identity identity = 2;
  string deny_reason = 3;
  repeated Diagnostic diagnostics = 4;
}

enum AuthDecision {
  AUTH_DECISION_UNSPECIFIED = 0;
  AUTH_DECISION_ALLOW = 1;
  AUTH_DECISION_DENY = 2;
}

message Identity {
  string subject = 1;
  string tenant_id = 2;
  repeated string groups = 3;
  map<string, string> claims = 4;
  string auth_method = 5;
}
```

Required behavior:

* Return `DENY` on invalid credentials.
* Return a stable tenant ID if available.
* Must respect deadlines.
* Must not require prompt contents.

Example plugins:

* static API key
* JWT/OIDC
* Cloudflare Access
* mTLS identity

### 8.2 Authorizer

Purpose: decide whether an authenticated identity can make a request.

```protobuf
service AuthorizerService {
  rpc Authorize(AuthorizeRequest) returns (AuthorizeResponse);
}

message AuthorizeRequest {
  CallContext context = 1;
  Identity identity = 2;
  RequestSummary request = 3;
}

message RequestSummary {
  string operation = 1; // chat.completions, models.list, embeddings, etc.
  string requested_model = 2;
  int64 estimated_input_tokens = 3;
  map<string, string> properties = 4;
}

message AuthorizeResponse {
  AuthzDecision decision = 1;
  string deny_reason = 2;
  RequestConstraints constraints = 3;
  repeated Diagnostic diagnostics = 4;
}

enum AuthzDecision {
  AUTHZ_DECISION_UNSPECIFIED = 0;
  AUTHZ_DECISION_ALLOW = 1;
  AUTHZ_DECISION_DENY = 2;
}

message RequestConstraints {
  repeated string allowed_models = 1;
  int64 max_output_tokens = 2;
  double max_estimated_cost_usd = 3;
  repeated string allowed_regions = 4;
}
```

Example plugins:

* OPA/Rego
* Cedar policy
* static YAML policy

### 8.3 RateLimiter

Purpose: enforce RPM/TPM/budget limits.

```protobuf
service RateLimiterService {
  rpc Check(CheckRateLimitRequest) returns (CheckRateLimitResponse);
  rpc Commit(CommitUsageRequest) returns (CommitUsageResponse);
}

message CheckRateLimitRequest {
  CallContext context = 1;
  Identity identity = 2;
  string model = 3;
  int64 estimated_input_tokens = 4;
  int64 requested_output_tokens = 5;
  map<string, string> properties = 6;
}

message CheckRateLimitResponse {
  RateLimitDecision decision = 1;
  string deny_reason = 2;
  int64 retry_after_ms = 3;
  RateLimitState state = 4;
}

enum RateLimitDecision {
  RATE_LIMIT_DECISION_UNSPECIFIED = 0;
  RATE_LIMIT_DECISION_ALLOW = 1;
  RATE_LIMIT_DECISION_DENY = 2;
  RATE_LIMIT_DECISION_DELAY = 3;
}

message RateLimitState {
  int64 request_limit = 1;
  int64 request_remaining = 2;
  int64 token_limit = 3;
  int64 token_remaining = 4;
  double budget_limit_usd = 5;
  double budget_remaining_usd = 6;
  int64 reset_unix_nano = 7;
}

message CommitUsageRequest {
  CallContext context = 1;
  Identity identity = 2;
  Usage usage = 3;
}

message CommitUsageResponse {
  repeated Diagnostic diagnostics = 1;
}
```

Must support:

* per key/user/tenant request limits;
* token limits;
* cost budgets;
* model-specific budgets;
* provider-specific budgets;
* tag/custom-property budgets.

### 8.4 Router

Purpose: choose upstream provider/model/fallback chain.

```protobuf
service RouterService {
  rpc Route(RouteRequest) returns (RouteResponse);
}

message RouteRequest {
  CallContext context = 1;
  Identity identity = 2;
  string requested_model = 3;
  RequestSummary request = 4;
  repeated RouteCandidate candidates = 5;
  RequestConstraints constraints = 6;
}

message RouteCandidate {
  string provider_instance = 1;
  string provider_model = 2;
  string logical_model = 3;
  int32 weight = 4;
  string region = 5;
  bool healthy = 6;
  double estimated_cost_per_1k_input = 7;
  double estimated_cost_per_1k_output = 8;
  map<string, string> properties = 9;
}

message RouteResponse {
  repeated SelectedRoute routes = 1;
  string reason = 2;
  repeated Diagnostic diagnostics = 3;
}

message SelectedRoute {
  string provider_instance = 1;
  string provider_model = 2;
  int32 priority = 3;
  map<string, string> properties = 4;
}
```

Must support through plugins/config:

* weighted routing;
* simple shuffle;
* fallback chains;
* provider budget-aware routing;
* model budget-aware routing;
* region-aware routing;
* latency-aware routing later;
* health-aware routing.

### 8.5 UpstreamProvider

Purpose: invoke actual LLM provider.

```protobuf
service UpstreamProviderService {
  rpc Invoke(InvokeRequest) returns (stream ResponseChunk);
  rpc ListModels(ListModelsRequest) returns (ListModelsResponse);
}

message InvokeRequest {
  CallContext context = 1;
  Identity identity = 2;
  string provider_model = 3;
  ChatCompletionRequest request = 4;
  map<string, string> properties = 5;
}

message ResponseChunk {
  oneof payload {
    ChatCompletionChunk chunk = 1;
    FinalUsage usage = 2;
    UpstreamError error = 3;
  }
}

message UpstreamError {
  string code = 1;
  string message = 2;
  int32 http_status = 3;
  bool retryable = 4;
  bool rate_limited = 5;
}
```

Requirements:

* Must support streaming.
* Must support cancellation.
* Must return normalized errors.
* Must expose retryability/rate-limit semantics.
* Must return usage metadata if provider returns it.
* Core handles fallback by trying next route if error is retryable and response has not started streaming to client.

### 8.6 CacheProvider

Purpose: implement response cache and prompt/provider cache metadata.

```protobuf
service CacheProviderService {
  rpc Lookup(CacheLookupRequest) returns (CacheLookupResponse);
  rpc Store(CacheStoreRequest) returns (CacheStoreResponse);
}

message CacheLookupRequest {
  CallContext context = 1;
  Identity identity = 2;
  string cache_key = 3;
  CachePolicy policy = 4;
}

message CacheLookupResponse {
  bool hit = 1;
  CachedResponse response = 2;
  string cache_id = 3;
}

message CacheStoreRequest {
  CallContext context = 1;
  Identity identity = 2;
  string cache_key = 3;
  CachedResponse response = 4;
  CachePolicy policy = 5;
}

message CachePolicy {
  int64 ttl_ms = 1;
  bool semantic = 2;
  map<string, string> vary = 3;
}
```

Must support via plugin ecosystem:

* exact response caching;
* semantic caching later;
* configurable TTL;
* cache hit/miss metadata;
* bypass rules.

### 8.7 Observer

Purpose: receive request lifecycle events.

Observer must be async/batched by default. It should not be called inline except when configured as blocking audit mode.

```protobuf
service ObserverService {
  rpc Emit(EventBatch) returns (EmitResponse);
}

message EventBatch {
  repeated GatewayEvent events = 1;
}

message GatewayEvent {
  string event_id = 1;
  string request_id = 2;
  string tenant_id = 3;
  string event_type = 4;
  int64 timestamp_unix_nano = 5;
  map<string, string> properties = 6;
  Usage usage = 7;
  ErrorSummary error = 8;
}
```

Events:

* `request.started`
* `auth.allowed`
* `auth.denied`
* `rate_limit.allowed`
* `rate_limit.denied`
* `cache.hit`
* `cache.miss`
* `route.selected`
* `upstream.started`
* `upstream.error`
* `upstream.completed`
* `request.completed`
* `request.failed`

Must support Helicone-like features through observer plugins:

* request logs;
* user/session analytics;
* cost tracking;
* custom properties;
* trace correlation;
* model/provider comparison data.

### 8.8 PromptProvider

Purpose: resolve prompt templates or prompt versions.

```protobuf
service PromptProviderService {
  rpc ResolvePrompt(ResolvePromptRequest) returns (ResolvePromptResponse);
}

message ResolvePromptRequest {
  CallContext context = 1;
  Identity identity = 2;
  string prompt_ref = 3;
  string version = 4;
  map<string, string> variables = 5;
}

message ResolvePromptResponse {
  repeated ChatMessage messages = 1;
  string resolved_version = 2;
  map<string, string> properties = 3;
}
```

This supports Helicone-style prompt management without making prompt mutation a generic hook.

### 8.9 CostProvider

Purpose: provide pricing and cost calculation.

```protobuf
service CostProviderService {
  rpc Estimate(EstimateCostRequest) returns (EstimateCostResponse);
  rpc Calculate(CalculateCostRequest) returns (CalculateCostResponse);
}
```

Must support:

* cost per provider/model;
* input/output token pricing;
* cached token pricing if available;
* custom enterprise pricing;
* budget routing input.

### 8.10 ModelRegistryProvider

Purpose: sync model metadata and capabilities.

```protobuf
service ModelRegistryProviderService {
  rpc ListModels(ListModelsRequest) returns (ListModelsResponse);
}
```

Model metadata should include:

* logical name;
* provider name;
* provider model ID;
* context window;
* supports streaming;
* supports tools;
* supports JSON mode;
* supports vision;
* region;
* health status;
* pricing fields if available.

---

## 9. HTTP API Surface

### 9.1 Minimum v1 Endpoints

Implement:

* `GET /healthz`
* `GET /readyz`
* `GET /v1/models`
* `POST /v1/chat/completions`

OpenAI-compatible enough for common SDKs.

### 9.2 Chat Completions

Support:

* non-streaming responses;
* streaming SSE responses;
* model field;
* messages;
* temperature;
* max tokens;
* tools/tool calls passthrough if upstream plugin supports it;
* custom metadata headers/properties.

Unsupported fields should be passed through to upstream provider plugins as provider params where possible, or rejected with clear diagnostics if strict mode is enabled.

### 9.3 Headers and Custom Properties

Support custom properties via headers:

```text
X-Gateway-Property-<Name>: <Value>
```

Core converts them into request properties for routing, rate limits, observers, and analytics.

Response headers should include when available:

```text
X-Gateway-Request-Id
X-Gateway-Cache: HIT|MISS|BYPASS
X-Gateway-Route-Provider
X-Gateway-Route-Model
X-Gateway-Fallback-Index
X-Gateway-RateLimit-Limit
X-Gateway-RateLimit-Remaining
X-Gateway-RateLimit-Reset
```

These headers intentionally mirror common gateway observability/rate-limit patterns.

---

## 10. Request Execution Flow

For `POST /v1/chat/completions`:

```text
1. Parse HTTP request.
2. Create request ID and trace context.
3. Normalize OpenAI-compatible request.
4. Extract request summary without copying full prompt where unnecessary.
5. Authenticate, unless anonymous mode is explicitly enabled.
6. Authorize if configured.
7. Resolve prompt if request references a managed prompt.
8. Estimate tokens/cost if possible.
9. Cache lookup if configured and request is cacheable.
10. Rate-limit check if configured.
11. Build route candidate list from model aliases and provider registry.
12. Call router.
13. Attempt selected upstream route.
14. If upstream fails before streaming starts and error is retryable, try fallback route.
15. Stream chunks to client as soon as possible.
16. Collect final usage.
17. Store cache entry if eligible.
18. Commit rate-limit usage/budget.
19. Emit observer events asynchronously.
20. Return completion/final stream.
```

Core must own fallback behavior. Router returns ordered candidates; core executes them.

---

## 11. Fallback Semantics

Fallback is only safe before response streaming has started.

Rules:

* If upstream fails before first response chunk, core may retry/fallback.
* If upstream fails after streaming started, core must terminate stream with error semantics and emit event.
* Core must track fallback index.
* Core must expose fallback index in response headers/events.
* Retry/fallback must respect overall request deadline.
* Retry/fallback must not violate rate/budget policy.

Fallback config:

```yaml
fallbacks:
  max_attempts: 3
  retry_on:
    - upstream_timeout
    - upstream_rate_limited
    - upstream_5xx
  do_not_retry_on:
    - auth_error
    - invalid_request
    - content_policy_block
```

---

## 12. Observability Architecture

### 12.1 Core Event Queue

Core should maintain a bounded event queue.

Config:

```yaml
observability:
  queue_size: 10000
  batch_size: 100
  flush_interval: 1s
  on_overflow: drop_oldest # or block/fail
```

Default observer fail mode: `open`.

Audit/compliance observer may be configured as blocking/fail-closed.

### 12.2 Metrics

Core should expose basic Prometheus-compatible metrics itself later, but v1 can expose internal metrics endpoint or leave Prometheus to observer plugin.

Recommended core metrics:

* request count
* request duration
* upstream duration
* time to first token
* tokens in/out
* cache hits/misses
* rate-limit allows/denies
* fallback attempts
* plugin RPC latency
* plugin restarts
* plugin health state

---

## 13. Security Model

### 13.1 Least Data to Plugins

Never send more data than required.

* Authenticator: headers/source info only; no prompt.
* Authorizer: identity + request summary; no full prompt by default.
* Router: candidates + summary; no full prompt by default.
* RateLimiter: identity + model + token/cost estimate; no prompt.
* Observer: sanitized event by default; raw prompt/response only if explicitly configured.
* UpstreamProvider: full request and provider credentials.
* CacheProvider: cache key and response only when configured.
* PromptProvider: prompt reference and variables.

### 13.2 Secret Scoping

Core owns secrets.

Plugins receive only explicitly permitted secrets.

Do not pass the whole environment to plugins.

Secret syntax:

```yaml
api_key: ${secret:OPENAI_API_KEY}
```

Core resolves this and sends only to plugins that declare and are configured to receive it.

### 13.3 Sandbox

Implement an abstraction in v1 even if some platforms get no-op sandbox initially.

Linux sandbox goals:

* no new privileges;
* cgroup CPU/memory limits;
* read-only filesystem where possible;
* restricted working directory;
* dropped capabilities;
* seccomp profile where available;
* network restrictions if practical;
* no arbitrary child processes by default.

Config example:

```yaml
sandbox:
  enabled: true
  memory_mb: 256
  cpu_quota: 0.5
  allow_network:
    - api.openai.com:443
  read_only_filesystem: true
  allow_child_processes: false
```

If full sandboxing is not implemented in first pass, code must have interfaces and tests for policy decisions so enforcement can be added without redesign.

### 13.4 Plugin Trust Levels

Add config field:

```yaml
trust: trusted | community | untrusted
```

v1 behavior:

* `trusted`: relaxed sandbox allowed.
* `community`: sandbox required if supported.
* `untrusted`: refuse to run unless strict sandbox available.

---

## 14. Error Model

Use consistent gateway errors.

```json
{
  "error": {
    "type": "gateway_error",
    "code": "upstream_unavailable",
    "message": "All configured upstream providers failed before response streaming started.",
    "request_id": "req_...",
    "details": {}
  }
}
```

Common codes:

* `missing_required_capabilities`
* `plugin_start_failed`
* `plugin_handshake_failed`
* `plugin_config_invalid`
* `authentication_failed`
* `authorization_denied`
* `rate_limit_exceeded`
* `model_not_found`
* `no_route_available`
* `upstream_unavailable`
* `upstream_rate_limited`
* `cache_error`
* `request_timeout`
* `stream_interrupted`

---

## 15. Testing Requirements

### 15.1 Unit Tests

Required packages with tests:

* config loading/validation
* plugin source parsing
* filesystem plugin resolution
* GitHub release URL construction/cache path logic
* safe archive extraction
* manifest parsing
* capability registry
* pipeline compilation
* missing capability diagnostics
* request execution happy path with fake plugins
* fallback behavior
* cache hit/miss behavior
* observer queue batching/drop behavior
* secret scoping
* deadline/cancellation propagation
* error mapping

### 15.2 Integration Tests

Create fake plugin binaries in test fixtures.

Test cases:

1. Gateway starts with zero plugins.

   * `/healthz` OK
   * `/readyz` not ready
   * chat completion returns missing capabilities

2. Gateway starts with fake auth/router/upstream plugins.

   * ready
   * chat completion works
   * upstream receives normalized request

3. Plugin process is started once, not per request.

   * send 100 requests
   * assert plugin launch count is 1

4. Fallback works before streaming.

   * first upstream returns retryable error
   * second upstream succeeds
   * response header has fallback index

5. No fallback after streaming begins.

   * upstream sends first chunk then error
   * core terminates stream and emits event

6. Observer is async.

   * observer blocks
   * request still completes if fail open

7. Blocking audit observer fail closed.

   * observer fails
   * request fails

8. Rate limit deny.

   * rate limiter denies
   * upstream not called

9. Cache hit.

   * cache returns response
   * upstream not called

10. Secret scoping.

* plugin only receives configured secrets

11. Config invalid.

* plugin schema rejects config
* gateway not ready

### 15.3 Benchmarks

Add benchmarks:

* pipeline execution with fake in-memory adapters;
* plugin RPC auth call over UDS;
* router RPC call over UDS;
* observer queue enqueue;
* streaming pass-through overhead;
* 1000 sequential requests against fake upstream;
* concurrent requests with warm plugins.

Benchmark names:

```go
BenchmarkPipelineNoPlugins
BenchmarkPipelineWarmPlugins
BenchmarkPluginRPCAuthenticateUDS
BenchmarkObserverEnqueue
BenchmarkStreamingPassthrough
```

### 15.4 Race Tests

Ensure `go test -race ./...` passes.

---

## 16. Documentation Requirements

Create docs:

### `README.md`

Include:

* what the gateway is;
* quickstart;
* zero plugin behavior;
* minimal working config;
* plugin architecture summary;
* current limitations.

### `docs/architecture.md`

Include:

* core vs plugins;
* data plane vs control plane;
* long-lived plugin process model;
* capability system;
* request flow diagram.

### `docs/plugin-authoring.md`

Include:

* how to write a plugin in Go;
* expected `main.go` shape;
* manifest;
* config schema;
* serving gRPC over UDS;
* capability implementation examples.

### `docs/capability-contracts.md`

Document each capability:

* purpose;
* input;
* output;
* whether it is hot path;
* whether it can see prompt;
* failure behavior;
* examples.

### `docs/plugin-security.md`

Include:

* plugin trust model;
* secret scoping;
* sandbox model;
* data minimization;
* fail open/closed;
* community plugin risks.

### `docs/configuration.md`

Include full YAML schema and examples.

### `docs/performance.md`

Include:

* why plugins are long-lived;
* no per-request startup;
* plugin pools;
* async observers;
* payload minimization;
* benchmarks.

### `docs/community-plugins.md`

Include:

* community repo layout;
* naming conventions;
* release asset conventions;
* checksum expectations;
* plugin folder template.

---

## 17. Idiomatic Go Requirements

* Use `context.Context` everywhere for request-scoped operations.
* Respect cancellation and deadlines.
* Keep interfaces small.
* Avoid global mutable state.
* Use structured logging via standard `slog` unless project chooses otherwise.
* Use `net/http` directly unless a router is genuinely needed.
* Use clear package boundaries.
* Avoid reflection-heavy magic.
* Avoid premature generics.
* Prefer table-driven tests.
* Return typed errors where useful.
* Wrap errors with context.
* Keep generated protobuf code isolated under `gen/`.
* Provide `make test`, `make lint`, `make proto`, `make build`.

Recommended dependencies:

* `google.golang.org/grpc`
* `google.golang.org/protobuf`
* YAML parser: `gopkg.in/yaml.v3`
* JSON schema validation: choose a maintained Go JSON Schema package
* `github.com/google/uuid` or similar for request IDs
* Optional Buf tooling for protobuf generation

Avoid unnecessary heavy dependencies.

---

## 18. Compatibility Feature Matrix

The plugin system must be able to implement the following through strict capabilities.

### LiteLLM-style features

| Feature                    | Core/Plugin Mapping                                       |
| -------------------------- | --------------------------------------------------------- |
| OpenAI-compatible endpoint | Core HTTP API                                             |
| Multiple provider support  | `UpstreamProvider` plugins                                |
| Model aliases/groups       | Core model alias config + `ModelRegistryProvider`         |
| Load balancing             | `Router` plugin                                           |
| Weighted routing           | `Router` plugin                                           |
| Fallbacks                  | Core executes fallback chain returned by `Router`         |
| Retry on provider errors   | Core retry/fallback policy + upstream error normalization |
| Per-user/key rate limits   | `RateLimiter` plugin                                      |
| Token limits               | `RateLimiter` plugin                                      |
| Budgets                    | `RateLimiter` + `CostProvider`                            |
| Provider/model/tag budgets | `RateLimiter` + `Router` + `CostProvider`                 |
| Cost tracking              | `CostProvider` + `Observer`                               |
| Provider health awareness  | Core health + `ModelRegistryProvider`/provider health     |
| Caching                    | `CacheProvider`                                           |

### Helicone-style features

| Feature                             | Core/Plugin Mapping                                                 |
| ----------------------------------- | ------------------------------------------------------------------- |
| Request logging                     | `Observer` plugin                                                   |
| Response logging                    | `Observer` plugin with explicit permission                          |
| LLM observability dashboard backend | `Observer` plugin                                                   |
| Caching                             | `CacheProvider`                                                     |
| Prompt caching metadata             | `CacheProvider` and provider-specific metadata                      |
| Rate limiting                       | `RateLimiter` plugin                                                |
| Cost tracking                       | `CostProvider` + `Observer`                                         |
| User analytics                      | `Observer` events with identity/tenant/session properties           |
| Custom properties                   | Core header/property extraction + `Observer`/`RateLimiter`/`Router` |
| Prompt templates/versioning         | `PromptProvider` plugin                                             |
| Gateway provider abstraction        | `UpstreamProvider` plugins                                          |
| Fallback index / route metadata     | Core headers/events                                                 |

---

## 19. Initial Implementation Milestones

### Milestone 1: Skeleton Core

Deliver:

* repo structure;
* config loader;
* HTTP server;
* `/healthz`, `/readyz`;
* OpenAI-compatible chat endpoint stub;
* zero plugin missing capability behavior;
* docs skeleton;
* tests for zero plugin mode.

### Milestone 2: Plugin Runtime

Deliver:

* protobuf common protocol;
* plugin process manager;
* UDS gRPC connection;
* handshake;
* metadata/config schema/configure/health;
* filesystem plugin source;
* fake plugin fixture;
* tests proving long-lived plugin startup.

### Milestone 3: Core Capabilities

Deliver:

* Authenticator;
* Router;
* UpstreamProvider;
* pipeline compilation;
* fake auth/router/upstream plugins;
* non-streaming chat completion through fake upstream;
* integration tests.

### Milestone 4: Streaming and Fallback

Deliver:

* streaming `UpstreamProvider.Invoke`;
* SSE streaming to client;
* retry/fallback before first chunk;
* no fallback after stream start;
* tests.

### Milestone 5: Rate Limit, Cache, Observer

Deliver:

* RateLimiter capability;
* CacheProvider capability;
* Observer capability;
* async event queue;
* usage commit;
* tests.

### Milestone 6: GitHub Plugin Import

Deliver:

* GitHub release download;
* checksum verify;
* safe extraction;
* plugin cache;
* tests with mocked downloader.

### Milestone 7: Docs and Community Plugin Template

Deliver:

* full docs;
* example plugin template;
* example configs;
* community repo bootstrap instructions.

---

## 20. Acceptance Criteria

The implementation is acceptable when:

1. `go test ./...` passes.
2. `go test -race ./...` passes.
3. Gateway can boot with zero plugins and returns helpful missing capability errors.
4. Gateway can boot with fake filesystem plugins and complete a chat request.
5. Plugin startup happens at boot, not per request.
6. Streaming responses work through a fake upstream provider.
7. Fallback works before streaming begins.
8. Observer plugin can be async and fail-open.
9. Rate limiter can deny before upstream invocation.
10. Cache hit bypasses upstream invocation.
11. GitHub plugin source resolver is implemented and tested without requiring live GitHub in unit tests.
12. Documentation explains how to write and package plugins.
13. The code is idiomatic Go and cleanly separated by package.

---

## 21. Non-Goals for v1

Do not implement in v1 unless trivial:

* WASM plugin runtime.
* OCI plugin registry.
* Arbitrary HTTP plugin webhooks.
* Generic middleware hooks.
* Plugin UI/dashboard.
* Full semantic caching.
* Full prompt/content guardrail system.
* Multi-node distributed control plane.
* Kubernetes operator.
* Hot reload of plugin binaries without restart.

Design should not preclude these, but do not implement them now.

---

## 22. Codex Working Instructions

When implementing:

1. Start with the repository skeleton and config model.
2. Implement zero-plugin mode first.
3. Implement plugin protocol and fake plugin fixtures before real plugins.
4. Keep the core independent from concrete plugin implementations.
5. Add tests with each feature.
6. Do not add generic hooks.
7. Do not start plugins per request.
8. Do not let observer plugins block the request path unless configured as blocking/fail-closed.
9. Keep all plugin capability interfaces strict.
10. Ensure docs stay in sync with code.

Prefer small, reviewable commits:

```text
1. repo skeleton + config + health endpoints
2. zero plugin readiness and errors
3. proto definitions + generated code
4. plugin process manager + fake plugin
5. capability registry + pipeline compile
6. fake auth/router/upstream integration
7. streaming and fallback
8. observer queue
9. rate limiter/cache capabilities
10. github/filesystem plugin importer
11. docs and examples
```

---

## 23. Final Architecture Summary

This project should be Terraform-inspired only in the sense that plugins are external, versioned, schema-declaring, capability-providing executables.

It must not inherit Terraform's slow command-oriented runtime model.

The final architecture is:

```text
Small Go core
  - OpenAI-compatible API
  - strict pipeline
  - plugin lifecycle manager
  - data plane/control plane separation
  - request streaming
  - fallback execution
  - events and usage

Long-lived plugin workers
  - gRPC over Unix sockets
  - typed capabilities
  - config schema
  - metadata and permissions
  - supervised and sandboxable

Community ecosystem
  - plugins imported from filesystem or GitHub
  - each plugin folder implements one or more strict capabilities
  - no generic hooks
```

This gives the project the ability to reproduce the useful surface area of LiteLLM and Helicone while staying small, strict, idiomatic, and security-conscious.

