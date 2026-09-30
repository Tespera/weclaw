package messaging

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"weclaw/ilink"
)

// MediaRef is a media source to send: a remote URL or a local file.
type MediaRef struct {
	URL  string // set for http(s) sources
	Path string // absolute path, set for local files
}

func (m MediaRef) String() string {
	if m.URL != "" {
		return m.URL
	}
	return m.Path
}

// ResolveMediaRef interprets ref as an http(s) URL, a file:// URL, or a local
// path (~ expanded, relative paths resolved against the current directory).
// Local paths must name an existing regular file; any file type is accepted.
func ResolveMediaRef(ref string) (MediaRef, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return MediaRef{}, fmt.Errorf("empty media reference")
	}
	lower := strings.ToLower(ref)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return MediaRef{URL: ref}, nil
	}

	path := ref
	if strings.HasPrefix(lower, "file://") {
		u, err := url.Parse(ref)
		if err != nil {
			return MediaRef{}, fmt.Errorf("invalid file URL %q: %w", ref, err)
		}
		path = u.Path
	} else if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return MediaRef{}, fmt.Errorf("expand %q: %w", ref, err)
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return MediaRef{}, fmt.Errorf("resolve %q: %w", ref, err)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return MediaRef{}, fmt.Errorf("media file %s: %w", abs, err)
	}
	if !fi.Mode().IsRegular() {
		return MediaRef{}, fmt.Errorf("media %s is not a regular file", abs)
	}
	return MediaRef{Path: abs}, nil
}

// SendMedia sends a resolved media reference.
func SendMedia(ctx context.Context, client *ilink.Client, toUserID string, m MediaRef, contextToken string) error {
	if m.URL != "" {
		return SendMediaFromURL(ctx, client, toUserID, m.URL, contextToken)
	}
	return SendMediaFromPath(ctx, client, toUserID, m.Path, contextToken)
}
