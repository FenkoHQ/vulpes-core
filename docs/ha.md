# High Availability

The gateway core is designed to be horizontally replicated rather than to implement HA orchestration itself.

Run multiple gateway replicas behind Kubernetes, a load balancer, or a service mesh. Each replica loads the same config, starts its own local plugin workers, and serves requests independently.

The core stores no durable state. In-process state is limited to compiled pipeline references, plugin connections/process handles, health snapshots, bounded observer queues, and request-local streaming state.

Stateful behavior belongs in plugin backends:

- rate limits and budgets: RateLimiter backend such as Redis
- cache: CacheProvider backend such as Redis/object storage
- prompt versions: PromptProvider backend
- logs/analytics: Observer backend
- auth/session/policy state: Authenticator/Authorizer backend
- model catalogs: ModelRegistryProvider backend or shared config

Observer delivery is best-effort unless configured as blocking audit. Local caches, if added, must be documented as per-replica only.
