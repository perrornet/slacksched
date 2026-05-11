package contextapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/slack-go/slack"

	"github.com/perrornet/slacksched/internal/slackthread"
)

// Handler serves GET /v1/slack/thread/messages with Bearer auth against Registry.
type Handler struct {
	api      *slack.Client
	reg      *Registry
	log      *slog.Logger
	serveMux *http.ServeMux
}

// NewHandler builds the HTTP handler tree.
func NewHandler(api *slack.Client, reg *Registry, log *slog.Logger) http.Handler {
	h := &Handler{api: api, reg: reg, log: log}
	m := http.NewServeMux()
	m.HandleFunc("/v1/slack/thread/messages", h.handleThreadMessages)
	m.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	h.serveMux = m
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.serveMux.ServeHTTP(w, r)
}

type msgJSON struct {
	TS      string `json:"ts"`
	User    string `json:"user,omitempty"`
	BotID   string `json:"bot_id,omitempty"`
	Text    string `json:"text"`
	Subtype string `json:"subtype,omitempty"`
}

type threadMessagesResponse struct {
	ChannelID    string    `json:"channel_id"`
	TeamID       string    `json:"team_id"`
	RootThreadTS string    `json:"root_thread_ts"`
	Messages     []msgJSON `json:"messages"`
}

func (h *Handler) handleThreadMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tok := bearerToken(r.Header.Get("Authorization"))
	if tok == "" {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}
	if h.reg == nil {
		http.Error(w, "context API registry disabled", http.StatusServiceUnavailable)
		return
	}
	key, ok := h.reg.Lookup(tok)
	if !ok {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}

	excl := strings.TrimSpace(r.URL.Query().Get("exclude_ts"))
	oldest := strings.TrimSpace(r.URL.Query().Get("oldest"))

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	msgs, err := slackthread.CollectReplies(ctx, h.api, key.ChannelID, key.RootThreadTS, limit)
	if err != nil {
		if h.log != nil {
			h.log.Warn("context_api_thread", "err", err, "channel_id", key.ChannelID, "thread_ts", key.RootThreadTS)
		}
		http.Error(w, "slack error", http.StatusBadGateway)
		return
	}

	var out []msgJSON
	for _, m := range msgs {
		ts := strings.TrimSpace(m.Timestamp)
		if excl != "" && ts == excl {
			continue
		}
		if oldest != "" && ts != "" && slackTSLessOrEqual(ts, oldest) {
			continue
		}
		out = append(out, msgJSON{
			TS:      ts,
			User:    m.User,
			BotID:   m.BotID,
			Text:    m.Text,
			Subtype: m.SubType,
		})
	}

	res := threadMessagesResponse{
		ChannelID:    key.ChannelID,
		TeamID:       key.TeamID,
		RootThreadTS: key.RootThreadTS,
		Messages:     out,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	if err := enc.Encode(res); err != nil && h.log != nil {
		h.log.Warn("context_api_encode", "err", err)
	}
}

// slackTSLessOrEqual reports whether a is <= b using Slack timestamp float ordering.
func slackTSLessOrEqual(a, b string) bool {
	fa, err1 := strconv.ParseFloat(a, 64)
	fb, err2 := strconv.ParseFloat(b, 64)
	if err1 != nil || err2 != nil {
		return a <= b
	}
	return fa <= fb
}

func bearerToken(h string) string {
	h = strings.TrimSpace(h)
	if h == "" {
		return ""
	}
	const pfx = "Bearer "
	if len(h) > len(pfx) && strings.EqualFold(h[:len(pfx)], pfx) {
		return strings.TrimSpace(h[len(pfx):])
	}
	return ""
}
