package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	port := fs.Int("port", defaultPort, "the port hollow listens on (kept on the host until changed)")
	idle := fs.Duration("idle", 0, "have the host stop desks unused for this long, 0 for never (kept on the host until changed)")
	noPull := fs.Bool("no-pull", false, "do not build the Linux image now")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: phaethon host add MACHINE   (MACHINE: tenet, tenet.makima, root@100.x.y.z, or local)")
	}
	// Only what is given changes: adding a host again keeps its port and
	// idle timeout.
	var want hostSettings
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "port":
			want.port = port
		case "idle":
			want.idle = idle
		}
	})
	if *port < 1 || *port > 65535 {
		return fmt.Errorf("--port %d: not a port", *port)
	}
	if *idle < 0 {
		return fmt.Errorf("--idle %s: cannot be negative", *idle)
	}
	fmt.Fprintf(os.Stderr, "\nsetting up %s\n\n", pos[0])

	r, err := findRemote(ctx, pos[0])
	if err != nil {
		return err
	}
	if !r.local {
		say("reached %s over ssh", r.dest)
	}
	listen, err := setupHollow(ctx, r, want)
	if err != nil {
		return err
	}

	info, err := connectCode(ctx, r, listen)
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
			hostName, listen, strings.Join(info.URLs, ", "), err)
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
		m[hostName] = managed{SSH: r.dest, URL: hostURL(r.host, listen), Added: time.Now().UTC().Truncate(time.Second)}
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

// hollow's port when nothing says otherwise.
const defaultPort = 7070

// A host's port and idle timeout live on the host, in the environment file
// hollow's service reads. An upgrade rewrites the service's unit but leaves
// this file alone, and every machine that manages the host sees the same
// values.
const hostEnvFile = "/etc/hollow.env"

// hostSettings is what to change on a host; a nil field keeps what it has.
type hostSettings struct {
	port *int
	idle *time.Duration
}

func hostURL(host string, port int) string {
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port))
}

// The probe's sections: the machine, hollow's environment file, and the idle
// timeout on the unit's command line, where phaethon used to put it.
const probeSep = "--phaethon-probe--"

const probeScript = `uname -s; uname -m; id -u; [ -e /dev/kvm ] && echo kvm || echo nokvm; command -v systemctl >/dev/null && echo systemd || echo nosystemd; (hollow version 2>/dev/null || echo "hollow none") | head -1
echo ` + probeSep + `
cat ` + hostEnvFile + ` 2>/dev/null || sudo -n cat ` + hostEnvFile + ` 2>/dev/null
echo ` + probeSep + `
sed -n 's/^ExecStart=.* --idle \([^ ]*\).*$/\1/p' /etc/systemd/system/hollow.service 2>/dev/null
true`

// envValue reads one variable from a systemd environment file; a later
// line wins, as it does for systemd.
func envValue(file, key string) string {
	v := ""
	for _, line := range strings.Split(file, "\n") {
		k, val, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.TrimSpace(k) == key {
			v = strings.Trim(strings.TrimSpace(val), `"'`)
		}
	}
	return v
}

// settle works out what a host should run with: what it runs with now —
// the unit's --idle outranks the environment file, as it does for hollow —
// with what was asked for on top. changes holds only the variables the
// environment file needs rewritten; "" removes one.
func settle(env, unitIdle string, want hostSettings) (port int, changes map[string]string) {
	changes = map[string]string{}
	port = defaultPort
	if p, err := strconv.Atoi(envValue(env, "HOLLOW_PORT")); err == nil && p > 0 {
		port = p
	}
	if want.port != nil {
		port = *want.port
		changes["HOLLOW_PORT"] = ""
		if port != defaultPort {
			changes["HOLLOW_PORT"] = strconv.Itoa(port)
		}
	}
	switch {
	case want.idle != nil:
		changes["HOLLOW_IDLE"] = ""
		if *want.idle > 0 {
			changes["HOLLOW_IDLE"] = want.idle.String()
		}
	case unitIdle != "":
		// The upgrade writes a unit without it; the file keeps it.
		if _, err := time.ParseDuration(unitIdle); err == nil {
			changes["HOLLOW_IDLE"] = unitIdle
		}
	}
	return port, changes
}

