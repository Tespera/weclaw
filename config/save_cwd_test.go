package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAgentCwdPreservesOtherFields(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	orig := `{
  "default_agent": "claude",
  "unknown_top": 42,
  "agents": {
    "claude": {"type": "acp", "command": "/x/claude-agent-acp", "model": "opus", "env": {"TZ": "Europe/Oslo"}, "future_field": true},
    "codex": {"type": "acp", "command": "/x/codex"}
  }
}`
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := SaveAgentCwd([]string{"claude", "missing"}, "/work/proj"); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(path)
	var doc struct {
		DefaultAgent string                            `json:"default_agent"`
		UnknownTop   int                               `json:"unknown_top"`
		Agents       map[string]map[string]interface{} `json:"agents"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	claude := doc.Agents["claude"]
	if claude["cwd"] != "/work/proj" {
		t.Errorf("claude cwd = %v, want /work/proj", claude["cwd"])
	}
	if claude["model"] != "opus" || claude["future_field"] != true || claude["env"].(map[string]interface{})["TZ"] != "Europe/Oslo" {
		t.Errorf("claude fields not preserved: %v", claude)
	}
	if _, ok := doc.Agents["codex"]["cwd"]; ok {
		t.Error("codex was not named but got a cwd")
	}
	if _, ok := doc.Agents["missing"]; ok {
		t.Error("unknown agent name was added to config")
	}
	if doc.DefaultAgent != "claude" || doc.UnknownTop != 42 {
		t.Errorf("top-level fields not preserved: %+v", doc)
	}
}
