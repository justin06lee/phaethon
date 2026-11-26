# phaethon — architecture

The shape of this project, for a model that has never seen it: the stack it is built as, top to bottom, how the parts connect, where things are, how to run it. This is the index. Each layer has a sheet of its own in `.ruri/layers/` — where to change what inside it, how it works, its key files, its traps — so read the one for the layer you are about to work in (`ruri layer <slug>` prints it), and leave the rest. Where the work stands — git, decisions, what worked and what didn't, what's open — is in catchup.md beside this file.
Don't edit this file: ruri writes it. `ruri architecture` prints it with every line numbered, and once you have read it that way you can put right what your work changed — `ruri architecture add|set|drop <section> …`. The user corrects it on the architecture page.

Read from the repo at d39b1ea; folded forward from finished turns since. Where it and the code disagree, the code is right.

phaethon is a Go installer that gives every agent harness on a machine its own computers: it bundles hollow (KVM VM desks) and bangboo (agent MCP tools), registers bangboo with installed harnesses, installs the usage skill, and provisions Linux hosts over ssh. For developers running Claude Code, Codex, Gemini, Cursor and similar apps who need self-hosted agent computers.

## The stack, top to bottom

1. **CLI** — Go entrypoint, subcommands install/host/sync/doctor/secret/view/uninstall (`main.go`) → `.ruri/layers/cli.md`
2. **Bundle** — embed.FS carrying hollow+bangboo binaries, versions.json, skills; payload extraction (`./, bundle/, skills/ · 2 files`) → `.ruri/layers/bundle.md`
3. **Local install** — Writes bangboo/hollow binaries to binDir, tracks changes, PATH handling (`./, scripts/, .github/ · 3 files`) → `.ruri/layers/local-install.md`
4. **Harness registration** — Detects Claude Code/Codex/Gemini/Cursor/Desktop/OpenCode, registers/unregisters bangboo MCP server, backs up configs (`./ · 3 files`) → `.ruri/layers/harness-registration.md`
5. **Host provisioning** — managed hosts.json state, ssh setup, hollow service install, URL discovery (`./ · 2 files`) → `.ruri/layers/host-provisioning.md`
6. **Sync and health** — phaethon sync/doctor/scan via bangboo host ls JSON, upgrades, image rebuilds (`sync.go`) → `.ruri/layers/sync-health.md`
7. **Runtime** — Go 1.26, macOS/Linux client; x86_64 Linux KVM hosts; ssh, QEMU, makima/Tailscale/WireGuard networking

## How it flows

- **Install computers:** make (Makefile: bundle hollow+bangboo) → CLI install (main.go) → installBinaries (install.go) from embed bundle (bundle.go) → harness register (harness.go) → skill install via bmo or copy (skills/phaethon/)
- **Add host:** phaethon host add MACHINE or connect code (hosts.go) → remote ssh (remote.go) or bangboo host add code → send hollow-linux-amd64, install service + QEMU or register served desktop → register reachable URL in hosts.json → build Linux image once for VM hosts
- **Keep current:** phaethon sync (sync.go) → reinstall binaries + harness registrations + skill → bangboo host ls --json for hostRows → upgrade hollow, rebuild outdated images, leave served desktops alone
- **Agent uses desk:** harness MCP bangboo tools → phaethon skill (skills/phaethon/SKILL.md) → hollow host serves VM desks, local serve shares real desktop → secret placeholders {{github}}, live view takeover

## Where things are

- main.go — CLI entrypoint, usage and subcommand dispatch
- bundle.go — embedded bundle FS, versions.json, payload extraction
- install.go — local binary install, binDir/stateDir helpers
- harness.go — per-harness detect/register/unregister/registered logic
- jsonedit.go — in-place JSON/JSONC config edits preserving formatting
- tomledit.go — in-place Codex TOML edits preserving formatting
- hosts.go — host add/ls/rm state in hosts.json
- remote.go — ssh-only remote execution, local-machine path
- sync.go — sync/doctor/scan via bangboo host ls JSON
- hosts_test.go, config_test.go — unit tests for host settings and config edits
- bundle/ — carried hollow-linux-amd64, hollow+bangboo client binaries, versions.json
- skills/phaethon/ — agent skill teaching desk tool use
- assets/ — logo image for README
- versions.env — the hollow and bangboo tags a release is built from (HOLLOW_REF, BANGBOO_REF)
- install.sh — curl | sh installer: detects os/arch, verifies SHA256SUMS, installs to ~/.local/bin
- scripts/formula.sh, .github/workflows/ — the Homebrew formula from a release's SHA256SUMS; CI, and the v* release (tarballs, GitHub release, tap)

## How to run it

- make — build hollow+bangboo, bundle, install, run phaethon install
- make build — produce ./build/phaethon with bundled binaries
- make install — put phaethon on $PATH ($PREFIX/bin)
- make update — rebuild everything then phaethon sync hosts
- phaethon install — install binaries, register harnesses, install skill
- phaethon host add MACHINE / host ls / scan / host rm — manage hosts
- phaethon sync / phaethon doctor --deep — update all, verify end-to-end
- phaethon uninstall — remove binaries, registrations, skill
- make dist — dist/phaethon-<os>-<arch>.tar.gz and SHA256SUMS for this platform; HOLLOW_REF/BANGBOO_REF (versions.env) pin the sources
- brew install justin06lee/tap/phaethon, or curl -fsSL …/phaethon/master/install.sh | sh — install without cloning, once a release exists

## Conventions

- Build with Go 1.26+; hollow/bangboo built by their own Makefiles from ../ or .cache/
- Never fetch at install time: everything installed is carried in embed bundle/
- Keep first harness config version as *.before-phaethon
- Use harness add-command where available, edit config file otherwise

## What it does

- One install bundles hollow, bangboo binaries and agent skill
- Registers bangboo MCP server with every installed agent harness
- Installs phaethon skill into harness skill folders via bmo
- Provisions any ssh-reachable KVM Linux machine as VM host
- Adds already-serving hosts by connect code (a hollow, or another machine's desktop as NAME/desktop); sync/doctor leave served desktops alone
- Serves local real desktop with share, trust CIDR, view-only
- Runs bangboo gateway embedded in apps with in-memory token
- Ships without cloning: importable Go packages, an npm SDK carrying the bangboo binary and <phaethon-desk>, and v* releases (GitHub tarballs, Homebrew tap, install.sh)
- Upgrades hollow on hosts and rebuilds missing Linux images
- Manages hosts: add, list, scan, remove, uninstall over ssh
- Proxies bangboo secret storage and live desk browser view
- Doctor command verifies binaries, harnesses, hosts end-to-end
- Syncs local install, registrations, skill and all hosts
- Uninstall removes binaries, harness registrations and skill
- Passes bangboo gateway (embed desks in other apps) and local (the desktop you sit at, shared with agents for a while) through as phaethon commands
