// Codex app-server: JSON-RPC over stdio (thread/*, turn/*).

package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/perrornet/slacksched/internal/config"
)

type codexAppConn struct {
	log       *slog.Logger
	prof      config.ProviderProfile
	workspace string
	acpTrace  bool

	cmd        *exec.Cmd
	stdin      io.WriteCloser
	readerDone chan struct{}

	mu      sync.Mutex
	nextID  int
	pending map[int]chan rpcResult

	threadID          string
	notificationProto string
	completedTurns    map[string]bool

	outputMu sync.Mutex
	output   strings.Builder

	turnErrMu sync.Mutex
	turnErr   string

	turnWaitMu sync.Mutex
	turnWait   chan turnFinished

	turnSeen atomic.Bool

	// Live Assistant status (same contract as Cursor stream-json): set for each runTurn from context.
	codexPhaseMu sync.Mutex
	streamPhase  func(phase, tool string)
	phaseStack   []codexPhaseFrame // LIFO stack of in-flight items that drive status
}

type codexPhaseFrame struct {
	id     string
	phase  string // "thinking" or "tool_call"
	detail string // tool short id; empty for thinking
}

type turnFinished struct {
	aborted bool
	err     error
}

type rpcResult struct {
	result json.RawMessage
	err    error
}

func startCodexAppServer(ctx context.Context, log *slog.Logger, prof config.ProviderProfile, absWorkspace string, initTO, newSessTO time.Duration, trace bool, extraEnv []string) (*Handle, error) {
	if log == nil {
		log = slog.Default()
	}
	execPath := strings.TrimSpace(prof.Command)
	if execPath == "" {
		execPath = "codex"
	}
	args := buildCodexAppArgs(prof)

	cmd := exec.CommandContext(ctx, execPath, args...)
	cmd.Dir = absWorkspace
	cmd.Env = append(os.Environ(), mergeProviderEnv(prof.Env, extraEnv)...)
	log.Info("codex_app_server command", "exec", execPath, "args", args)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	c := &codexAppConn{
		log: log, prof: prof, workspace: absWorkspace, acpTrace: trace,
		stdin: stdin, pending: make(map[int]chan rpcResult),
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start codex: %w", err)
	}
	c.cmd = cmd

	go c.drainStderr(stderr)

	readerDone := make(chan struct{})
	c.readerDone = readerDone
	go func() {
		defer close(readerDone)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			if c.acpTrace && c.log != nil {
				c.log.Info("codex_rpc", "raw", line)
			}
			c.handleLine(line)
		}
		c.closeAllPending(fmt.Errorf("codex stdout closed"))
	}()

	initCtx, cancel := context.WithTimeout(ctx, initTO)
	defer cancel()
	if _, err := c.request(initCtx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "slacksched",
			"title":   "slacksched",
			"version": "0.1.0",
		},
		"capabilities": map[string]any{"experimentalApi": true},
	}); err != nil {
		_ = c.shutdownProcess()
		<-readerDone
		return nil, fmt.Errorf("codex initialize: %w", err)
	}
	c.notify("initialized")

	tsCtx, cancel2 := context.WithTimeout(ctx, newSessTO)
	defer cancel2()
	res, err := c.request(tsCtx, "thread/start", map[string]any{
		"model":                  nilIfEmpty(prof.Model),
		"modelProvider":          nil,
		"profile":                nil,
		"cwd":                    absWorkspace,
		"approvalPolicy":         nil,
		"sandbox":                nil,
		"config":                 nil,
		"baseInstructions":       nil,
		"developerInstructions":  nil,
		"compactPrompt":          nil,
		"includeApplyPatchTool":  nil,
		"experimentalRawEvents":  false,
		"persistExtendedHistory": true,
	})
	if err != nil {
		_ = c.shutdownProcess()
		<-readerDone
		return nil, fmt.Errorf("codex thread/start: %w", err)
	}
	tid := extractCodexThreadID(res)
	if tid == "" {
		_ = c.shutdownProcess()
		<-readerDone
		return nil, fmt.Errorf("codex thread/start: empty thread id")
	}
	c.threadID = tid

	return &Handle{
		transport: "codex_app_server",
		workspace: absWorkspace,
		log:       log,
		prof:      prof,
		sessionID: tid,
		acpTrace:  trace,
		codex:     c,
	}, nil
}

