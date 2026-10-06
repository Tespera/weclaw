package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeACP stubs claude-agent-acp: each prompt streams "reply:<text>" as an
// agent_message_chunk on its session, then ends the turn.
type fakeACP struct {
	a          *ACPAgent
	sessions   atomic.Int64
	inflight   atomic.Int64
	overlapped atomic.Bool
	newGate    func(n int64) // called inside session/new n (1-based)
	promptGate func()        // called inside every session/prompt
	closed     chan string   // session IDs passed to session/close
}

func newFakeACP() *fakeACP {
	f := &fakeACP{a: NewACPAgent(ACPAgentConfig{Cwd: "/tmp"}), closed: make(chan string, 16)}
	f.a.started = true
	f.a.canCloseSession = true
	f.a.rpcCall = f.rpc
	return f
}

func (f *fakeACP) rpc(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	switch method {
	case "session/new":
		n := f.sessions.Add(1)
		if f.newGate != nil {
			f.newGate(n)
		}
		return json.RawMessage(fmt.Sprintf(`{"sessionId":"s%d"}`, n)), nil
	case "session/close":
		f.closed <- params.(closeSessionParams).SessionID
		return json.RawMessage(`{}`), nil
	case "session/prompt":
		p := params.(promptParams)
		if f.promptGate != nil {
			f.promptGate()
		}
		if f.inflight.Add(1) > 1 {
			f.overlapped.Store(true)
		}
		defer f.inflight.Add(-1)
		time.Sleep(20 * time.Millisecond) // a turn takes a while
		upd, _ := json.Marshal(sessionUpdateParams{
			SessionID: p.SessionID,
			Update:    sessionUpdate{SessionUpdate: "agent_message_chunk", Content: json.RawMessage(fmt.Sprintf(`{"type":"text","text":"reply:%s"}`, p.Prompt[0].Text))},
		})
		f.a.handleSessionUpdate(upd)
		return json.RawMessage(`{"stopReason":"end_turn"}`), nil
	}
	return json.RawMessage(`{}`), nil
}

// Two messages from one chat arrive while the session is still being created
// (the 2026-09-30 incident): both must share one session, run one at a time,
// and each get its own reply instead of "agent returned empty response".
func TestACPConcurrentMessagesSameConversation(t *testing.T) {
	f := newFakeACP()
	f.newGate = func(int64) { time.Sleep(30 * time.Millisecond) }
	ctx := context.Background()

	msgs := []string{"one", "two", "three"}
	replies := make([]string, len(msgs))
	errs := make([]error, len(msgs))
	var wg sync.WaitGroup
	for i, m := range msgs {
		wg.Add(1)
		go func(i int, m string) {
			defer wg.Done()
			replies[i], errs[i] = f.a.Chat(ctx, "wx-user", m)
		}(i, m)
		time.Sleep(5 * time.Millisecond)
	}
	wg.Wait()

	for i, m := range msgs {
		if errs[i] != nil {
			t.Errorf("message %q: %v", m, errs[i])
		} else if want := "reply:" + m; replies[i] != want {
			t.Errorf("message %q: got reply %q, want %q", m, replies[i], want)
		}
	}
	if n := f.sessions.Load(); n != 1 {
		t.Errorf("created %d sessions, want 1", n)
	}
	if f.overlapped.Load() {
		t.Error("prompts overlapped on one session")
	}
}

