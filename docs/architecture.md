# Architecture

Vulpes Core is a small stateless data-plane process. It exposes OpenAI-compatible HTTP endpoints and compiles a deterministic request pipeline from configuration at startup.

The core owns request IDs, deadlines, request normalization, readiness, error mapping, fallback execution, streaming/SSE coordination, plugin process supervision, source resolution, and capability registration.

Plugins own specialized stateful behavior: authentication, authorization, rate limits, budgets, upstream provider calls, cache, observers, prompt templates, cost metadata, and model registry metadata.

The intended HA model is external: run multiple gateway replicas behind Kubernetes, a load balancer, or a service mesh. Each gateway replica starts local long-lived plugin workers. Shared state belongs in plugin backends such as Redis, Postgres, object storage, OTLP collectors, or policy engines.

Request flow:

1. parse OpenAI-compatible request
2. authenticate unless anonymous mode is enabled
3. authorize if configured
4. resolve managed prompt if configured
5. cache lookup if configured
6. rate-limit check if configured
7. build model candidates
8. call router
9. invoke selected upstream/fallback chain
10. stream or collect response
11. store cache / commit usage
12. emit observer events asynchronously
