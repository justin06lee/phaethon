package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/term"
)

// managed is what phaethon remembers about a host it set up: how to get
// back to it over ssh, to upgrade it later.
type managed struct {
	SSH   string    `json:"ssh"`
	URL   string    `json:"url,omitempty"`
	Added time.Time `json:"added"`
}

func loadManaged() map[string]managed {
	m := map[string]managed{}
	data, err := os.ReadFile(filepath.Join(stateDir(), "hosts.json"))
	if err == nil {
		var f struct {
			Hosts map[string]managed `json:"hosts"`
		}
		if json.Unmarshal(data, &f) == nil && f.Hosts != nil {
			m = f.Hosts
		}
	}
	return m
}

func saveManaged(m map[string]managed) error {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(map[string]any{"hosts": m}, "", "  ")
	return os.WriteFile(filepath.Join(stateDir(), "hosts.json"), append(data, '\n'), 0o600)
}

func cmdHost(ctx context.Context, args []string) error {
	if len(args) == 0 {
		args = []string{"ls"}
	}
	switch args[0] {
	case "add":
		return hostAdd(ctx, args[1:])
	case "ls", "list":
		if err := runBangboo(ctx, append([]string{"host", "ls"}, args[1:]...)...); err != nil {
			return err
		}
		m := loadManaged()
		if len(m) > 0 {
			names := make([]string, 0, len(m))
			for n := range m {
				names = append(names, n)
			}
			sort.Strings(names)
			fmt.Println()
			for _, n := range names {
				fmt.Printf("%s is managed by phaethon over ssh as %s\n", n, m[n].SSH)
			}
		}
		return nil
	case "rm", "remove":
		return hostRm(ctx, args[1:])
	}
	return fmt.Errorf("unknown host command %q: add, ls or rm", args[0])
}

// parse reads flags wherever they sit among the arguments.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return pos, nil
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

type connectInfo struct {
	Code    string   `json:"code"`
	Name    string   `json:"name"`
	URLs    []string `json:"urls"`
	Version string   `json:"version"`
}

func hostAdd(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("host add", flag.ContinueOnError)
	name := fs.String("name", "", "what to call it (default: its hostname)")
	port := fs.Int("port", 7070, "the port hollow listens on")
	idle := fs.Duration("idle", 0, "have the host stop desks unused for this long (0: never)")
	noPull := fs.Bool("no-pull", false, "do not build the Linux image now")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: phaethon host add MACHINE   (MACHINE: tenet, tenet.makima, root@100.x.y.z, or local)")
	}
	fmt.Fprintf(os.Stderr, "\nsetting up %s\n\n", pos[0])

	r, err := findRemote(ctx, pos[0])
	if err != nil {
		return err
	}
	if !r.local {
		say("reached %s over ssh", r.dest)
	}
	if err := setupHollow(ctx, r, *port, *idle); err != nil {
		return err
	}

	info, err := connectCode(ctx, r, *port)
	if err != nil {
		return err
	}
	hostName := *name
	if hostName == "" {
		hostName = info.Name
	}
	if _, err := bangbooOut(ctx, "host", "add", info.Code, "--name", hostName, "--quiet"); err != nil {
		return fmt.Errorf("%s is set up, but this machine cannot reach it on port %d at any of %s.\n"+
			"hollow listens only on loopback and on makima / Tailscale / WireGuard addresses: put both machines on the same one.\n(%v)",
			hostName, *port, strings.Join(info.URLs, ", "), err)
	}
	reach, _ := bangbooOut(ctx, "host", "ls", "--json")
	var rows []struct {
		Name  string `json:"name"`
		Reach string `json:"reachable_at"`
	}
	_ = json.Unmarshal(reach, &rows)
	for _, row := range rows {
		if row.Name == hostName {
			say("bangboo can reach %s at %s", hostName, row.Reach)
		}
	}
	if !r.local {
		m := loadManaged()
		m[hostName] = managed{SSH: r.dest, URL: fmt.Sprintf("http://%s:%d", r.host, *port), Added: time.Now().UTC().Truncate(time.Second)}
		if err := saveManaged(m); err != nil {
			return err
		}
	}

	if !*noPull {
		if err := ensureImage(ctx, hostName, info.Code); err != nil {
			return err
		}
	}
	fmt.Fprintln(os.Stderr)
	say("%s is a host. Agents here can start desks on it now (desk_new).", hostName)
	fmt.Fprintln(os.Stderr)
	return nil
}

