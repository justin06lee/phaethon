package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
			return bytes.Contains(data, []byte("[mcp_servers.bangboo]"))
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
			return jsonSet(home(".gemini", "settings.json"), []string{"mcpServers", "bangboo"},
				map[string]any{"command": bin, "args": []string{"mcp"}})
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
			return jsonSet(home(".cursor", "mcp.json"), []string{"mcpServers", "bangboo"},
				map[string]any{"command": bin, "args": []string{"mcp"}})
		},
		unregister: func() error { return jsonDelete(home(".cursor", "mcp.json"), "mcpServers", "bangboo") },
		registered: func() bool { return jsonHas(home(".cursor", "mcp.json"), "mcpServers", "bangboo") },
	},
	{
		name:    "Claude Desktop",
		present: func() bool { return exists(filepath.Dir(claudeDesktopConfig())) },
		register: func(bin string) error {
			return jsonSet(claudeDesktopConfig(), []string{"mcpServers", "bangboo"},
				map[string]any{"command": bin, "args": []string{"mcp"}})
		},
		unregister: func() error { return jsonDelete(claudeDesktopConfig(), "mcpServers", "bangboo") },
		registered: func() bool { return jsonHas(claudeDesktopConfig(), "mcpServers", "bangboo") },
	},
	{
		name:    "OpenCode",
		present: func() bool { return onPath("opencode") || exists(home(".config", "opencode")) },
		register: func(bin string) error {
			return jsonSet(home(".config", "opencode", "opencode.json"), []string{"mcp", "bangboo"},
				map[string]any{"type": "local", "command": []string{bin, "mcp"}, "enabled": true})
		},
		unregister: func() error { return jsonDelete(home(".config", "opencode", "opencode.json"), "mcp", "bangboo") },
		registered: func() bool { return jsonHas(home(".config", "opencode", "opencode.json"), "mcp", "bangboo") },
	},
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

func readJSON(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := map[string]any{}
	if len(bytes.TrimSpace(data)) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s is not plain JSON (%v); add bangboo to it by hand", path, err)
	}
	return m, nil
}

func writeJSON(path string, m map[string]any) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp := path + ".phaethon-tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func jsonSet(path string, keys []string, value any) error {
	m, err := readJSON(path)
	if err != nil {
		return err
	}
	backup(path)
	cur := m
	for _, k := range keys[:len(keys)-1] {
		next, ok := cur[k].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[k] = next
		}
		cur = next
	}
	cur[keys[len(keys)-1]] = value
	return writeJSON(path, m)
}

func jsonDelete(path, section, key string) error {
	m, err := readJSON(path)
	if err != nil || len(m) == 0 {
		return err
	}
	sec, ok := m[section].(map[string]any)
	if !ok {
		return nil
	}
	if _, ok := sec[key]; !ok {
		return nil
	}
	delete(sec, key)
	return writeJSON(path, m)
}

func jsonHas(path, section, key string) bool {
	m, err := readJSON(path)
	if err != nil {
		return false
	}
	sec, ok := m[section].(map[string]any)
	if !ok {
		return false
	}
	_, ok = sec[key]
	return ok
}

var tomlBlock = regexp.MustCompile(`(?ms)^\[mcp_servers\.bangboo\]\n.*?(^\[|\z)`)

func tomlServer(path, bin string) error {
	data, _ := os.ReadFile(path)
	backup(path)
	block := fmt.Sprintf("[mcp_servers.bangboo]\ncommand = %q\nargs = [\"mcp\"]\n\n", bin)
	s := string(data)
	if tomlBlock.MatchString(s) {
		s = tomlBlock.ReplaceAllStringFunc(s, func(m string) string {
			if strings.HasSuffix(m, "[") {
				return block + "["
			}
			return block
		})
	} else {
		if s != "" && !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		s += "\n" + block
	}
	return os.WriteFile(path, []byte(s), 0o644)
}

func tomlRemove(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	s := tomlBlock.ReplaceAllStringFunc(string(data), func(m string) string {
		if strings.HasSuffix(m, "[") {
			return "["
		}
		return ""
	})
	return os.WriteFile(path, []byte(s), 0o644)
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
