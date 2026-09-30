package messaging

import (
	"context"
	"sort"
	"strings"
	"testing"

	"weclaw/agent"
)

// recordingAgent records SetCwd and ResetSession calls.
type recordingAgent struct {
	fakeAgent
	cwd    string
	resets []string
}

func (r *recordingAgent) SetCwd(cwd string) { r.cwd = cwd }
func (r *recordingAgent) Info() agent.AgentInfo {
	return agent.AgentInfo{Name: "/bin/x", Type: "acp", Cwd: r.cwd}
}
func (r *recordingAgent) ResetSession(_ context.Context, userID string) (string, error) {
	r.resets = append(r.resets, userID)
	return "sess-new", nil
}

func TestHandleCwdSwitchesPersistsAndStartsNewSession(t *testing.T) {
	dir := t.TempDir()
	lazy := &recordingAgent{}
	h := NewHandler(func(ctx context.Context, name string) agent.Agent { return lazy }, nil)
	claude := &recordingAgent{cwd: "/old"}
	h.SetDefaultAgent("claude", claude)
	h.SetAgentMetas([]AgentMeta{{Name: "claude"}, {Name: "codex"}})

	var savedNames []string
	var savedCwd string
	h.SetSaveCwd(func(names []string, cwd string) error {
		savedNames, savedCwd = append([]string(nil), names...), cwd
		return nil
	})

	reply := h.handleCwd(context.Background(), "user-1", "/cwd "+dir)

	if claude.cwd != dir {
		t.Errorf("running agent cwd = %q, want %q", claude.cwd, dir)
	}
	if len(claude.resets) != 1 || claude.resets[0] != "user-1" {
		t.Errorf("default agent resets = %v, want [user-1]", claude.resets)
	}
	sort.Strings(savedNames)
	if savedCwd != dir || strings.Join(savedNames, ",") != "claude,codex" {
		t.Errorf("persisted (%v, %q), want ([claude codex], %q)", savedNames, savedCwd, dir)
	}
	for _, want := range []string{"已切换工作区", "已新建 claude 会话", "会话: sess-new"} {
		if !strings.Contains(reply, want) {
			t.Errorf("reply %q missing %q", reply, want)
		}
	}

	// An agent started after /cwd picks up the new workspace.
	if _, err := h.getAgent(context.Background(), "codex"); err != nil {
		t.Fatal(err)
	}
	if lazy.cwd != dir {
		t.Errorf("on-demand agent cwd = %q, want %q", lazy.cwd, dir)
	}

	// No argument shows the current workspace.
	if got := h.handleCwd(context.Background(), "user-1", "/cwd"); !strings.Contains(got, "工作区: "+displayPath(dir)) {
		t.Errorf("/cwd reply %q does not show the workspace", got)
	}
}

func TestHandleCwdRejectsMissingDir(t *testing.T) {
	h := NewHandler(nil, nil)
	claude := &recordingAgent{cwd: "/old"}
	h.SetDefaultAgent("claude", claude)
	reply := h.handleCwd(context.Background(), "u", "/cwd /definitely/not/here")
	if !strings.Contains(reply, "Path not found") || claude.cwd != "/old" || len(claude.resets) != 0 {
		t.Errorf("reply=%q cwd=%q resets=%v; want rejection with no changes", reply, claude.cwd, claude.resets)
	}
}
