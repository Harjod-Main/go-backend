package main

import "testing"

// Guards against the //go:embed directive silently degrading into a plain
// comment (e.g. a stray space in "//go:embed") or the VERSION file going
// missing — both leave version == "" with no compile error.
func TestVersionEmbedded(t *testing.T) {
	if version == "" {
		t.Fatal("version is empty: check the //go:embed VERSION directive has no stray spaces and VERSION file exists at repo root")
	}
}
