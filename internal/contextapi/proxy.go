package contextapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/perrornet/slacksched/internal/session"
)

// Read-only Web API methods the proxy may call on behalf of a session.
// Keep this list small and auditable.
var allowedSlackWebAPIMethods = map[string]struct{}{
	"conversations.replies": {},
	"conversations.info":    {},
	"conversations.history": {},
	"users.info":            {},
	"users.profile.get":     {},
	"team.info":             {},
	"files.info":            {},
}

var slackMethodName = regexp.MustCompile(`^[a-z][a-z0-9_.]*$`)

const slackAPIBase = "https://slack.com/api/"

// AllowedWebAPIMethods returns sorted names of Slack Web API methods accepted by the read-only proxy (for docs and tooling).
func AllowedWebAPIMethods() []string {
	out := make([]string, 0, len(allowedSlackWebAPIMethods))
	for m := range allowedSlackWebAPIMethods {
		out = append(out, m)
	}
	slices.Sort(out)
	return out
}

// handleSlackWebAPIProxy forwards a whitelisted Slack Web API method using the server-held bot token.
// Path: POST /v1/slack/web-api/<method> e.g. conversations.replies
// Body: application/json object or application/x-www-form-urlencoded (Slack-style params).
func (h *Handler) handleSlackWebAPIProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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
	if strings.TrimSpace(h.botToken) == "" {
		http.Error(w, "slack token not configured", http.StatusServiceUnavailable)
		return
	}

	method, err := slackMethodFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, ok := allowedSlackWebAPIMethods[method]; !ok {
		http.Error(w, "method not allowed", http.StatusForbidden)
		return
	}

	vals, err := parseProxyParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	stripTokenParam(vals)
	if err := enforceProxySessionRules(method, vals, key); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	upstreamURL := slackAPIBase + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, strings.NewReader(vals.Encode()))
	if err != nil {
		if h.log != nil {
			h.log.Warn("slack_proxy_req", "err", err, "method", method)
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+h.botToken)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		if h.log != nil {
			h.log.Warn("slack_proxy_do", "err", err, "slack_method", method)
		}
		http.Error(w, "slack unreachable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if ra := resp.Header.Get("Retry-After"); ra != "" {
		w.Header().Set("Retry-After", ra)
	}
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	if ct := w.Header().Get("Content-Type"); ct == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil && h.log != nil {
		h.log.Warn("slack_proxy_copy", "err", err, "slack_method", method)
	}
}

func slackMethodFromPath(path string) (string, error) {
	const pfx = "/v1/slack/web-api/"
	if !strings.HasPrefix(path, pfx) {
		return "", fmt.Errorf("invalid path")
	}
	method := strings.Trim(strings.TrimPrefix(path, pfx), "/")
	if method == "" || strings.Contains(method, "/") {
		return "", fmt.Errorf("missing slack method in path")
	}
	if !slackMethodName.MatchString(method) {
		return "", fmt.Errorf("invalid slack method name")
	}
	return method, nil
}

func parseProxyParams(r *http.Request) (url.Values, error) {
	ct := r.Header.Get("Content-Type")
	ctBase := strings.TrimSpace(strings.Split(ct, ";")[0])
	switch strings.ToLower(ctBase) {
	case "application/json":
		return parseProxyJSONBody(r.Body)
	default:
		if err := r.ParseForm(); err != nil {
			return nil, err
		}
		cp := make(url.Values)
		for k, vs := range r.PostForm {
			cp[k] = append([]string(nil), vs...)
		}
		return cp, nil
	}
}

func parseProxyJSONBody(body io.Reader) (url.Values, error) {
	const max = 1 << 20
	data, err := io.ReadAll(io.LimitReader(body, max+1))
	if err != nil {
		return nil, err
	}
	if len(data) > max {
		return nil, fmt.Errorf("body too large")
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return url.Values{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw map[string]interface{}
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("invalid json body")
	}
	return mapToURLValues(raw), nil
}

func mapToURLValues(m map[string]interface{}) url.Values {
	vals := make(url.Values)
	for k, v := range m {
		if v == nil {
			continue
		}
		switch x := v.(type) {
		case string:
			vals.Set(k, x)
		case json.Number:
			vals.Set(k, x.String())
		case bool:
			if x {
				vals.Set(k, "true")
			} else {
				vals.Set(k, "false")
			}
		case []interface{}:
			// Slack expects comma-separated lists for some params.
			parts := make([]string, 0, len(x))
			for _, el := range x {
				parts = append(parts, fmt.Sprint(el))
			}
			if len(parts) > 0 {
				vals.Set(k, strings.Join(parts, ","))
			}
		default:
			vals.Set(k, fmt.Sprint(x))
		}
	}
	return vals
}

func stripTokenParam(v url.Values) {
	for k := range v {
		if strings.EqualFold(k, "token") {
			v.Del(k)
		}
	}
}

func enforceProxySessionRules(method string, vals url.Values, key session.Key) error {
	ch := strings.TrimSpace(vals.Get("channel"))
	switch method {
	case "conversations.replies":
		if ch != key.ChannelID {
			return fmt.Errorf("channel must match this session's channel_id")
		}
		if strings.TrimSpace(vals.Get("ts")) != key.RootThreadTS {
			return fmt.Errorf("ts must match this session's root_thread_ts")
		}
	case "conversations.info", "conversations.history":
		if ch != key.ChannelID {
			return fmt.Errorf("channel must match this session's channel_id")
		}
	case "team.info":
		if t := strings.TrimSpace(vals.Get("team")); t != "" && t != key.TeamID {
			return fmt.Errorf("team must match this session's team_id")
		}
	}
	return nil
}
