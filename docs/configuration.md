# Configuration

Minimal development config:

```yaml
server:
  listen: 127.0.0.1:8080
plugins: []
pipeline: {}
```

Anonymous mode removes the Authenticator requirement:

```yaml
auth:
  anonymous: true
```

A production pipeline names concrete plugin instances:

```yaml
pipeline:
  authenticator: authn-static
  router: weighted-router
  upstream_providers: [openai]
  observers: [otel]
```

Observer payload capture is off by default. Enable it only when transcript/audit plugins need raw request and response payloads:

```yaml
observability:
  capture_payloads: true
```

Model aliases map OpenAI-facing logical names to provider candidates:

```yaml
models:
  aliases:
    gpt-4o-mini:
      candidates:
        - provider: openai
          model: gpt-4o-mini
          weight: 100
```
