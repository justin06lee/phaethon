package main

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Codex keeps its MCP servers in TOML, a table each: [mcp_servers.bangboo],
// and under it tables of its own such as [mcp_servers.bangboo.env]. As with
// the JSON files, phaethon edits only what it owns and leaves the rest of
// the file as it was.

// A tomlTable is one table's lines, from its header to the next header.
type tomlTable struct {
	key        []string // nil for what comes before the first header
	start, end int
}

// tomlTables splits a file into its tables.
func tomlTables(doc string) []tomlTable {
	tables := []tomlTable{{}}
	for pos := 0; pos < len(doc); {
		end := strings.IndexByte(doc[pos:], '\n') + 1
		if end == 0 {
			end = len(doc) - pos
		}
		if key, ok := tomlHeader(doc[pos : pos+end]); ok {
			tables[len(tables)-1].end = pos
			tables = append(tables, tomlTable{key: key, start: pos})
		}
		pos += end
	}
	tables[len(tables)-1].end = len(doc)
	return tables
}

var tomlBare = regexp.MustCompile(`^[A-Za-z0-9_-]+`)

// tomlHeader reads a table header, [a.b] or [[a.b]], and its dotted key.
// Anything else, like a line of a multi-line array, is not one.
func tomlHeader(line string) ([]string, bool) {
	s := strings.TrimSpace(line)
	if !strings.HasPrefix(s, "[") {
		return nil, false
	}
	closing := "]"
	if strings.HasPrefix(s, "[[") {
		s, closing = s[2:], "]]"
	} else {
		s = s[1:]
	}
	var key []string
	for {
		s = strings.TrimLeft(s, " \t")
		switch {
		case strings.HasPrefix(s, `"`):
			i := 1
			for i < len(s) && s[i] != '"' {
				if s[i] == '\\' {
					i++
				}
				i++
			}
			if i >= len(s) {
				return nil, false
			}
			k, err := strconv.Unquote(s[:i+1])
			if err != nil {
				return nil, false
			}
			key, s = append(key, k), s[i+1:]
		case strings.HasPrefix(s, "'"):
			i := strings.IndexByte(s[1:], '\'')
			if i < 0 {
				return nil, false
			}
			key, s = append(key, s[1:i+1]), s[i+2:]
		default:
			k := tomlBare.FindString(s)
			if k == "" {
				return nil, false
			}
			key, s = append(key, k), s[len(k):]
		}
		s = strings.TrimLeft(s, " \t")
		if strings.HasPrefix(s, ".") {
			s = s[1:]
			continue
		}
		if !strings.HasPrefix(s, closing) {
			return nil, false
		}
		rest := strings.TrimSpace(s[len(closing):])
		return key, rest == "" || strings.HasPrefix(rest, "#")
	}
}

func keyIs(key []string, want ...string) bool {
	return len(key) == len(want) && keyUnder(key, want...)
}

func keyUnder(key []string, want ...string) bool {
	if len(key) < len(want) {
		return false
	}
	for i := range want {
		if key[i] != want[i] {
			return false
		}
	}
	return true
}

var tomlServerKey = []string{"mcp_servers", "bangboo"}

func tomlHasServer(doc string) bool {
	for _, t := range tomlTables(doc) {
		if keyIs(t.key, tomlServerKey...) {
			return true
		}
	}
	return false
}

// tomlQuote writes s as a TOML basic string.
func tomlQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

var (
	tomlOwnKey    = regexp.MustCompile(`^\s*(command|args|"command"|"args"|'command'|'args')\s*=`)
	tomlInlineKey = regexp.MustCompile(`^\s*(bangboo|"bangboo"|'bangboo')\s*[.=]`)
)

// depth is how many brackets and braces a line leaves open, outside strings.
func depth(line string) int {
	d := 0
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#':
			return d
		case c == '[' || c == '{':
			d++
		case c == ']' || c == '}':
			d--
		}
	}
	return d
}

// tomlSetServer points [mcp_servers.bangboo] at bin. Only its command and
// args are written: whatever else is in the table, and the tables under
// it, stay.
func tomlSetServer(doc, bin string) (string, error) {
	nl := newline([]byte(doc))
	own := "command = " + tomlQuote(bin) + nl + `args = ["mcp"]` + nl
	for _, t := range tomlTables(doc) {
		if keyIs(t.key, "mcp_servers") {
			for _, line := range strings.SplitAfter(doc[t.start:t.end], "\n") {
				if tomlInlineKey.MatchString(line) {
					return "", errors.New("bangboo is written inside [mcp_servers] there; change it by hand")
				}
			}
		}
		if !keyIs(t.key, tomlServerKey...) {
			continue
		}
		lines := strings.SplitAfter(doc[t.start:t.end], "\n")
		out := lines[0]
		if !strings.HasSuffix(out, "\n") {
			out += nl
		}
		out += own
		open := 0
		for _, line := range lines[1:] {
			if open > 0 { // the rest of a value phaethon is replacing
				open += depth(line)
				continue
			}
			if tomlOwnKey.MatchString(line) {
				open = depth(line)
				continue
			}
			out += line
		}
		return doc[:t.start] + out + doc[t.end:], nil
	}
	if doc != "" && !strings.HasSuffix(doc, "\n") {
		doc += nl
	}
	if doc != "" {
		doc += nl
	}
	return doc + "[mcp_servers.bangboo]" + nl + own, nil
}

// tomlRemoveServer takes out [mcp_servers.bangboo] and every table under it.
// It reports whether there was anything to take out.
func tomlRemoveServer(doc string) (string, bool) {
	tables := tomlTables(doc)
	removed := false
	for i := len(tables) - 1; i >= 0; i-- {
		t := tables[i]
		if !keyUnder(t.key, tomlServerKey...) {
			continue
		}
		// Comments just above the next table are about that table: they stay.
		end := t.end
		lines := strings.SplitAfter(doc[t.start:t.end], "\n")
		for j := len(lines) - 1; j > 0; j-- {
			s := strings.TrimSpace(lines[j])
			if s != "" && !strings.HasPrefix(s, "#") {
				break
			}
			if strings.HasPrefix(s, "#") {
				end = t.start + len(strings.Join(lines[:j], ""))
			}
		}
		doc = doc[:t.start] + doc[end:]
		removed = true
	}
	if removed && strings.HasSuffix(doc, "\n\n") {
		doc = strings.TrimRight(doc, "\n") + "\n"
	}
	return doc, removed
}
