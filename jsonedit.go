package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// Agent harnesses keep their MCP servers in files people also edit by hand,
// some with comments and trailing commas (JSONC). phaethon changes the one
// key it owns and leaves every other byte as it was: the order of keys, the
// indentation, the comments. Decoding into a map and encoding it again would
// sort the keys and drop the comments.

// A jvalue is where one value sits in the document.
type jvalue struct {
	start, end int // doc[start:end] is the value
	members    []jmember
	object     bool
}

type jmember struct {
	key      string
	keyStart int
	val      jvalue
	comma    int // the comma after the value; -1 when there is none
}

type jparser struct {
	doc []byte
	pos int
}

func (p *jparser) fail(what string) error {
	line := 1 + bytes.Count(p.doc[:min(p.pos, len(p.doc))], []byte("\n"))
	return fmt.Errorf("line %d: %s", line, what)
}

// skip passes whitespace and comments.
func (p *jparser) skip() error {
	for p.pos < len(p.doc) {
		switch c := p.doc[p.pos]; {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			p.pos++
		case bytes.HasPrefix(p.doc[p.pos:], []byte("//")):
			i := bytes.IndexByte(p.doc[p.pos:], '\n')
			if i < 0 {
				p.pos = len(p.doc)
			} else {
				p.pos += i
			}
		case bytes.HasPrefix(p.doc[p.pos:], []byte("/*")):
			i := bytes.Index(p.doc[p.pos+2:], []byte("*/"))
			if i < 0 {
				return p.fail("a comment that never ends")
			}
			p.pos += 2 + i + 2
		default:
			return nil
		}
	}
	return nil
}

func (p *jparser) value() (jvalue, error) {
	if err := p.skip(); err != nil {
		return jvalue{}, err
	}
	if p.pos >= len(p.doc) {
		return jvalue{}, p.fail("a value is missing")
	}
	switch p.doc[p.pos] {
	case '{':
		return p.object()
	case '[':
		return p.array()
	case '"':
		start := p.pos
		if _, err := p.str(); err != nil {
			return jvalue{}, err
		}
		return jvalue{start: start, end: p.pos}, nil
	}
	start := p.pos
	for p.pos < len(p.doc) && !strings.ContainsRune(",:]} \t\r\n/", rune(p.doc[p.pos])) {
		p.pos++
	}
	if p.pos == start {
		return jvalue{}, p.fail(fmt.Sprintf("unexpected %q", p.doc[p.pos]))
	}
	var v any
	if err := json.Unmarshal(p.doc[start:p.pos], &v); err != nil {
		return jvalue{}, p.fail(fmt.Sprintf("%q is not a value", p.doc[start:p.pos]))
	}
	return jvalue{start: start, end: p.pos}, nil
}

// str reads a string, with the parser at its opening quote.
func (p *jparser) str() (string, error) {
	start := p.pos
	for p.pos++; p.pos < len(p.doc); p.pos++ {
		switch p.doc[p.pos] {
		case '\\':
			p.pos++
		case '"':
			p.pos++
			var s string
			if err := json.Unmarshal(p.doc[start:p.pos], &s); err != nil {
				return "", p.fail("a string that is not one")
			}
			return s, nil
		}
	}
	return "", p.fail("a string that never ends")
}

