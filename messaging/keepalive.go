package messaging

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"weclaw/ilink"
)

const (
	// remindBefore is how long before the window closes the reminder goes out.
	remindBefore = 4 * time.Hour
	// remindCutoff: closer than this to expiry, a reminder is pointless.
	remindCutoff = 15 * time.Minute
	// maintenanceEvery is how often the bridge flushes the outbox and checks
	// reminders.
	maintenanceEvery = 5 * time.Minute
	// keepaliveMaxRunes: a reply this short to an outstanding reminder only
	// renews the window and is not passed to the agent.
	keepaliveMaxRunes = 4
)

// ReminderConfig controls the "window about to close" reminder.
type ReminderConfig struct {
	Enabled  bool
	Location *time.Location
	// Quiet hours as minutes after local midnight; a reminder that would land
	// inside them goes out at QuietStart-30min the evening before instead.
	QuietStart int
	QuietEnd   int
}

// DefaultReminderConfig: on, Beijing time, quiet 23:00–08:00.
func DefaultReminderConfig() ReminderConfig {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return ReminderConfig{Enabled: true, Location: loc, QuietStart: 23 * 60, QuietEnd: 8 * 60}
}

func (rc ReminderConfig) loc() *time.Location {
	if rc.Location == nil {
		return time.Local
	}
	return rc.Location
}

func (rc ReminderConfig) inQuiet(t time.Time) bool {
	lt := t.In(rc.loc())
	m := lt.Hour()*60 + lt.Minute()
	if rc.QuietStart == rc.QuietEnd {
		return false
	}
	if rc.QuietStart < rc.QuietEnd {
		return m >= rc.QuietStart && m < rc.QuietEnd
	}
	return m >= rc.QuietStart || m < rc.QuietEnd
}

// ReminderAt returns when to remind for a window opened at lastInbound.
func (rc ReminderConfig) ReminderAt(lastInbound time.Time) time.Time {
	r := lastInbound.Add(windowLength - remindBefore)
	if !rc.inQuiet(r) {
		return r
	}
	// Move to the evening before the quiet hours start.
	lr := r.In(rc.loc())
	evening := time.Date(lr.Year(), lr.Month(), lr.Day(), 0, 0, 0, 0, rc.loc()).
		Add(time.Duration(rc.QuietStart-30) * time.Minute)
	if evening.After(r) {
		evening = evening.AddDate(0, 0, -1)
	}
	if evening.Before(lastInbound) {
		return r
	}
	return evening
}

// ReminderDue reports whether a reminder should be sent to p now.
func (rc ReminderConfig) ReminderDue(now time.Time, p Peer) bool {
	if !rc.Enabled || p.LastInboundAt.IsZero() || p.ReminderOutstanding() {
		return false
	}
	expiry := p.LastInboundAt.Add(windowLength)
	if now.Before(rc.ReminderAt(p.LastInboundAt)) || !now.Before(expiry.Add(-remindCutoff)) {
		return false
	}
	return !rc.inQuiet(now)
}

func (rc ReminderConfig) reminderText(now time.Time, p Peer) string {
	expiry := p.LastInboundAt.Add(windowLength)
	return fmt.Sprintf("提醒：微信只允许机器人在你上次发消息后 24 小时内主动推送，这个窗口约 %s后（%s）关闭。回我任意一个字（比如 1）就能续上，这条回复不会转给 AI。",
		humanDuration(expiry.Sub(now)), expiry.In(rc.loc()).Format("01-02 15:04"))
}

const keepaliveAck = "已续上，接下来 24 小时内的通知都能正常推送。"

// isKeepaliveReply reports whether text looks like a reply to the reminder
// rather than a request for the agent.
func isKeepaliveReply(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" || strings.HasPrefix(t, "/") || strings.HasPrefix(t, "@") {
		return false
	}
	return len([]rune(t)) <= keepaliveMaxRunes
}

// SetStore enables persistent peer state, the outbox and reminders.
func (h *Handler) SetStore(s *Store) {
	h.store = s
}

