package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type hostRow struct {
	Name    string   `json:"name"`
	URLs    []string `json:"urls"`
	Reach   string   `json:"reachable_at"`
	Error   string   `json:"error"`
	Version string   `json:"version"`
	FreeMB  int      `json:"free_mb"`
	Desks   int      `json:"desks"`
	Images  []struct {
		OS       string `json:"os"`
		State    string `json:"state"`
		Progress string `json:"progress"`
		Outdated bool   `json:"outdated"`
	} `json:"images"`
}

func hostRows(ctx context.Context) ([]hostRow, error) {
	out, err := bangbooOut(ctx, "host", "ls", "--json")
	if err != nil {
		return nil, err
	}
	var rows []hostRow
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("bangboo host ls: %w", err)
	}
	return rows, nil
}

func linuxImage(h hostRow) (state string, outdated bool) {
	for _, img := range h.Images {
		if img.OS == "linux" {
			return img.State, img.Outdated
		}
	}
	return "missing", false
}

// ensureImage builds a host's Linux image if it has none, a failed one, or
// one from an older recipe — and leaves a current one alone.
func ensureImage(ctx context.Context, name, code string) error {
	rows, err := hostRows(ctx)
	if err != nil {
		return err
	}
	for _, h := range rows {
		if h.Name != name || h.Error != "" {
			continue
		}
		state, outdated := linuxImage(h)
		if state == "ready" && !outdated {
			say("%s: the Linux image is built and current", name)
			return nil
		}
		why := "building the Linux image (a few minutes, once)"
		if outdated {
			why = "rebuilding the Linux image with the newer recipe (a few minutes; desks keep running)"
		}
		say("%s: %s", name, why)
		if code == "" {
			c, err := bangbooOut(ctx, "host", "code", name)
			if err != nil {
				return err
			}
			code = strings.TrimSpace(string(c))
		}
		cmd := exec.CommandContext(ctx, hollowPath(), "pull", "linux")
		cmd.Env = append(os.Environ(), "HOLLOW_CONNECT="+code)
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: building the image: %w", name, err)
		}
		return nil
	}
	return fmt.Errorf("%s does not answer; cannot build its image", name)
}

func cmdSync(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	force := fs.Bool("force", false, "upgrade hosts even when desks are running on them (it stops those desks)")
	noImages := fs.Bool("no-images", false, "do not build or rebuild images")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	// This machine first: programs, harness registrations, the skill.
	if err := cmdInstall(nil); err != nil {
		return err
	}
	want := bundleVersions().Hollow
	managedHosts := loadManaged()
	rows, err := hostRows(ctx)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	fmt.Fprintln(os.Stderr, "hosts:")
	for _, h := range rows {
		if h.Error != "" {
			say("%s: does not answer (%s)", h.Name, h.Error)
			continue
		}
		if want != "" && h.Version != want {
			mg, ok := managedHosts[h.Name]
			switch {
			case !ok:
				say("%s: runs hollow %s, this phaethon carries %s — it was not set up by phaethon; `phaethon host add` it to manage it", h.Name, h.Version, want)
			case h.Desks > 0 && !*force:
				say("%s: runs hollow %s, this phaethon carries %s — %d desk(s) running, and upgrading stops them; `phaethon sync --force` to do it anyway", h.Name, h.Version, want, h.Desks)
			default:
				say("%s: upgrading hollow %s → %s", h.Name, h.Version, want)
				r, err := findRemote(ctx, mg.SSH)
				if err != nil {
					say("%s: %v", h.Name, err)
					continue
				}
				if err := setupHollow(ctx, r, 7070, 0); err != nil {
					say("%s: %v", h.Name, err)
					continue
				}
				// The upgrade restarted it; let it come back before asking
				// about images.
				time.Sleep(2 * time.Second)
			}
		} else {
			say("%s: hollow %s, current", h.Name, h.Version)
		}
		if !*noImages {
			if err := ensureImage(ctx, h.Name, ""); err != nil {
				say("%v", err)
			}
		}
	}
	return nil
}

type check struct {
	ok   bool
	what string
	fix  string
}

func (c check) print() {
	mark := "✓"
	if !c.ok {
		mark = "✗"
	}
	fmt.Printf("  %s %s\n", mark, c.what)
	if !c.ok && c.fix != "" {
		fmt.Printf("      %s\n", c.fix)
	}
}

