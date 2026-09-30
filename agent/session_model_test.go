package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfigOptionID(t *testing.T) {
	// Shape taken from a real claude-agent-acp session/new response.
	raw := `{"sessionId":"s1","configOptions":[
		{"id":"mode","category":"mode","type":"select"},
		{"id":"model","category":"model","type":"select"},
		{"id":"effort","category":"thought_level","type":"select"}]}`
	var res newSessionResult
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		t.Fatal(err)
	}
	if got := configOptionID(res.ConfigOptions, "model"); got != "model" {
		t.Fatalf("got %q, want model", got)
	}

	if got := configOptionID(res.ConfigOptions, "mode"); got != "mode" {
		t.Fatalf("mode: got %q, want mode", got)
	}
	if got := configOptionID(res.ConfigOptions, "thought_level"); got != "effort" {
		t.Fatalf("thought_level: got %q, want effort", got)
	}

	tests := []struct {
		name    string
		options []sessionConfigOption
		want    string
	}{
		{"category wins over id", []sessionConfigOption{{ID: "model", Category: "other"}, {ID: "llm", Category: "model"}}, "llm"},
		{"id fallback without category", []sessionConfigOption{{ID: "mode"}, {ID: "model"}}, "model"},
		{"none offered", []sessionConfigOption{{ID: "mode", Category: "mode"}}, ""},
		{"no options", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := configOptionID(tt.options, "model"); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestACPModelE2E drives a real claude-agent-acp and checks that model, mode
// and env reach the session: the transcript model is haiku, a Bash call runs
// without any permission prompt (bypassPermissions), and TZ is honored.
// Opt-in: WECLAW_ACP_E2E=1 go test ./agent -run TestACPModelE2E -v
func TestACPModelE2E(t *testing.T) {
	if os.Getenv("WECLAW_ACP_E2E") != "1" {
		t.Skip("set WECLAW_ACP_E2E=1 to run against a real claude-agent-acp")
	}
	cwd := t.TempDir()
	a := NewACPAgent(ACPAgentConfig{
		Command: "claude-agent-acp",
		Model:   "haiku",
		Mode:    "bypassPermissions",
		Env:     map[string]string{"TZ": "Europe/Oslo"},
		Cwd:     cwd,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer a.Stop()

	reply, err := a.Chat(ctx, "e2e", "Use the Bash tool to run exactly: date +%Z. Then reply with only its output.")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("reply: %q", reply)
	if !strings.Contains(reply, "CEST") && !strings.Contains(reply, "CET") {
		t.Errorf("reply %q: want Europe/Oslo zone (CET/CEST); TZ env not applied?", reply)
	}
	if n := a.permissionRequests.Load(); n != 0 {
		t.Errorf("got %d permission requests, want 0 under bypassPermissions", n)
	}

	a.mu.Lock()
	sid := a.sessions["e2e"]
	a.mu.Unlock()
	home, _ := os.UserHomeDir()
	matches, _ := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", sid+".jsonl"))
	if len(matches) == 0 {
		t.Fatalf("transcript for session %s not found", sid)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	var models []string
	for _, line := range strings.Split(string(data), "\n") {
		var rec struct {
			Message struct {
				Model string `json:"model"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(line), &rec) == nil && rec.Message.Model != "" {
			models = append(models, rec.Message.Model)
		}
	}
	if len(models) == 0 {
		t.Fatal("no assistant model recorded in transcript")
	}
	for _, m := range models {
		if !strings.Contains(m, "haiku") {
			t.Fatalf("transcript model = %v, want haiku", models)
		}
	}
	t.Logf("transcript models: %v", models)
}
