package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"

	"weclaw/ilink"
	"weclaw/messaging"
)

// Server provides an HTTP API for sending messages.
type Server struct {
	clients []*ilink.Client
	addr    string
}

// NewServer creates an API server.
func NewServer(clients []*ilink.Client, addr string) *Server {
	if addr == "" {
		addr = "127.0.0.1:18011"
	}
	return &Server{clients: clients, addr: addr}
}

// SendRequest is the JSON body for POST /api/send.
type SendRequest struct {
	To       string `json:"to"`
	Text     string `json:"text,omitempty"`
	Media    string `json:"media,omitempty"`     // image/video/file: URL or absolute local path
	MediaURL string `json:"media_url,omitempty"` // deprecated alias of Media, kept for compatibility
}

// Run starts the HTTP server. Blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/send", s.handleSend)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	srv := &http.Server{Addr: s.addr, Handler: mux}

	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()

	log.Printf("[api] listening on %s", s.addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var req SendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.To == "" {
		http.Error(w, `"to" is required`, http.StatusBadRequest)
		return
	}
	mediaRef := req.Media
	if mediaRef == "" {
		mediaRef = req.MediaURL
	}
	if req.Text == "" && mediaRef == "" {
		http.Error(w, `"text" or "media" is required`, http.StatusBadRequest)
		return
	}
	// Resolve media before sending text so a bad path fails the whole request.
	var media *messaging.MediaRef
	if mediaRef != "" {
		if !isRemoteURL(mediaRef) && !isLoopback(r.RemoteAddr) {
			// The API may be bound to a non-loopback address (WECLAW_API_ADDR);
			// never let remote callers read local files.
			http.Error(w, "local file paths are only accepted from localhost", http.StatusForbidden)
			return
		}
		m, err := resolveAPIMedia(mediaRef)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		media = &m
	}

	if len(s.clients) == 0 {
		http.Error(w, "no accounts configured", http.StatusServiceUnavailable)
		return
	}

	// Use the first client
	client := s.clients[0]
	ctx := r.Context()

	// Send text if provided
	if req.Text != "" {
		if err := messaging.SendTextReply(ctx, client, req.To, req.Text, "", ""); err != nil {
			log.Printf("[api] send text failed: %v", err)
			http.Error(w, "send text failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("[api] sent text to %s: %q", req.To, req.Text)

		// Extract and send any markdown images embedded in text
		for _, imgURL := range messaging.ExtractImageURLs(req.Text) {
			if err := messaging.SendMediaFromURL(ctx, client, req.To, imgURL, ""); err != nil {
				log.Printf("[api] send extracted image failed: %v", err)
			} else {
				log.Printf("[api] sent extracted image to %s: %s", req.To, imgURL)
			}
		}
	}

	// Send media if provided
	if media != nil {
		if err := messaging.SendMedia(ctx, client, req.To, *media, ""); err != nil {
			log.Printf("[api] send media failed: %v", err)
			http.Error(w, "send media failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("[api] sent media to %s: %s", req.To, media)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// resolveAPIMedia accepts a URL or an absolute local path. Relative paths are
// rejected: they would resolve against the daemon's directory, not the caller's.
func resolveAPIMedia(ref string) (messaging.MediaRef, error) {
	lower := strings.ToLower(strings.TrimSpace(ref))
	isURL := strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "file://")
	if !isURL && !strings.HasPrefix(ref, "~") && !filepath.IsAbs(ref) {
		return messaging.MediaRef{}, fmt.Errorf("media path must be absolute: %q", ref)
	}
	return messaging.ResolveMediaRef(ref)
}

func isRemoteURL(ref string) bool {
	lower := strings.ToLower(strings.TrimSpace(ref))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
