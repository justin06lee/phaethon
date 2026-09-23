// Command phaethon puts it all together: one install that gives every agent
// harness on this machine computers to use.
//
// It carries bangboo (the tools an agent calls) and hollow (the VM host),
// installs bangboo here and registers it with every harness it finds,
// installs the skill that teaches an agent to use it, and turns any Linux
// machine ssh can reach into a host.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

var version = "dev"

const usage = `phaethon — computers for every agent on this machine.

  phaethon install             install bangboo and hollow, register bangboo with
                               every agent harness here, install the skill
  phaethon host add MACHINE    make a Linux machine a host, over ssh: MACHINE is
                               anything ssh reaches (tenet, tenet.makima,
                               root@100.101.102.103), or "local" for this one
  phaethon host ls             the hosts, and whether each answers
  phaethon host rm NAME        forget a host (--uninstall also removes hollow from it)
  phaethon scan                find hollows on your makima and Tailscale networks
  phaethon sync                bring everything up to date: this machine's
                               programs and harnesses, every host's hollow, images
  phaethon doctor [--deep]     check it all; --deep also drives a real desk
  phaethon uninstall           undo phaethon install
  phaethon version

Agents use it through bangboo's tools (desk_new, computer, browser_open, …),
which install registers with Claude Code, Codex, Gemini CLI, Cursor, Claude
Desktop and OpenCode — whichever are here.
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	args := os.Args[2:]
	var err error
	switch os.Args[1] {
	case "install", "setup":
		err = cmdInstall(args)
	case "uninstall":
		err = cmdUninstall(args)
	case "host", "hosts":
		err = cmdHost(ctx, args)
	case "scan", "discover":
		err = runBangboo(ctx, append([]string{"host", "scan"}, args...)...)
	case "sync", "update":
		err = cmdSync(ctx, args)
	case "doctor", "check":
		err = cmdDoctor(ctx, args)
	case "version", "-v", "--version":
		v := bundleVersions()
		fmt.Printf("phaethon %s (hollow %s, bangboo %s)\n", version, orDash(v.Hollow), orDash(v.Bangboo))
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "phaethon: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, "phaethon:", err)
		}
		os.Exit(1)
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
