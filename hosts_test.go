package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func intp(n int) *int                     { return &n }
func durp(d time.Duration) *time.Duration { return &d }
func sh(t *testing.T, script string) string {
	t.Helper()
	out, err := exec.Command("sh", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("sh: %v\n%s\n--- script:\n%s", err, out, script)
	}
	return string(out)
}

func TestEnvValue(t *testing.T) {
	file := "# hollow\nHOLLOW_PORT=8080\n  HOLLOW_IDLE = \"30m\"\nOTHER=x\nHOLLOW_PORT='9090'\n"
	if got := envValue(file, "HOLLOW_PORT"); got != "9090" {
		t.Errorf("HOLLOW_PORT = %q, want the later line, 9090", got)
	}
	if got := envValue(file, "HOLLOW_IDLE"); got != "30m" {
		t.Errorf("HOLLOW_IDLE = %q, want 30m", got)
	}
	if got := envValue(file, "MISSING"); got != "" {
		t.Errorf("MISSING = %q", got)
	}
}

func TestSettle(t *testing.T) {
	for _, c := range []struct {
		name     string
		env      string
		unitIdle string
		want     hostSettings
		port     int
		changes  map[string]string
	}{
		{"fresh host", "", "", hostSettings{}, 7070, map[string]string{}},
		{"keeps its port", "HOLLOW_PORT=8080\n", "", hostSettings{}, 8080, map[string]string{}},
		{"keeps its idle timeout", "HOLLOW_IDLE=1h0m0s\n", "", hostSettings{}, 7070, map[string]string{}},
		{"a new port", "", "", hostSettings{port: intp(9090)}, 9090, map[string]string{"HOLLOW_PORT": "9090"}},
		{"back to the default port", "HOLLOW_PORT=8080\n", "", hostSettings{port: intp(7070)}, 7070, map[string]string{"HOLLOW_PORT": ""}},
		{"a new idle timeout", "", "", hostSettings{idle: durp(2 * time.Hour)}, 7070, map[string]string{"HOLLOW_IDLE": "2h0m0s"}},
		{"idle turned off", "HOLLOW_IDLE=1h\n", "", hostSettings{idle: durp(0)}, 7070, map[string]string{"HOLLOW_IDLE": ""}},
		{"an idle timeout on the unit moves to the file", "", "30m0s", hostSettings{}, 7070, map[string]string{"HOLLOW_IDLE": "30m0s"}},
		{"the unit's idle outranks the file's", "HOLLOW_IDLE=1h\n", "30m0s", hostSettings{}, 7070, map[string]string{"HOLLOW_IDLE": "30m0s"}},
		{"asking outranks the unit", "", "30m0s", hostSettings{idle: durp(0)}, 7070, map[string]string{"HOLLOW_IDLE": ""}},
		{"a unit idle that is not a duration is left alone", "", "soon", hostSettings{}, 7070, map[string]string{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			port, changes := settle(c.env, c.unitIdle, c.want)
			if port != c.port || !reflect.DeepEqual(changes, c.changes) {
				t.Errorf("settle = %d %v, want %d %v", port, changes, c.port, c.changes)
			}
		})
	}
}

