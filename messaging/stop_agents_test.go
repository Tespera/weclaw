package messaging

import (
	"context"
	"testing"

	"weclaw/agent"
)

type fakeAgent struct{ stopped int }

func (f *fakeAgent) Chat(context.Context, string, string) (string, error) { return "", nil }
func (f *fakeAgent) ResetSession(context.Context, string) (string, error) { return "", nil }
func (f *fakeAgent) Info() agent.AgentInfo                                { return agent.AgentInfo{} }
func (f *fakeAgent) SetCwd(string)                                        {}

type stoppableAgent struct{ fakeAgent }

func (s *stoppableAgent) Stop() { s.stopped++ }

func TestStopAgentsStopsSubprocessAgents(t *testing.T) {
	h := NewHandler(nil, nil)
	acp := &stoppableAgent{}
	plain := &fakeAgent{}
	h.SetDefaultAgent("claude", acp)
	h.mu.Lock()
	h.agents["http"] = plain
	h.mu.Unlock()

	h.StopAgents()

	if acp.stopped != 1 {
		t.Fatalf("stoppable agent stopped %d times, want 1", acp.stopped)
	}
}
