# Nix / Flakes Deployment

This repo includes a flake that builds:

- `vulpes-core`
- all core MVP plugins from `../vulpes-core-plugins`
- a `plugin-bundle`
- a NixOS module: `nixosModules.default`

Example NixOS import:

```nix
{
  inputs.vulpes-core.url = "github:FenkoHQ/vulpes-core";
}
```

Then import:

```nix
imports = [ inputs.vulpes-core.nixosModules.default ];
```

A complete sample module is in:

```text
examples/nixos/vulpes-core.nix
```

The module renders gateway settings as JSON. JSON is valid YAML, so it is accepted by the gateway config loader.

Local development:

```bash
nix develop
nix build .#vulpes-core
nix build .#plugin-bundle
```

Note: the local flake currently points plugin source at `path:../vulpes-core-plugins` for development. When publishing, update that input to the real plugin repo URL.
