{
  description = "Vulpes Core gateway with MVP plugin bundle";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    vulpes-core-plugins = {
      # Pinned in flake.lock to a specific commit. For local co-development
      # against an unpushed checkout, override with:
      #   nix build --override-input vulpes-core-plugins git+file:///path/to/vulpes-core-plugins
      url = "github:FenkoHQ/vulpes-core-plugins";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, vulpes-core-plugins }:
  let
    systems = [ "x86_64-linux" "aarch64-linux" ];
    forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
  in {
    # Re-export the plugin packages and plugin-bundle from the plugins flake,
    # and add the gateway itself.
    packages = forAllSystems (pkgs:
      vulpes-core-plugins.packages.${pkgs.system} // {
        vulpes-core = pkgs.buildGoModule {
          pname = "vulpes-core";
          version = "0.1.0";
          src = self;
          vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
          subPackages = [ "cmd/gateway" "cmd/pluginctl" ];
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
