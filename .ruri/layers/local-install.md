# phaethon — Local install

One layer of phaethon's stack, owning `install.go, install.sh, scripts/, .github/, versions.env`. The whole stack, and how the layers connect, is in `.ruri/architecture.md`.
Don't edit this file: ruri writes it. `ruri layer local-install` prints it with every line numbered, what git says changed in this layer lately and what sessions learned working here; once you have read it that way, put right what your work changed — `ruri layer local-install add|set|drop <section> …`. Where it and the code disagree, the code is right.

This layer installs the bangboo and hollow binaries beside phaethon in binDir and reports what changed so repeat installs are no-ops. It resolves binary paths by preferring binDir then falling back to PATH lookup. Newcomers must grasp: writes are content-addressed by SHA — identical binaries are skipped, never blindly overwritten.

## Where to change what

- **Binary install / upgrade:** install.go
- **Binary path resolution (bangboo/hollow):** install.go
- **PATH / binDir handling:** install.go

## How it works

- **Install binaries:** Resolve binDir → Load embedded payload per program (bangboo, hollow) → Compare fileSHA of existing file to payload SHA and skip if equal → writeBinary changed programs → Return dir plus changed list

## Key files

- install.go — writes bangboo/hollow binaries to binDir, SHA-skips unchanged files, resolves binary paths with PATH fallback
- install.sh — curl | sh installer for phaethon itself: os/arch, the latest or PHAETHON_VERSION release, SHA256SUMS check, ~/.local/bin or $PREFIX/bin; must run under dash
- versions.env — the hollow and bangboo tags a release is built from
- .github/workflows/release.yml — v* tag: per-platform tarballs, GitHub release with SHA256SUMS, Homebrew formula via scripts/formula.sh pushed to the tap

## Rules and traps

- Never rewrite a binary whose SHA already matches — repeat install must report no change.
- Always install bangboo and hollow together beside phaethon in binDir; do not split locations.
- Path lookup must prefer binDir first, then PATH via exec.LookPath.
- Emit user messages to stderr via say, not stdout.
- binDir() skips Homebrew's Cellar (packaged() in bundle.go): run from brew, bangboo and hollow go to ~/.local/bin, since brew upgrade deletes the Cellar version and harnesses would point at nothing
- A release builds hollow and bangboo from the tags in versions.env (cloned into .cache, GOWORK=off), never sibling checkouts; never pass VERSION= to make, it reaches their Makefiles too

## What it talks to

- Bundle — payload bytes and versions for bangboo/hollow come from embedded bundle
- CLI — install command drives installBinaries and reports changed list
- Host provisioning / Sync — consume bangbooPath and hollowPath to invoke the installed CLIs
