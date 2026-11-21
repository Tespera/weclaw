package messaging

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"weclaw/ilink"
)

const (
	testBot  = "bot@im.bot"
	testUser = "user@im.wechat"
)

type sent struct {
	text  string
	media string
	token string
}

// fakeSender records sends; refuse decides the error for each attempt.
type fakeSender struct {
	sent   []sent
	refuse func(s sent) error
}

func (f *fakeSender) do(s sent) error {
	if f.refuse != nil {
		if err := f.refuse(s); err != nil {
			return err
		}
	}
	f.sent = append(f.sent, s)
	return nil
}

func (f *fakeSender) SendText(_ context.Context, _, text, token string) error {
	return f.do(sent{text: text, token: token})
}

func (f *fakeSender) SendMedia(_ context.Context, _ string, m MediaRef, token string) error {
	return f.do(sent{media: m.String(), token: token})
}

var refused = &ilink.RetError{Op: "send message", Ret: ilink.RetNotPrepared, ErrMsg: "prepare failed"}

func newTestDeliverer(t *testing.T, f *fakeSender) (*Deliverer, *Store) {
	t.Helper()
	st := NewStore(t.TempDir())
	return &Deliverer{BotID: testBot, Sender: f, Store: st, Now: time.Now}, st
}

func TestDeliverUsesStoredContextToken(t *testing.T) {
	f := &fakeSender{}
	d, st := newTestDeliverer(t, f)
	if _, err := st.RecordInbound(testBot, testUser, "tok-1", time.Now()); err != nil {
		t.Fatal(err)
	}

	if err := d.Deliver(context.Background(), testUser, Outgoing{Text: "hi"}, true, nil); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if len(f.sent) != 1 || f.sent[0].token != "tok-1" {
		t.Fatalf("sent %+v, want one send with tok-1", f.sent)
	}
}

func TestDeliverFallsBackToTokenlessWhenTokenRefused(t *testing.T) {
	f := &fakeSender{refuse: func(s sent) error {
		if s.token != "" {
			return refused
		}
		return nil
	}}
	d, st := newTestDeliverer(t, f)
	st.RecordInbound(testBot, testUser, "tok-1", time.Now())

	if err := d.Deliver(context.Background(), testUser, Outgoing{Text: "hi"}, true, nil); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if len(f.sent) != 1 || f.sent[0].token != "" {
		t.Fatalf("sent %+v, want the tokenless retry to go out", f.sent)
	}
}

func TestDeliverQueuesWhenWindowClosed(t *testing.T) {
	f := &fakeSender{refuse: func(sent) error { return refused }}
	d, st := newTestDeliverer(t, f)
	st.RecordInbound(testBot, testUser, "tok-1", time.Now().Add(-30*time.Hour))

	err := d.Deliver(context.Background(), testUser, Outgoing{Text: "日报"}, true, nil)
	de, ok := IsDeliveryError(err)
	if !ok || de.Reason != ReasonWindow || !de.Queued || de.ExitCode() != 3 {
		t.Fatalf("err = %#v, want queued window DeliveryError", err)
	}
	if !strings.Contains(de.Error(), "30.0 小时") || !strings.Contains(de.Error(), "待发箱") {
		t.Fatalf("message %q should say how long ago and that it was queued", de.Error())
	}
	items, _ := st.Pending(testBot, testUser)
	if len(items) != 1 || items[0].Text != "日报" || items[0].Reason != ReasonWindow {
		t.Fatalf("outbox = %+v", items)
	}
}

func TestDeliverNoQueue(t *testing.T) {
	f := &fakeSender{refuse: func(sent) error { return refused }}
	d, st := newTestDeliverer(t, f)

	err := d.Deliver(context.Background(), testUser, Outgoing{Text: "x"}, false, nil)
	if de, ok := IsDeliveryError(err); !ok || de.Queued {
		t.Fatalf("err = %v, want unqueued DeliveryError", err)
	}
	if items, _ := st.Pending(testBot, testUser); len(items) != 0 {
		t.Fatalf("outbox should be empty, got %+v", items)
	}
}

func TestDeliverNetworkErrorQueued(t *testing.T) {
	f := &fakeSender{refuse: func(sent) error { return &ilink.NetworkError{Attempts: 4} }}
	d, _ := newTestDeliverer(t, f)

	err := d.Deliver(context.Background(), testUser, Outgoing{Text: "x"}, true, nil)
	de, ok := IsDeliveryError(err)
	if !ok || de.Reason != ReasonNetwork || !de.Queued || de.ExitCode() != 4 {
		t.Fatalf("err = %#v, want queued network DeliveryError", err)
	}
}

