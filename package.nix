{
  lib,
  buildGoModule,
  roboto,
  version ? "0.9.0-unstable",
  commit ? "",
}: let
  fontDir = "${roboto}/share/fonts/truetype";
in
  buildGoModule {
    pname = "deckmaster";
    inherit version;

    src = lib.fileset.toSource {
      root = ./.;
      fileset = lib.fileset.unions [
        ./go.mod
        ./go.sum
        ./assets
        ./third_party
        (lib.fileset.fileFilter (file: file.hasExt "go") ./.)
      ];
    };

    vendorHash = "sha256-QxdHDVQPqKiVvKCYQ3fFPzGm6nzvbeg0ZCB8jK1VBdc=";

    proxyVendor = true;

    # third_party/streamdeck is a module of its own, pulled in by a replace.
    subPackages = ["."];

    # subPackages cannot reach that module's tests, so run them by import path.
    postCheck = ''
      go test github.com/muesli/streamdeck
    '';

    # The binary looks for Roboto in the store itself instead of through a
    # wrapper that sets XDG_DATA_DIRS: go-findfont ignores that variable on
    # macOS, and a wrapper script at bin/deckmaster is not a binary macOS can
    # grant Accessibility to. `go test` links package main under its import
    # path, so the tests need the second -X.
    ldflags = [
      "-s"
      "-w"
      "-X main.Version=${version}"
      "-X main.CommitSHA=${commit}"
      "-X main.fontDir=${fontDir}"
      "-X github.com/muesli/deckmaster.fontDir=${fontDir}"
    ];

    passthru = {inherit fontDir;};

    meta = {
      description = "Application to control your Elgato Stream Deck on Linux and macOS";
      mainProgram = "deckmaster";
      homepage = "https://github.com/psoldunov/deckmaster";
      license = lib.licenses.mit;
      platforms = lib.platforms.linux ++ lib.platforms.darwin;
    };
  }
