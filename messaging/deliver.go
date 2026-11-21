package messaging

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"weclaw/ilink"
)

// Failure reasons for DeliveryError.
const (
	ReasonWindow  = "window"  // iLink refused: reply window closed or used up
	ReasonNetwork = "network" // could not reach iLink even after retries
)

// windowLength is how long after the user's last message iLink accepts bot
// messages, as observed (not documented by Tencent).
const windowLength = 24 * time.Hour

// Outgoing is a message to deliver: optional text followed by media.
type Outgoing struct {
	Text  string
	Media []MediaRef
}

// DeliveryError explains why a message was not delivered and whether it is
// waiting in the outbox.
type DeliveryError struct {
	Reason string
	Queued bool
	// Peer state at the time, for the explanation.
	LastInboundAt time.Time
	Now           time.Time
	Err           error
}

func (e *DeliveryError) Unwrap() error { return e.Err }

func (e *DeliveryError) Error() string {
	var b strings.Builder
	b.WriteString("未送达：")
	switch e.Reason {
	case ReasonWindow:
		b.WriteString("微信只允许机器人在你最后一次给它发消息后约 24 小时内主动推送，你不回复时最多连发约 10 条。")
		if e.LastInboundAt.IsZero() {
			b.WriteString("（还没有记录到你给机器人发过消息。）")
		} else {
			fmt.Fprintf(&b, "（你上次发消息是 %s 前。）", humanDuration(e.Now.Sub(e.LastInboundAt)))
		}
		if e.Queued {
			b.WriteString("已放进待发箱，你在微信里随便发一句就会自动补发。")
		}
	case ReasonNetwork:
		b.WriteString("连不上微信服务器（网络问题，已自动重试）。")
		if e.Queued {
			b.WriteString("已放进待发箱，网络恢复后自动补发。")
		}
	}
	if e.Err != nil {
		fmt.Fprintf(&b, " [%v]", e.Err)
	}
	return b.String()
}

// ExitCode maps the failure to a process exit code for `weclaw send`.
func (e *DeliveryError) ExitCode() int {
	if e.Reason == ReasonNetwork {
		return 4
	}
	return 3
}

func humanDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%d 分钟", int(d.Minutes()))
	}
	h := d.Hours()
	if h < 48 {
		return fmt.Sprintf("%.1f 小时", h)
	}
	return fmt.Sprintf("%.0f 天", h/24)
}

// classify returns the DeliveryError reason for err, or "" if it is neither a
// window refusal nor a network failure.
func classify(err error) string {
	switch {
	case ilink.IsNotPrepared(err):
		return ReasonWindow
	case ilink.IsNetworkError(err):
		return ReasonNetwork
	}
	return ""
}

// Sender abstracts the two sends Deliver makes, for tests.
type Sender interface {
	SendText(ctx context.Context, to, text, contextToken string) error
	SendMedia(ctx context.Context, to string, m MediaRef, contextToken string) error
}

type clientSender struct{ c *ilink.Client }

func (s clientSender) SendText(ctx context.Context, to, text, token string) error {
	return SendTextReply(ctx, s.c, to, text, token, "")
}

func (s clientSender) SendMedia(ctx context.Context, to string, m MediaRef, token string) error {
	return SendMedia(ctx, s.c, to, m, token)
}

// Deliverer sends bot-initiated messages (not replies to a message being
// handled) using the user's latest stored context token.
type Deliverer struct {
	BotID  string
	Sender Sender
	Store  *Store
	Now    func() time.Time
}

// NewDeliverer returns a Deliverer for client backed by store.
func NewDeliverer(client *ilink.Client, store *Store) *Deliverer {
	return &Deliverer{BotID: client.BotID(), Sender: clientSender{client}, Store: store, Now: time.Now}
}

