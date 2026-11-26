# phaethon — Sync and health

One layer of phaethon's stack, owning `sync.go`. The whole stack, and how the layers connect, is in `.ruri/architecture.md`.
Don't edit this file: ruri writes it. `ruri layer sync-health` prints it with every line numbered, what git says changed in this layer lately and what sessions learned working here; once you have read it that way, put right what your work changed — `ruri layer sync-health add|set|drop <section> …`. Where it and the code disagree, the code is right.

Sync and health layer keeps managed hosts usable via `phaethon sync/doctor/scan`, driven by `bangboo host ls --json` output parsed into hostRow. It is built in a single file (sync.go) around hostRows, linuxImage inspection, and ensureImage rebuild/upgrade logic. Newcomers must grasp: upgrades preserve each host's port/idle settings and wait until the host answers as the new version, a current Linux image is left alone — only missing, failed, or outdated-recipe images trigger a rebuild — and sync can upgrade hosts set up through the bangboo library, not just `phaethon host add`.

## Where to change what

- **host listing via bangboo:** sync.go
- **Linux image state check:** sync.go
- **image rebuild / upgrade:** sync.go
- **sync command:** sync.go
- **doctor health check:** sync.go
- **scan command:** sync.go

## How it works

- **Ensure host image:** Run bangboo host ls --json via hostRows → Parse rows into hostRow structs → Pick linux entry via linuxImage → Leave current image alone → Upgrade preserving host port/idle settings → Wait until host answers as new version instead of fixed sleep

## Key files

- sync.go — hostRow parsing, linuxImage check, settings-preserving ensureImage rebuild/upgrade with wait-for-answer, and sync/doctor/scan commands

## Rules and traps

- Never rebuild a current Linux image; only missing, failed, or older-recipe ones.
- Linux image lookup matches on OS == linux; absence means state missing.
- Host truth comes from bangboo host ls --json, not local state; a row whose backend is "desktop" (another machine desktop, bangboo local serve) runs no hollow and has no images, so sync and doctor report it answering and skip upgrades and image checks
- Never hardcode port 7070 / idle 0 on upgrade; keep each host's existing settings including legacy unit-file idle moved into env.
- After upgrading a host, wait until it answers as the new version, not a fixed 2s sleep.
- doctor --deep must restore bangboo's remembered current desk when finished, and say so when no usable host exists instead of printing nothing.

## What it talks to

- CLI — exposes this layer as sync/doctor/scan subcommands
- Host provisioning — consumes its hosts.json/URL state via live host ls rows
- Runtime — image rebuilds target x86_64 Linux KVM host images
- Bundle — upgrades/images depend on bundled binaries and versions carried there
- Bangboo library setup — sync upgrades hosts an app set up through bangboo, keeping port/idle/token