func buildCodexAppArgs(prof config.ProviderProfile) []string {
	out := []string{"app-server", "--listen", "stdio://"}
	out = append(out, filterCodexListenPair(prof.Args)...)
	return out
}

// filterCodexListenPair strips user-supplied --listen and its value (daemon controls stdio).
func filterCodexListenPair(extra []string) []string {
	var o []string
	for i := 0; i < len(extra); i++ {
		if extra[i] == "--listen" {
			if i+1 < len(extra) {
				i++
			}
			continue
		}
		o = append(o, extra[i])
	}
	return o
}

func extractCodexThreadID(result json.RawMessage) string {
	var r struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(result, &r); err != nil {
		return ""
	}
	return r.Thread.ID
}

func nilIfEmpty(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

func (c *codexAppConn) drainStderr(r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 && c.log != nil {
			c.log.Info("provider stderr", "text", strings.TrimSpace(string(buf[:n])))
		}
		if err != nil {
			return
		}
	}
}

func (c *codexAppConn) notify(method string) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method})
	_, _ = c.stdin.Write(append(b, '\n'))
}

func (c *codexAppConn) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan rpcResult, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	b, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": method, "params": params,
	})
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	b = append(b, '\n')
	if _, err := c.stdin.Write(b); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	select {
	case res := <-ch:
		return res.result, res.err
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (c *codexAppConn) closeAllPending(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		select {
		case ch <- rpcResult{err: err}:
		default:
		}
		delete(c.pending, id)
	}
}

func (c *codexAppConn) handleLine(line string) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return
	}
	if _, hasID := raw["id"]; hasID {
		if _, ok := raw["result"]; ok {
			c.handleResponse(raw)
			return
		}
		if _, ok := raw["error"]; ok {
			c.handleResponse(raw)
			return
		}
		if _, ok := raw["method"]; ok {
			c.handleServerRequest(raw)
			return
		}
	}
	if _, ok := raw["method"]; ok {
		c.handleNotification(raw)
	}
}

func (c *codexAppConn) handleResponse(raw map[string]json.RawMessage) {
	var id int
	_ = json.Unmarshal(raw["id"], &id)
	c.mu.Lock()
	ch, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	if !ok {
		return
	}
	if errB, has := raw["error"]; has {
		var e struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(errB, &e)
		ch <- rpcResult{err: fmt.Errorf("%s (code=%d)", e.Message, e.Code)}
		return
	}
	ch <- rpcResult{result: raw["result"]}
}

func (c *codexAppConn) handleServerRequest(raw map[string]json.RawMessage) {
	var id int
	_ = json.Unmarshal(raw["id"], &id)
	var method string
	_ = json.Unmarshal(raw["method"], &method)
	switch method {
	case "item/commandExecution/requestApproval", "execCommandApproval":
		c.respondOK(id, map[string]any{"decision": "accept"})
	case "item/fileChange/requestApproval", "applyPatchApproval":
		c.respondOK(id, map[string]any{"decision": "accept"})
	case "mcpServer/elicitation/request":
		c.respondOK(id, map[string]any{"action": "accept", "content": nil, "_meta": nil})
	default:
		c.log.Warn("codex: unhandled server request", "method", method, "id", id)
		c.respondErr(id, -32601, "unhandled: "+method)
	}
}

func (c *codexAppConn) respondOK(id int, v any) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": v})
	_, _ = c.stdin.Write(append(b, '\n'))
}

func (c *codexAppConn) respondErr(id int, code int, msg string) {
	b, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": code, "message": msg},
	})
	_, _ = c.stdin.Write(append(b, '\n'))
}

func (c *codexAppConn) handleNotification(raw map[string]json.RawMessage) {
	var method string
	_ = json.Unmarshal(raw["method"], &method)
	var params map[string]any
	if p, ok := raw["params"]; ok {
		_ = json.Unmarshal(p, &params)
	}

	if method == "codex/event" || strings.HasPrefix(method, "codex/event/") {
		c.notificationProto = "legacy"
		msg, _ := params["msg"].(map[string]any)
		if msg != nil {
			c.handleLegacy(msg)
		}
		return
	}

	if c.notificationProto != "legacy" {
		if c.notificationProto == "" && (method == "turn/started" || method == "turn/completed" ||
			strings.HasPrefix(method, "item/")) {
			c.notificationProto = "raw"
		}
		if c.notificationProto == "raw" {
			c.handleRaw(method, params)
		}
	}
}

