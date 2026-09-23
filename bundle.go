package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// phaethon carries what it installs: bangboo and the hollow CLI for this
// machine, hollow for the Linux machines it turns into hosts, and the skill
// that teaches an agent to use all of it. Installing phaethon is therefore
// the whole install — nothing is fetched from anywhere.
//
//go:embed bundle skills
var bundled embed.FS

type versions struct {
	Phaethon string `json:"phaethon"`
	Hollow   string `json:"hollow"`
	Bangboo  string `json:"bangboo"`
}

func bundleVersions() versions {
	var v versions
	if data, err := fs.ReadFile(bundled, "bundle/versions.json"); err == nil {
		_ = json.Unmarshal(data, &v)
	}
	return v
}

// payload is one embedded binary.
func payload(name string) ([]byte, error) {
	data, err := fs.ReadFile(bundled, "bundle/"+name)
	if err != nil {
		return nil, fmt.Errorf("this phaethon was built without %s; build it with make, next to hollow and bangboo checkouts", name)
	}
	return data, nil
}

func localName(prog string) string {
	return fmt.Sprintf("%s-%s-%s", prog, runtime.GOOS, runtime.GOARCH)
}

func sha(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func fileSHA(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return sha(data)
}

// binDir is where phaethon puts the programs it installs: beside itself when
// it can write there, ~/.local/bin otherwise.
func binDir() string {
	if v := os.Getenv("PHAETHON_BIN"); v != "" {
		return v
	}
	if self, err := os.Executable(); err == nil {
		if self, err = filepath.EvalSymlinks(self); err == nil {
			dir := filepath.Dir(self)
			if writable(dir) && filepath.Base(filepath.Dir(dir)) != "go-build" && !isTemp(dir) {
				return dir
			}
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin")
}

func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".phaethon-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

func isTemp(dir string) bool {
	tmp, _ := filepath.EvalSymlinks(os.TempDir())
	d, _ := filepath.EvalSymlinks(dir)
	return tmp != "" && len(d) >= len(tmp) && d[:len(tmp)] == tmp
}

// stateDir is phaethon's own state: which hosts it set up and how to reach
// them over ssh, and the skill as last installed.
func stateDir() string {
	if v := os.Getenv("PHAETHON_HOME"); v != "" {
		return v
	}
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return filepath.Join(v, "phaethon")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "phaethon")
}

// writeBinary puts data at path atomically, so a program that is running
// from the old file keeps running and the next start gets the new one.
func writeBinary(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".phaethon-new"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
