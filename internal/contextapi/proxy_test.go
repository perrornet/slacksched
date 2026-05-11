package contextapi

import (
	"net/url"
	"testing"

	"github.com/perrornet/slacksched/internal/session"
)

func TestSlackMethodFromPath(t *testing.T) {
	m, err := slackMethodFromPath("/v1/slack/web-api/conversations.replies")
	if err != nil || m != "conversations.replies" {
		t.Fatalf("got %q %v", m, err)
	}
	_, err = slackMethodFromPath("/v1/slack/web-api/foo/../replies")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEnforceProxySessionRules(t *testing.T) {
	key := session.Key{TeamID: "T1", ChannelID: "C1", RootThreadTS: "100.000001"}
	ok := url.Values{
		"channel": {"C1"},
		"ts":      {"100.000001"},
	}
	if err := enforceProxySessionRules("conversations.replies", ok, key); err != nil {
		t.Fatal(err)
	}
	badCh := url.Values{"channel": {"CX"}, "ts": {"100.000001"}}
	if enforceProxySessionRules("conversations.replies", badCh, key) == nil {
		t.Fatal("expected error")
	}
	badTs := url.Values{"channel": {"C1"}, "ts": {"other"}}
	if enforceProxySessionRules("conversations.replies", badTs, key) == nil {
		t.Fatal("expected error")
	}
}

func TestStripTokenParam(t *testing.T) {
	v := url.Values{"token": {"xoxb-secret"}, "channel": {"C1"}}
	stripTokenParam(v)
	if v.Get("token") != "" {
		t.Fatalf("token not stripped: %v", v)
	}
	if v.Get("channel") != "C1" {
		t.Fatal("channel lost")
	}
}
