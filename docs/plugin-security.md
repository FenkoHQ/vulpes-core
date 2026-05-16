# Plugin Security

The core minimizes data sent to plugins. Auth, route, rate-limit, and observer capabilities receive summaries unless explicitly allowed to see raw prompt/response data. Upstream providers necessarily receive the full request.

Secrets are owned by the core. `${secret:NAME}` references are resolved only when the plugin metadata permits `NAME`.

Trust levels:

- `trusted`: relaxed sandbox allowed.
- `community`: sandbox recommended/required where supported.
- `untrusted`: refused unless strict sandboxing is enabled.

Sandbox configuration is represented in v1 config so enforcement can be strengthened without changing plugin contracts.