func (c *codexAppConn) handleLegacy(msg map[string]any) {
	typ, _ := msg["type"].(string)
	switch typ {
	case "agent_message":
		if t, _ := msg["message"].(string); t != "" {
			c.outputMu.Lock()
			c.output.WriteString(t)
			c.outputMu.Unlock()
		}
	case "task_complete":
		c.signalTurn(turnFinished{aborted: false})
	case "turn_aborted":
		c.signalTurn(turnFinished{aborted: true})
	}
}

func (c *codexAppConn) handleRaw(method string, params map[string]any) {
	if tid, ok := params["threadId"].(string); ok && c.threadID != "" && tid != c.threadID {
		return
	}
	switch method {
	case "turn/completed":
		turnID := nestedStr(params, "turn", "id")
		status := nestedStr(params, "turn", "status")
		if status == "failed" {
			em := nestedStr(params, "turn", "error", "message")
			if em == "" {
				em = "codex turn failed"
			}
			c.setTurnErr(em)
		}
		if turnID != "" {
			if c.completedTurns == nil {
				c.completedTurns = map[string]bool{}
			}
			if c.completedTurns[turnID] {
				return
			}
			c.completedTurns[turnID] = true
		}
		aborted := status == "cancelled" || status == "canceled" || status == "aborted" || status == "interrupted"
		c.signalTurn(turnFinished{aborted: aborted})

	case "error":
		willRetry, _ := params["willRetry"].(bool)
		em := nestedStr(params, "error", "message")
		if em == "" {
			em = nestedStr(params, "message")
		}
		if em != "" && !willRetry {
			c.setTurnErr(em)
			c.signalTurn(turnFinished{aborted: false})
		}

	default:
		if !strings.HasPrefix(method, "item/") {
			return
		}
		item, _ := params["item"].(map[string]any)
		if item == nil {
			return
		}
		switch method {
		case "item/started":
			c.codexPhaseItemStarted(item)
		case "item/completed":
			c.codexPhaseItemCompleted(item)
		}
		it, _ := item["type"].(string)
		if method == "item/completed" && it == "agentMessage" {
			tx, _ := item["text"].(string)
			if tx != "" {
				c.outputMu.Lock()
				c.output.WriteString(tx)
				c.outputMu.Unlock()
			}
			if ph, _ := item["phase"].(string); ph == "final_answer" {
				c.signalTurn(turnFinished{aborted: false})
			}
		}
	}
}

func (c *codexAppConn) codexPhaseItemStarted(item map[string]any) {
	c.codexPhaseMu.Lock()
	emit := c.streamPhase
	c.codexPhaseMu.Unlock()
	if emit == nil {
		return
	}
	typ, _ := item["type"].(string)
	phase, detail, track := codexItemPhase(typ, item)
	if !track {
		return
	}
	id := itemStringID(item["id"])
	if id == "" {
		return
	}
	c.codexPhaseMu.Lock()
	c.phaseStack = append(c.phaseStack, codexPhaseFrame{id: id, phase: phase, detail: detail})
	c.codexPhaseMu.Unlock()
	emit(phase, detail)
}

func (c *codexAppConn) codexPhaseItemCompleted(item map[string]any) {
	typ, _ := item["type"].(string)
	if _, _, track := codexItemPhase(typ, item); !track {
		return
	}
	c.codexPhaseMu.Lock()
	emit := c.streamPhase
	if emit == nil {
		c.codexPhaseMu.Unlock()
		return
	}
	id := itemStringID(item["id"])
	if id != "" {
		for i, fr := range c.phaseStack {
			if fr.id == id {
				c.phaseStack = append(c.phaseStack[:i], c.phaseStack[i+1:]...)
				break
			}
		}
	}
	var nextPhase, nextDetail string
	if len(c.phaseStack) == 0 {
		nextPhase = "idle"
	} else {
		top := c.phaseStack[len(c.phaseStack)-1]
		nextPhase, nextDetail = top.phase, top.detail
	}
	c.codexPhaseMu.Unlock()
	if nextPhase == "idle" {
		emit("idle", "")
	} else {
		emit(nextPhase, nextDetail)
	}
}

