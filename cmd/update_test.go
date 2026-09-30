package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateSourceDir(t *testing.T) {
	orig := SourceDir
	t.Cleanup(func() { SourceDir = orig })

	SourceDir = ""
	if _, err := updateSourceDir(); err == nil || !strings.Contains(err.Error(), "make install") {
		t.Fatalf("empty SourceDir error = %v, want hint to use make install", err)
	}

	SourceDir = filepath.Join(t.TempDir(), "missing")
	if _, err := updateSourceDir(); err == nil {
		t.Fatal("missing checkout should error")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module weclaw\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	SourceDir = dir
	if got, err := updateSourceDir(); err != nil || got != dir {
		t.Fatalf("updateSourceDir() = (%q, %v), want (%q, nil)", got, err, dir)
	}
}
