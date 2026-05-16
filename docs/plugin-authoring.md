# Plugin Authoring

A plugin is an executable package that declares metadata, permissions, config schema, and one or more strict capabilities.

At startup the gateway resolves the plugin source, starts the executable, connects over the configured Unix socket, handshakes, validates protocol version, retrieves metadata/schema, validates config, sends scoped secrets, and registers advertised capabilities.

Plugin environment:

```text
GATEWAY_PLUGIN_SOCKET=/tmp/llm-gateway/plugins/openai.sock
GATEWAY_PLUGIN_INSTANCE=openai
GATEWAY_PLUGIN_PROTOCOL_VERSION=1
```

A plugin must not expect the full host environment. Secrets are resolved by the core and sent only if declared in permissions and referenced by configuration.

The protobuf ABI lives under `proto/gateway/v1`. Run `make proto` to generate Go stubs when Buf is installed.
