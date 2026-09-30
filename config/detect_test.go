package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestLookPath_InPath verifies that lookPath finds binaries already in PATH.
func TestLookPath_InPath(t *testing.T) {
	p, err := lookPath("ls")
	if err != nil {
		t.Fatalf("expected to find ls, got error: %v", err)
	}
	if p == "" {
		t.Fatal("expected non-empty path for ls")
	}
}

// TestLookPath_NotExist verifies that lookPath returns an error for missing binaries.
func TestLookPath_NotExist(t *testing.T) {
	_, err := lookPath("nonexistent-binary-xyz-12345")
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}
}

// TestLookPath_LoginShellFallback reproduces the daemon scenario:
// PATH is stripped to system-only dirs (no nvm), so exec.LookPath fails,
// but lookPath resolves claude via login shell fallback.
func TestLookPath_LoginShellFallback(t *testing.T) {
	// Precondition: claude must be discoverable via login shell (i.e. nvm in .zshrc)
	fullPath, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("claude not installed, skipping login shell fallback test")
	}

	// Simulate daemon environment: strip PATH to system-only dirs
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin")
	defer os.Setenv("PATH", origPath)

	// Reproduce the bug: exec.LookPath must fail under stripped PATH
	_, err = exec.LookPath("claude")
	if err == nil {
		t.Skip("claude found in minimal PATH, cannot reproduce nvm issue")
	}

	// Verify fix: lookPath should find claude via login shell
	p, err := lookPath("claude")
	if err != nil {
		t.Fatalf("lookPath should find claude via login shell, got: %v", err)
	}
	if p != fullPath {
		t.Logf("resolved path differs: direct=%s, login-shell=%s (acceptable)", fullPath, p)
	}
	t.Logf("lookPath resolved claude via login shell: %s", p)
}

// TestDetectAndConfigure_StrippedPath is an end-to-end test:
// empty config + stripped PATH → DetectAndConfigure should still find claude.
func TestDetectAndConfigure_StrippedPath(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude not installed, skipping")
	}

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin")
	defer os.Setenv("PATH", origPath)

	cfg := DefaultConfig()
	DetectAndConfigure(cfg)

	agent, ok := cfg.Agents["claude"]
	if !ok {
		t.Fatal("expected claude to be detected via login shell fallback")
	}
	// Either claude-agent-acp (acp) or claude (cli) may be installed; the point
	// is that the login-shell fallback resolves a real absolute path.
	if agent.Type != "cli" && agent.Type != "acp" {
		t.Fatalf("expected type cli or acp, got %s", agent.Type)
	}
	if !filepath.IsAbs(agent.Command) {
		t.Fatalf("expected absolute command path, got %q", agent.Command)
	}
	if _, err := os.Stat(agent.Command); err != nil {
		t.Fatalf("detected command does not exist: %v", err)
	}
	t.Logf("detected claude: type=%s, command=%s", agent.Type, agent.Command)
}

// After first run, detection must not spawn a login shell for every agent that
// isn't installed: that cost ~10s on each bridge start.
func TestDetectAndConfigure_LoginShellOnlyOnFirstRun(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // nothing resolvable via PATH
	calls := 0
	orig := shellWhich
	shellWhich = func(shell, binary string) ([]byte, error) {
		calls++
		return nil, os.ErrNotExist
	}
	t.Cleanup(func() { shellWhich = orig })

	cfg := DefaultConfig()
	cfg.Agents["claude"] = AgentConfig{Type: "acp", Command: "/opt/homebrew/bin/claude-agent-acp"}
	DetectAndConfigure(cfg)
	if calls != 0 {
		t.Fatalf("configured setup spawned %d login shells, want 0", calls)
	}

	DetectAndConfigure(DefaultConfig())
	if calls == 0 {
		t.Fatal("first run should fall back to the login shell")
	}
}

func TestClaudeCandidatesDefaultToOpus(t *testing.T) {
	for _, c := range agentCandidates {
		if c.Name == "claude" && c.Model != "opus" {
			t.Errorf("claude candidate %s (%s) defaults to model %q, want opus", c.Binary, c.Type, c.Model)
		}
	}
}
