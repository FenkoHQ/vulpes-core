# Capability Contracts

Strict v1 capabilities:

- `Authenticator`: headers/source only, no prompt body.
- `Authorizer`: identity plus request summary.
- `RateLimiter`: RPM/TPM/budget check and usage commit.
- `Router`: chooses ordered upstream routes from core candidates.
- `UpstreamProvider`: sees the full request and invokes the LLM provider.
- `CacheProvider`: lookup/store normalized responses.
- `Observer`: receives lifecycle events; async/fail-open by default.
- `PromptProvider`: resolves prompt references into messages.
- `CostProvider`: estimates/calculates costs.
- `ModelRegistryProvider`: lists provider/model metadata.

No generic hooks are part of v1. New behavior must be added as a typed capability with a narrow input/output contract.
