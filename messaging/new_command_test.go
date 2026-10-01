package messaging

import (
	"context"
	"testing"
)

func TestParseNewCommand(t *testing.T) {
	tests := []struct {
		in      string
		message string
		ok      bool
	}{
		{"/new", "", true},
		{"/clear", "", true},
		{"/new 把桌面上的 pdf 发给我", "把桌面上的 pdf 发给我", true},
		{"/clear  hello ", "hello", true},
		{"/new\n第二行", "第二行", true},
		{"/newer", "", false},
		{"/news today", "", false},
		{"hello /new", "", false},
	}
	for _, tt := range tests {
		message, ok := parseNewCommand(tt.in)
		if message != tt.message || ok != tt.ok {
			t.Errorf("parseNewCommand(%q) = (%q, %v), want (%q, %v)", tt.in, message, ok, tt.message, tt.ok)
		}
	}
}

// orderAgent records the order of ResetSession and Chat calls.
type orderAgent struct {
	fakeAgent
	calls []string
}

func (o *orderAgent) ResetSession(_ context.Context, userID string) (string, error) {
	o.calls = append(o.calls, "reset:"+userID)
	return "sess-new", nil
}

func (o *orderAgent) Chat(_ context.Context, userID, message string) (string, error) {
	o.calls = append(o.calls, "chat:"+userID+":"+message)
	return "done", nil
}

func TestChatInNewSessionResetsThenSends(t *testing.T) {
	ag := &orderAgent{}
	h := NewHandler(nil, nil)
	h.SetDefaultAgent("claude", ag)

	name, reply := h.chatInNewSession(context.Background(), "user-1", "hi")
	if name != "claude" || reply != "done" {
		t.Errorf("got (%q, %q), want (claude, done)", name, reply)
	}
	want := []string{"reset:user-1", "chat:user-1:hi"}
	if len(ag.calls) != 2 || ag.calls[0] != want[0] || ag.calls[1] != want[1] {
		t.Errorf("calls = %v, want %v", ag.calls, want)
	}
}
