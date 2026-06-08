{
  description = "wuzapi development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = {
    self,
    nixpkgs,
    flake-utils,
  }:
    flake-utils.lib.eachDefaultSystem (system: let
      pkgs = import nixpkgs {inherit system;};
    in {
      devShells.default = pkgs.mkShell {
        packages = with pkgs; [
          go_1_24
          gopls
          delve
          gotools
          golangci-lint
          air
        ];

        shellHook = ''
          export GOWORK=off
          echo "Nix dev shell listo: $(go version)"
        '';
      };
    });
}
