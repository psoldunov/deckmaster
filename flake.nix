{
  description = "Deckmaster: control an Elgato Stream Deck on Linux and macOS";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = {
    self,
    nixpkgs,
  }: let
    inherit (nixpkgs) lib;

    systems = ["x86_64-linux" "aarch64-linux" "aarch64-darwin"];
    forAllSystems = f: lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});

    date = self.lastModifiedDate or "19700101";
    deckmaster = pkgs:
      pkgs.callPackage ./package.nix {
        version = "0.9.0-unstable-${lib.substring 0 4 date}-${lib.substring 4 2 date}-${lib.substring 6 2 date}";
        commit = self.rev or self.dirtyRev or "";
      };
  in {
    packages = forAllSystems (pkgs: {
      deckmaster = deckmaster pkgs;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.deckmaster;
    });

    overlays.default = final: _prev: {deckmaster = deckmaster final;};

    devShells = forAllSystems (pkgs: let
      package = self.packages.${pkgs.stdenv.hostPlatform.system}.deckmaster;
    in {
      default = pkgs.mkShell {
        inputsFrom = [package];
        packages = [pkgs.go pkgs.gopls pkgs.golangci-lint pkgs.alejandra];
        # `go run`, `go build` and `go test` find Roboto the way the package does.
        GOFLAGS = "'-ldflags=-X main.fontDir=${package.fontDir} -X github.com/muesli/deckmaster.fontDir=${package.fontDir}'";
      };
    });

    formatter = forAllSystems (pkgs: pkgs.alejandra);

    checks = forAllSystems (pkgs: {
      deckmaster = self.packages.${pkgs.stdenv.hostPlatform.system}.deckmaster;
    });
  };
}
