# Testing

Run:

```bash
go test ./...
go test -race ./...
```

The tests cover config loading, zero-plugin readiness, missing capability diagnostics, request execution, fallback, streaming, rate-limit denial, cache hits, observer fail-open behavior, secret scoping, filesystem source validation, GitHub URL/cache helpers, and safe archive extraction.
