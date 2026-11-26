# phaethon — Bundle

One layer of phaethon's stack, owning `bundle.go, bundle/, skills/, bundle_test.go`. The whole stack, and how the layers connect, is in `.ruri/architecture.md`.
Don't edit this file: ruri writes it. `ruri layer bundle` prints it with every line numbered, what git says changed in this layer lately and what sessions learned working here; once you have read it that way, put right what your work changed — `ruri layer bundle add|set|drop <section> …`. Where it and the code disagree, the code is right.

The Bundle layer is phaethon's self-contained install payload: an embed.FS carrying hollow+bangboo binaries, versions.json, and skills, so install requires no network fetch. It is built by make next to hollow and bangboo checkouts and read via helpers in bundle.go. A newcomer must grasp that if a payload is missing the binary was mis-built, not mis-downloaded.

## Where to change what

- **Embedded payload (binaries + skills + versions):** bundle.go, skills/phaethon/SKILL.md
- **Version reporting:** bundle.go
- **Payload extraction and verification:** bundle.go

## How it works

- **Read an embedded payload:** Call payload(name) to read bundle/<name> from embed.FS → Return missing-build error if not embedded → Caller writes/verifies bytes to disk → Verify via sha hash helper

## Key files

- bundle.go — embed.FS (bundle+skills), versions.json reading, payload() / localName() / sha helpers

## Rules and traps

- Never fetch payloads from the network here; installing phaethon is the whole install.
- Build with make next to hollow and bangboo checkouts or payload() fails with a build error.
- Binary names are OS/arch-qualified via localName(); do not hardcode them.
- skills/phaethon/SKILL.md must stay in step with bangboo's tools: it teaches the rules for the user's own desktop (local/desktop, shared with bangboo local share); change both together

## What it talks to

- Local install — consumes extracted binaries written to binDir
- Host provisioning — consumes embedded Linux hollow payload for remote hosts
- Harness registration — consumes embedded skill content taught to agents
