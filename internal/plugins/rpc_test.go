package plugins

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/rpc"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FenkoHQ/vulpes-core/internal/capabilities"
)

// fakeService mirrors the SDK's streaming RPC surface: InvokeStart kicks off
// a producer that writes to a per-stream channel, InvokeNext blocks until a
// chunk arrives, InvokeCancel cancels the producer. Tests inject the chunk
// stream via the `chunks` channel so they can control exactly when and how
// many chunks arrive.
type fakeService struct {
	mu         sync.Mutex
	invokeReqs []capabilities.InvokeRequest

	// chunkSource produces the chunks for the next InvokeStart call.
	chunkSource func(ctx context.Context, out chan<- capabilities.ResponseChunk)

	streams sync.Map // streamID -> *fakeStream
	nextID  atomic.Uint64
}

type fakeStream struct {
	ch     chan capabilities.ResponseChunk
	cancel context.CancelFunc
}

type FakeInvokeStartResponse struct{ StreamID uint64 }
type FakeInvokeNextRequest struct{ StreamID uint64 }
type FakeInvokeNextResponse struct {
	Chunk capabilities.ResponseChunk
	EOF   bool
}
type FakeInvokeCancelRequest struct{ StreamID uint64 }
type FakeInvokeCancelResponse struct{}

func (s *fakeService) InvokeStart(req capabilities.InvokeRequest, resp *FakeInvokeStartResponse) error {
	s.mu.Lock()
	s.invokeReqs = append(s.invokeReqs, req)
	source := s.chunkSource
	s.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan capabilities.ResponseChunk, 32)
	id := s.nextID.Add(1)
	s.streams.Store(id, &fakeStream{ch: ch, cancel: cancel})
	if source == nil {
		source = func(ctx context.Context, out chan<- capabilities.ResponseChunk) {
			out <- capabilities.ResponseChunk{Chunk: &capabilities.ChatCompletionChunk{Model: req.Request.Model, Choices: []capabilities.ChatChoice{{Message: capabilities.ChatMessage{Role: "assistant", Content: "ok"}}}}}
		}
	}
	go func() {
		defer close(ch)
		defer cancel()
		source(ctx, ch)
	}()
	resp.StreamID = id
	return nil
}

func (s *fakeService) InvokeNext(req FakeInvokeNextRequest, resp *FakeInvokeNextResponse) error {
	v, ok := s.streams.Load(req.StreamID)
	if !ok {
		return errors.New("unknown stream")
	}
	stream := v.(*fakeStream)
	chunk, ok := <-stream.ch
	if !ok {
		s.streams.Delete(req.StreamID)
		resp.EOF = true
		return nil
	}
	resp.Chunk = chunk
	return nil
}

func (s *fakeService) InvokeCancel(req FakeInvokeCancelRequest, resp *FakeInvokeCancelResponse) error {
	v, ok := s.streams.LoadAndDelete(req.StreamID)
	if !ok {
		return nil
	}
	v.(*fakeStream).cancel()
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

// TestInvoke_RealStreaming is the proof that the gateway forwards chunks as
// they arrive at the plugin — not after the upstream stream closes. The fake
// plugin emits chunk #1, waits 200ms, then emits chunks #2 and #3. The test
// asserts the gateway delivers chunk #1 before that 200ms gap closes; with
// the old buffered Invoke it could not arrive until after all three were
// produced.
func TestInvoke_RealStreaming(t *testing.T) {
	svc := &fakeService{
		chunkSource: func(ctx context.Context, out chan<- capabilities.ResponseChunk) {
			out <- capabilities.ResponseChunk{Chunk: &capabilities.ChatCompletionChunk{Choices: []capabilities.ChatChoice{{Delta: capabilities.ChatMessage{Content: "one"}}}}}
			select {
			case <-time.After(200 * time.Millisecond):
			case <-ctx.Done():
				return
			}
			out <- capabilities.ResponseChunk{Chunk: &capabilities.ChatCompletionChunk{Choices: []capabilities.ChatChoice{{Delta: capabilities.ChatMessage{Content: "two"}}}}}
			out <- capabilities.ResponseChunk{Chunk: &capabilities.ChatCompletionChunk{Choices: []capabilities.ChatChoice{{Delta: capabilities.ChatMessage{Content: "three"}}}}}
		},
	}
	sock := startFakePlugin(t, svc)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := DialRPCPlugin(ctx, "unix", sock, "fake")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	start := time.Now()
	ch, err := client.Invoke(ctx, capabilities.InvokeRequest{Request: capabilities.ChatCompletionRequest{Model: "alias1"}})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}

	first, ok := <-ch
	if !ok || first.Chunk == nil {
		t.Fatalf("expected first chunk")
	}
	firstLatency := time.Since(start)
	// Allow generous slack on slow CI, but the whole point of streaming is
	// that the first chunk arrives well before the producer's 200ms gap.
	if firstLatency > 150*time.Millisecond {
		t.Fatalf("first chunk took %v — gateway is buffering the whole response (old behavior)", firstLatency)
	}
	if got, want := contentOf(first), "one"; got != want {
		t.Fatalf("first chunk = %q, want %q", got, want)
	}

	var rest []string
	for chunk := range ch {
		rest = append(rest, contentOf(chunk))
	}
	if fmt.Sprint(rest) != "[two three]" {
		t.Fatalf("rest = %v, want [two three]", rest)
	}
}

// TestInvoke_CancelStopsProducer verifies that cancelling the gateway-side
// context propagates back to the plugin's producer via InvokeCancel — the
// producer's ctx.Done() fires and it stops emitting chunks rather than
// running to completion against a disconnected client.
func TestInvoke_CancelStopsProducer(t *testing.T) {
	producerStopped := make(chan struct{})
	svc := &fakeService{
		chunkSource: func(ctx context.Context, out chan<- capabilities.ResponseChunk) {
			defer close(producerStopped)
			out <- capabilities.ResponseChunk{Chunk: &capabilities.ChatCompletionChunk{Choices: []capabilities.ChatChoice{{Delta: capabilities.ChatMessage{Content: "hello"}}}}}
			select {
			case <-time.After(2 * time.Second):
				out <- capabilities.ResponseChunk{Chunk: &capabilities.ChatCompletionChunk{Choices: []capabilities.ChatChoice{{Delta: capabilities.ChatMessage{Content: "late"}}}}}
			case <-ctx.Done():
				// Expected: cancel propagated back through InvokeCancel.
			}
		},
	}
	sock := startFakePlugin(t, svc)

	ctx, cancel := context.WithCancel(context.Background())
	client, err := DialRPCPlugin(context.Background(), "unix", sock, "fake")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	ch, err := client.Invoke(ctx, capabilities.InvokeRequest{Request: capabilities.ChatCompletionRequest{Model: "alias1"}})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if _, ok := <-ch; !ok {
		t.Fatalf("expected first chunk before cancel")
	}
	cancel()

	select {
	case <-producerStopped:
	case <-time.After(1 * time.Second):
		t.Fatalf("producer never stopped — InvokeCancel did not propagate")
	}
}

func contentOf(c capabilities.ResponseChunk) string {
	if c.Chunk == nil || len(c.Chunk.Choices) == 0 {
		return ""
	}
	v := c.Chunk.Choices[0].Delta.Content
	if s, ok := v.(string); ok {
		return s
	}
	if s, ok := c.Chunk.Choices[0].Message.Content.(string); ok {
		return s
	}
	return ""
}