// Deliver sends out to userID. On a window refusal or network failure the
// undelivered part is queued (when queue is true) and a *DeliveryError is
// returned. onSent, if set, is called after each part goes out.
func (d *Deliverer) Deliver(ctx context.Context, to string, out Outgoing, queue bool, onSent func(part string)) error {
	var peer Peer
	if d.Store != nil {
		p, err := d.Store.Peer(d.BotID, to)
		if err != nil {
			log.Printf("[deliver] read peer state: %v", err)
		}
		peer = p
	}

	fail := func(err error, text string, media []MediaRef) error {
		reason := classify(err)
		if reason == "" {
			return err
		}
		de := &DeliveryError{Reason: reason, LastInboundAt: peer.LastInboundAt, Now: d.Now(), Err: err}
		if queue && d.Store != nil {
			if _, qerr := d.Store.Enqueue(d.BotID, to, text, media, reason); qerr != nil {
				log.Printf("[deliver] could not queue message for %s: %v", to, qerr)
			} else {
				de.Queued = true
				log.Printf("[deliver] queued message for %s (%s)", to, reason)
			}
		}
		return de
	}

	if out.Text != "" {
		if err := d.withToken(peer.ContextToken, func(tok string) error {
			return d.Sender.SendText(ctx, to, out.Text, tok)
		}); err != nil {
			return fail(err, out.Text, out.Media)
		}
		if onSent != nil {
			onSent("text")
		}
	}
	for i, m := range out.Media {
		if err := d.withToken(peer.ContextToken, func(tok string) error {
			return d.Sender.SendMedia(ctx, to, m, tok)
		}); err != nil {
			return fail(err, "", out.Media[i:])
		}
		if onSent != nil {
			onSent(m.String())
		}
	}
	return nil
}

// withToken sends with the stored token and, if iLink refuses that, once
// more without one: tokenless sends were what `weclaw send` always did and
// they do get through at times the token is refused.
func (d *Deliverer) withToken(token string, send func(tok string) error) error {
	err := send(token)
	if err != nil && token != "" && ilink.IsNotPrepared(err) {
		log.Printf("[deliver] refused with stored context token, retrying without it")
		if err2 := send(""); err2 == nil {
			return nil
		} else if !ilink.IsNotPrepared(err2) {
			return err2
		}
	}
	return err
}

// FlushOutbox resends userID's waiting messages with contextToken. Waiting
// texts are merged into one message (each counts against the ~10-message
// budget). It stops at the first refusal and keeps the rest. Returns how many
// items were delivered.
func (d *Deliverer) FlushOutbox(ctx context.Context, to, contextToken string) (int, error) {
	if d.Store == nil {
		return 0, nil
	}
	items, err := d.Store.Pending(d.BotID, to)
	if err != nil || len(items) == 0 {
		return 0, err
	}

	var texts []string
	var textIDs []string
	for _, it := range items {
		if it.Text != "" {
			texts = append(texts, fmt.Sprintf("〔%s〕\n%s", it.QueuedAt.Local().Format("01-02 15:04"), it.Text))
		}
		if it.Text != "" {
			textIDs = append(textIDs, it.ID)
		}
	}

	delivered := 0
	if len(texts) > 0 {
		header := fmt.Sprintf("有 %d 条消息之前没发出去，现在补上：", len(texts))
		for _, chunk := range chunkTexts(header, texts, maxMergedRunes) {
			if err := d.withToken(contextToken, func(tok string) error {
				return d.Sender.SendText(ctx, to, chunk, tok)
			}); err != nil {
				return delivered, err
			}
		}
	}
	// Text-only items are done once the merged text is out.
	if len(textIDs) > 0 {
		if err := d.Store.Remove(d.BotID, textIDs); err != nil {
			return delivered, err
		}
		delivered += len(textIDs)
	}

	for _, it := range items {
		if it.Text != "" || len(it.Media) == 0 {
			continue
		}
		for _, ref := range it.Media {
			m, err := ResolveMediaRef(ref)
			if err != nil {
				log.Printf("[outbox] skipping media %s: %v", ref, err)
				continue
			}
			if err := d.withToken(contextToken, func(tok string) error {
				return d.Sender.SendMedia(ctx, to, m, tok)
			}); err != nil {
				return delivered, err
			}
		}
		if err := d.Store.Remove(d.BotID, []string{it.ID}); err != nil {
			return delivered, err
		}
		delivered++
	}
	log.Printf("[outbox] delivered %d waiting message(s) to %s", delivered, to)
	return delivered, nil
}

// maxMergedRunes caps one merged outbox message; very long single sends have
// been reported to fail with "prepare failed" too.
const maxMergedRunes = 3000

func chunkTexts(header string, texts []string, limit int) []string {
	var chunks []string
	cur := header
	for _, t := range texts {
		piece := "\n\n" + t
		if len([]rune(cur))+len([]rune(piece)) > limit && cur != header {
			chunks = append(chunks, cur)
			cur = strings.TrimPrefix(piece, "\n\n")
			continue
		}
		cur += piece
	}
	return append(chunks, cur)
}

// IsDeliveryError reports whether err is a *DeliveryError and returns it.
func IsDeliveryError(err error) (*DeliveryError, bool) {
	var de *DeliveryError
	ok := errors.As(err, &de)
	return de, ok
}
