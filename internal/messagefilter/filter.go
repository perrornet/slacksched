package messagefilter

import (
	"strings"
	"sync"
	"time"
)

// Input describes a normalized Slack message for filtering.
type Input struct {
	TeamID        string
	EventID       string
	ClientMsgID   string
	UserID        string
	ChannelID     string
	Text          string
	ThreadTS      string // parent thread (empty if top-level)
	MessageTS     string
	Subtype       string
	IsIM          bool
	Hidden        bool // e.g. message_changed inner
	UserIsBot     bool
	MessageHasBot bool // bot_id set on message
}

// SessionBoundFunc returns whether this Slack thread already has an active scheduler session.
type SessionBoundFunc func(teamID, channelID, rootThreadTS string) bool

// Filter decides whether the message should be processed.
type Filter struct {
	AllowedDMUserIDs        map[string]struct{}
	AllowedChannelIDs       map[string]struct{}
	AnyChannelAllowed       bool
	RequireMentionInChannels bool
	AppUserID               string

	SessionBound SessionBoundFunc

	dedupeTTL time.Duration
	dedupe    *deduper
}

// New builds a filter from lists and mention requirement.
func New(allowedDM, allowedCh []string, requireMention bool, appUserID string, bound SessionBoundFunc) *Filter {
	dm := make(map[string]struct{})
	for _, id := range allowedDM {
		id = strings.TrimSpace(id)
		if id != "" {
			dm[id] = struct{}{}
		}
	}
	ch := make(map[string]struct{})
	anyCh := len(allowedCh) == 0
	for _, id := range allowedCh {
		id = strings.TrimSpace(id)
		if id != "" {
			ch[id] = struct{}{}
		}
	}
	return &Filter{
		AllowedDMUserIDs:         dm,
		AllowedChannelIDs:        ch,
		AnyChannelAllowed:        anyCh,
		RequireMentionInChannels: requireMention,
		AppUserID:                strings.TrimSpace(appUserID),
		SessionBound:             bound,
		dedupeTTL:                5 * time.Minute,
		dedupe:                   newDeduper(5 * time.Minute),
	}
}

// mentionsUser reports whether text includes a Slack mention of userID
// (<@U123> or <@U123|label>).
func mentionsUser(text, userID string) bool {
	id := strings.TrimSpace(userID)
	if id == "" {
		return false
	}
	return strings.Contains(text, "<@"+id+">") || strings.Contains(text, "<@"+id+"|")
}

// ShouldProcess returns accept, reason.
func (f *Filter) ShouldProcess(in Input) (bool, string) {
	if in.Hidden {
		return false, "hidden"
	}
	if in.UserIsBot || in.MessageHasBot {
		return false, "bot"
	}
	if in.UserID != "" && in.UserID == f.AppUserID {
		return false, "self"
	}
	if sub := strings.TrimSpace(in.Subtype); sub != "" {
		switch sub {
		case "message_deleted", "message_changed", "bot_message",
			"channel_join", "channel_leave", "group_join", "group_leave":
			return false, "subtype:" + sub
		}
	}
	// Dedupe Slack retries
	if in.EventID != "" && f.dedupe.seen("e:" + in.EventID) {
		return false, "dedupe:event_id"
	}
	if in.ClientMsgID != "" && f.dedupe.seen("c:" + in.ClientMsgID) {
		return false, "dedupe:client_msg_id"
	}
	if in.ChannelID != "" && strings.TrimSpace(in.MessageTS) != "" {
		if f.dedupe.seen("m:" + in.ChannelID + ":" + strings.TrimSpace(in.MessageTS)) {
			return false, "dedupe:message_ts"
		}
	}

	rootTS := in.ThreadTS
	if rootTS == "" {
		rootTS = in.MessageTS
	}

	if in.IsIM {
		if _, ok := f.AllowedDMUserIDs[in.UserID]; !ok {
			return false, "dm:not_allowed"
		}
		return true, "dm"
	}

	if !f.AnyChannelAllowed {
		if _, ok := f.AllowedChannelIDs[in.ChannelID]; !ok {
			if f.SessionBound == nil || !f.SessionBound(in.TeamID, in.ChannelID, rootTS) {
				return false, "channel:not_allowed"
			}
			if f.RequireMentionInChannels && f.AppUserID != "" && !mentionsUser(in.Text, f.AppUserID) {
				return false, "channel:mention_required"
			}
			return true, "channel:bound_thread"
		}
	}

	if f.SessionBound != nil && f.SessionBound(in.TeamID, in.ChannelID, rootTS) {
		if f.RequireMentionInChannels && f.AppUserID != "" && !mentionsUser(in.Text, f.AppUserID) {
			return false, "channel:mention_required"
		}
		return true, "channel:bound_thread"
	}

	if f.RequireMentionInChannels && f.AppUserID != "" {
		if !mentionsUser(in.Text, f.AppUserID) {
			return false, "channel:mention_required"
		}
	}

	return true, "channel"
}

type deduper struct {
	mu  sync.Mutex
	ttl time.Duration
	m   map[string]time.Time
}

func newDeduper(ttl time.Duration) *deduper {
	return &deduper{ttl: ttl, m: make(map[string]time.Time)}
}

func (d *deduper) seen(key string) bool {
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()
	for k, t := range d.m {
		if now.Sub(t) > d.ttl {
			delete(d.m, k)
		}
	}
	if _, ok := d.m[key]; ok {
		return true
	}
	d.m[key] = now
	return false
}

// SetDedupeTTL is used in tests.
func (f *Filter) SetDedupeTTL(d time.Duration) {
	f.dedupeTTL = d
	f.dedupe = newDeduper(d)
}