func TestDeliverQueuesOnlyUnsentMedia(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.png")
	b := filepath.Join(dir, "b.png")
	os.WriteFile(a, []byte("a"), 0o600)
	os.WriteFile(b, []byte("b"), 0o600)

	f := &fakeSender{refuse: func(s sent) error {
		if strings.HasSuffix(s.media, "b.png") {
			return refused
		}
		return nil
	}}
	d, st := newTestDeliverer(t, f)

	err := d.Deliver(context.Background(), testUser, Outgoing{Text: "t", Media: []MediaRef{{Path: a}, {Path: b}}}, true, nil)
	if _, ok := IsDeliveryError(err); !ok {
		t.Fatalf("err = %v", err)
	}
	items, _ := st.Pending(testBot, testUser)
	if len(items) != 1 || items[0].Text != "" || len(items[0].Media) != 1 || originalName(items[0].Media[0]) != "b.png" {
		t.Fatalf("outbox = %+v, want only b.png", items)
	}
	// The queued copy survives the original being deleted (e.g. /tmp cleanup).
	os.Remove(b)
	if _, err := os.Stat(items[0].Media[0]); err != nil {
		t.Fatalf("queued copy missing: %v", err)
	}
}

func TestEnqueueSkipsDuplicates(t *testing.T) {
	st := NewStore(t.TempDir())
	for i := 0; i < 3; i++ {
		st.Enqueue(testBot, testUser, "same", nil, ReasonWindow)
	}
	if items, _ := st.Pending(testBot, testUser); len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
}

func TestPendingDropsExpired(t *testing.T) {
	st := NewStore(t.TempDir())
	st.now = func() time.Time { return time.Now().Add(-8 * 24 * time.Hour) }
	st.Enqueue(testBot, testUser, "old", nil, ReasonWindow)
	st.now = time.Now
	st.Enqueue(testBot, testUser, "new", nil, ReasonWindow)

	items, _ := st.Pending(testBot, testUser)
	if len(items) != 1 || items[0].Text != "new" {
		t.Fatalf("items = %+v, want only the fresh one", items)
	}
}

func TestFlushOutboxMergesTextsIntoOneMessage(t *testing.T) {
	f := &fakeSender{}
	d, st := newTestDeliverer(t, f)
	st.Enqueue(testBot, testUser, "第一条", nil, ReasonWindow)
	st.Enqueue(testBot, testUser, "第二条", nil, ReasonNetwork)

	n, err := d.FlushOutbox(context.Background(), testUser, "tok-new")
	if err != nil || n != 2 {
		t.Fatalf("FlushOutbox = %d, %v", n, err)
	}
	if len(f.sent) != 1 || f.sent[0].token != "tok-new" ||
		!strings.Contains(f.sent[0].text, "第一条") || !strings.Contains(f.sent[0].text, "第二条") {
		t.Fatalf("sent %+v, want one merged message with the new token", f.sent)
	}
	if items, _ := st.Pending(testBot, testUser); len(items) != 0 {
		t.Fatalf("outbox not emptied: %+v", items)
	}
}

func TestFlushOutboxKeepsItemsWhenRefused(t *testing.T) {
	f := &fakeSender{refuse: func(sent) error { return refused }}
	d, st := newTestDeliverer(t, f)
	st.Enqueue(testBot, testUser, "waiting", nil, ReasonWindow)

	if _, err := d.FlushOutbox(context.Background(), testUser, "tok"); !ilink.IsNotPrepared(err) {
		t.Fatalf("err = %v, want refusal", err)
	}
	if items, _ := st.Pending(testBot, testUser); len(items) != 1 {
		t.Fatalf("item lost: %+v", items)
	}
}

func TestChunkTexts(t *testing.T) {
	long := strings.Repeat("字", 40)
	chunks := chunkTexts("头", []string{long, long, long}, 90)
	if len(chunks) != 2 || !strings.HasPrefix(chunks[0], "头") {
		t.Fatalf("chunks = %d %q", len(chunks), chunks)
	}
	for _, c := range chunks {
		if len([]rune(c)) > 90 {
			t.Fatalf("chunk too long: %d", len([]rune(c)))
		}
	}
}
