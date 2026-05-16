package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/FenkoHQ/vulpes-core/internal/capabilities"
	"github.com/FenkoHQ/vulpes-core/internal/config"
	"github.com/FenkoHQ/vulpes-core/internal/httpapi"
	"github.com/FenkoHQ/vulpes-core/internal/models"
	"github.com/FenkoHQ/vulpes-core/internal/pipeline"
	"github.com/FenkoHQ/vulpes-core/internal/plugins"
	"github.com/FenkoHQ/vulpes-core/internal/testutil"
)

func baseConfig() config.Config {
	cfg := config.Config{
		Auth:     config.AuthConfig{Anonymous: false},
		Pipeline: config.PipelineConfig{Authenticator: "auth", Router: "router", UpstreamProviders: []string{"up1"}},
		Models:   config.ModelsConfig{Aliases: map[string]config.ModelAlias{"gpt-test": {Candidates: []config.ModelCandidate{{Provider: "up1", Model: "fake-model", Weight: 100}}}}},
	}
	config.ApplyDefaults(&cfg)
	return cfg
}

func compileTestPipeline(t *testing.T, cfg config.Config, setup func(*plugins.Registry)) *pipeline.Pipeline {
	t.Helper()
	reg := plugins.NewRegistry()
	setup(reg)
	p, err := pipeline.Compile(cfg, reg, models.NewRegistry(cfg.Models))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestZeroPluginMode(t *testing.T) {
	cfg := config.Config{Pipeline: config.PipelineConfig{}, Plugins: []config.PluginConfig{}, Models: config.ModelsConfig{Aliases: map[string]config.ModelAlias{}}}
	config.ApplyDefaults(&cfg)
	p, err := pipeline.Compile(cfg, plugins.NewRegistry(), models.NewRegistry(cfg.Models))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(httpapi.New(p, nil).Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("healthz status %d", resp.StatusCode)
	}
	resp, err = http.Get(srv.URL + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 503 {
		t.Fatalf("readyz status %d", resp.StatusCode)
	}
	resp = postChat(t, srv.URL, `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != 503 {
		t.Fatalf("chat status %d", resp.StatusCode)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if !strings.Contains(body["error"].(map[string]any)["code"].(string), "missing_required_capabilities") {
		t.Fatalf("unexpected body %#v", body)
	}
}

func TestChatCompletionHappyPath(t *testing.T) {
	cfg := baseConfig()
	up := &testutil.Upstream{Text: "hello"}
	p := compileTestPipeline(t, cfg, func(r *plugins.Registry) {
		r.AddAuthenticator("auth", &testutil.Authenticator{Allow: true})
		r.AddRouter("router", &testutil.Router{})
		r.AddUpstreamProvider("up1", up)
	})
	srv := httptest.NewServer(httpapi.New(p, nil).Handler())
	defer srv.Close()
	resp := postChat(t, srv.URL, `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if up.Calls.Load() != 1 {
		t.Fatalf("upstream calls = %d", up.Calls.Load())
	}
}

func TestRateLimitDenySkipsUpstream(t *testing.T) {
	cfg := baseConfig()
	cfg.Pipeline.RateLimiter = "rl"
	up := &testutil.Upstream{}
	p := compileTestPipeline(t, cfg, func(r *plugins.Registry) {
		r.AddAuthenticator("auth", &testutil.Authenticator{Allow: true})
		r.AddRouter("router", &testutil.Router{})
		r.AddRateLimiter("rl", &testutil.RateLimiter{Deny: true})
		r.AddUpstreamProvider("up1", up)
	})
	srv := httptest.NewServer(httpapi.New(p, nil).Handler())
	defer srv.Close()
	resp := postChat(t, srv.URL, `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != 429 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if up.Calls.Load() != 0 {
		t.Fatalf("upstream called")
	}
}

func TestCacheHitSkipsUpstream(t *testing.T) {
	cfg := baseConfig()
	cfg.Pipeline.CacheProvider = "cache"
	up := &testutil.Upstream{}
	p := compileTestPipeline(t, cfg, func(r *plugins.Registry) {
		r.AddAuthenticator("auth", &testutil.Authenticator{Allow: true})
		r.AddRouter("router", &testutil.Router{})
		r.AddCacheProvider("cache", &testutil.Cache{Hit: true})
		r.AddUpstreamProvider("up1", up)
	})
	srv := httptest.NewServer(httpapi.New(p, nil).Handler())
	defer srv.Close()
	resp := postChat(t, srv.URL, `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Gateway-Cache"); got != "HIT" {
		t.Fatalf("cache header %q", got)
	}
	if up.Calls.Load() != 0 {
		t.Fatalf("upstream called")
	}
}

func TestFallbackBeforeStreaming(t *testing.T) {
	cfg := baseConfig()
	cfg.Pipeline.UpstreamProviders = []string{"up1", "up2"}
	cfg.Models.Aliases["gpt-test"] = config.ModelAlias{Candidates: []config.ModelCandidate{{Provider: "up1", Model: "m1"}, {Provider: "up2", Model: "m2"}}}
	up1 := &testutil.Upstream{}
	up1.FailFirst.Store(true)
	up2 := &testutil.Upstream{Text: "ok"}
	p := compileTestPipeline(t, cfg, func(r *plugins.Registry) {
		r.AddAuthenticator("auth", &testutil.Authenticator{Allow: true})
		r.AddRouter("router", &testutil.Router{})
		r.AddUpstreamProvider("up1", up1)
		r.AddUpstreamProvider("up2", up2)
	})
	srv := httptest.NewServer(httpapi.New(p, nil).Handler())
	defer srv.Close()
	resp := postChat(t, srv.URL, `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Gateway-Fallback-Index"); got != "1" {
		t.Fatalf("fallback index %q", got)
	}
}

func TestStreaming(t *testing.T) {
	cfg := baseConfig()
	p := compileTestPipeline(t, cfg, func(r *plugins.Registry) {
		r.AddAuthenticator("auth", &testutil.Authenticator{Allow: true})
		r.AddRouter("router", &testutil.Router{})
		r.AddUpstreamProvider("up1", &testutil.Upstream{Text: "hi"})
	})
	srv := httptest.NewServer(httpapi.New(p, nil).Handler())
	defer srv.Close()
	resp := postChat(t, srv.URL, `{"model":"gpt-test","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	if !strings.Contains(buf.String(), "[DONE]") {
		t.Fatalf("stream did not finish: %s", buf.String())
	}
}

func TestObserverFailOpenDoesNotBlock(t *testing.T) {
	cfg := baseConfig()
	cfg.Pipeline.Observers = []string{"obs"}
	cfg.Observability.FlushInterval = config.Duration(10 * time.Millisecond)
	p := compileTestPipeline(t, cfg, func(r *plugins.Registry) {
		r.AddAuthenticator("auth", &testutil.Authenticator{Allow: true})
		r.AddRouter("router", &testutil.Router{})
		r.AddObserver("obs", &testutil.Observer{Block: true})
		r.AddUpstreamProvider("up1", &testutil.Upstream{})
	})
	srv := httptest.NewServer(httpapi.New(p, nil).Handler())
	defer srv.Close()
	start := time.Now()
	resp := postChat(t, srv.URL, `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("observer blocked request")
	}
}

func TestBlockingObserverFailClosed(t *testing.T) {
	cfg := baseConfig()
	cfg.Pipeline.BlockingObservers = []string{"audit"}
	p := compileTestPipeline(t, cfg, func(r *plugins.Registry) {
		r.AddAuthenticator("auth", &testutil.Authenticator{Allow: true})
		r.AddRouter("router", &testutil.Router{})
		r.AddObserver("audit", &testutil.Observer{Fail: true})
		r.AddUpstreamProvider("up1", &testutil.Upstream{})
	})
	srv := httptest.NewServer(httpapi.New(p, nil).Handler())
	defer srv.Close()
	resp := postChat(t, srv.URL, `{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != 500 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func postChat(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

var _ = capabilities.ChatCompletionRequest{}