func (p *jparser) object() (jvalue, error) {
	v := jvalue{start: p.pos, object: true}
	p.pos++
	for {
		if err := p.skip(); err != nil {
			return v, err
		}
		if p.pos >= len(p.doc) {
			return v, p.fail("an object that never ends")
		}
		if p.doc[p.pos] == '}' {
			p.pos++
			v.end = p.pos
			return v, nil
		}
		if p.doc[p.pos] != '"' {
			return v, p.fail("a key that is not a string")
		}
		m := jmember{keyStart: p.pos, comma: -1}
		key, err := p.str()
		if err != nil {
			return v, err
		}
		m.key = key
		if err := p.skip(); err != nil {
			return v, err
		}
		if p.pos >= len(p.doc) || p.doc[p.pos] != ':' {
			return v, p.fail(fmt.Sprintf("no colon after %q", key))
		}
		p.pos++
		if m.val, err = p.value(); err != nil {
			return v, err
		}
		if err := p.skip(); err != nil {
			return v, err
		}
		if p.pos < len(p.doc) && p.doc[p.pos] == ',' {
			m.comma = p.pos
			p.pos++
		} else if p.pos < len(p.doc) && p.doc[p.pos] != '}' {
			return v, p.fail(fmt.Sprintf("no comma after %q", key))
		}
		v.members = append(v.members, m)
	}
}

func (p *jparser) array() (jvalue, error) {
	v := jvalue{start: p.pos}
	p.pos++
	for {
		if err := p.skip(); err != nil {
			return v, err
		}
		if p.pos >= len(p.doc) {
			return v, p.fail("an array that never ends")
		}
		if p.doc[p.pos] == ']' {
			p.pos++
			v.end = p.pos
			return v, nil
		}
		if _, err := p.value(); err != nil {
			return v, err
		}
		if err := p.skip(); err != nil {
			return v, err
		}
		if p.pos < len(p.doc) && p.doc[p.pos] == ',' {
			p.pos++
		} else if p.pos < len(p.doc) && p.doc[p.pos] != ']' {
			return v, p.fail("no comma between array items")
		}
	}
}

// parseDoc reads a whole document, which must be an object. A blank one has
// no root: the caller makes it.
func parseDoc(doc []byte) (*jvalue, error) {
	p := &jparser{doc: doc}
	if bytes.HasPrefix(doc, []byte("\xef\xbb\xbf")) {
		p.pos = 3
	}
	if err := p.skip(); err != nil {
		return nil, err
	}
	if p.pos == len(doc) {
		return nil, nil
	}
	root, err := p.value()
	if err != nil {
		return nil, err
	}
	if err := p.skip(); err != nil {
		return nil, err
	}
	if p.pos != len(doc) {
		return nil, p.fail("more after the end of the document")
	}
	if !root.object {
		return nil, errors.New("it is not a JSON object")
	}
	return &root, nil
}

func (v *jvalue) member(key string) int {
	for i, m := range v.members {
		if m.key == key {
			return i
		}
	}
	return -1
}

type splice struct {
	start, end int
	text       string
}

// apply makes the edits, which must not overlap.
func apply(doc []byte, edits ...splice) []byte {
	for i := 1; i < len(edits); i++ {
		for j := i; j > 0 && edits[j].start > edits[j-1].start; j-- {
			edits[j], edits[j-1] = edits[j-1], edits[j]
		}
	}
	out := append([]byte(nil), doc...)
	for _, e := range edits { // from the end, so earlier offsets hold
		out = append(out[:e.start], append([]byte(e.text), out[e.end:]...)...)
	}
	return out
}

// Layout: how the document is written, so what goes in matches it.

func newline(doc []byte) string {
	if bytes.Contains(doc, []byte("\r\n")) {
		return "\r\n"
	}
	return "\n"
}

func lineStart(doc []byte, pos int) int {
	return bytes.LastIndexByte(doc[:pos], '\n') + 1
}

// indentOf is the whitespace a line starts with.
func indentOf(doc []byte, pos int) string {
	s := lineStart(doc, pos)
	e := s
	for e < len(doc) && (doc[e] == ' ' || doc[e] == '\t') {
		e++
	}
	return string(doc[s:e])
}

// startsLine says whether only whitespace comes before pos on its line.
func startsLine(doc []byte, pos int) bool {
	return len(strings.Trim(string(doc[lineStart(doc, pos):pos]), " \t")) == 0
}

