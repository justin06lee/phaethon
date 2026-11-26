# phaethon — CLI

One layer of phaethon's stack, owning `main.go`. The whole stack, and how the layers connect, is in `.ruri/architecture.md`.
Don't edit this file: ruri writes it. `ruri layer cli` prints it with every line numbered, what git says changed in this layer lately and what sessions learned working here; once you have read it that way, put right what your work changed — `ruri layer cli add|set|drop <section> …`. Where it and the code disagree, the code is right.

CLI layer is the Go entrypoint that wires every phaethon operation together: install, host add/ls/rm, sync, doctor, scan, secret, view, gateway, local, uninstall. It now splits dispatch across main.go plus hosts.go and sync.go, parsing subcommands/flags including host add via connect code and delegating to Bundle, Local install, Harness, Host, and Sync layers. Newcomers must grasp it owns no logic itself — it is dispatch plus usage text and exit codes, and served desktops (NAME/desktop) are left alone by sync/doctor.

## Where to change what

- **install flow:** main.go
- **host management (add/ls/rm):** main.go, hosts.go
- **sync / doctor / scan health commands:** main.go, sync.go
- **secret handling:** main.go
- **view / uninstall commands:** main.go
- **CLI usage text and version flag:** main.go
- **gateway and local desktop commands:** main.go

## How it works

- **Run any subcommand:** parse global flags (--version) in main.go → match subcommand (install/host/sync/doctor/scan/secret/view/uninstall) → delegate to owning layer's function → report error and set exit code

## Key files

- main.go — Go entrypoint; usage text, flag/subcommand dispatch, delegation to all other layers
- hosts.go — host subcommand handling including add via connect code for served desktops
- sync.go — sync/doctor handling that leaves served desktops alone

## Rules and traps

- Keep CLI files dispatch-only; implement behavior in the owning layer, not here

## What it talks to

- Bundle — install path pulls embedded binaries/versions/skills it carries
- Local install — install/uninstall path writes/removes bangboo/hollow binaries
- Harness registration — install path registers bangboo MCP with detected harnesses
- Host provisioning — host add/ls/rm path manages hosts.json and remote hollow setup
- Sync and health — sync/doctor/scan path queries hosts via bangboo host ls JSON