func cmdDoctor(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	deep := fs.Bool("deep", false, "also start a desk, drive its browser and screen, and close it")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	v := bundleVersions()
	failed := 0
	add := func(c check) {
		c.print()
		if !c.ok {
			failed++
		}
	}
	fmt.Printf("phaethon %s, carrying hollow %s and bangboo %s\n\nthis machine:\n", version, orDash(v.Hollow), orDash(v.Bangboo))
	for _, prog := range []string{"bangboo", "hollow"} {
		path := filepath.Join(binDir(), prog)
		data, err := payload(localName(prog))
		switch {
		case err != nil:
			add(check{false, prog + ": not bundled in this phaethon", "rebuild phaethon with make"})
		case fileSHA(path) == "":
			add(check{false, prog + " is not installed", "phaethon install"})
		case fileSHA(path) != sha(data):
			add(check{false, prog + " at " + path + " is not the version phaethon carries", "phaethon install"})
		default:
			add(check{true, prog + " installed at " + path, ""})
		}
	}
	any := false
	for _, h := range harnesses {
		if !h.present() {
			continue
		}
		any = true
		add(check{h.registered(), h.name + ": bangboo registered as an MCP server", "phaethon install"})
	}
	if !any {
		add(check{false, "no agent harness found", "install one (Claude Code, Codex, …) and run phaethon install"})
	}
	places := skillPlaces()
	add(check{len(places) > 0, fmt.Sprintf("phaethon skill installed in %d harness folder(s)", len(places)), "phaethon install"})

	fmt.Println("\nhosts:")
	rows, err := hostRows(ctx)
	if err != nil {
		add(check{false, "bangboo cannot list hosts: " + err.Error(), "phaethon install"})
	}
	if len(rows) == 0 && err == nil {
		add(check{false, "no hosts", "phaethon host add MACHINE, or phaethon scan"})
	}
	usable := 0
	for _, h := range rows {
		if h.Error != "" {
			add(check{false, h.Name + ": does not answer — " + h.Error, "is it on, and on the same makima or Tailscale network as this machine?"})
			continue
		}
		add(check{true, fmt.Sprintf("%s: answering at %s, %.1f GB free, %d desk(s)", h.Name, h.Reach, float64(h.FreeMB)/1024, h.Desks), ""})
		if v.Hollow != "" && h.Version != v.Hollow {
			add(check{false, fmt.Sprintf("%s: runs hollow %s, phaethon carries %s", h.Name, h.Version, v.Hollow), "phaethon sync"})
		}
		state, outdated := linuxImage(h)
		switch {
		case state == "ready" && !outdated:
			add(check{true, h.Name + ": Linux image built", ""})
			usable++
		case state == "ready":
			add(check{false, h.Name + ": Linux image is from an older recipe (it works; a rebuild adds what is new)", "phaethon sync"})
			usable++
		default:
			add(check{false, h.Name + ": Linux image " + state, "phaethon sync"})
		}
	}

	if *deep && usable > 0 {
		fmt.Println("\na desk, end to end:")
		deepCheck(ctx, add)
	}
	fmt.Println()
	if failed > 0 {
		return fmt.Errorf("%d problem(s)", failed)
	}
	fmt.Println("all good")
	return nil
}

func deepCheck(ctx context.Context, add func(check)) {
	env := append(os.Environ(), "BANGBOO_DESK=")
	call := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, bangbooPath(), append([]string{"call"}, args...)...)
		cmd.Env = env
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		err := cmd.Run()
		return out.String(), err
	}
	name := fmt.Sprintf("phaethon-doctor-%d", time.Now().Unix()%100000)
	start := time.Now()
	out, err := call("desk_new", "name="+name, "mem_mb=1024")
	if err != nil {
		add(check{false, "start a desk: " + firstLine(out), ""})
		return
	}
	add(check{true, fmt.Sprintf("started desk %q in %s", name, time.Since(start).Round(time.Second)), ""})
	desk := "--desk=" + name
	out, err = call("browser_open", "url=https://example.com", desk)
	add(check{err == nil && strings.Contains(out, "Example Domain"), "opened example.com and read it as text", firstLine(out)})
	out, err = call("computer", "action=screenshot", desk)
	add(check{err == nil && strings.Contains(out, "[screenshot:"), "took a screenshot", firstLine(out)})
	out, err = call("shell", "command=echo ok-from-desk", desk)
	add(check{err == nil && strings.Contains(out, "ok-from-desk"), "ran a command", firstLine(out)})
	out, err = call("desk_close", desk)
	add(check{err == nil, "closed the desk", firstLine(out)})
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
