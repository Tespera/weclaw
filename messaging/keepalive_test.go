package messaging

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"weclaw/agent"
	"weclaw/ilink"
)

func bj(t *testing.T, s string) time.Time {
	t.Helper()
	loc := DefaultReminderConfig().Location
	v, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestReminderAt(t *testing.T) {
	rc := DefaultReminderConfig()
	cases := []struct{ last, want string }{
		{"2026-11-20 14:00", "2026-11-21 10:00"}, // 20h later, daytime
		{"2026-11-20 09:00", "2026-11-20 22:30"}, // 20h later is 05:00: the evening before
		{"2026-11-20 03:00", "2026-11-20 23:00"}, // 23:00 is quiet: 22:30
		{"2026-11-20 12:00", "2026-11-21 08:00"}, // exactly at quiet end
	}
	cases[2].want = "2026-11-20 22:30"
	for _, c := range cases {
		got := rc.ReminderAt(bj(t, c.last))
		if !got.Equal(bj(t, c.want)) {
			t.Errorf("ReminderAt(%s) = %s, want %s", c.last, got.In(rc.Location).Format("2006-01-02 15:04"), c.want)
		}
	}
}

func TestReminderDue(t *testing.T) {
	rc := DefaultReminderConfig()
	last := bj(t, "2026-11-20 14:00")
	p := Peer{LastInboundAt: last}

	if rc.ReminderDue(bj(t, "2026-11-21 09:59"), p) {
		t.Error("due before reminder time")
	}
	if !rc.ReminderDue(bj(t, "2026-11-21 10:00"), p) {
		t.Error("not due at reminder time")
	}
	if rc.ReminderDue(bj(t, "2026-11-21 13:50"), p) {
		t.Error("due within cutoff of expiry")
	}
	reminded := p
	reminded.RemindedFor = last
	if rc.ReminderDue(bj(t, "2026-11-21 10:00"), reminded) {
		t.Error("due again after reminding")
	}
	off := rc
	off.Enabled = false
	if off.ReminderDue(bj(t, "2026-11-21 10:00"), p) {
		t.Error("due while disabled")
	}
	// Missed the 22:30 slot: do not remind in the middle of the night.
	p2 := Peer{LastInboundAt: bj(t, "2026-11-20 09:00")}
	if rc.ReminderDue(bj(t, "2026-11-21 02:00"), p2) {
		t.Error("due during quiet hours")
	}
}

func TestIsKeepaliveReply(t *testing.T) {
	for text, want := range map[string]bool{
		"1": true, " 好 ": true, "ok": true, "续上续上": true,
		"": false, "/new": false, "@cc": false, "帮我看看这个问题": false,
	} {
		if got := isKeepaliveReply(text); got != want {
			t.Errorf("isKeepaliveReply(%q) = %v, want %v", text, got, want)
		}
	}
}

// recordingILink is an iLink server that records sendmessage texts.
type recordingILink struct {
	mu    sync.Mutex
	texts []string
	srv   *httptest.Server
}

func newRecordingILink(t *testing.T) *recordingILink {
	f := &recordingILink{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.HasSuffix(r.URL.Path, "/sendmessage") {
			var req ilink.SendMessageRequest
			json.Unmarshal(body, &req)
			f.mu.Lock()
			for _, it := range req.Msg.ItemList {
				if it.TextItem != nil {
					f.texts = append(f.texts, it.TextItem.Text)
				}
			}
			f.mu.Unlock()
		}
		w.Write([]byte(`{"ret":0}`))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *recordingILink) sentTexts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.texts...)
}

func textMsg(text, token string) ilink.WeixinMessage {
	return ilink.WeixinMessage{
		FromUserID:   testUser,
		MessageType:  ilink.MessageTypeUser,
		MessageState: ilink.MessageStateFinish,
		ContextToken: token,
		ItemList:     []ilink.MessageItem{{Type: ilink.ItemTypeText, TextItem: &ilink.TextItem{Text: text}}},
	}
}

