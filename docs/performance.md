# Performance

The hot path is compiled at startup. Plugins are started once and kept warm. No plugin installation, download, or process startup happens per request.

Performance choices:

- precompiled pipeline references
- long-lived subprocess workers
- Unix sockets by default
- minimal payloads to non-upstream capabilities
- async observer queue
- core-owned fallback execution

Benchmarks are available with:

```bash
go test -bench=. ./...
```