// indentUnit is one step of the document's indentation: two spaces unless
// it says otherwise.
func indentUnit(doc []byte, root *jvalue) string {
	if root != nil && len(root.members) > 0 {
		k := root.members[0].keyStart
		if startsLine(doc, k) {
			if in := indentOf(doc, k); in != "" && in != indentOf(doc, root.start) {
				return strings.TrimPrefix(in, indentOf(doc, root.start))
			}
		}
	}
	return "  "
}

// render writes v as JSON for a line indented by indent. unit "" writes it
// on one line.
func render(v any, indent, unit, nl string) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if unit != "" {
		enc.SetIndent(indent, unit)
	}
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.ReplaceAll(strings.TrimRight(b.String(), "\n"), "\n", nl), nil
}

// nest wraps value in one object per key: nest([a b], v) is {"a":{"b":v}}.
func nest(keys []string, value any) any {
	for i := len(keys) - 1; i >= 0; i-- {
		value = map[string]any{keys[i]: value}
	}
	return value
}

func sameJSON(raw []byte, value any) bool {
	var a, b any
	if json.Unmarshal(raw, &a) != nil {
		return false
	}
	enc, err := json.Marshal(value)
	if err != nil || json.Unmarshal(enc, &b) != nil {
		return false
	}
	return reflect.DeepEqual(a, b)
}

// jsonSetText sets the value at keys, making the objects on the way when
// they are missing. A value that is already there unchanged leaves the
// document as it was.
func jsonSetText(doc []byte, keys []string, value any) ([]byte, error) {
	root, err := parseDoc(doc)
	if err != nil {
		return nil, err
	}
	nl := newline(doc)
	if root == nil || string(bytes.TrimSpace(doc)) == "{}" {
		s, err := render(nest(keys, value), "", "  ", nl)
		if rest := bytes.TrimRight(doc, " \t\r\n"); root == nil && len(rest) > 0 {
			s = string(rest) + nl + s // only comments so far: they stay
		}
		return []byte(s + nl), err
	}
	unit := indentUnit(doc, root)
	obj := root
	for i, k := range keys {
		j := obj.member(k)
		if j < 0 {
			return insertMember(doc, obj, k, nest(keys[i+1:], value), unit, nl)
		}
		m := obj.members[j]
		if i < len(keys)-1 && m.val.object {
			obj = &obj.members[j].val
			continue
		}
		v := nest(keys[i+1:], value)
		if sameJSON(doc[m.val.start:m.val.end], v) {
			return doc, nil
		}
		multi := bytes.Contains(doc[obj.start:obj.end], []byte("\n"))
		u := unit
		if !multi {
			u = ""
		}
		s, err := render(v, indentOf(doc, m.keyStart), u, nl)
		if err != nil {
			return nil, err
		}
		return apply(doc, splice{m.val.start, m.val.end, s}), nil
	}
	return doc, nil
}

