package plugins

import (
	"context"
	"net"
	"net/rpc"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/FenkoHQ/vulpes-core/internal/capabilities"
)

// fakeService mimics the surface upstream-openai exposes over net/rpc. It only
// needs Invoke to exercise the gob-encoding path that previously panicked when
// ChatMessage.Content carried a []any decoded from JSON.
type fakeService struct {
	mu         sync.Mutex
	invokeReqs []capabilities.InvokeRequest
}

func (s *fakeService) Invoke(req capabilities.InvokeRequest, resp *[]capabilities.ResponseChunk) error {
	s.mu.Lock()
	s.invokeReqs = append(s.invokeReqs, req)
	s.mu.Unlock()
	*resp = []capabilities.ResponseChunk{{
		Chunk: &capabilities.ChatCompletionChunk{
			Model:   req.Request.Model,
			Choices: []capabilities.ChatChoice{{Index: 0, Message: capabilities.ChatMessage{Role: "assistant", Content: "ok"}}},
		},
	}}
	return nil
}

func startFakePlugin(t *testing.T, svc *fakeService) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "plugin.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := rpc.NewServer()
	if err := srv.RegisterName("Plugin", svc); err != nil {
		t.Fatalf("register: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go srv.ServeConn(conn)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return sock
}

// TestInvoke_InterfaceContentEncodes guards the regression where a pi-style
// request — ChatMessage.Content as []any decoded from JSON — would panic
// gob encoding because the concrete type was not registered in the gateway
// process, killing the rpc.Client with "connection is shut down".
func TestInvoke_InterfaceContentEncodes(t *testing.T) {
	svc := &fakeService{}
	sock := startFakePlugin(t, svc)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := DialRPCPlugin(ctx, "unix", sock, "fake")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	req := capabilities.InvokeRequest{
		Request: capabilities.ChatCompletionRequest{
			Model: "alias1",
			Messages: []capabilities.ChatMessage{
				{Role: "system", Content: "you are helpful"},
				{Role: "user", Content: []any{
					map[string]any{"type": "text", "text": "reply with vulpes-ok"},
				}},
			},
			Tools:    []any{map[string]any{"type": "function", "function": map[string]any{"name": "t"}}},
			Metadata: map[string]any{"trace_id": "abc"},
		},
	}

	ch, err := client.Invoke(ctx, req)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	var got int
	for chunk := range ch {
		if chunk.Chunk != nil {
			got++
		}
	}
	if got == 0 {
		t.Fatalf("expected at least one chunk")
	}

	// A second call must also succeed: if encoding had panicked on the first,
	// the rpc.Client would now return ErrShutdown on every subsequent call.
	if _, err := client.Invoke(ctx, req); err != nil {
		t.Fatalf("second invoke: %v", err)
	}
}

// TestInvoke_ReconnectsAfterShutdown forces the rpc.Client into ErrShutdown
// by closing it under the manager's feet, then verifies the next call
// transparently redials the still-listening plugin.
func TestInvoke_ReconnectsAfterShutdown(t *testing.T) {
	svc := &fakeService{}
	sock := startFakePlugin(t, svc)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := DialRPCPlugin(ctx, "unix", sock, "fake")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	req := capabilities.InvokeRequest{Request: capabilities.ChatCompletionRequest{Model: "alias1"}}
	if _, err := client.Invoke(ctx, req); err != nil {
		t.Fatalf("first invoke: %v", err)
	}

	// Simulate the connection dying mid-flight.
	if err := client.getClient().Close(); err != nil {
		t.Fatalf("close underlying: %v", err)
	}

	if _, err := client.Invoke(ctx, req); err != nil {
		t.Fatalf("invoke after shutdown should redial: %v", err)
	}
}
