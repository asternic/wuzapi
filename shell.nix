{pkgs ? import <nixpkgs> {}}:
pkgs.mkShell {
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
}