// envScript rewrites the variables in changes in hollow's environment file,
// leaving every other line as it was. It creates the file only to put
// something in it.
func envScript(changes map[string]string) string {
	var drop []string
	add := ""
	for _, k := range []string{"HOLLOW_PORT", "HOLLOW_IDLE"} {
		v, ok := changes[k]
		if !ok {
			continue
		}
		drop = append(drop, "-e "+shQuote("^[[:space:]]*"+k+"="))
		if v != "" {
			add += k + "=" + v + "\n"
		}
	}
	if len(drop) == 0 {
		return ""
	}
	return fmt.Sprintf(`f=%s
if [ -e "$f" ] || [ -n %s ]; then
	{ [ -e "$f" ] && grep -v %s "$f"; printf %%s %s; } > "$f.phaethon" && cat "$f.phaethon" > "$f" && rm -f "$f.phaethon" || exit 1
fi
`, hostEnvFile, shQuote(add), strings.Join(drop, " "), shQuote(add))
}

// installScript is what runs as root on the machine: hollow's environment
// file brought up to date, then the hollow at bin installing itself as the
// service. HOLLOW_PORT tells the installer where to wait for the service to
// answer; the service itself reads it from the file.
func installScript(changes map[string]string, bin, url string, port int) string {
	return envScript(changes) + fmt.Sprintf("HOLLOW_PORT=%d %s service install --quiet --url %s >/dev/null\n",
		port, shQuote(bin), shQuote(url))
}

// setupHollow sends the bundled hollow to the machine and has it install
// itself as a service — which also upgrades one that is already there. It
// returns the port hollow listens on there.
func setupHollow(ctx context.Context, r remote, want hostSettings) (int, error) {
	probe, err := r.run(ctx, probeScript, nil)
	if err != nil {
		return 0, err
	}
	sections := strings.SplitN(probe, probeSep+"\n", 3)
	f := strings.Fields(sections[0])
	if len(f) < 5 || len(sections) < 3 {
		return 0, fmt.Errorf("unexpected answer from %s: %q", r.name(), probe)
	}
	osName, arch, uid, kvm, systemd := f[0], f[1], f[2], f[3], f[4]
	switch {
	case osName != "Linux":
		return 0, fmt.Errorf("%s runs %s; hollow hosts are Linux machines with KVM (macOS and Windows guests are not supported yet)", r.name(), osName)
	case arch != "x86_64":
		return 0, fmt.Errorf("%s is %s; hollow needs an x86_64 machine", r.name(), arch)
	case kvm != "kvm":
		return 0, fmt.Errorf("%s has no /dev/kvm: turn on virtualization (VT-x or AMD-V) in its firmware", r.name())
	case systemd != "systemd":
		return 0, fmt.Errorf("%s has no systemd; run `hollow serve` there under whatever supervises processes, then `bangboo host add` its connect code", r.name())
	}
	port, changes := settle(sections[1], strings.TrimSpace(sections[2]), want)
	data, err := payload("hollow-linux-amd64")
	if err != nil {
		return 0, err
	}
	say("sending hollow %s (%.0f MB)", orDash(bundleVersions().Hollow), float64(len(data))/(1<<20))
	// A fresh name each time: a file left in /tmp by another user cannot be
	// written over, not even by root, where fs.protected_regular is on.
	var tmp string
	if r.local {
		f, err := os.CreateTemp("", "hollow.phaethon.*")
		if err != nil {
			return 0, err
		}
		tmp = f.Name()
		f.Close()
		if err := writeBinary(tmp, data); err != nil {
			return 0, err
		}
	} else {
		out, err := r.run(ctx, `t=$(mktemp /tmp/hollow.phaethon.XXXXXX) && cat > "$t" && chmod 0755 "$t" && echo "$t"`, bytesReader(data))
		if err != nil {
			return 0, err
		}
		tmp = strings.TrimSpace(out)
	}
	defer r.run(context.Background(), "rm -f "+shQuote(tmp), nil)
	script := installScript(changes, tmp, hostURL(r.host, port), port)
	say("installing it as a service (QEMU too, if it is missing)")
	switch {
	case uid == "0":
		_, err = r.run(ctx, script, nil)
	default:
		_, err = r.run(ctx, "sudo -n sh -c "+shQuote(script), nil)
		if err != nil && strings.Contains(err.Error(), "password") {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return 0, fmt.Errorf("sudo on %s wants a password, and there is no terminal here to type it in; run phaethon host add from a terminal, or ssh in as root", r.name())
			}
			say("sudo on %s wants your password:", r.name())
			err = r.interactive(ctx, "sudo sh -c "+shQuote(script))
		}
	}
	if err != nil {
		return 0, fmt.Errorf("installing hollow on %s: %w", r.name(), err)
	}
	say("hollow is running on %s, and will be after every boot", r.name())
	return port, nil
}

func connectCode(ctx context.Context, r remote, port int) (connectInfo, error) {
	// HOLLOW_PORT: the addresses in the code carry the port hollow listens on.
	cmd := fmt.Sprintf("env HOLLOW_PORT=%d hollow connect --state /var/lib/hollow --json --url %s", port, shQuote(hostURL(r.host, port)))
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
