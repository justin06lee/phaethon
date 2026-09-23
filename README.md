<div align="center">

<img src="assets/phaethon.svg" alt="phaethon" width="340" />

# phaethon

**Computers for every agent on this machine.**<br>
*One install gives Claude Code, Codex, Gemini, Cursor and the rest their own computers, on any Linux box you can ssh to.*

</div>

---

phaethon is the driver of a three-piece, self-hosted take on an agent with
a computer of its own:

- [**hollow**](https://github.com/justin06lee/hollow) runs the computers.
  These are desks: small Linux VMs with a screen, a browser and a terminal,
  on any Linux machine with KVM.
- [**bangboo**](https://github.com/justin06lee/bangboo) is what an agent
  talks to. It is an MCP server and a CLI with tools to start a desk, read a
  page, click, type, run commands and move files.
- **phaethon** installs both, registers bangboo with every agent harness on
  the machine, installs the skill that teaches an agent to use it well, and
  turns machines into hosts over ssh.

Each piece is its own program and works without the others. phaethon is
what makes them one install: it **carries** hollow and bangboo inside
itself, so installing it installs them, and nothing is downloaded from
anywhere.

## Install

```sh
git clone https://github.com/justin06lee/phaethon && cd phaethon && make
```

`make` builds hollow and bangboo from checkouts beside this one, or clones
them if they are not there. It bundles them into phaethon, installs it, and
runs `phaethon install`:

```
  installed bangboo and hollow in ~/.local/bin
  Claude Code     bangboo registered as an MCP server
  Codex           bangboo registered as an MCP server
  Gemini CLI      bangboo registered as an MCP server
  Cursor          bangboo registered as an MCP server
  Claude Desktop  bangboo registered as an MCP server
  OpenCode        bangboo registered as an MCP server
  the phaethon skill is in 5 harness skill folders
```

Only harnesses that are installed are touched. Where a harness has a command
for adding MCP servers, that command is used. Where phaethon edits a config
file, the first version is kept beside it as `*.before-phaethon`. The skill
goes in through [bmo](https://github.com/justin06lee/bmo) when it is
present, and is copied into each harness's skills folder when it is not.

Needs Go 1.26 or newer to build. It runs on macOS and Linux.

## Add a host

Any x86_64 Linux machine with KVM that ssh reaches:

```sh
phaethon host add tenet
```

```
  reached root@tenet over ssh
  sending hollow v0.2.0 (16 MB)
  installing it as a service (QEMU too, if it is missing)
  hollow is running on root@tenet, and will be after every boot
  bangboo can reach tenet at http://tenet.makima:7070
  tenet: building the Linux image (a few minutes, once)

  tenet is a host. Agents here can start desks on it now (desk_new).
```

`MACHINE` can be anything ssh understands. That includes a makima name
(`tenet` or `tenet.makima`), a Tailscale name or address, an alias from
`~/.ssh/config`, or `user@host`. Without a user, root is tried first, then
your ssh default. Setting a machine up means installing a service, and a
normal user's sudo may want a password. From a terminal, phaethon lets you
type it.

hollow listens only on loopback and on makima, Tailscale and WireGuard
addresses, so the host and this machine need one of those networks in
common. The connect code carries every address the host has, and bangboo
uses whichever answers.

Running it again on a machine that is already a host upgrades hollow there
and refreshes its registration. From another laptop, the same command adds
the same host to that laptop.

```sh
phaethon host ls                      # hosts, where they answer, which phaethon manages
phaethon scan                         # hollows on your networks that you have not added
phaethon host rm tenet                # forget it; --uninstall also takes hollow off it
phaethon host add local               # this machine, if it is Linux with KVM
```

## Keep it current

```sh
phaethon sync
```

This reinstalls this machine's programs, harness registrations and skill.
It upgrades hollow on every host phaethon manages, skipping any host with
desks running unless you pass `--force`. It then builds or rebuilds any
image that is missing or from an older recipe. Desks keep running through
an image rebuild.

`make update` is a rebuild followed by `phaethon sync`.

## Check it

```sh
phaethon doctor --deep
```

```
this machine:
  ✓ bangboo installed at ~/.local/bin/bangboo
  ✓ Claude Code: bangboo registered as an MCP server
  …
hosts:
  ✓ tenet: answering at http://tenet.makima:7070, 11.2 GB free, 0 desk(s)
  ✓ tenet: Linux image built
a desk, end to end:
  ✓ started desk "phaethon-doctor-36576" in 19s
  ✓ opened example.com and read it as text
  ✓ took a screenshot
  ✓ ran a command
  ✓ closed the desk
```

## What an agent sees

After `phaethon install`, a new agent session has bangboo's tools. It also
has the phaethon skill, [`skills/phaethon/SKILL.md`](skills/phaethon/SKILL.md),
which the harness loads when a task needs a computer. The skill covers
which tool to use when, how to read pages as text instead of squinting at
screenshots, keys and coordinates, running several desks at once, and
cleaning up.

Asked headlessly to find the top Hacker News story and summarize its
article, Claude Code (Sonnet) loaded the skill and made five tool calls:
`desk_new`, `browser_open`, `browser_click`, one `computer` screenshot, and
`desk_close`. It answered in about a minute.

## Uninstall

```sh
phaethon uninstall        # bangboo out of every harness, the skill removed, binaries removed
```

Hosts keep running. `phaethon host rm NAME --uninstall` takes hollow off one.

## Known limits

- **Hosts are x86_64 Linux machines with KVM, and guests are Linux.** macOS
  and Windows guests are not here yet.
- **Gemini CLI turns MCP servers off in folders it does not trust.** bangboo
  shows as "Disabled" there until you trust the folder in Gemini.
- **A sudo password needs a terminal.** Run `phaethon host add` from one, or
  let it ssh in as root.
- **The binaries are built for this machine.** A phaethon built on an M1 Mac
  carries darwin/arm64 bangboo and hollow CLIs, plus the linux/amd64 hollow
  for hosts. Build it again on another kind of machine.

## Layout

```
main.go          commands
bundle.go        what phaethon carries, and where it installs it
harness.go       registering bangboo with each harness, installing the skill
hosts.go         host add / rm: hollow over ssh, connect codes, bangboo registration
remote.go        ssh: finding a way in, running scripts, sending files
sync.go          sync, image builds, doctor
skills/phaethon  the skill an agent reads
bundle/          the binaries the Makefile builds and phaethon embeds
```
