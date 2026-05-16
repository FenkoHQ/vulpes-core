{
  description = "Vulpes Core gateway with MVP plugin bundle";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    vulpes-core-plugins = {
      url = "path:../vulpes-core-plugins";
      flake = false;
    };
  };

  outputs = { self, nixpkgs, vulpes-core-plugins }:
  let
    systems = [ "x86_64-linux" "aarch64-linux" ];
    forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
  in {
    packages = forAllSystems (pkgs:
      let
        buildPlugin = name: hash: pkgs.buildGoModule {
          pname = name;
          version = "0.1.0";
          src = vulpes-core-plugins;
          modRoot = "plugins/${name}";
          vendorHash = hash;
          subPackages = [ "." ];
        };
        plugins = rec {
          authn-static-api-key = buildPlugin "authn-static-api-key" "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
          router-weighted = buildPlugin "router-weighted" "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
          router-litellm = buildPlugin "router-litellm" "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
          router-consul = buildPlugin "router-consul" "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
          prompt-context-injector = buildPlugin "prompt-context-injector" "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
          upstream-openai = buildPlugin "upstream-openai" "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
          observer-stdout = buildPlugin "observer-stdout" "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
          observer-prometheus = buildPlugin "observer-prometheus" "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
          observer-otel = buildPlugin "observer-otel" "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
          observer-s3-transcripts = buildPlugin "observer-s3-transcripts" "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
        };
      in plugins // {
        vulpes-core = pkgs.buildGoModule {
          pname = "vulpes-core";
          version = "0.1.0";
          src = self;
          vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
          subPackages = [ "cmd/gateway" "cmd/pluginctl" ];
        };
        plugin-bundle = pkgs.symlinkJoin {
          name = "vulpes-core-plugin-bundle";
          paths = builtins.attrValues plugins;
        };
        default = self.packages.${pkgs.system}.vulpes-core;
      });

    apps = forAllSystems (pkgs: {
      default = {
        type = "app";
        program = "${self.packages.${pkgs.system}.vulpes-core}/bin/gateway";
      };
    });

    devShells = forAllSystems (pkgs: {
      default = pkgs.mkShell {
        packages = [ pkgs.go pkgs.gopls pkgs.buf pkgs.protobuf pkgs.protoc-gen-go pkgs.protoc-gen-go-grpc ];
      };
    });

    nixosModules.default = { config, lib, pkgs, ... }:
      let
        cfg = config.services.vulpes-core;
        pkg = cfg.package;
        settingsFile = pkgs.writeText "vulpes-gateway.json" (builtins.toJSON cfg.settings);
      in {
        options.services.vulpes-core = {
          enable = lib.mkEnableOption "Vulpes Core LLM gateway";
          package = lib.mkOption {
            type = lib.types.package;
            default = self.packages.${pkgs.system}.vulpes-core;
          };
          settings = lib.mkOption {
            type = lib.types.attrs;
            default = { server = { listen = "127.0.0.1:8080"; }; plugins = []; pipeline = {}; };
            description = "Gateway config rendered as JSON, which is valid YAML for the gateway loader.";
          };
          environmentFile = lib.mkOption {
            type = lib.types.nullOr lib.types.path;
            default = null;
            description = "Optional environment file containing OPENAI_API_KEY, R2 secrets, etc.";
          };
        };
        config = lib.mkIf cfg.enable {
          systemd.services.vulpes-core = {
            description = "Vulpes Core LLM gateway";
            wantedBy = [ "multi-user.target" ];
            after = [ "network-online.target" ];
            wants = [ "network-online.target" ];
            serviceConfig = {
              ExecStart = "${pkg}/bin/gateway -config ${settingsFile}";
              Restart = "always";
              RestartSec = "3s";
              DynamicUser = true;
              StateDirectory = "vulpes-core";
              RuntimeDirectory = "vulpes-core";
            } // lib.optionalAttrs (cfg.environmentFile != null) {
              EnvironmentFile = cfg.environmentFile;
            };
          };
        };
      };
  };
}