// SetReminder configures the window reminder.
func (h *Handler) SetReminder(rc ReminderConfig) {
	h.reminder = rc
}

// recordInbound persists the user's latest context token and message time,
// returning the state before this message.
func (h *Handler) recordInbound(client *ilink.Client, msg ilink.WeixinMessage) Peer {
	if h.store == nil {
		return Peer{}
	}
	prev, err := h.store.RecordInbound(client.BotID(), msg.FromUserID, msg.ContextToken, time.Now())
	if err != nil {
		log.Printf("[state] record inbound from %s: %v", msg.FromUserID, err)
	}
	return prev
}

// flushOutbox resends waiting messages now that the user opened a window.
func (h *Handler) flushOutbox(ctx context.Context, client *ilink.Client, msg ilink.WeixinMessage) {
	if h.store == nil {
		return
	}
	h.flushMu.Lock()
	defer h.flushMu.Unlock()
	d := NewDeliverer(client, h.store)
	if _, err := d.FlushOutbox(ctx, msg.FromUserID, msg.ContextToken); err != nil {
		log.Printf("[outbox] flush to %s failed: %v", msg.FromUserID, err)
	}
}

// queueFailedReply keeps an agent reply that could not be sent.
func (h *Handler) queueFailedReply(client *ilink.Client, userID, text string, err error) {
	reason := classify(err)
	if h.store == nil || reason == "" || text == "" {
		return
	}
	if _, qerr := h.store.Enqueue(client.BotID(), userID, text, nil, reason); qerr != nil {
		log.Printf("[outbox] could not queue reply for %s: %v", userID, qerr)
		return
	}
	log.Printf("[outbox] queued undelivered reply for %s (%s)", userID, reason)
}

// RunMaintenance periodically resends waiting messages while a window is
// open (e.g. after a network outage) and sends window reminders. It returns
// when ctx is done.
func (h *Handler) RunMaintenance(ctx context.Context, clients []*ilink.Client) {
	if h.store == nil {
		return
	}
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		for _, c := range clients {
			h.maintain(ctx, c, time.Now())
		}
		timer.Reset(maintenanceEvery)
	}
}

func (h *Handler) maintain(ctx context.Context, client *ilink.Client, now time.Time) {
	botID := client.BotID()
	peers, err := h.store.Peers(botID)
	if err != nil {
		log.Printf("[maintain] read state: %v", err)
		return
	}
	d := NewDeliverer(client, h.store)

	users, _ := h.store.UsersWithPending(botID)
	for _, u := range users {
		p := peers[u]
		if p.LastInboundAt.IsZero() || now.Sub(p.LastInboundAt) >= windowLength || now.Before(p.FlushAfter) {
			continue // waits for the user's next message
		}
		h.flushMu.Lock()
		_, err := d.FlushOutbox(ctx, u, p.ContextToken)
		h.flushMu.Unlock()
		if err != nil {
			backoff := 5 * time.Minute
			if classify(err) == ReasonWindow {
				backoff = 30 * time.Minute
			}
			log.Printf("[outbox] background flush to %s failed: %v; next try in %s", u, err, backoff)
			_ = h.store.DeferFlush(botID, u, now.Add(backoff))
		}
	}

	for u, p := range peers {
		if !h.reminder.ReminderDue(now, p) {
			continue
		}
		err := d.Deliver(ctx, u, Outgoing{Text: h.reminder.reminderText(now, p)}, false, nil)
		if err != nil && classify(err) == ReasonNetwork {
			log.Printf("[reminder] network error reminding %s, will retry: %v", u, err)
			continue
		}
		if err != nil {
			log.Printf("[reminder] reminder to %s refused: %v", u, err)
		} else {
			log.Printf("[reminder] reminded %s that the window closes at %s", u, p.LastInboundAt.Add(windowLength).Format(time.RFC3339))
		}
		_ = h.store.MarkReminded(botID, u, p.LastInboundAt)
	}
}