// envScript run for real, against a file standing in for /etc/hollow.env.
func TestEnvScript(t *testing.T) {
	if envScript(map[string]string{}) != "" {
		t.Error("no changes should make no script")
	}
	for _, c := range []struct {
		name    string
		before  *string // nil: no file
		changes map[string]string
		after   *string // nil: still no file
	}{
		{"removing from no file makes none", nil, map[string]string{"HOLLOW_PORT": ""}, nil},
		{"setting makes the file", nil, map[string]string{"HOLLOW_PORT": "8080"}, strp("HOLLOW_PORT=8080\n")},
		{"other lines stay", strp("# mine\nFOO=1\nHOLLOW_PORT=9000\n"), map[string]string{"HOLLOW_PORT": "8080", "HOLLOW_IDLE": "1h0m0s"},
			strp("# mine\nFOO=1\nHOLLOW_PORT=8080\nHOLLOW_IDLE=1h0m0s\n")},
		{"removing", strp("FOO=1\n  HOLLOW_IDLE=1h\n"), map[string]string{"HOLLOW_IDLE": ""}, strp("FOO=1\n")},
		{"removing the last line leaves the file", strp("HOLLOW_PORT=8080\n"), map[string]string{"HOLLOW_PORT": ""}, strp("")},
		{"untouched variables stay", strp("HOLLOW_PORT=8080\nHOLLOW_IDLE=1h\n"), map[string]string{"HOLLOW_IDLE": "2h0m0s"},
			strp("HOLLOW_PORT=8080\nHOLLOW_IDLE=2h0m0s\n")},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "hollow.env")
			if c.before != nil {
				if err := os.WriteFile(path, []byte(*c.before), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			sh(t, strings.ReplaceAll(envScript(c.changes), hostEnvFile, path))
			data, err := os.ReadFile(path)
			switch {
			case c.after == nil && err == nil:
				t.Fatalf("made %s: %q", path, data)
			case c.after == nil:
				return
			case err != nil:
				t.Fatal(err)
			case string(data) != *c.after:
				t.Errorf("file = %q, want %q", data, *c.after)
			}
			if c.before != nil {
				if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
					t.Errorf("mode = %v, want the file's own 0600", st.Mode().Perm())
				}
			}
			if _, err := os.Stat(path + ".phaethon"); err == nil {
				t.Error("left its temporary file behind")
			}
		})
	}
}

func strp(s string) *string { return &s }

// The probe's last section: the idle timeout an older phaethon put on the
// unit's command line.
func TestProbeReadsUnitIdle(t *testing.T) {
	i := strings.LastIndex(probeScript, "sed -n")
	sed := probeScript[i : strings.Index(probeScript[i:], "\n")+i]
	for unit, want := range map[string]string{
		"ExecStart=/usr/local/bin/hollow serve --quiet --idle 30m0s\n":  "30m0s",
		"ExecStart=/usr/local/bin/hollow serve --quiet\n":               "",
		"ExecStart=/usr/local/bin/hollow serve --idle 2h0m0s --quiet\n": "2h0m0s",
	} {
		path := filepath.Join(t.TempDir(), "hollow.service")
		os.WriteFile(path, []byte("[Service]\n"+unit), 0o644)
		got := strings.TrimSpace(sh(t, strings.ReplaceAll(sed, "/etc/systemd/system/hollow.service", path)))
		if got != want {
			t.Errorf("%q: got %q, want %q", unit, got, want)
		}
	}
}

func TestHostURL(t *testing.T) {
	for host, want := range map[string]string{
		"tenet.makima": "http://tenet.makima:7070",
		"100.64.0.1":   "http://100.64.0.1:7070",
		"fd7a::1":      "http://[fd7a::1]:7070",
	} {
		if got := hostURL(host, 7070); got != want {
			t.Errorf("hostURL(%q) = %q, want %q", host, got, want)
		}
	}
}

// The whole root script, through the same layers of quoting a machine
// reached as a user with sudo gets: ssh's sh -c, then sudo's sh -c.
func TestInstallScript(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "hollow.env")
	log := filepath.Join(dir, "log")
	bin := filepath.Join(dir, "hollow it's")
	os.WriteFile(bin, []byte("#!/bin/sh\necho \"port=$HOLLOW_PORT args=$*\" > '"+log+"'\necho hollow1-code\n"), 0o755)
	script := installScript(map[string]string{"HOLLOW_PORT": "8080", "HOLLOW_IDLE": "1h0m0s"}, bin, "http://tenet.makima:8080", 8080)
	script = strings.ReplaceAll(script, hostEnvFile, env)
	if out := sh(t, "sh -c "+shQuote("sh -c "+shQuote(script))); out != "" {
		t.Errorf("printed %q; the connect code should go nowhere", out)
	}
	got, _ := os.ReadFile(log)
	if want := "port=8080 args=service install --quiet --url http://tenet.makima:8080\n"; string(got) != want {
		t.Errorf("hollow ran as %q, want %q", got, want)
	}
	if data, _ := os.ReadFile(env); string(data) != "HOLLOW_PORT=8080\nHOLLOW_IDLE=1h0m0s\n" {
		t.Errorf("env file = %q", data)
	}
}
