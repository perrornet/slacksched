package messagefilter

import (
	"testing"
	"time"
)

func TestFilterDM(t *testing.T) {
	f := New([]string{"U1"}, nil, true, "BOT", func(_, _, _ string) bool { return false })
	ok, _ := f.ShouldProcess(Input{TeamID: "T", UserID: "U1", ChannelID: "D1", IsIM: true, Text: "hi", MessageTS: "1"})
	if !ok {
		t.Fatal("expected allow DM")
	}
	ok, _ = f.ShouldProcess(Input{TeamID: "T", UserID: "U2", ChannelID: "D1", IsIM: true, Text: "hi", MessageTS: "2"})
	if ok {
		t.Fatal("expected deny")
	}
}

func TestDedupe(t *testing.T) {
	f := New([]string{"U1"}, nil, false, "B", nil)
	f.SetDedupeTTL(10 * time.Second)
	e := Input{TeamID: "T", UserID: "U1", ChannelID: "C", Text: "x", MessageTS: "1", EventID: "E1"}
	if ok, _ := f.ShouldProcess(e); !ok {
		t.Fatal()
	}
	if ok, _ := f.ShouldProcess(e); ok {
		t.Fatal("dup")
	}
}

func TestMentionsUser(t *testing.T) {
	if !mentionsUser("hi <@U1> there", "U1") {
		t.Fatal()
	}
	if !mentionsUser("hi <@U1|bot> there", "U1") {
		t.Fatal("labeled mention")
	}
	if mentionsUser("no", "U1") {
		t.Fatal()
	}
}

func TestBoundThreadRequiresMention(t *testing.T) {
	bound := func(team, ch, root string) bool {
		return team == "T" && ch == "C1" && root == "10.0"
	}
	f := New(nil, []string{"C1"}, true, "BOT", bound)
	ok, reason := f.ShouldProcess(Input{
		TeamID: "T", UserID: "U1", ChannelID: "C1", Text: "no mention",
		ThreadTS: "10.0", MessageTS: "11.0", EventID: "e1",
	})
	if ok || reason != "channel:mention_required" {
		t.Fatalf("got ok=%v reason=%q want mention_required", ok, reason)
	}
	ok, _ = f.ShouldProcess(Input{
		TeamID: "T", UserID: "U1", ChannelID: "C1", Text: "yo <@BOT|x> ok",
		ThreadTS: "10.0", MessageTS: "12.0", EventID: "e2",
	})
	if !ok {
		t.Fatal("expected allow with mention")
	}
}

func TestBoundThreadAllowsSkipMentionWhenNotConfigured(t *testing.T) {
	bound := func(team, ch, root string) bool {
		return team == "T" && ch == "C1" && root == "10.0"
	}
	f := New(nil, []string{"C1"}, false, "BOT", bound)
	ok, _ := f.ShouldProcess(Input{
		TeamID: "T", UserID: "U1", ChannelID: "C1", Text: "no mention",
		ThreadTS: "10.0", MessageTS: "11.0", EventID: "e3",
	})
	if !ok {
		t.Fatal("require_mention false should allow bound thread without @")
	}
}

func TestDedupeMessageTS(t *testing.T) {
	f := New(nil, nil, false, "B", nil)
	f.SetDedupeTTL(10 * time.Second)
	in := Input{TeamID: "T", UserID: "U1", ChannelID: "C1", Text: "x", MessageTS: "99.0", EventID: "E1"}
	if ok, _ := f.ShouldProcess(in); !ok {
		t.Fatal("first")
	}
	in2 := Input{TeamID: "T", UserID: "U1", ChannelID: "C1", Text: "y", MessageTS: "99.0", EventID: "E2"}
	if ok, r := f.ShouldProcess(in2); ok || r != "dedupe:message_ts" {
		t.Fatalf("want dedupe:message_ts, ok=%v r=%q", ok, r)
	}
}
