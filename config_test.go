package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var server = stdio("/bin/bangboo")

func TestJSONSet(t *testing.T) {
	for _, c := range []struct {
		name, doc, want string
	}{
		{"no file", "", `{
  "mcpServers": {
    "bangboo": {
      "command": "/bin/bangboo",
      "args": [
        "mcp"
      ]
    }
  }
}
`},
		{"an empty object", "{}\n", `{
  "mcpServers": {
    "bangboo": {
      "command": "/bin/bangboo",
      "args": [
        "mcp"
      ]
    }
  }
}
`},
		{"keys keep their order, and a new one goes last", `{
  "zeta": 1,
  "alpha": {"b": 2, "a": 1},
  "mcpServers": {
    "other": {"command": "x"}
  }
}
`, `{
  "zeta": 1,
  "alpha": {"b": 2, "a": 1},
  "mcpServers": {
    "other": {"command": "x"},
    "bangboo": {
      "command": "/bin/bangboo",
      "args": [
        "mcp"
      ]
    }
  }
}
`},
		{"a missing section goes last, in the file's own indentation", "{\n    \"theme\": \"dark\"\n}\n", `{
    "theme": "dark",
    "mcpServers": {
        "bangboo": {
            "command": "/bin/bangboo",
            "args": [
                "mcp"
            ]
        }
    }
}
`},
		{"tabs", "{\n\t\"mcpServers\": {}\n}\n", "{\n\t\"mcpServers\": {\n\t\t\"bangboo\": {\n\t\t\t\"command\": \"/bin/bangboo\",\n\t\t\t\"args\": [\n\t\t\t\t\"mcp\"\n\t\t\t]\n\t\t}\n\t}\n}\n"},
		{"an old registration is replaced where it is", `{
  "mcpServers": {
    "bangboo": {"command": "/old/bangboo", "args": ["mcp"]},
    "other": {}
  }
}
`, `{
  "mcpServers": {
    "bangboo": {
      "command": "/bin/bangboo",
      "args": [
        "mcp"
      ]
    },
    "other": {}
  }
}
`},
		{"comments and trailing commas stay", `// my settings
{
  /* the look */
  "theme": "dark", // not light
  "mcpServers": {
    "other": {"command": "x"}, // mine
  },
}
`, `// my settings
{
  /* the look */
  "theme": "dark", // not light
  "mcpServers": {
    "other": {"command": "x"}, // mine
    "bangboo": {
      "command": "/bin/bangboo",
      "args": [
        "mcp"
      ]
    },
  },
}
`},
		{"a comment beside the last member stays beside it", "{\n  \"theme\": \"dark\" // not light\n}\n", `{
  "theme": "dark", // not light
  "mcpServers": {
    "bangboo": {
      "command": "/bin/bangboo",
      "args": [
        "mcp"
      ]
    }
  }
}
`},
		{"one line stays one line", `{"theme": "dark", "mcpServers": {}}`,
			`{"theme": "dark", "mcpServers": {"bangboo": {"command":"/bin/bangboo","args":["mcp"]}}}`},
		{"windows line endings", "{\r\n  \"theme\": \"dark\"\r\n}\r\n",
			"{\r\n  \"theme\": \"dark\",\r\n  \"mcpServers\": {\r\n    \"bangboo\": {\r\n      \"command\": \"/bin/bangboo\",\r\n      \"args\": [\r\n        \"mcp\"\r\n      ]\r\n    }\r\n  }\r\n}\r\n"},
		{"a section that is not an object is replaced", "{\n  \"mcpServers\": null\n}\n", `{
  "mcpServers": {
    "bangboo": {
      "command": "/bin/bangboo",
      "args": [
        "mcp"
      ]
    }
  }
}
`},
		{"only comments so far", "// nothing yet\n", `// nothing yet
{
  "mcpServers": {
    "bangboo": {
      "command": "/bin/bangboo",
      "args": [
        "mcp"
      ]
    }
  }
}
`},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := jsonSetText([]byte(c.doc), []string{"mcpServers", "bangboo"}, server)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, c.want)
			}
			if !jsonHasText(got, []string{"mcpServers", "bangboo"}) {
				t.Error("the result does not have it")
			}
			// Plain JSON in, plain JSON out.
			if json.Valid([]byte(c.doc)) && !json.Valid(got) {
				t.Errorf("no longer valid JSON:\n%s", got)
			}
		})
	}
}

