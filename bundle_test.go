package main

import "testing"

func TestPackaged(t *testing.T) {
	for dir, want := range map[string]bool{
		"/opt/homebrew/Cellar/phaethon/0.4.0/bin":              true,
		"/home/linuxbrew/.linuxbrew/Cellar/phaethon/0.4.0/bin": true,
		"/usr/local/bin":       false,
		"/Users/me/.local/bin": false,
	} {
		if got := packaged(dir); got != want {
			t.Errorf("packaged(%s) = %v", dir, got)
		}
	}
}