// codexItemPhase maps Codex app-server item types (item/started payload) to stream phases.
func codexItemPhase(typ string, item map[string]any) (phase, detail string, track bool) {
	switch typ {
	case "reasoning":
		return "thinking", "", true
	case "mcpToolCall":
		t, _ := item["tool"].(string)
		if strings.TrimSpace(t) == "" {
			t = "mcp"
		}
		return "tool_call", sanitizeToolLabel(t), true
	case "commandExecution":
		return "tool_call", "shell", true
	case "fileChange":
		return "tool_call", "patch", true
	case "dynamicToolCall":
		t, _ := item["tool"].(string)
		if strings.TrimSpace(t) == "" {
			t = "tool"
		}
		return "tool_call", sanitizeToolLabel(t), true
	case "webSearch":
		return "tool_call", "search", true
	default:
		return "", "", false
	}
}

func sanitizeToolLabel(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, " ", "_")
	return s
}

func itemStringID(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		return x.String()
	default:
		return fmt.Sprint(x)
	}
}

func nestedStr(m map[string]any, path ...string) string {
	var cur any = m
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = mm[p]
	}
	s, _ := cur.(string)
	return s
}

func (c *codexAppConn) setTurnErr(s string) {
	c.turnErrMu.Lock()
	defer c.turnErrMu.Unlock()
	if c.turnErr == "" {
		c.turnErr = s
	}
}

func (c *codexAppConn) getTurnErr() string {
	c.turnErrMu.Lock()
	defer c.turnErrMu.Unlock()
	return c.turnErr
}

func (c *codexAppConn) signalTurn(tf turnFinished) {
	if !c.turnSeen.CompareAndSwap(false, true) {
		return
	}
	c.turnWaitMu.Lock()
	ch := c.turnWait
	c.turnWaitMu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- tf:
	default:
	}
}

func (c *codexAppConn) runTurn(ctx context.Context, prompt string) (string, string, error) {
	c.turnSeen.Store(false)
	c.turnErrMu.Lock()
	c.turnErr = ""
	c.turnErrMu.Unlock()
	if c.completedTurns != nil {
		for k := range c.completedTurns {
			delete(c.completedTurns, k)
		}
	}

	c.codexPhaseMu.Lock()
	c.streamPhase = streamPhaseCallback(ctx)
	c.phaseStack = nil
	c.codexPhaseMu.Unlock()
	defer func() {
		c.codexPhaseMu.Lock()
		c.streamPhase = nil
		c.phaseStack = nil
		c.codexPhaseMu.Unlock()
	}()

	wait := make(chan turnFinished, 1)
	c.turnWaitMu.Lock()
	c.turnWait = wait
	c.turnWaitMu.Unlock()

	c.outputMu.Lock()
	c.output.Reset()
	c.outputMu.Unlock()

	_, err := c.request(ctx, "turn/start", map[string]any{
		"threadId": c.threadID,
		"input": []map[string]any{
			{"type": "text", "text": prompt},
		},
	})
	if err != nil {
		c.turnWaitMu.Lock()
		c.turnWait = nil
		c.turnWaitMu.Unlock()
		return "", "", err
	}

	var tf turnFinished
	select {
	case tf = <-wait:
	case <-ctx.Done():
		c.turnWaitMu.Lock()
		c.turnWait = nil
		c.turnWaitMu.Unlock()
		return "", "", ctx.Err()
	}
	c.turnWaitMu.Lock()
	c.turnWait = nil
	c.turnWaitMu.Unlock()

	if tf.err != nil {
		return "", "", tf.err
	}
	if tf.aborted {
		return "", "", fmt.Errorf("codex turn aborted")
	}
	if em := c.getTurnErr(); em != "" {
		return "", "", fmt.Errorf("%s", em)
	}
	c.outputMu.Lock()
	out := strings.TrimSpace(c.output.String())
	c.outputMu.Unlock()
	if out == "" {
		out = "(no assistant text captured)"
	}
	return out, "end_turn", nil
}

func (c *codexAppConn) shutdownProcess() error {
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	if c.cmd != nil {
		return c.cmd.Wait()
	}
	return nil
}

func (h *Handle) promptCodexApp(ctx context.Context, userText string) (string, string, error) {
	if h.codex == nil {
		return "", "", fmt.Errorf("codex app not initialized")
	}
	return h.codex.runTurn(ctx, userText)
}

func (h *Handle) closeCodexApp() error {
	if h.codex == nil {
		return nil
	}
	err := h.codex.shutdownProcess()
	if h.codex.readerDone != nil {
		select {
		case <-h.codex.readerDone:
		case <-time.After(30 * time.Second):
		}
	}
	return err
}
