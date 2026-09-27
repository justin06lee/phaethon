package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// A harness is an agent program that can start bangboo as an MCP server.
// Each keeps its list of servers its own way; where a harness has a command
// for it, the command is used, since it knows its own file format best.
type harness struct {
	name       string
	present    func() bool
	register   func(bin string) error
	unregister func() error
	registered func() bool
}

func home(p ...string) string {
	h, _ := os.UserHomeDir()
	return filepath.Join(append([]string{h}, p...)...)
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func onPath(name string) bool { _, err := exec.LookPath(name); return err == nil }

func quiet(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func claudeDesktopConfig() string {
	if runtime.GOOS == "darwin" {
		return home("Library", "Application Support", "Claude", "claude_desktop_config.json")
	}
	return home(".config", "Claude", "claude_desktop_config.json")
}

var harnesses = []harness{
	{
		name:    "Claude Code",
		present: func() bool { return onPath("claude") },
		register: func(bin string) error {
			_ = quiet("claude", "mcp", "remove", "bangboo", "-s", "user")
			return quiet("claude", "mcp", "add", "-s", "user", "bangboo", "--", bin, "mcp")
		},
		unregister: func() error { return quiet("claude", "mcp", "remove", "bangboo", "-s", "user") },
		registered: func() bool {
			// ~/.claude.json holds user-scoped servers; reading it is much
			// cheaper than `claude mcp get`, which starts the server.
			return jsonHas(home(".claude.json"), "mcpServers", "bangboo")
		},
	},
	{
		name:    "Codex",
		present: func() bool { return onPath("codex") || exists(home(".codex")) },
		register: func(bin string) error {
			if onPath("codex") {
				_ = quiet("codex", "mcp", "remove", "bangboo")
				return quiet("codex", "mcp", "add", "bangboo", "--", bin, "mcp")
			}
			return tomlServer(home(".codex", "config.toml"), bin)
		},
		unregister: func() error {
			if onPath("codex") {
				return quiet("codex", "mcp", "remove", "bangboo")
			}
			return tomlRemove(home(".codex", "config.toml"))
		},
		registered: func() bool {
			data, _ := os.ReadFile(home(".codex", "config.toml"))
			return tomlHasServer(string(data))
		},
	},
	{
		name:    "Gemini CLI",
		present: func() bool { return onPath("gemini") || exists(home(".gemini")) },
		register: func(bin string) error {
			if onPath("gemini") {
				_ = quiet("gemini", "mcp", "remove", "-s", "user", "bangboo")
				return quiet("gemini", "mcp", "add", "-s", "user", "bangboo", bin, "mcp")
			}
			return jsonSet(home(".gemini", "settings.json"), stdio(bin), "mcpServers", "bangboo")
		},
		unregister: func() error {
			if onPath("gemini") {
				return quiet("gemini", "mcp", "remove", "-s", "user", "bangboo")
			}
			return jsonDelete(home(".gemini", "settings.json"), "mcpServers", "bangboo")
		},
		registered: func() bool { return jsonHas(home(".gemini", "settings.json"), "mcpServers", "bangboo") },
	},
	{
		name:    "Cursor",
		present: func() bool { return exists(home(".cursor")) },
		register: func(bin string) error {
			return jsonSet(home(".cursor", "mcp.json"), stdio(bin), "mcpServers", "bangboo")
		},
		unregister: func() error { return jsonDelete(home(".cursor", "mcp.json"), "mcpServers", "bangboo") },
		registered: func() bool { return jsonHas(home(".cursor", "mcp.json"), "mcpServers", "bangboo") },
	},
	{
		name:    "Claude Desktop",
		present: func() bool { return exists(filepath.Dir(claudeDesktopConfig())) },
		register: func(bin string) error {
			return jsonSet(claudeDesktopConfig(), stdio(bin), "mcpServers", "bangboo")
		},
		unregister: func() error { return jsonDelete(claudeDesktopConfig(), "mcpServers", "bangboo") },
		registered: func() bool { return jsonHas(claudeDesktopConfig(), "mcpServers", "bangboo") },
	},
	{
		name:    "OpenCode",
		present: func() bool { return onPath("opencode") || exists(home(".config", "opencode")) },
		register: func(bin string) error {
			return jsonSet(opencodeConfig(), opencodeServer{Type: "local", Command: []string{bin, "mcp"}, Enabled: true}, "mcp", "bangboo")
		},
		unregister: func() error {
			for _, f := range opencodeConfigs() {
				if err := jsonDelete(f, "mcp", "bangboo"); err != nil {
					return err
				}
			}
			return nil
		},
		registered: func() bool { return jsonHas(opencodeConfig(), "mcp", "bangboo") },
	},
}

// stdioServer is how most harnesses write a server they start themselves:
// the fields in the order people write them.
type stdioServer struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func stdio(bin string) stdioServer { return stdioServer{Command: bin, Args: []string{"mcp"}} }

type opencodeServer struct {
	Type    string   `json:"type"`
	Command []string `json:"command"`
	Enabled bool     `json:"enabled"`
}

// OpenCode reads opencode.json and opencode.jsonc from its config folder.
func opencodeConfigs() []string {
	dir := home(".config", "opencode")
	return []string{filepath.Join(dir, "opencode.jsonc"), filepath.Join(dir, "opencode.json")}
}

// opencodeConfig is the one bangboo goes in: the file that already has it,
// or else the one there is, opencode.json when there is neither.
func opencodeConfig() string {
	files := opencodeConfigs()
	for _, f := range files {
		if jsonHas(f, "mcp", "bangboo") {
			return f
		}
	}
	for _, f := range files {
		if exists(f) {
			return f
		}
	}
	return files[1]
}

// backup keeps the first version of a config file phaethon touches, beside
// it, so that whatever phaethon did can be undone by hand.
func backup(path string) {
	b := path + ".before-phaethon"
	if exists(path) && !exists(b) {
		if data, err := os.ReadFile(path); err == nil {
			_ = os.WriteFile(b, data, 0o600)
		}
	}
}

// readConfig reads a config file; one that is not there is empty.
func readConfig(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

// writeConfig replaces a config file with data, keeping its permissions,
// after keeping its first version beside it.
func writeConfig(path string, data []byte) error {
	backup(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp := path + ".phaethon-tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func jsonSet(path string, value any, keys ...string) error {
	data, err := readConfig(path)
	if err != nil {
		return err
	}
	out, err := jsonSetText(data, keys, value)
	if err != nil {
		return fmt.Errorf("%s: %v; add bangboo to it by hand", path, err)
	}
	if bytes.Equal(out, data) {
		return nil
	}
	return writeConfig(path, out)
}

func jsonDelete(path string, keys ...string) error {
	data, err := readConfig(path)
	if err != nil {
		return err
	}
	out, found, err := jsonDeleteText(data, keys)
	if err != nil {
		return fmt.Errorf("%s: %v; take bangboo out of it by hand", path, err)
	}
	if !found {
		return nil
	}
	return writeConfig(path, out)
}

func jsonHas(path string, keys ...string) bool {
	data, err := readConfig(path)
	return err == nil && jsonHasText(data, keys)
}

func tomlServer(path, bin string) error {
	data, err := readConfig(path)
	if err != nil {
		return err
	}
	out, err := tomlSetServer(string(data), bin)
	if err != nil {
		return fmt.Errorf("%s: %v", path, err)
	}
	if out == string(data) {
		return nil
	}
	return writeConfig(path, []byte(out))
}

func tomlRemove(path string) error {
	data, err := readConfig(path)
	if err != nil {
		return err
	}
	out, found := tomlRemoveServer(string(data))
	if !found {
		return nil
	}
	return writeConfig(path, []byte(out))
}

// The skill goes where each harness looks for skills. bmo, when it is here,
// does that and keeps track of it; otherwise it is copied into the global
// skills directory of every harness that has one.
var skillDirs = []string{
	".claude/skills", ".agents/skills", ".cursor/skills", ".gemini/skills", ".config/opencode/skills",
}

func skillSource() (string, error) {
	data, err := bundled.ReadFile("skills/phaethon/SKILL.md")
	if err != nil {
		return "", err
	}
	dir := filepath.Join(stateDir(), "skills", "phaethon")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, os.WriteFile(filepath.Join(dir, "SKILL.md"), data, 0o644)
}

func installSkill() ([]string, error) {
	src, err := skillSource()
	if err != nil {
		return nil, err
	}
	if onPath("bmo") {
		out, err := exec.Command("bmo", "add", "everywhere", src, "everyone", "--global", "--yes", "--force").CombinedOutput()
		if err == nil {
			return skillPlaces(), nil
		}
		fmt.Fprintf(os.Stderr, "  bmo could not install the skill (%s); copying it instead\n", strings.TrimSpace(string(out)))
	}
	data, _ := os.ReadFile(filepath.Join(src, "SKILL.md"))
	var placed []string
	for _, d := range skillDirs {
		parent := home(filepath.Dir(d))
		if !exists(parent) {
			continue
		}
		dir := home(d, "phaethon")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return placed, err
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), data, 0o644); err != nil {
			return placed, err
		}
		placed = append(placed, dir)
	}
	return placed, nil
}

func skillPlaces() []string {
	var out []string
	for _, d := range skillDirs {
		if p := home(d, "phaethon", "SKILL.md"); exists(p) {
			out = append(out, filepath.Dir(p))
		}
	}
	return out
}

func removeSkill() {
	if onPath("bmo") {
		_ = quiet("bmo", "remove", "phaethon", "everywhere", "everyone", "--yes")
	}
	for _, d := range skillDirs {
		dir := home(d, "phaethon")
		if exists(filepath.Join(dir, "SKILL.md")) {
			os.RemoveAll(dir)
		}
	}
}
