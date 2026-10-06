package agent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func startStoppable(t *testing.T, script string) *ACPAgent {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	a := NewACPAgent(ACPAgentConfig{Cwd: "/tmp"})
	a.cmd, a.stdin, a.started = cmd, stdin, true
	return a
}

func TestACPInitializeDetectsCloseCapability(t *testing.T) {
	for _, tc := range []struct {
		result string
		want   bool
	}{
		{`{"protocolVersion":1,"agentCapabilities":{"sessionCapabilities":{"close":{},"list":{}}}}`, true},
		{`{"protocolVersion":1,"agentCapabilities":{"sessionCapabilities":{"list":{}}}}`, false},
		{`{"protocolVersion":1,"agentCapabilities":{}}`, false},
	} {
		var init initializeResult
		if err := json.Unmarshal([]byte(tc.result), &init); err != nil {
			t.Fatal(err)
		}
		if got := len(init.AgentCapabilities.SessionCapabilities.Close) > 0; got != tc.want {
			t.Errorf("%s: close capability = %v, want %v", tc.result, got, tc.want)
		}
	}
}

// Stop must give the agent a chance to exit on stdin EOF (claude-agent-acp
// then closes every session) instead of killing it outright.
func TestACPStopLetsAgentExitOnEOF(t *testing.T) {
	a := startStoppable(t, "cat >/dev/null")
	start := time.Now()
	a.Stop()
	if d := time.Since(start); d >= stopGracePeriod {
		t.Fatalf("Stop took %s, want a prompt exit on EOF", d)
	}
	if !a.cmd.ProcessState.Exited() || a.cmd.ProcessState.ExitCode() != 0 {
		t.Fatalf("agent did not exit cleanly: %v", a.cmd.ProcessState)
	}
}

func TestACPStopKillsAgentIgnoringEOF(t *testing.T) {
	old := stopGracePeriod
	stopGracePeriod = 100 * time.Millisecond
	defer func() { stopGracePeriod = old }()

	a := startStoppable(t, "exec sleep 30")
	start := time.Now()
	a.Stop()
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("Stop took %s, want kill after grace period", d)
	}
	if a.cmd.ProcessState.Exited() {
		t.Fatalf("agent should have been killed, got %v", a.cmd.ProcessState)
	}
	if a.started {
		t.Fatal("agent still marked started")
	}
}

// childPIDs lists the direct children of pid (the agent's Claude Code
// subprocesses, one per open session).
func childPIDs(t *testing.T, pid int) []string {
	t.Helper()
	out, _ := exec.Command("pgrep", "-P", strconv.Itoa(pid)).Output()
	return strings.Fields(string(out))
}

func waitChildren(t *testing.T, pid, want int) []string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		kids := childPIDs(t, pid)
		if len(kids) == want || time.Now().After(deadline) {
			return kids
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// TestACPSessionLifecycleE2E drives a real claude-agent-acp without sending any
// prompt: /new must end the old session's Claude Code subprocess, and Stop must
// end all of them.
// Opt-in: WECLAW_ACP_E2E=1 go test ./agent -run TestACPSessionLifecycleE2E -v
func TestACPSessionLifecycleE2E(t *testing.T) {
	if os.Getenv("WECLAW_ACP_E2E") != "1" {
		t.Skip("set WECLAW_ACP_E2E=1 to run against a real claude-agent-acp")
	}
	a := NewACPAgent(ACPAgentConfig{Command: "claude-agent-acp", Cwd: t.TempDir()})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer a.Stop()
	if !a.canCloseSession {
		t.Fatal("claude-agent-acp did not advertise sessionCapabilities.close")
	}
	agentPID := a.pid()

	if _, err := a.ResetSession(ctx, "wx-e2e"); err != nil {
		t.Fatal(err)
	}
	first := waitChildren(t, agentPID, 1)
	if len(first) != 1 {
		t.Fatalf("after first session: children %v, want 1", first)
	}
	if _, err := a.ResetSession(ctx, "wx-e2e"); err != nil {
		t.Fatal(err)
	}
	second := waitChildren(t, agentPID, 1)
	time.Sleep(time.Second)
	second = waitChildren(t, agentPID, 1)
	if len(second) != 1 || second[0] == first[0] {
		t.Fatalf("after /new: children %v (before %v), want only the new session's process", second, first)
	}

	a.Stop()
	// The agent ends its sessions' subprocesses asynchronously on shutdown.
	deadline := time.Now().Add(10 * time.Second)
	for exec.Command("kill", "-0", second[0]).Run() == nil {
		if time.Now().After(deadline) {
			t.Fatalf("Claude Code process %s still alive 10s after Stop", second[0])
		}
		time.Sleep(200 * time.Millisecond)
	}
}
