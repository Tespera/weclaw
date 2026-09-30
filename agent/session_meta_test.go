package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSessionMetaAppendsPromptAndContext(t *testing.T) {
	a := NewACPAgent(ACPAgentConfig{
		SystemPrompt:   "  config prompt  ",
		SessionContext: func(id string) string { return "user is " + id },
	})
	meta := a.sessionMeta("wx-1")
	sp, _ := meta["systemPrompt"].(map[string]interface{})
	if sp == nil || sp["append"] != "config prompt\n\nuser is wx-1" {
		t.Fatalf("meta = %#v", meta)
	}
	if _, ok := sp["type"]; ok {
		t.Error("meta must not set type; the adapter keeps its claude_code preset")
	}

	raw, _ := json.Marshal(newSessionParams{Cwd: "/w", McpServers: []interface{}{}, Meta: meta})
	if !strings.Contains(string(raw), `"_meta":{"systemPrompt":{"append":"config prompt\n\nuser is wx-1"}}`) {
		t.Errorf("session/new params = %s", raw)
	}
}

func TestSessionMetaOmittedWhenEmpty(t *testing.T) {
	a := NewACPAgent(ACPAgentConfig{SessionContext: func(string) string { return "  " }})
	if meta := a.sessionMeta("wx-1"); meta != nil {
		t.Fatalf("meta = %#v, want nil", meta)
	}
	raw, _ := json.Marshal(newSessionParams{Cwd: "/w", McpServers: []interface{}{}})
	if strings.Contains(string(raw), "_meta") {
		t.Errorf("empty meta should be omitted: %s", raw)
	}
}