func insertMember(doc []byte, obj *jvalue, key string, value any, unit, nl string) ([]byte, error) {
	k, _ := json.Marshal(key)
	if len(obj.members) == 0 {
		inside := doc[obj.start+1 : obj.end-1]
		blank := len(bytes.TrimSpace(inside)) == 0
		at := splice{obj.start + 1, obj.start + 1, ""}
		if blank {
			at.end = obj.end - 1
		}
		if !bytes.Contains(doc, []byte("\n")) {
			s, err := render(value, "", "", nl)
			at.text = string(k) + ": " + s
			return apply(doc, at), err
		}
		indent := indentOf(doc, obj.start) + unit
		s, err := render(value, indent, unit, nl)
		at.text = nl + indent + string(k) + ": " + s
		if blank {
			at.text += nl + indentOf(doc, obj.start)
		}
		return apply(doc, at), err
	}

	first, last := obj.members[0], obj.members[len(obj.members)-1]
	if !startsLine(doc, first.keyStart) { // one line: {"a": 1}
		s, err := render(value, "", "", nl)
		if last.comma >= 0 {
			return apply(doc, splice{last.comma + 1, last.comma + 1, " " + string(k) + ": " + s + ","}), err
		}
		return apply(doc, splice{last.val.end, last.val.end, ", " + string(k) + ": " + s}), err
	}
	indent := indentOf(doc, first.keyStart)
	s, err := render(value, indent, unit, nl)
	if err != nil {
		return nil, err
	}
	line := nl + indent + string(k) + ": " + s
	after, comma := last.val.end, ","
	if last.comma >= 0 {
		// The file ends its members with commas; so does this one.
		after, comma = last.comma+1, ""
		line += ","
	}
	// The new member goes after whatever else is on the last one's line: a
	// comment stays beside the member it was written for.
	at := after
	for at < len(doc) && (doc[at] == ' ' || doc[at] == '\t') {
		at++
	}
	if bytes.HasPrefix(doc[at:], []byte("//")) {
		if n := bytes.IndexByte(doc[at:], '\n'); n >= 0 {
			at += n
		}
	}
	switch {
	case at < len(doc) && doc[at] == '\n' && doc[at-1] == '\r':
		at--
	case at < len(doc) && (doc[at] == '\n' || doc[at] == '\r'):
	default:
		at = after
	}
	if at == after {
		return apply(doc, splice{after, after, comma + line}), nil
	}
	return apply(doc, splice{after, after, comma}, splice{at, at, line}), nil
}

// jsonDeleteText takes out the member at keys. It reports whether there
// was one.
func jsonDeleteText(doc []byte, keys []string) ([]byte, bool, error) {
	root, err := parseDoc(doc)
	if err != nil || root == nil {
		return doc, false, err
	}
	obj := root
	for _, k := range keys[:len(keys)-1] {
		j := obj.member(k)
		if j < 0 || !obj.members[j].val.object {
			return doc, false, nil
		}
		obj = &obj.members[j].val
	}
	i := obj.member(keys[len(keys)-1])
	if i < 0 {
		return doc, false, nil
	}
	m := obj.members[i]
	switch {
	case len(obj.members) == 1: // the object is left empty: {}
		return apply(doc, splice{obj.start + 1, obj.end - 1, ""}), true, nil

	case i < len(obj.members)-1: // from its key through its comma
		start, end := m.keyStart, m.comma+1
		for end < len(doc) && (doc[end] == ' ' || doc[end] == '\t') {
			end++
		}
		if startsLine(doc, m.keyStart) {
			start = lineStart(doc, m.keyStart)
			if bytes.HasPrefix(doc[end:], []byte("//")) {
				if n := bytes.IndexByte(doc[end:], '\n'); n >= 0 {
					end += n
				}
			}
			if end < len(doc) && doc[end] == '\r' {
				end++
			}
			if end < len(doc) && doc[end] == '\n' {
				end++
			}
		}
		return apply(doc, splice{start, end, ""}), true, nil

	default: // the last of several: the comma before it goes too
		prev := obj.members[i-1]
		start := lineStart(doc, m.keyStart) - 1 // the line break before it
		if start > 0 && doc[start-1] == '\r' {
			start--
		}
		if !startsLine(doc, m.keyStart) || prev.comma > start {
			return apply(doc, splice{prev.val.end, m.val.end, ""}), true, nil
		}
		return apply(doc, splice{prev.comma, prev.comma + 1, ""}, splice{start, m.val.end, ""}), true, nil
	}
}

// jsonHasText says whether there is a member at keys.
func jsonHasText(doc []byte, keys []string) bool {
	root, err := parseDoc(doc)
	if err != nil || root == nil {
		return false
	}
	obj := root
	for i, k := range keys {
		j := obj.member(k)
		if j < 0 {
			return false
		}
		if i < len(keys)-1 {
			if !obj.members[j].val.object {
				return false
			}
			obj = &obj.members[j].val
		}
	}
	return true
}
