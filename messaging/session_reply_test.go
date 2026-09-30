package messaging

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"weclaw/agent"
)

// acpLikeAgent mimics ACPAgent: Info().Name is the command path.
type acpLikeAgent struct {
	fakeAgent
	cwd string
}

func (a *acpLikeAgent) Info() agent.AgentInfo {
	return agent.AgentInfo{Name: "/opt/homebrew/bin/claude-agent-acp", Type: "acp", Model: "opus", Cwd: a.cwd}
}
func (a *acpLikeAgent) ResetSession(context.Context, string) (string, error) {
	return "sess-123", nil
}

func TestResetReplyShowsConfigNameAndWorkspace(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	h := NewHandler(nil, nil)
	h.SetDefaultAgent("claude", &acpLikeAgent{cwd: filepath.Join(home, ".weclaw", "workspace")})

	reply := h.resetDefaultSession(context.Background(), "user")
	for _, want := range []string{"已创建新的 claude 会话", "工作区: ~/.weclaw/workspace", "会话: sess-123"} {
		if !strings.Contains(reply, want) {
			t.Errorf("reply %q missing %q", reply, want)
		}
	}
	if strings.Contains(reply, "claude-agent-acp") {
		t.Errorf("reply %q leaks the command path", reply)
	}

	status := h.buildStatus()
	if !strings.Contains(status, "workspace: ~/.weclaw/workspace") {
		t.Errorf("status %q missing workspace", status)
	}
}

func TestDisplayPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	cases := map[string]string{
		home:                          "~",
		filepath.Join(home, "a", "b"): "~/a/b",
		"/opt/x":                      "/opt/x",
		home + "other":                home + "other", // sibling prefix, not under home
	}
	for in, want := range cases {
		if got := displayPath(in); got != want {
			t.Errorf("displayPath(%q) = %q, want %q", in, got, want)
		}
	}
}
