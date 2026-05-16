# Vulpes Core LLM Gateway

A small OpenAI-compatible LLM gateway core with strict, capability-based external plugins.

## Status

This repository contains the gateway core, strict capability interfaces, plugin process/source management, OpenAI-compatible HTTP endpoints, zero-plugin diagnostics, routing/fallback execution, async observers, rate-limit/cache integration points, GitHub/filesystem plugin source resolvers, tests, examples, and protocol definitions.

## Quickstart

```bash
go test ./...
go run ./cmd/gateway -config examples/minimal-zero-plugins/gateway.yaml
```

With zero plugins the process is healthy, but not ready for inference:

- `GET /healthz` -> `200`
- `GET /readyz` -> `503`
- `POST /v1/chat/completions` -> structured `missing_required_capabilities` error

## Architecture

The core owns HTTP, OpenAI request normalization, pipeline compilation, plugin lifecycle, readiness, fallback, streaming coordination, and error shape. Plugins own auth, routing, upstream provider integrations, rate limits, cache, observers, prompts, cost, and model catalog behavior.

Plugins are intended to run as long-lived subprocesses connected through a Unix socket. The checked-in proto contracts define the public gRPC/protobuf ABI; the current Go runtime includes a lightweight local RPC adapter used by the fake fixtures and process manager while generated protobuf stubs can be produced with `make proto`.

## Commands

```bash
make test
make race
make build
make proto
```

## Current limitations

- Linux sandbox enforcement is represented by policy/config boundaries; strict seccomp/cgroup enforcement is not yet wired.
- Public community plugins are not included in this core repo.
- Generated protobuf files are intentionally not checked in; run `make proto` when Buf is available.

## License

AGPL-3.0-only. See [LICENSE](LICENSE).
