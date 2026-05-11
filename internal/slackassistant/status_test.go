package slackassistant

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestThreadStatus_Validation(t *testing.T) {
	err := ThreadStatus(context.Background(), nil, "tok", ThreadStatusParams{
		ChannelID: "",
		ThreadTS:  "1.0",
		Status:    "x",
	})
	if err == nil {
		t.Fatal("expected error for empty channel_id")
	}
}

func TestThreadStatus_OK(t *testing.T) {
	prev := setStatusEndpoint
	defer func() { setStatusEndpoint = prev }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer t" {
			t.Error("expected Bearer token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	setStatusEndpoint = srv.URL

	if err := ThreadStatus(context.Background(), nil, "t", ThreadStatusParams{
		ChannelID: "C1",
		ThreadTS:  "1.2",
		Status:    "is thinking",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestThreadStatus_SlackError(t *testing.T) {
	prev := setStatusEndpoint
	defer func() { setStatusEndpoint = prev }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"error":"missing_scope"}`))
	}))
	defer srv.Close()

	setStatusEndpoint = srv.URL

	err := ThreadStatus(context.Background(), nil, "t", ThreadStatusParams{
		ChannelID: "C1",
		ThreadTS:  "1.2",
		Status:    "x",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
