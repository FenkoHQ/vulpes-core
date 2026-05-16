# Community Plugins

Recommended repository layout:

```text
llm-gateway-plugins/
  plugins/
    authn-static-api-key/
    router-weighted/
    upstream-openai/
    cache-redis/
    observer-otel/
```

Each plugin directory should include:

- `README.md`
- `gateway-plugin.yaml`
- `main.go`
- tests
- example gateway config

Release assets should be checksummed and named by plugin, OS, and architecture, for example `upstream-openai_linux_amd64.tar.gz`.
