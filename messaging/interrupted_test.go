package messaging

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"weclaw/ilink"
)

// fakeILink records sendmessage calls. Requests carrying rejectToken fail with
// a non-zero ret, like iLink with an expired context token.
type fakeILink struct {
	mu          sync.Mutex
	sent        []ilink.SendMsg
	rejectToken string
}

func newFakeILink(t *testing.T) (*fakeILink, *ilink.Client) {
	t.Helper()
	f := &fakeILink{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ilink.SendMessageRequest
		json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.rejectToken != "" && req.Msg.ContextToken == f.rejectToken {
			w.Write([]byte(`{"ret":-14,"errmsg":"context token expired"}`))
			return
		}
		f.sent = append(f.sent, req.Msg)
		w.Write([]byte(`{"ret":0}`))
	}))
	t.Cleanup(srv.Close)
	return f, ilink.NewClient(&ilink.Credentials{BaseURL: srv.URL, ILinkBotID: "bot@im.bot", BotToken: "t"})
}

func (f *fakeILink) messages() []ilink.SendMsg {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ilink.SendMsg(nil), f.sent...)
}

func userMsg(from, token string) ilink.WeixinMessage {
	return ilink.WeixinMessage{FromUserID: from, ContextToken: token}
}

// A reply produced (or failed) after shutdown began is not sent: the chat is
// recorded once, and nothing goes out on the dying instance.
func TestShutdownRecordsInterruptedReply(t *testing.T) {
	f, client := newFakeILink(t)
	h := NewHandler(nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	h.sendReplyWithMedia(ctx, client, userMsg("u1", "tok1"), "claude", "Error: context canceled", "")
	h.sendReplyWithMedia(ctx, client, userMsg("u1", "tok1"), "codex", "Error: context canceled", "")

	if n := len(f.messages()); n != 0 {
		t.Fatalf("sent %d messages during shutdown, want 0", n)
	}
	if len(h.interrupted) != 1 || h.interrupted[0].UserID != "u1" || h.interrupted[0].BotID != "bot@im.bot" {
		t.Fatalf("interrupted = %+v, want one entry for u1", h.interrupted)
	}
}

func TestLiveReplyIsNotRecorded(t *testing.T) {
	f, client := newFakeILink(t)
	h := NewHandler(nil, nil)

	h.sendReplyWithMedia(context.Background(), client, userMsg("u1", "tok1"), "claude", "hello", "")

	if n := len(f.messages()); n != 1 {
		t.Fatalf("sent %d messages, want 1", n)
	}
	if len(h.interrupted) != 0 {
		t.Fatalf("interrupted = %+v, want none", h.interrupted)
	}
}

// The next instance tells each interrupted chat it restarted, falls back to a
// plain send when the saved context token is rejected, skips stale entries,
// and consumes the file so the notice is sent only once.
func TestNotifyInterruptedAfterRestart(t *testing.T) {
	f, client := newFakeILink(t)
	f.rejectToken = "expired"
	path := filepath.Join(t.TempDir(), "interrupted.json")

	h := NewHandler(nil, nil)
	h.interrupted = []InterruptedReply{
		{BotID: "bot@im.bot", UserID: "u1", ContextToken: "tok1", InterruptedAt: time.Now()},
		{BotID: "bot@im.bot", UserID: "u2", ContextToken: "expired", InterruptedAt: time.Now()},
		{BotID: "bot@im.bot", UserID: "old", InterruptedAt: time.Now().Add(-48 * time.Hour)},
		{BotID: "gone@im.bot", UserID: "u3", InterruptedAt: time.Now()},
	}
	if err := h.SaveInterrupted(path); err != nil {
		t.Fatal(err)
	}

	NotifyInterrupted(context.Background(), path, []*ilink.Client{client}, "v1.2.3")

	sent := f.messages()
	got := map[string]ilink.SendMsg{}
	for _, m := range sent {
		got[m.ToUserID] = m
	}
	if len(sent) != 2 || got["u1"].ContextToken != "tok1" || got["u2"].ContextToken != "" {
		t.Fatalf("sent = %+v, want u1 (with its token) and u2 (plain fallback)", sent)
	}
	if text := got["u1"].ItemList[0].TextItem.Text; !strings.Contains(text, "已重启（v1.2.3）") || !strings.Contains(text, "重新发送") {
		t.Fatalf("notice = %q", text)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("interrupted file still present (err=%v); notices would repeat", err)
	}
}

func TestSaveInterruptedWritesNothingWhenEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "interrupted.json")
	if err := NewHandler(nil, nil).SaveInterrupted(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file written with no interrupted replies (err=%v)", err)
	}
}
