package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveAPIMediaRejectsRelative(t *testing.T) {
	if _, err := resolveAPIMedia("report.pdf"); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative path error = %v, want absolute-path error", err)
	}
	f := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if m, err := resolveAPIMedia(f); err != nil || m.Path == "" {
		t.Fatalf("absolute path: %+v, %v", m, err)
	}
	if m, err := resolveAPIMedia("https://example.com/x.png"); err != nil || m.URL == "" {
		t.Fatalf("url: %+v, %v", m, err)
	}
}

// A bad media path must fail the request before any text is sent.
func TestHandleSendBadMediaFailsBeforeSending(t *testing.T) {
	s := NewServer(nil, "")
	body := `{"to":"u@im.wechat","text":"hello","media":"/definitely/missing.pdf"}`
	req := httptest.NewRequest(http.MethodPost, "/api/send", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:50000"
	rec := httptest.NewRecorder()
	s.handleSend(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %q)", rec.Code, rec.Body.String())
	}
}

// Local paths are refused for non-loopback callers even if the file exists.
func TestHandleSendLocalPathForbiddenFromRemote(t *testing.T) {
	f := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewServer(nil, "")
	for _, remote := range []string{"192.168.1.20:4000", "[fe80::1]:4000"} {
		req := httptest.NewRequest(http.MethodPost, "/api/send", strings.NewReader(`{"to":"u","media":"`+f+`"}`))
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		s.handleSend(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("remote %s: status = %d, want 403", remote, rec.Code)
		}
	}
	for _, remote := range []string{"127.0.0.1:1", "[::1]:1"} {
		if !isLoopback(remote) {
			t.Errorf("isLoopback(%q) = false", remote)
		}
	}
}
