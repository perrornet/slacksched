package slackapp

import "testing"

func TestStripSelfMentions(t *testing.T) {
	re := newSelfMentionRE("U0ASKQY1YJH")
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"hello", "hello"},
		{"<@U0ASKQY1YJH> hi", " hi"},
		{"see <@U0ASKQY1YJH|bot> ok", "see  ok"},
		{"mixed <@UOTHER> and <@U0ASKQY1YJH>", "mixed <@UOTHER> and "},
	}
	for _, tc := range cases {
		got := stripSelfMentions(re, tc.in)
		if got != tc.want {
			t.Errorf("stripSelfMentions(..., %q) = %q; want %q", tc.in, got, tc.want)
		}
	}
	if stripSelfMentions(nil, "x") != "x" {
		t.Fatal("nil re should noop")
	}
	if newSelfMentionRE("") != nil || newSelfMentionRE("   ") != nil {
		t.Fatal("empty userID should yield nil re")
	}
}
