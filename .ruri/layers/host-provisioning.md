# phaethon — Host provisioning

One layer of phaethon's stack, owning `hosts.go, remote.go`. The whole stack, and how the layers connect, is in `.ruri/architecture.md`.
Don't edit this file: ruri writes it. `ruri layer host-provisioning` prints it with every line numbered, what git says changed in this layer lately and what sessions learned working here; once you have read it that way, put right what your work changed — `ruri layer host-provisioning add|set|drop <section> …`. Where it and the code disagree, the code is right.

This layer provisions remote (or local) machines as hollow hosts and remembers them in stateDir()/hosts.json. hosts.go owns the managed map and the `host add/ls/rm` commands; remote.go is the only transport — shell scripts run over ssh (or locally) plus hollow binary install and URL discovery. `host add` now also accepts a desktop connect code, creating a served-desktop host (NAME/desktop) that sync and doctor leave alone, alongside ssh-installed VM hosts.

## Where to change what

- **Add a host:** hosts.go, remote.go
- **List / remove hosts:** hosts.go
- **SSH execution:** remote.go
- **Hollow service install:** hosts.go, remote.go, sync.go
- **Host URL discovery:** remote.go, hosts.go
- **Host port / idle settings:** hosts.go, sync.go, hosts_test.go
- **Served desktop hosts via connect code:** hosts.go

## How it works

- **Host add:** Parse dest or connect code plus --port/--idle flags in hosts.go (hostAdd) → For a code, register the served desktop as NAME/desktop without ssh or install → For an ssh dest, build remote{} in remote.go (local vs ssh dest) → Probe ssh reachability → Send and install hollow binary as service with settings in /etc/hollow.env → Discover host URL for the configured port → Save {ssh,url,added} to hosts.json via saveManaged

## Key files

- hosts.go — managed{ssh,url,added} state, loadManaged/saveManaged on hosts.json, host add/ls/rm commands including connect-code desktop hosts
- remote.go — remote{dest,host,local} type, ssh invocation with BatchMode/ConnectTimeout/accept-new, script runner, hollow install and URL discovery
- hosts_test.go — unit tests for host-settings logic, generated shell run against temp file under dash and nested sh -c quoting

## Rules and traps

- Hosts are reached over ssh and nothing else — any makima/Tailscale name, address, or ~/.ssh/config alias works; nothing is preinstalled remotely, phaethon ships hollow.
- ssh always uses BatchMode=yes, ConnectTimeout=10, StrictHostKeyChecking=accept-new; do not add interactive auth.
- hosts.json lives under stateDir() with dir 0700 / file 0600 and shape {"hosts": {name: {ssh,url,added}}}; tolerate missing or malformed file as empty.
- Local destination bypasses ssh entirely (sh -c directly); keep local and remote paths distinct.
- Per-host port/idle live in /etc/hollow.env, not hardcoded (hollow always listens where told via env); --port must change both the service and the URL given to bangboo, upgrades must carry settings forward including moving legacy unit-file idle into the env file, and wait until the host answers as the new version instead of a fixed sleep.
- Served desktops added via connect code are not ssh hosts: sync and doctor must leave them alone.

## What it talks to

- CLI — cmdHost add/ls/rm entrypoint dispatches here from main.go
- Bundle — hollow binary payload this layer sends and installs remotely
- Sync and health — reads hosts.json state and discovered URLs to run bangboo host ls / doctor; sync preserves per-host settings on upgrade
