package messaging

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"weclaw/ilink"
)

// outboxMaxAge is how long an undelivered message waits for the user before
// it is dropped.
const outboxMaxAge = 7 * 24 * time.Hour

// Peer is what the bridge remembers about one WeChat user across processes.
// The context token comes with each inbound message; iLink wants it echoed on
// every bot send, and `weclaw send` (a separate process) needs it too.
type Peer struct {
	ContextToken  string    `json:"context_token,omitempty"`
	LastInboundAt time.Time `json:"last_inbound_at"`
	// RemindedFor is the LastInboundAt a window reminder was sent for; equal
	// to LastInboundAt means a reminder is outstanding.
	RemindedFor time.Time `json:"reminded_for,omitempty"`
	// FlushAfter delays the next background outbox flush after a refusal.
	FlushAfter time.Time `json:"flush_after,omitempty"`
}

// ReminderOutstanding reports whether a reminder was sent since the user's
// last message.
func (p Peer) ReminderOutstanding() bool {
	return !p.LastInboundAt.IsZero() && p.RemindedFor.Equal(p.LastInboundAt)
}

// OutboxItem is a message that could not be delivered and waits to be resent.
type OutboxItem struct {
	ID       string    `json:"id"`
	UserID   string    `json:"user_id"`
	Text     string    `json:"text,omitempty"`
	Media    []string  `json:"media,omitempty"` // URLs or absolute paths (local files are copied into the state dir)
	QueuedAt time.Time `json:"queued_at"`
	Reason   string    `json:"reason"` // "window" or "network"
}

type stateFile struct {
	Peers  map[string]*Peer `json:"peers"`
	Outbox []OutboxItem     `json:"outbox,omitempty"`
}

// Store persists per-account peer state and the outbox under ~/.weclaw/state.
// The bridge and `weclaw send` both use it, so every access takes a file lock.
type Store struct {
	dir string
	now func() time.Time
}

// NewStore returns a store rooted at dir.
func NewStore(dir string) *Store {
	return &Store{dir: dir, now: time.Now}
}

// DefaultStore returns the store at ~/.weclaw/state.
func DefaultStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return NewStore(filepath.Join(home, ".weclaw", "state")), nil
}

func (s *Store) base(botID string) string {
	return filepath.Join(s.dir, ilink.NormalizeAccountID(botID))
}

func (s *Store) filesDir(botID string) string {
	return s.base(botID) + ".outbox-files"
}

// update runs fn on the account's state under an exclusive lock and saves it.
func (s *Store) update(botID string, fn func(st *stateFile) error) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	unlock, err := lockFile(s.base(botID) + ".lock")
	if err != nil {
		return err
	}
	defer unlock()

	path := s.base(botID) + ".json"
	st := &stateFile{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, st); err != nil {
			log.Printf("[state] %s is unreadable, starting fresh: %v", filepath.Base(path), err)
			st = &stateFile{}
		}
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("read state: %w", err)
	}
	if st.Peers == nil {
		st.Peers = make(map[string]*Peer)
	}

	if err := fn(st); err != nil {
		return err
	}

	out, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// RecordInbound notes a message from userID and returns the peer state as it
// was before this message.
func (s *Store) RecordInbound(botID, userID, contextToken string, at time.Time) (Peer, error) {
	var prev Peer
	err := s.update(botID, func(st *stateFile) error {
		p := st.Peers[userID]
		if p == nil {
			p = &Peer{}
			st.Peers[userID] = p
		}
		prev = *p
		if contextToken != "" {
			p.ContextToken = contextToken
		}
		p.LastInboundAt = at
		p.FlushAfter = time.Time{}
		return nil
	})
	return prev, err
}

// Peer returns the stored state for userID (zero value if unknown).
func (s *Store) Peer(botID, userID string) (Peer, error) {
	var p Peer
	err := s.update(botID, func(st *stateFile) error {
		if sp := st.Peers[userID]; sp != nil {
			p = *sp
		}
		return nil
	})
	return p, err
}

// Peers returns all stored peers.
func (s *Store) Peers(botID string) (map[string]Peer, error) {
	out := make(map[string]Peer)
	err := s.update(botID, func(st *stateFile) error {
		for id, p := range st.Peers {
			out[id] = *p
		}
		return nil
	})
	return out, err
}

// MarkReminded records that a reminder was sent for the given inbound time.
func (s *Store) MarkReminded(botID, userID string, forInbound time.Time) error {
	return s.update(botID, func(st *stateFile) error {
		if p := st.Peers[userID]; p != nil && p.LastInboundAt.Equal(forInbound) {
			p.RemindedFor = forInbound
		}
		return nil
	})
}