func TestJSONSetUnchanged(t *testing.T) {
	doc := `{
  "mcpServers": {
    "bangboo": { "args": ["mcp"], "command": "/bin/bangboo" }
  }
}
`
	got, err := jsonSetText([]byte(doc), []string{"mcpServers", "bangboo"}, server)
	if err != nil || string(got) != doc {
		t.Errorf("the same registration, written differently, should be left alone; got %s (%v)", got, err)
	}
}

func TestJSONDelete(t *testing.T) {
	for _, c := range []struct {
		name, doc, want string
	}{
		{"the only one", "{\n  \"mcpServers\": {\n    \"bangboo\": {\"command\": \"x\"}\n  }\n}\n", "{\n  \"mcpServers\": {}\n}\n"},
		{"first of several", `{
  "mcpServers": {
    "bangboo": {"command": "x"}, // phaethon's
    "other": {"command": "y"}
  }
}
`, `{
  "mcpServers": {
    "other": {"command": "y"}
  }
}
`},
		{"last of several", `{
  "mcpServers": {
    "other": {"command": "y"}, // mine
    "bangboo": {
      "command": "x"
    }
  }
}
`, `{
  "mcpServers": {
    "other": {"command": "y"} // mine
  }
}
`},
		{"middle", "{\n  \"a\": 1,\n  \"bangboo\": 2,\n  \"c\": 3\n}\n", "{\n  \"a\": 1,\n  \"c\": 3\n}\n"},
		{"last, with a trailing comma", "{\n  \"a\": 1,\n  \"bangboo\": 2,\n}\n", "{\n  \"a\": 1,\n}\n"},
		{"one line, first", `{"bangboo": 1, "b": 2}`, `{"b": 2}`},
		{"one line, last", `{"a": 1, "bangboo": 2}`, `{"a": 1}`},
		{"windows line endings", "{\r\n  \"a\": 1,\r\n  \"bangboo\": 2\r\n}\r\n", "{\r\n  \"a\": 1\r\n}\r\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			keys := []string{"mcpServers", "bangboo"}
			if !strings.Contains(c.doc, "mcpServers") {
				keys = []string{"bangboo"}
			}
			got, found, err := jsonDeleteText([]byte(c.doc), keys)
			if err != nil || !found {
				t.Fatalf("found=%v err=%v", found, err)
			}
			if string(got) != c.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, c.want)
			}
		})
	}
	doc := []byte(`{"mcpServers": {"other": {}}}`)
	if got, found, err := jsonDeleteText(doc, []string{"mcpServers", "bangboo"}); found || err != nil || string(got) != string(doc) {
		t.Errorf("deleting what is not there changed something: %s %v %v", got, found, err)
	}
}

func TestJSONBroken(t *testing.T) {
	for _, doc := range []string{`{"a": }`, `{"a": 1`, `[1, 2]`, `{"a": 1} x`, `{a: 1}`, `{"a": 1 "b": 2}`} {
		if _, err := jsonSetText([]byte(doc), []string{"mcpServers", "bangboo"}, server); err == nil {
			t.Errorf("%s: no error", doc)
		}
	}
}

func TestTOMLHeader(t *testing.T) {
	for line, want := range map[string]string{
		"[mcp_servers.bangboo]\n":            "mcp_servers/bangboo",
		"  [ mcp_servers . bangboo ] # hi\n": "mcp_servers/bangboo",
		`[mcp_servers."bangboo"]`:            "mcp_servers/bangboo",
		`[mcp_servers.'bangboo'.env]`:        "mcp_servers/bangboo/env",
		"[[profiles]]":                       "profiles",
		`  ["a"],`:                           "",
		"[1, 2]":                             "",
		"args = [":                           "",
	} {
		key, ok := tomlHeader(line)
		if got := strings.Join(key, "/"); ok != (want != "") || (ok && got != want) {
			t.Errorf("%q: %q %v, want %q", line, got, ok, want)
		}
	}
}

