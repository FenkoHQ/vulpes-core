package plugins

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"time"
)

type Process struct {
	Name     string
	Path     string
	Socket   string
	cmd      *exec.Cmd
	launches atomic.Int64
	logger   *slog.Logger
}

func NewProcess(name, path string, logger *slog.Logger) *Process {
	if logger == nil {
		logger = slog.Default()
	}
	return &Process{Name: name, Path: path, logger: logger}
}

func (p *Process) Start(ctx context.Context) error {
	dir := filepath.Join(os.TempDir(), "llm-gateway", "plugins")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	p.Socket = filepath.Join(dir, fmt.Sprintf("%s-%d.sock", p.Name, time.Now().UnixNano()))
	_ = os.Remove(p.Socket)
	cmd := exec.Command(p.Path)
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"GATEWAY_PLUGIN_SOCKET=" + p.Socket,
		"GATEWAY_PLUGIN_INSTANCE=" + p.Name,
		"GATEWAY_PLUGIN_PROTOCOL_VERSION=1",
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start plugin %s: %w", p.Name, err)
	}
	p.cmd = cmd
	p.launches.Add(1)
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", p.Socket, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return fmt.Errorf("plugin %s did not create socket %s", p.Name, p.Socket)
}

func (p *Process) Stop(ctx context.Context) error {
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	_ = p.cmd.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
	select {
	case <-ctx.Done():
		_ = p.cmd.Process.Kill()
		return ctx.Err()
	case <-time.After(3 * time.Second):
		_ = p.cmd.Process.Kill()
		return nil
	case err := <-done:
		return err
	}
}
func (p *Process) LaunchCount() int64 { return p.launches.Load() }