// DeferFlush postpones background outbox flushes for userID until t.
func (s *Store) DeferFlush(botID, userID string, t time.Time) error {
	return s.update(botID, func(st *stateFile) error {
		if p := st.Peers[userID]; p != nil {
			p.FlushAfter = t
		}
		return nil
	})
}

// Enqueue adds an undelivered message: the text and the media become separate
// items, so a resend never repeats text that already went out. Local media
// files are copied so they survive temp-dir cleanup. An identical item
// already waiting is not added twice (callers often retry the same
// notification). Returns whether anything was added.
func (s *Store) Enqueue(botID, userID, text string, media []MediaRef, reason string) (bool, error) {
	added := false
	err := s.update(botID, func(st *stateFile) error {
		exists := func(text string, refs []string) bool {
			for _, it := range st.Outbox {
				if it.UserID == userID && it.Text == text && sameOrigins(it.Media, refs) {
					return true
				}
			}
			return false
		}
		add := func(text string, stored []string) {
			st.Outbox = append(st.Outbox, OutboxItem{
				ID:       uuid.NewString(),
				UserID:   userID,
				Text:     text,
				Media:    stored,
				QueuedAt: s.now(),
				Reason:   reason,
			})
			added = true
		}

		if text != "" && !exists(text, nil) {
			add(text, nil)
		}
		if len(media) == 0 {
			return nil
		}
		refs := make([]string, 0, len(media))
		for _, m := range media {
			refs = append(refs, m.String())
		}
		if exists("", refs) {
			return nil
		}
		id := uuid.NewString()
		stored := make([]string, 0, len(media))
		for _, m := range media {
			if m.URL != "" {
				stored = append(stored, m.URL)
				continue
			}
			dst, err := s.copyMedia(botID, id, m.Path)
			if err != nil {
				return err
			}
			stored = append(stored, dst)
		}
		add("", stored)
		return nil
	})
	return added, err
}

// sameOrigins compares queued media against new refs by base file name (local
// copies are renamed with an id prefix) or full URL.
func sameOrigins(stored, refs []string) bool {
	if len(stored) != len(refs) {
		return false
	}
	for i := range stored {
		if stored[i] == refs[i] {
			continue
		}
		if filepath.IsAbs(refs[i]) && originalName(stored[i]) == filepath.Base(refs[i]) {
			continue
		}
		return false
	}
	return true
}

func originalName(stored string) string {
	base := filepath.Base(stored)
	// copies are named "<uuid>-<original>"; a uuid is 36 chars.
	if len(base) > 37 && base[36] == '-' {
		return base[37:]
	}
	return base
}

func (s *Store) copyMedia(botID, id, src string) (string, error) {
	dir := s.filesDir(botID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, id+"-"+filepath.Base(src))
	in, err := os.Open(src)
	if err != nil {
		return "", fmt.Errorf("keep media for resend: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return "", err
	}
	return dst, out.Close()
}

// Pending returns userID's waiting messages, oldest first, dropping (and
// deleting files of) any older than outboxMaxAge.
func (s *Store) Pending(botID, userID string) ([]OutboxItem, error) {
	var items []OutboxItem
	err := s.update(botID, func(st *stateFile) error {
		kept := st.Outbox[:0]
		for _, it := range st.Outbox {
			if s.now().Sub(it.QueuedAt) > outboxMaxAge {
				log.Printf("[outbox] dropping message for %s queued %s ago", it.UserID, s.now().Sub(it.QueuedAt).Round(time.Hour))
				s.removeFiles(botID, it)
				continue
			}
			kept = append(kept, it)
			if it.UserID == userID {
				items = append(items, it)
			}
		}
		st.Outbox = kept
		return nil
	})
	return items, err
}

// UsersWithPending returns the users that have waiting messages.
func (s *Store) UsersWithPending(botID string) ([]string, error) {
	var users []string
	err := s.update(botID, func(st *stateFile) error {
		seen := map[string]bool{}
		for _, it := range st.Outbox {
			if !seen[it.UserID] {
				seen[it.UserID] = true
				users = append(users, it.UserID)
			}
		}
		return nil
	})
	return users, err
}

// Remove deletes delivered items (and their copied files).
func (s *Store) Remove(botID string, ids []string) error {
	drop := make(map[string]bool, len(ids))
	for _, id := range ids {
		drop[id] = true
	}
	return s.update(botID, func(st *stateFile) error {
		kept := st.Outbox[:0]
		for _, it := range st.Outbox {
			if drop[it.ID] {
				s.removeFiles(botID, it)
				continue
			}
			kept = append(kept, it)
		}
		st.Outbox = kept
		return nil
	})
}

func (s *Store) removeFiles(botID string, it OutboxItem) {
	dir := s.filesDir(botID)
	for _, m := range it.Media {
		if filepath.Dir(m) == dir {
			os.Remove(m)
		}
	}
}
