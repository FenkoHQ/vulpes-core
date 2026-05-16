{ inputs, pkgs, ... }:

let
  packages = inputs.vulpes-core.packages.${pkgs.system};
in {
  imports = [ inputs.vulpes-core.nixosModules.default ];

  services.vulpes-core = {
    enable = true;
    environmentFile = /etc/vulpes-core/secrets.env;
    settings = {
      server = {
        listen = "127.0.0.1:8080";
        request_timeout = "120s";
        stream_idle_timeout = "60s";
      };
      secrets.env.enabled = true;
      observability = {
        flush_interval = "1s";
        capture_payloads = true;
      };
      plugins = [
        {
          name = "authn-static";
          source = { type = "filesystem"; path = "${packages.authn-static-api-key}/bin/authn-static-api-key"; };
          capabilities = [ "authenticator" ];
          config.keys = [{ id = "prod"; value = "\${secret:GATEWAY_API_KEY}"; tenant = "prod"; }];
        }
        {
          name = "context-injector";
          source = { type = "filesystem"; path = "${packages.prompt-context-injector}/bin/prompt-context-injector"; };
          capabilities = [ "prompt_provider" ];
          config.rules = [{ name = "default"; model = "gpt-4o-mini"; mode = "prepend_system"; content = "You are running behind Vulpes Core."; }];
        }
        {
          name = "consul-router";
          source = { type = "filesystem"; path = "${packages.router-consul}/bin/router-consul"; };
          capabilities = [ "router" ];
          config = { address = "http://127.0.0.1:8500"; service = "litellm"; provider_instance = "litellm"; };
        }
        {
          name = "litellm";
          source = { type = "filesystem"; path = "${packages.upstream-openai}/bin/upstream-openai"; };
          capabilities = [ "upstream_provider" ];
          config = { base_url = "http://placeholder/v1"; api_key = "\${secret:LITELLM_API_KEY}"; };
        }
        {
          name = "otel";
          source = { type = "filesystem"; path = "${packages.observer-otel}/bin/observer-otel"; };
          capabilities = [ "observer" ];
          fail_mode = "open";
          config = { endpoint = "localhost:4318"; service_name = "vulpes-gateway"; insecure = true; };
        }
        {
          name = "r2-transcripts";
          source = { type = "filesystem"; path = "${packages.observer-s3-transcripts}/bin/observer-s3-transcripts"; };
          capabilities = [ "observer" ];
          fail_mode = "open";
          config = {
            endpoint = "https://<account-id>.r2.cloudflarestorage.com";
            region = "auto";
            bucket = "llm-transcripts";
            prefix = "vulpes/prod";
            access_key_id = "\${secret:R2_ACCESS_KEY_ID}";
            secret_access_key = "\${secret:R2_SECRET_ACCESS_KEY}";
            force_path_style = true;
          };
        }
      ];
      pipeline = {
        authenticator = "authn-static";
        prompt_provider = "context-injector";
        router = "consul-router";
        upstream_providers = [ "litellm" ];
        observers = [ "otel" "r2-transcripts" ];
      };
      models.aliases.gpt-4o-mini.candidates = [{ provider = "litellm"; model = "gpt-4o-mini"; weight = 100; }];
    };
  };
}
