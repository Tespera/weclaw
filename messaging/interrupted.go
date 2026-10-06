package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"weclaw/ilink"
)

// interruptedMaxAge bounds how stale a saved interruption may be and still be
// announced after startup.
const interruptedMaxAge = 24 * time.Hour

// InterruptedReply is a chat whose reply was cut off because the bridge shut
// down (restart, update, logout) while the agent was still working on it. The
// next instance tells the user, who would otherwise just never get an answer.
type InterruptedReply struct {
	BotID         string    `json:"bot_id"`
	UserID        string    `json:"user_id"`
	ContextToken  string    `json:"context_token,omitempty"`
	InterruptedAt time.Time `json:"interrupted_at"`
}

// recordInterrupted notes that the reply to msg could not be delivered because
// the bridge is shutting down. One entry per chat.
func (h *Handler) recordInterrupted(client *ilink.Client, msg ilink.WeixinMessage) {
	h.interruptedMu.Lock()
	defer h.interruptedMu.Unlock()
	for _, r := range h.interrupted {
		if r.BotID == client.BotID() && r.UserID == msg.FromUserID {
			return
		}
	}
	h.interrupted = append(h.interrupted, InterruptedReply{
		BotID:         client.BotID(),
		UserID:        msg.FromUserID,
		ContextToken:  msg.ContextToken,
		InterruptedAt: time.Now(),
	})
	log.Printf("[handler] reply to %s interrupted by shutdown; will notify after restart", msg.FromUserID)
}

// WaitInflight waits up to timeout for messages still being handled. After
// shutdown begins they finish quickly: agent calls and sends see the canceled
// context and return.
func (h *Handler) WaitInflight(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		h.inflight.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		log.Printf("[handler] messages still in flight after %s, not waiting further", timeout)
	}
}

// SaveInterrupted writes the interrupted chats to path for the next instance.
// Nothing is written when there are none.
func (h *Handler) SaveInterrupted(path string) error {
	h.interruptedMu.Lock()
	pending := append([]InterruptedReply(nil), h.interrupted...)
	h.interruptedMu.Unlock()
	if len(pending) == 0 {
		return nil
	}
	data, err := json.MarshalIndent(pending, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// NotifyInterrupted tells each chat recorded in path by a previous instance
// that the bridge restarted before its reply was sent, then removes the file.
func NotifyInterrupted(ctx context.Context, path string, clients []*ilink.Client, version string) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	// Remove first: a malformed or partly delivered file must not be replayed
	// on every start.
	os.Remove(path)
	if err != nil {
		log.Printf("[handler] read %s: %v", filepath.Base(path), err)
		return
	}
	var pending []InterruptedReply
	if err := json.Unmarshal(data, &pending); err != nil {
		log.Printf("[handler] parse %s: %v", filepath.Base(path), err)
		return
	}

	text := interruptedNotice(version)
	for _, r := range pending {
		if time.Since(r.InterruptedAt) > interruptedMaxAge {
			continue
		}
		client := clientForBot(clients, r.BotID)
		if client == nil {
			log.Printf("[handler] no account %s to notify %s of interrupted reply", r.BotID, r.UserID)
			continue
		}
		err := SendTextReply(ctx, client, r.UserID, text, r.ContextToken, "")
		if err != nil && r.ContextToken != "" {
			// The saved context token may have expired; a plain send works too.
			err = SendTextReply(ctx, client, r.UserID, text, "", "")
		}
		if err != nil {
			log.Printf("[handler] failed to notify %s of interrupted reply: %v", r.UserID, err)
			continue
		}
		log.Printf("[handler] notified %s that their reply was interrupted by a restart", r.UserID)
	}
}

func interruptedNotice(version string) string {
	v := ""
	if version != "" {
		v = fmt.Sprintf("（%s）", version)
	}
	return "weclaw 已重启" + v + "。刚才那条消息还没回复完就被中断了，需要的话请重新发送。"
}

func clientForBot(clients []*ilink.Client, botID string) *ilink.Client {
	for _, c := range clients {
		if c.BotID() == botID {
			return c
		}
	}
	if len(clients) == 1 && botID == "" {
		return clients[0]
	}
	return nil
}
