package httpapi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/FenkoHQ/vulpes-core/internal/capabilities"
	"github.com/FenkoHQ/vulpes-core/internal/pipeline"
)

type Server struct {
	pipeline *pipeline.Pipeline
	logger   *slog.Logger
}

func New(p *pipeline.Pipeline, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{pipeline: p, logger: logger}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.HandleFunc("GET /v1/models", s.models)
	mux.HandleFunc("POST /v1/chat/completions", s.chat)
	return mux
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	if !s.pipeline.Ready() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "not_ready", "missing_capabilities": capsToStrings(s.pipeline.MissingCapabilities())})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
}
func (s *Server) models(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": s.pipeline.ListModels(r.Context())})
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req capabilities.ChatCompletionRequest
	if err := decodeChatRequest(r, &req); err != nil {
		writeError(w, "", pipeline.GatewayError{Type: "invalid_request_error", Code: "invalid_request", Message: err.Error(), Status: 400})
		return
	}
	res, err := s.pipeline.ExecuteChat(r.Context(), req, requestHeaders(r), sourceIP(r))
	for k, v := range res.Headers {
		w.Header().Set(k, v)
	}
	if err != nil {
		writeError(w, res.RequestID, err)
		return
	}
	if req.Stream {
		s.writeStream(w, res.Stream)
		return
	}
	writeJSON(w, http.StatusOK, res.Response)
}

func decodeChatRequest(r *http.Request, out *capabilities.ChatCompletionRequest) error {
	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return err
	}
	b, _ := json.Marshal(raw)
	if err := json.Unmarshal(b, out); err != nil {
		return err
	}
	out.Extra = map[string]any{}
	known := map[string]bool{"model": true, "messages": true, "stream": true, "temperature": true, "max_tokens": true, "tools": true, "tool_choice": true, "metadata": true}
	for k, v := range raw {
		if !known[k] {
			out.Extra[k] = v
		}
	}
	if out.Model == "" {
		return fmt.Errorf("model is required")
	}
	if len(out.Messages) == 0 { /* prompt provider may fill */
	}
	return nil
}

func (s *Server) writeStream(w http.ResponseWriter, ch <-chan capabilities.ResponseChunk) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	bw := bufio.NewWriter(w)
	for chunk := range ch {
		if chunk.Error != nil {
			b, _ := json.Marshal(map[string]any{"error": chunk.Error})
			_, _ = fmt.Fprintf(bw, "event: error\ndata: %s\n\n", b)
			_ = bw.Flush()
			if flusher != nil {
				flusher.Flush()
			}
			return
		}
		if chunk.Chunk != nil {
			b, _ := json.Marshal(chunk.Chunk)
			_, _ = fmt.Fprintf(bw, "data: %s\n\n", b)
			_ = bw.Flush()
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
	_, _ = bw.WriteString("data: [DONE]\n\n")
	_ = bw.Flush()
	if flusher != nil {
		flusher.Flush()
	}
}

func requestHeaders(r *http.Request) map[string]string {
	h := map[string]string{}
	for k, vs := range r.Header {
		h[k] = strings.Join(vs, ",")
	}
	return h
}
func sourceIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "")
	_ = enc.Encode(v)
}

func NewHTTPServer(addr string, handler http.Handler, timeout time.Duration) *http.Server {
	return &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: timeout, WriteTimeout: timeout}
}
