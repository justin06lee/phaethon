# phaethon — Harness registration

One layer of phaethon's stack, owning `harness.go, jsonedit.go, tomledit.go`. The whole stack, and how the layers connect, is in `.ruri/architecture.md`.
Don't edit this file: ruri writes it. `ruri layer harness-registration` prints it with every line numbered, what git says changed in this layer lately and what sessions learned working here; once you have read it that way, put right what your work changed — `ruri layer harness-registration add|set|drop <section> …`. Where it and the code disagree, the code is right.

This layer wires the bangboo MCP server into the agent harnesses installed on the machine (Claude Code, Codex, Gemini, Cursor, Desktop, OpenCode). It is built as a table of per-harness adapters sharing one shape: detect presence, register, unregister, and check registration. The key idea is that each harness stores its server list its own way, so registration prefers each harness's own command and only edits config files directly where no command exists, with backups taken first.

## Where to change what

- **Claude Code registration:** harness.go
- **Codex registration:** harness.go
- **Gemini registration:** harness.go
- **Cursor registration:** harness.go
- **Claude Desktop registration:** harness.go
- **OpenCode registration:** harness.go
- **Harness detection:** harness.go
- **Config backup:** harness.go
- **Unregister / cleanup:** harness.go

## How it works

- **Register bangboo with a harness:** Detect which harnesses are present on PATH or via config paths → Resolve the installed bangboo binary path → Prefer the harness's own add/register command where one exists → Otherwise read, merge, and rewrite its JSON config with backup → Verify via registered check
- **Unregister bangboo:** Detect present harnesses → Run the harness's own remove command where available → Otherwise remove the bangboo entry from its config file → Leave backup files in place

## Key files

- harness.go — harness table plus per-harness present/register/unregister/registered adapters, config paths, and backup helpers

## Rules and traps

- Where a harness has its own server-management command, use it rather than hand-editing its config.
- Never overwrite a harness config without backing it up first.
- Detection is by presence (binary on PATH or config file exists); do not register into harnesses that are not installed.
- Config paths are OS-specific (e.g. Claude Desktop on darwin vs linux); keep the platform switch intact.

## What it talks to

- CLI — install/uninstall subcommands drive register/unregister across all detected harnesses
- Local install — supplies the bangboo binary path that gets registered
- Runtime — registered harnesses launch bangboo as an MCP server to drive desks/hosts
