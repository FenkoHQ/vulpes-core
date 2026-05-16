package pipeline

import (
	"context"
	"testing"

	"github.com/FenkoHQ/vulpes-core/internal/capabilities"
	"github.com/FenkoHQ/vulpes-core/internal/config"
	"github.com/FenkoHQ/vulpes-core/internal/models"
	"github.com/FenkoHQ/vulpes-core/internal/plugins"
	"github.com/FenkoHQ/vulpes-core/internal/testutil"
)

func benchPipeline() *Pipeline {
	cfg := config.Config{Auth: config.AuthConfig{Anonymous: true}, Pipeline: config.PipelineConfig{Router: "router", UpstreamProviders: []string{"up"}}, Models: config.ModelsConfig{Aliases: map[string]config.ModelAlias{"m": {Candidates: []config.ModelCandidate{{Provider: "up", Model: "m"}}}}}}
	config.ApplyDefaults(&cfg)
	reg := plugins.NewRegistry()
	reg.AddRouter("router", &testutil.Router{})
	reg.AddUpstreamProvider("up", &testutil.Upstream{Text: "hello"})
	p, _ := Compile(cfg, reg, models.NewRegistry(cfg.Models))
	return p
}

func BenchmarkPipelineNoPlugins(b *testing.B) {
	cfg := config.Config{}
	config.ApplyDefaults(&cfg)
	p, _ := Compile(cfg, plugins.NewRegistry(), models.NewRegistry(cfg.Models))
	for i := 0; i < b.N; i++ {
		_, _ = p.ExecuteChat(context.Background(), testReq(), nil, "")
	}
}

func BenchmarkPipelineWarmPlugins(b *testing.B) {
	p := benchPipeline()
	for i := 0; i < b.N; i++ {
		_, _ = p.ExecuteChat(context.Background(), testReq(), nil, "")
	}
}

func BenchmarkObserverEnqueue(b *testing.B) {
	p := benchPipeline()
	for i := 0; i < b.N; i++ {
		p.emit("x", "r", "t", nil, nil)
	}
}

func testReq() capabilities.ChatCompletionRequest {
	return capabilities.ChatCompletionRequest{Model: "m", Messages: []capabilities.ChatMessage{{Role: "user", Content: "hi"}}}
}
