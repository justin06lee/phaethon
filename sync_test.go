package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A bangboo that answers the deep check the way the real one does, and
// moves its remembered desk the way the real one does.
const fakeBangboo = `#!/bin/sh
cur="$BANGBOO_HOME/current"
case "$2" in
desk_new) echo "${3#name=}" > "$cur"; echo "started" ;;
browser_open) echo "Example Domain" ;;
computer) echo "[screenshot: /tmp/x.png]" ;;
shell) echo "ok-from-desk" ;;
desk_close) : > "$cur"; echo "closed" ;;
esac
`

func TestDeepCheckKeepsTheCurrentDesk(t *testing.T) {
	for _, had := range []string{"research\n", ""} {
		bin, state := t.TempDir(), t.TempDir()
		os.WriteFile(filepath.Join(bin, "bangboo"), []byte(fakeBangboo), 0o755)
		t.Setenv("PHAETHON_BIN", bin)
		t.Setenv("BANGBOO_HOME", state)
		current := filepath.Join(state, "current")
		if had != "" {
			os.WriteFile(current, []byte(had), 0o600)
		}
		failed := 0
		deepCheck(context.Background(), func(c check) {
			if !c.ok {
				failed++
				t.Errorf("check failed: %s (%s)", c.what, c.fix)
			}
		})
		data, err := os.ReadFile(current)
		switch {
		case had == "" && err == nil:
			t.Errorf("left a current desk behind: %q", data)
		case had != "" && string(data) != had:
			t.Errorf("current desk = %q, want %q back", data, had)
		}
	}
}
