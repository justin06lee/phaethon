package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func say(format string, a ...any) { fmt.Fprintf(os.Stderr, "  "+format+"\n", a...) }

// installBinaries puts bangboo and the hollow CLI beside phaethon. It
// reports what changed, so a repeat install can say it did nothing.
func installBinaries() (dir string, changed []string, err error) {
	dir = binDir()
	for _, prog := range []string{"bangboo", "hollow"} {
		data, err := payload(localName(prog))
		if err != nil {
			return dir, changed, err
		}
		path := filepath.Join(dir, prog)
		if fileSHA(path) == sha(data) {
			continue
		}
		if err := writeBinary(path, data); err != nil {
			return dir, changed, err
		}
		changed = append(changed, prog)
	}
	return dir, changed, nil
}

func bangbooPath() string {
	p := filepath.Join(binDir(), "bangboo")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	if q, err := exec.LookPath("bangboo"); err == nil {
		return q
	}
	return p
}

func hollowPath() string {
	p := filepath.Join(binDir(), "hollow")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	if q, err := exec.LookPath("hollow"); err == nil {
		return q
	}
	return p
}

// runBangboo runs bangboo with its output going straight to the terminal.
func runBangboo(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, bangbooPath(), args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// bangbooOut runs bangboo and returns what it printed.
func bangbooOut(ctx context.Context, args ...string) ([]byte, error) {
	var out, errb bytes.Buffer
	cmd := exec.CommandContext(ctx, bangbooPath(), args...)
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.Bytes(), fmt.Errorf("bangboo %s: %s", strings.Join(args, " "), strings.TrimSpace(errb.String()+" "+out.String()))
	}
	return out.Bytes(), nil
}

func cmdInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	noMCP := fs.Bool("no-mcp", false, "do not register bangboo with agent harnesses")
	noSkill := fs.Bool("no-skill", false, "do not install the phaethon skill")
	if err := fs.Parse(args); err != nil {
		return err
	}
	v := bundleVersions()
	fmt.Fprintf(os.Stderr, "\nphaethon %s\n\n", version)

	dir, changed, err := installBinaries()
	if err != nil {
		return err
	}
	if len(changed) > 0 {
		say("installed %s in %s (bangboo %s, hollow %s)", strings.Join(changed, " and "), dir, orDash(v.Bangboo), orDash(v.Hollow))
	} else {
		say("bangboo %s and hollow %s are already in %s", orDash(v.Bangboo), orDash(v.Hollow), dir)
	}
	if !onPathDir(dir) {
		say("%s is not on your PATH; add it: export PATH=\"%s:$PATH\"", dir, dir)
	}

	if !*noMCP {
		bin := filepath.Join(dir, "bangboo")
		n := 0
		for _, h := range harnesses {
			if !h.present() {
				continue
			}
			n++
			if err := h.register(bin); err != nil {
				say("%-15s could not register bangboo: %v", h.name, err)
				continue
			}
			say("%-15s bangboo registered as an MCP server", h.name)
		}
		if n == 0 {
			say("no agent harness found here; register `%s mcp` as a stdio MCP server in yours", bin)
		}
	}
	if !*noSkill {
		places, err := installSkill()
		if err != nil {
			say("could not install the skill: %v", err)
		} else if len(places) > 0 {
			say("the phaethon skill is in %d harness skill folders", len(places))
		}
	}

	hosts, _ := bangbooOut(context.Background(), "host", "ls", "--json")
	fmt.Fprintln(os.Stderr)
	if bytes.HasPrefix(bytes.TrimSpace(hosts), []byte("[]")) || len(bytes.TrimSpace(hosts)) == 0 || bytes.Equal(bytes.TrimSpace(hosts), []byte("null")) {
		say("next: give it a host — any Linux machine with KVM that ssh reaches:")
		say("    phaethon host add MACHINE      (or: phaethon scan, to find ones already running hollow)")
	} else {
		say("ready. Agents started from now on have bangboo's desk tools.")
	}
	fmt.Fprintln(os.Stderr)
	return nil
}

func onPathDir(dir string) bool {
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.Clean(p) == filepath.Clean(dir) {
			return true
		}
	}
	return false
}

func cmdUninstall(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	keepBins := fs.Bool("keep-binaries", false, "leave bangboo and hollow installed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	for _, h := range harnesses {
		if h.present() && h.registered() {
			if err := h.unregister(); err != nil {
				say("%-15s %v", h.name, err)
			} else {
				say("%-15s bangboo removed", h.name)
			}
		}
	}
	removeSkill()
	say("the phaethon skill is removed")
	if !*keepBins {
		for _, prog := range []string{"bangboo", "hollow"} {
			p := filepath.Join(binDir(), prog)
			if _, err := os.Stat(p); err == nil {
				os.Remove(p)
				say("removed %s", p)
			}
		}
	}
	say("hosts keep running; phaethon host rm NAME --uninstall takes hollow off one")
	return nil
}