// setupHollow sends the bundled hollow to the machine and has it install
// itself as a service — which also upgrades one that is already there.
func setupHollow(ctx context.Context, r remote, port int, idle time.Duration) error {
	probe, err := r.run(ctx, `uname -s; uname -m; id -u; [ -e /dev/kvm ] && echo kvm || echo nokvm; command -v systemctl >/dev/null && echo systemd || echo nosystemd; (hollow version 2>/dev/null || echo "hollow none") | head -1`, nil)
	if err != nil {
		return err
	}
	f := strings.Fields(probe)
	if len(f) < 5 {
		return fmt.Errorf("unexpected answer from %s: %q", r.name(), probe)
	}
	osName, arch, uid, kvm, systemd := f[0], f[1], f[2], f[3], f[4]
	switch {
	case osName != "Linux":
		return fmt.Errorf("%s runs %s; hollow hosts are Linux machines with KVM (macOS and Windows guests are not supported yet)", r.name(), osName)
	case arch != "x86_64":
		return fmt.Errorf("%s is %s; hollow needs an x86_64 machine", r.name(), arch)
	case kvm != "kvm":
		return fmt.Errorf("%s has no /dev/kvm: turn on virtualization (VT-x or AMD-V) in its firmware", r.name())
	case systemd != "systemd":
		return fmt.Errorf("%s has no systemd; run `hollow serve` there under whatever supervises processes, then `bangboo host add` its connect code", r.name())
	}
	data, err := payload("hollow-linux-amd64")
	if err != nil {
		return err
	}
	say("sending hollow %s (%.0f MB)", orDash(bundleVersions().Hollow), float64(len(data))/(1<<20))
	// A fresh name each time: a file left in /tmp by another user cannot be
	// written over, not even by root, where fs.protected_regular is on.
	var tmp string
	if r.local {
		f, err := os.CreateTemp("", "hollow.phaethon.*")
		if err != nil {
			return err
		}
		tmp = f.Name()
		f.Close()
		if err := writeBinary(tmp, data); err != nil {
			return err
		}
	} else {
		out, err := r.run(ctx, `t=$(mktemp /tmp/hollow.phaethon.XXXXXX) && cat > "$t" && chmod 0755 "$t" && echo "$t"`, bytesReader(data))
		if err != nil {
			return err
		}
		tmp = strings.TrimSpace(out)
	}
	defer r.run(context.Background(), "rm -f "+tmp, nil)
	install := fmt.Sprintf("%s service install --quiet --url http://%s:%d", tmp, r.host, port)
	if idle > 0 {
		install += " --idle " + idle.String()
	}
	say("installing it as a service (QEMU too, if it is missing)")
	switch {
	case uid == "0":
		_, err = r.run(ctx, install+" >/dev/null", nil)
	default:
		_, err = r.run(ctx, "sudo -n "+install+" >/dev/null", nil)
		if err != nil && strings.Contains(err.Error(), "password") {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return fmt.Errorf("sudo on %s wants a password, and there is no terminal here to type it in; run phaethon host add from a terminal, or ssh in as root", r.name())
			}
			say("sudo on %s wants your password:", r.name())
			err = r.interactive(ctx, "sudo "+install+" >/dev/null")
		}
	}
	if err != nil {
		return fmt.Errorf("installing hollow on %s: %w", r.name(), err)
	}
	say("hollow is running on %s, and will be after every boot", r.name())
	return nil
}

func connectCode(ctx context.Context, r remote, port int) (connectInfo, error) {
	cmd := fmt.Sprintf("hollow connect --state /var/lib/hollow --json --url http://%s:%d", r.host, port)
	out, err := r.run(ctx, cmd, nil)
	if err != nil {
		// A user who has only just joined the hollow group gets it at their
		// next login; sudo does not have to wait for that.
		if out, err = r.run(ctx, "sudo -n "+cmd, nil); err != nil {
			return connectInfo{}, fmt.Errorf("reading the connect code on %s: %w", r.name(), err)
		}
	}
	var info connectInfo
	if err := json.Unmarshal([]byte(out), &info); err != nil || info.Code == "" {
		return connectInfo{}, fmt.Errorf("unexpected connect answer from %s: %q", r.name(), out)
	}
	return info, nil
}

func hostRm(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("host rm", flag.ContinueOnError)
	uninstall := fs.Bool("uninstall", false, "also stop hollow on the machine and remove its service")
	purge := fs.Bool("purge", false, "with --uninstall: delete its images and token too")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: phaethon host rm NAME [--uninstall [--purge]]")
	}
	name := pos[0]
	m := loadManaged()
	if *uninstall {
		mg, ok := m[name]
		if !ok {
			return fmt.Errorf("phaethon did not set %s up, so it does not know how to reach it over ssh; run `sudo hollow service uninstall` there", name)
		}
		r := remote{dest: mg.SSH}
		cmd := "hollow service uninstall"
		if *purge {
			cmd += " --purge"
		}
		if _, err := r.run(ctx, "if [ \"$(id -u)\" = 0 ]; then "+cmd+"; else sudo -n "+cmd+"; fi", nil); err != nil {
			return err
		}
		say("hollow is off %s", name)
	}
	if _, err := bangbooOut(ctx, "host", "rm", name); err != nil && !strings.Contains(err.Error(), "no host") {
		return err
	}
	delete(m, name)
	if err := saveManaged(m); err != nil {
		return err
	}
	say("forgot %s", name)
	return nil
}
