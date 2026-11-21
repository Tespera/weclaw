package ilink

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(url string, budget time.Duration) *Client {
	c := NewClient(&Credentials{BotToken: "t", ILinkBotID: "bot@im.bot", BaseURL: url})
	c.retryBudget = budget
	c.retryDelay = 10 * time.Millisecond
	return c
}

// dropFirst closes the connection without a response for the first n
// requests, as a proxy dropping out does, then answers with body.
func dropFirst(n int32, body string) (*httptest.Server, *int32) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) <= n {
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		w.Write([]byte(body))
	}))
	return srv, &calls
}

func TestSendMessageRetriesDroppedConnection(t *testing.T) {
	srv, calls := dropFirst(2, `{"ret":0}`)
	defer srv.Close()

	resp, err := testClient(srv.URL, 5*time.Second).SendMessage(context.Background(), &SendMessageRequest{})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if resp.Ret != 0 || atomic.LoadInt32(calls) != 3 {
		t.Fatalf("ret=%d calls=%d, want ret=0 after 3 calls", resp.Ret, *calls)
	}
}

func TestSendMessageGivesUpWithNetworkError(t *testing.T) {
	srv, _ := dropFirst(1000, `{"ret":0}`)
	defer srv.Close()

	_, err := testClient(srv.URL, 50*time.Millisecond).SendMessage(context.Background(), &SendMessageRequest{})
	if !IsNetworkError(err) {
		t.Fatalf("err = %v, want NetworkError", err)
	}
}

func TestSendMessageDoesNotRetryRefusal(t *testing.T) {
	srv, calls := dropFirst(0, `{"ret":-2,"errmsg":"prepare failed"}`)
	defer srv.Close()

	resp, err := testClient(srv.URL, 5*time.Second).SendMessage(context.Background(), &SendMessageRequest{})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if resp.Ret != RetNotPrepared || atomic.LoadInt32(calls) != 1 {
		t.Fatalf("ret=%d calls=%d, want one call returning -2", resp.Ret, *calls)
	}
}

func TestSendMessageRetries5xxButNot4xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	_, err := testClient(srv.URL, 5*time.Second).SendMessage(context.Background(), &SendMessageRequest{})
	if err == nil || IsNetworkError(err) || atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("err=%v calls=%d, want plain HTTP 400 error after 2 calls", err, calls)
	}
}
