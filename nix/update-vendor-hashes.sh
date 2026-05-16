#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

attrs=(
  vulpes-core
  authn-static-api-key
  router-weighted
  router-litellm
  router-consul
  prompt-context-injector
  upstream-openai
  observer-stdout
  observer-prometheus
  observer-otel
  observer-s3-transcripts
)

for attr in "${attrs[@]}"; do
  echo "==> nix build .#$attr"
  if nix build ".#$attr" --no-link 2>"/tmp/vulpes-$attr.nix.log"; then
    echo "ok: $attr"
    continue
  fi
  cat "/tmp/vulpes-$attr.nix.log"
  echo
  echo "If this reports 'got: sha256-...', replace the placeholder hash for $attr in flake.nix."
  echo
  exit 1
done