// /new while the first message's session is still being created must win:
// later messages go to the session created by the reset.
func TestACPResetWaitsForInflightCreation(t *testing.T) {
	f := newFakeACP()
	release := make(chan struct{})
	f.newGate = func(n int64) {
		if n == 1 {
			<-release
		}
	}
	ctx := context.Background()

	firstDone := make(chan error, 1)
	go func() {
		_, err := f.a.Chat(ctx, "wx-user", "first")
		firstDone <- err
	}()
	for f.sessions.Load() == 0 {
		time.Sleep(time.Millisecond)
	}

	resetDone := make(chan string, 1)
	go func() {
		sid, err := f.a.ResetSession(ctx, "wx-user")
		if err != nil {
			t.Errorf("reset: %v", err)
		}
		resetDone <- sid
	}()
	time.Sleep(10 * time.Millisecond)
	close(release)

	if err := <-firstDone; err != nil {
		t.Fatalf("first message: %v", err)
	}
	if sid := <-resetDone; sid != "s2" {
		t.Fatalf("reset created %q, want s2", sid)
	}
	f.a.mu.Lock()
	cur := f.a.sessions["wx-user"]
	f.a.mu.Unlock()
	if cur != "s2" {
		t.Fatalf("conversation maps to %q after /new, want s2", cur)
	}
}

func TestKeyedLockHonorsContext(t *testing.T) {
	var l keyedLock
	unlock, waited, err := l.lock(context.Background(), "k")
	if err != nil || waited {
		t.Fatalf("first lock: waited=%v err=%v", waited, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, _, err := l.lock(ctx, "k"); err == nil {
		t.Fatal("second lock should time out while held")
	}
	if _, _, err := l.lock(context.Background(), "other"); err != nil {
		t.Fatalf("other key blocked: %v", err)
	}
	unlock()
	if _, waited, err := l.lock(context.Background(), "k"); err != nil || waited {
		t.Fatalf("relock after unlock: waited=%v err=%v", waited, err)
	}
}

func waitClosed(t *testing.T, f *fakeACP, want string) {
	t.Helper()
	select {
	case got := <-f.closed:
		if got != want {
			t.Fatalf("closed session %q, want %q", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("session %q was never closed", want)
	}
}

func assertNoClose(t *testing.T, f *fakeACP) {
	t.Helper()
	select {
	case got := <-f.closed:
		t.Fatalf("unexpected session/close for %q", got)
	case <-time.After(50 * time.Millisecond):
	}
}

// /new must close the dropped session: claude-agent-acp keeps a Claude Code
// subprocess per open session, so an unclosed one leaks until weclaw restarts.
func TestACPResetClosesIdleSession(t *testing.T) {
	f := newFakeACP()
	ctx := context.Background()

	if _, err := f.a.Chat(ctx, "wx-user", "hi"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.ResetSession(ctx, "wx-user"); err != nil {
		t.Fatal(err)
	}
	waitClosed(t, f, "s1")
	assertNoClose(t, f) // the new session s2 stays open
}

// /new while a reply is being generated must not cut that reply off: the old
// session is closed only after its prompt finishes.
func TestACPResetDefersCloseUntilPromptDone(t *testing.T) {
	f := newFakeACP()
	inPrompt := make(chan struct{})
	release := make(chan struct{})
	f.promptGate = func() {
		select {
		case inPrompt <- struct{}{}:
			<-release
		default:
		}
	}
	ctx := context.Background()

	reply := make(chan string, 1)
	go func() {
		r, err := f.a.Chat(ctx, "wx-user", "long")
		if err != nil {
			t.Errorf("chat: %v", err)
		}
		reply <- r
	}()
	<-inPrompt

	if _, err := f.a.ResetSession(ctx, "wx-user"); err != nil {
		t.Fatal(err)
	}
	assertNoClose(t, f)

	close(release)
	if r := <-reply; r != "reply:long" {
		t.Fatalf("in-flight reply = %q, want reply:long", r)
	}
	waitClosed(t, f, "s1")
}

// Agents that do not advertise sessionCapabilities.close are never sent it.
func TestACPResetSkipsCloseWithoutCapability(t *testing.T) {
	f := newFakeACP()
	f.a.canCloseSession = false
	ctx := context.Background()

	if _, err := f.a.Chat(ctx, "wx-user", "hi"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.ResetSession(ctx, "wx-user"); err != nil {
		t.Fatal(err)
	}
	assertNoClose(t, f)
}