func TestTOMLSet(t *testing.T) {
	for _, c := range []struct {
		name, doc, want string
	}{
		{"no file", "", "[mcp_servers.bangboo]\ncommand = \"/bin/bangboo\"\nargs = [\"mcp\"]\n"},
		{"after what is there", "model = \"o3\"", "model = \"o3\"\n\n[mcp_servers.bangboo]\ncommand = \"/bin/bangboo\"\nargs = [\"mcp\"]\n"},
		{"replaced where it is, keeping the rest of the table and the tables under it", `model = "o3"

[mcp_servers.bangboo]
command = "/old/bangboo"
args = [
  "mcp",
]
startup_timeout_sec = 30

[mcp_servers.bangboo.env]
BANGBOO_HOME = "/tmp/b"

[profiles.fast]
model = "mini"
`, `model = "o3"

[mcp_servers.bangboo]
command = "/bin/bangboo"
args = ["mcp"]
startup_timeout_sec = 30

[mcp_servers.bangboo.env]
BANGBOO_HOME = "/tmp/b"

[profiles.fast]
model = "mini"
`},
		{"a table left without its server gets it back", "[mcp_servers.bangboo.env]\nX = \"1\"\n", "[mcp_servers.bangboo.env]\nX = \"1\"\n\n[mcp_servers.bangboo]\ncommand = \"/bin/bangboo\"\nargs = [\"mcp\"]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := tomlSetServer(c.doc, "/bin/bangboo")
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, c.want)
			}
			if !tomlHasServer(got) {
				t.Error("the result does not have it")
			}
			if again, _ := tomlSetServer(got, "/bin/bangboo"); again != got {
				t.Errorf("a second run changed it:\n%s", again)
			}
		})
	}
	if _, err := tomlSetServer("[mcp_servers]\nbangboo = { command = \"x\" }\n", "/bin/bangboo"); err == nil {
		t.Error("an inline bangboo would be defined twice")
	}
	if got, _ := tomlSetServer(`[x]`+"\n"+`path = "C:\\bang\"boo"`+"\n", `C:\bang"boo`); !strings.Contains(got, `command = "C:\\bang\"boo"`) {
		t.Errorf("quoting: %s", got)
	}
}

func TestTOMLRemove(t *testing.T) {
	doc := `model = "o3"

[mcp_servers.bangboo]
command = "/bin/bangboo"
args = ["mcp"]

[mcp_servers.bangboo.env]
BANGBOO_HOME = "/tmp/b"

# the fast one
[profiles.fast]
model = "mini"

[mcp_servers.bangboo.tools.shell]
enabled = true
`
	want := `model = "o3"

# the fast one
[profiles.fast]
model = "mini"
`
	got, found := tomlRemoveServer(doc)
	if !found || got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if tomlHasServer(got) {
		t.Error("still has it")
	}
	if _, found := tomlRemoveServer(want); found {
		t.Error("found one to remove in a file without one")
	}
}

// The file-backed harnesses end to end: a registration, a second one that
// changes nothing, and taking it out again — each leaving the rest of the
// file, and its permissions, as they were.
func TestConfigFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude_desktop_config.json")
	before := "{\n  \"globalShortcut\": \"Alt+Space\",\n  \"mcpServers\": {\n    \"mine\": {\"command\": \"m\"}\n  }\n}\n"
	os.WriteFile(path, []byte(before), 0o600)
	if err := jsonSet(path, server, "mcpServers", "bangboo"); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(path)
	if !jsonHas(path, "mcpServers", "bangboo") || !strings.HasPrefix(string(first), "{\n  \"globalShortcut\": \"Alt+Space\",\n  \"mcpServers\": {\n    \"mine\": {\"command\": \"m\"},\n") {
		t.Fatalf("after register:\n%s", first)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", st.Mode().Perm())
	}
	if kept, _ := os.ReadFile(path + ".before-phaethon"); string(kept) != before {
		t.Errorf("backup = %q", kept)
	}
	if err := jsonSet(path, server, "mcpServers", "bangboo"); err != nil {
		t.Fatal(err)
	}
	if again, _ := os.ReadFile(path); string(again) != string(first) {
		t.Errorf("a second register changed the file:\n%s", again)
	}
	if err := jsonDelete(path, "mcpServers", "bangboo"); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(path); string(after) != before {
		t.Errorf("after unregister:\n%s\nwant:\n%s", after, before)
	}

	toml := filepath.Join(dir, "config.toml")
	if err := tomlServer(toml, "/bin/bangboo"); err != nil {
		t.Fatal(err)
	}
	if err := tomlRemove(toml); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(toml); len(data) != 0 {
		t.Errorf("config.toml after register and unregister: %q", data)
	}
}