func TestReplyToReminderRenewsWithoutAgent(t *testing.T) {
	fi := newRecordingILink(t)
	client := ilink.NewClient(&ilink.Credentials{BotToken: "t", ILinkBotID: testBot, BaseURL: fi.srv.URL})
	st := NewStore(t.TempDir())
	last := time.Now().Add(-21 * time.Hour)
	st.RecordInbound(testBot, testUser, "old", last)
	st.MarkReminded(testBot, testUser, last)
	st.Enqueue(testBot, testUser, "早上的日报", nil, ReasonWindow)

	h := &Handler{agents: make(map[string]agent.Agent)} // no agent: anything forwarded would answer "no agent"
	h.SetStore(st)
	h.HandleMessage(context.Background(), client, textMsg("1", "fresh"))

	texts := fi.sentTexts()
	if len(texts) != 2 || !strings.Contains(texts[0], "早上的日报") || texts[1] != keepaliveAck {
		t.Fatalf("sent %q, want flushed outbox then the ack", texts)
	}
	p, _ := st.Peer(testBot, testUser)
	if p.ContextToken != "fresh" || p.ReminderOutstanding() {
		t.Fatalf("peer = %+v, want fresh token and no outstanding reminder", p)
	}
}

func TestShortMessageWithoutReminderGoesToAgent(t *testing.T) {
	fi := newRecordingILink(t)
	client := ilink.NewClient(&ilink.Credentials{BotToken: "t", ILinkBotID: testBot, BaseURL: fi.srv.URL})
	st := NewStore(t.TempDir())
	st.RecordInbound(testBot, testUser, "old", time.Now().Add(-time.Hour))

	h := &Handler{agents: make(map[string]agent.Agent)}
	h.SetStore(st)
	h.HandleMessage(context.Background(), client, textMsg("好", "fresh"))

	for _, txt := range fi.sentTexts() {
		if txt == keepaliveAck {
			t.Fatalf("short message was swallowed as keepalive without a reminder: %q", fi.sentTexts())
		}
	}
}

func TestMaintainSendsReminderOnce(t *testing.T) {
	fi := newRecordingILink(t)
	client := ilink.NewClient(&ilink.Credentials{BotToken: "t", ILinkBotID: testBot, BaseURL: fi.srv.URL})
	st := NewStore(t.TempDir())
	last := bj(t, "2026-11-20 14:00")
	st.RecordInbound(testBot, testUser, "tok", last)

	h := &Handler{agents: make(map[string]agent.Agent)}
	h.SetStore(st)
	h.SetReminder(DefaultReminderConfig())
	h.maintain(context.Background(), client, bj(t, "2026-11-21 10:05"))
	h.maintain(context.Background(), client, bj(t, "2026-11-21 10:10"))

	texts := fi.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "回我任意一个字") || !strings.Contains(texts[0], "11-21 14:00") {
		t.Fatalf("sent %q, want exactly one reminder naming 11-21 14:00", texts)
	}
}

func TestMaintainFlushesOutboxWhileWindowOpen(t *testing.T) {
	fi := newRecordingILink(t)
	client := ilink.NewClient(&ilink.Credentials{BotToken: "t", ILinkBotID: testBot, BaseURL: fi.srv.URL})
	st := NewStore(t.TempDir())
	now := time.Now()
	st.RecordInbound(testBot, testUser, "tok", now.Add(-2*time.Hour))
	st.Enqueue(testBot, testUser, "断网时没发出去的", nil, ReasonNetwork)

	h := &Handler{agents: make(map[string]agent.Agent)}
	h.SetStore(st)
	h.maintain(context.Background(), client, now)

	if texts := fi.sentTexts(); len(texts) != 1 || !strings.Contains(texts[0], "断网时没发出去的") {
		t.Fatalf("sent %q", texts)
	}
}

func TestRenewCommandRenewsWithoutAgent(t *testing.T) {
	fi := newRecordingILink(t)
	client := ilink.NewClient(&ilink.Credentials{BotToken: "t", ILinkBotID: testBot, BaseURL: fi.srv.URL})
	st := NewStore(t.TempDir())
	st.RecordInbound(testBot, testUser, "old", time.Now().Add(-3*time.Hour)) // no reminder outstanding

	h := &Handler{agents: make(map[string]agent.Agent)}
	h.SetStore(st)
	h.SetReminder(DefaultReminderConfig())
	h.HandleMessage(context.Background(), client, textMsg(" /renew ", "fresh"))

	texts := fi.sentTexts()
	if len(texts) != 1 || !strings.HasPrefix(texts[0], "已续上") || !strings.Contains(texts[0], "24 小时") {
		t.Fatalf("sent %q, want only the renew ack", texts)
	}
	p, _ := st.Peer(testBot, testUser)
	if p.ContextToken != "fresh" {
		t.Fatalf("token not renewed: %+v", p)
	}
}
