package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

// A machine phaethon sets up is reached over ssh, and nothing else: whatever
// ssh reaches — a makima name, a Tailscale name or address, an alias in
// ~/.ssh/config — can become a host. Nothing needs to be installed on it
// first; phaethon carries the hollow binary and sends it.

type remote struct {
	dest  string // user@host, or host
	host  string // the host part, for URLs
	local bool   // this machine: no ssh at all
}

func sshArgs(extra ...string) []string {
	return append([]string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-o", "StrictHostKeyChecking=accept-new"}, extra...)
}

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// run runs a shell script there and returns its stdout.
func (r remote) run(ctx context.Context, script string, stdin io.Reader) (string, error) {
	var cmd *exec.Cmd
	if r.local {
		cmd = exec.CommandContext(ctx, "sh", "-c", script)
	} else {
		cmd = exec.CommandContext(ctx, "ssh", sshArgs(r.dest, "sh -c "+shQuote(script))...)
	}
	var out, errb bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		return out.String(), fmt.Errorf("%s: %s", r.name(), lastLines(msg, 15))
	}
	return out.String(), nil
}

// interactive runs a command there attached to this terminal, for a sudo
// that wants a password.
func (r remote) interactive(ctx context.Context, command string) error {
	var cmd *exec.Cmd
	if r.local {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	} else {
		cmd = exec.CommandContext(ctx, "ssh", "-t", "-o", "ConnectTimeout=10", r.dest, command)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stderr, os.Stderr
	return cmd.Run()
}

func (r remote) name() string {
	if r.local {
		return "this machine"
	}
	return r.dest
}

func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func isLocal(target string) bool {
	switch target {
	case "local", "localhost", "this", ".":
		return true
	}
	return false
}

// findRemote works out how to ssh to target. A bare name is tried as given
// — ssh's own config and MagicDNS may know it — and then as a makima name;
// with no user given, as ssh's default user and then as root.
func findRemote(ctx context.Context, target string) (remote, error) {
	if isLocal(target) {
		return remote{local: true, host: "127.0.0.1"}, nil
	}
	user, host, hasUser := strings.Cut(target, "@")
	if !hasUser {
		host, user = target, ""
	}
	hostsToTry := []string{host}
	if !strings.Contains(host, ".") && net.ParseIP(host) == nil {
		if _, err := net.DefaultResolver.LookupHost(ctx, host+".makima"); err == nil {
			hostsToTry = append(hostsToTry, host+".makima")
		}
	}
	// Root first when no user is given: setting a machine up means
	// installing a service, and a normal user's sudo may want a password
	// that nobody is there to type.
	usersToTry := []string{user}
	if user == "" {
		usersToTry = []string{"root", ""}
	}
	var tried []string
	for _, h := range hostsToTry {
		for _, u := range usersToTry {
			dest := h
			if u != "" {
				dest = u + "@" + h
			}
			tried = append(tried, dest)
			cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			err := exec.CommandContext(cctx, "ssh", sshArgs(dest, "true")...).Run()
			cancel()
			if err == nil {
				return remote{dest: dest, host: apiHost(ctx, h, dest)}, nil
			}
		}
	}
	return remote{}, fmt.Errorf("could not ssh to %s (tried %s). phaethon needs ssh with a key to the machine — over makima, Tailscale, or anything else ssh reaches — as root or as a user with sudo", target, strings.Join(tried, ", "))
}

// apiHost is the name to reach hollow on that machine by. hollow listens on
// mesh addresses, not the LAN, so a makima name is preferred over whatever
// ssh used — which may be an alias for a .local address — and ssh's own
// idea of the hostname is the fallback.
func apiHost(ctx context.Context, h, dest string) string {
	base := h
	if i := strings.IndexByte(base, '.'); i > 0 && net.ParseIP(base) == nil {
		base = base[:i]
	}
	if net.ParseIP(h) == nil {
		if _, err := net.DefaultResolver.LookupHost(ctx, base+".makima"); err == nil {
			return base + ".makima"
		}
	}
	out, err := exec.CommandContext(ctx, "ssh", "-G", dest).Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if v, ok := strings.CutPrefix(line, "hostname "); ok && v != "" {
				return v
			}
		}
	}
	return h
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
