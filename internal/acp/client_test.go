package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestClientRoundTripAndNotify(t *testing.T) {
	pc, ps := io.Pipe() // client writes to ps, server reads pc
	sc, ss := io.Pipe() // server writes ss, client reads sc

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		rd := bufio.NewReader(pc)
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var req struct {
				ID     int64           `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if err := json.Unmarshal([]byte(line), &req); err != nil {
				continue
			}
			if req.Method == "" {
				continue
			}
			switch req.Method {
			case "initialize":
				_, _ = ss.Write([]byte(`{"jsonrpc":"2.0","id":` + jsonNum(req.ID) + `,"result":{"protocolVersion":"1","agentCapabilities":{}}}` + "\n"))
			case "session/new":
				_, _ = ss.Write([]byte(`{"jsonrpc":"2.0","id":` + jsonNum(req.ID) + `,"result":{"sessionId":"sess1"}}` + "\n"))
			case "session/prompt":
				_, _ = ss.Write([]byte(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"sess1","sessionUpdate":{"type":"agent_message_chunk","content":{"type":"text","text":"hi"}}}}` + "\n"))
				_, _ = ss.Write([]byte(`{"jsonrpc":"2.0","id":` + jsonNum(req.ID) + `,"result":{"stopReason":"end_turn"}}` + "\n"))
			default:
				_, _ = ss.Write([]byte(`{"jsonrpc":"2.0","id":` + jsonNum(req.ID) + `,"error":{"code":-32601,"message":"no"}}` + "\n"))
			}
		}
	}()

	var gotNotify int
	cli := NewClient(ps, sc, func(method string, params json.RawMessage) {
		if method == MethodNotifyUpdate {
			gotNotify++
		}
	}, nil)
	ctx := context.Background()
	if err := cli.Call(ctx, MethodInitialize, InitializeParams{ProtocolVersion: "1", ClientCapabilities: json.RawMessage(`{}`)}, nil); err != nil {
		t.Fatal(err)
	}
	var ns NewSessionResult
	if err := cli.Call(ctx, MethodSessionNew, NewSessionParams{Cwd: "/tmp", McpServers: json.RawMessage(`[]`)}, &ns); err != nil {
		t.Fatal(err)
	}
	if ns.SessionID != "sess1" {
		t.Fatal(ns.SessionID)
	}
	var pr PromptResult
	if err := cli.Call(ctx, MethodSessionPrompt, PromptParams{SessionID: ns.SessionID, Prompt: TextBlock("yo")}, &pr); err != nil {
		t.Fatal(err)
	}
	if pr.StopReason != "end_turn" {
		t.Fatal(pr.StopReason)
	}
	if gotNotify != 1 {
		t.Fatalf("notifies %d", gotNotify)
	}
	_ = ps.Close()
	_ = sc.Close()
	wg.Wait()
	cli.WaitReadLoop()
}

func jsonNum(id int64) string {
	b, _ := json.Marshal(id)
	return string(b)
}
