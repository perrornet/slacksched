package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/perrornet/slacksched/internal/acp"
	"github.com/perrornet/slacksched/internal/config"
	"github.com/perrornet/slacksched/internal/finalanswer"
	"github.com/perrornet/slacksched/internal/workspace"
)

// Handle is a running provider session (ACP long-lived process, or cursor_cli state).
type Handle struct {
	cmd       *exec.Cmd
	collector *finalanswer.Collector
	client    *acp.Client
	sessionID string // ACP session id, or Cursor CLI session_id for --resume
	workspace string
	log       *slog.Logger

	transport string // "" or acp | cursor_cli | codex_app_server
	prof        config.ProviderProfile
	acpTrace    bool
	codex       *codexAppConn // set when transport is codex_app_server

	extraEnv []string // appended to provider subprocess (Cursor CLI per run; ACP/Codex at start)

	stderrWg sync.WaitGroup
}

// Start launches the provider (ACP stdio session or cursor_cli placeholder).
// If acpTrace is true, every JSON-RPC line (ACP) or stream-json line (Cursor) is logged when applicable.
// extraEnv entries must be well-formed KEY=value pairs (e.g. SCHDULER_CONTEXT_API_URL=...).
func Start(ctx context.Context, log *slog.Logger, prof config.ProviderProfile, absWorkspace string, initTimeout, newSessionTimeout time.Duration, acpTrace bool, extraEnv ...string) (*Handle, error) {
	t := strings.TrimSpace(prof.Transport)
	if t == "" {
		t = "acp"
	}
	if t == "cursor_cli" {
		return startCursorCLI(log, prof, absWorkspace, acpTrace, extraEnv)
	}
	if t == "codex_app_server" {
		return startCodexAppServer(ctx, log, prof, absWorkspace, initTimeout, newSessionTimeout, acpTrace, extraEnv)
	}

	cmd := exec.Command(prof.Command, prof.Args...)
	cmd.Env = append(os.Environ(), mergeProviderEnv(prof.Env, extraEnv)...)
	cmd.Dir = absWorkspace
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start provider: %w", err)
	}
	if log == nil {
		log = slog.Default()
	}
	h := &Handle{
		collector: &finalanswer.Collector{},
		workspace: absWorkspace,
		log:       log,
		cmd:       cmd,
		transport: "acp",
		prof:      prof,
		acpTrace:  acpTrace,
		extraEnv:  append([]string(nil), extraEnv...),
	}
	h.stderrWg.Add(1)
	go func() {
		defer h.stderrWg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				log.Info("provider stderr", "text", strings.TrimSpace(string(buf[:n])))
			}
			if err != nil {
				return
			}
		}
	}()

	col := h.collector
	notify := func(method string, params json.RawMessage) {
		if method == acp.MethodNotifyUpdate {
			col.OnSessionUpdateJSON(params)
		}
	}
	h.client = acp.NewClient(stdin, stdout, notify, workspaceServerHandler(absWorkspace, log), acp.WithACPTrace(log, acpTrace))

	initCtx, cancel := context.WithTimeout(ctx, initTimeout)
	defer cancel()
	initParams := acp.InitializeParams{
		ProtocolVersion:    "1",
		ClientCapabilities: json.RawMessage(`{"fs":{"readTextFile":true,"writeTextFile":true},"terminal":false}`),
		ClientInfo: &acp.Implementation{
			Name:    "slacksched",
			Version: "0.1.0",
		},
	}
	var initRes acp.InitializeResult
	if err := h.client.Call(initCtx, acp.MethodInitialize, initParams, &initRes); err != nil {
		_ = h.Close()
		return nil, fmt.Errorf("acp initialize: %w", err)
	}
	h.log.Info("acp initialized", "protocol", initRes.ProtocolVersion)

	nsCtx, cancel2 := context.WithTimeout(ctx, newSessionTimeout)
	defer cancel2()
	nsParams := acp.NewSessionParams{
		Cwd:        absWorkspace,
		McpServers: json.RawMessage(`[]`),
	}
	var nsRes acp.NewSessionResult
	if err := h.client.Call(nsCtx, acp.MethodSessionNew, nsParams, &nsRes); err != nil {
		_ = h.Close()
		return nil, fmt.Errorf("session/new: %w", err)
	}
	h.sessionID = nsRes.SessionID
	h.log.Info("acp session", "session_id", h.sessionID)
	return h, nil
}

// Prompt runs one user turn; returns visible assistant text.
func (h *Handle) Prompt(ctx context.Context, userText string) (string, string, error) {
	if h.transport == "cursor_cli" {
		return h.promptCursorCLI(ctx, userText)
	}
	if h.transport == "codex_app_server" || h.codex != nil {
		return h.promptCodexApp(ctx, userText)
	}
	h.collector.Reset()
	params := acp.PromptParams{
		SessionID: h.sessionID,
		Prompt:    acp.TextBlock(userText),
	}
	var res acp.PromptResult
	if err := h.client.Call(ctx, acp.MethodSessionPrompt, params, &res); err != nil {
		return "", "", err
	}
	text := strings.TrimSpace(h.collector.Text())
	if text == "" {
		text = finalanswer.FallbackMessage(res.StopReason)
	}
	return text, res.StopReason, nil
}

// SessionID returns ACP session id.
func (h *Handle) SessionID() string { return h.sessionID }

// WorkspacePath returns on-disk workspace.
func (h *Handle) WorkspacePath() string { return h.workspace }

// Close stops the provider process.
func (h *Handle) Close() error {
	if h.transport == "cursor_cli" {
		return nil
	}
	if h.codex != nil {
		return h.closeCodexApp()
	}
	if h.client != nil {
		h.client.ClosePending()
	}
	var err error
	if h.cmd != nil && h.cmd.Process != nil {
		_ = h.cmd.Process.Kill()
		err = h.cmd.Wait()
	}
	h.stderrWg.Wait()
	if h.client != nil {
		h.client.WaitReadLoop()
	}
	if err != nil && strings.Contains(err.Error(), "signal") {
		return nil
	}
	return err
}

func pathUnderRoot(root, p string) bool {
	if root == "" || p == "" {
		return false
	}
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func envPairs(m map[string]string) []string {
	var out []string
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

func mergeProviderEnv(prof map[string]string, extra []string) []string {
	out := envPairs(prof)
	if len(extra) == 0 {
		return out
	}
	return append(out, extra...)
}

func workspaceServerHandler(root string, log *slog.Logger) acp.ServerHandler {
	root = filepath.Clean(root)
	return func(ctx context.Context, id int64, method string, params json.RawMessage) (json.RawMessage, error) {
		switch method {
		case "fs/read_text_file":
			var req struct {
				Path      string `json:"path"`
				SessionID string `json:"sessionId"`
				_         struct{}
			}
			if err := json.Unmarshal(params, &req); err != nil {
				return nil, err
			}
			path := filepath.Clean(req.Path)
			if !pathUnderRoot(root, path) {
				return nil, fmt.Errorf("path outside workspace")
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			type res struct {
				Text string `json:"text"`
			}
			out, _ := json.Marshal(res{Text: string(b)})
			return out, nil

		case "fs/write_text_file":
			var req struct {
				Path      string `json:"path"`
				Text      string `json:"text"`
				SessionID string `json:"sessionId"`
			}
			if err := json.Unmarshal(params, &req); err != nil {
				return nil, err
			}
			path := filepath.Clean(req.Path)
			if !pathUnderRoot(root, path) {
				return nil, fmt.Errorf("path outside workspace")
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(path, []byte(req.Text), 0o644); err != nil {
				return nil, err
			}
			return json.RawMessage(`{}`), nil

		case "session/request_permission":
			var req struct {
				PermissionOptions []struct {
					ID string `json:"id"`
				} `json:"permissionOptions"`
			}
			_ = json.Unmarshal(params, &req)
			optID := ""
			if len(req.PermissionOptions) > 0 {
				optID = req.PermissionOptions[0].ID
			}
			if log != nil {
				log.Debug("auto-selected permission option", "option_id", optID, "rpc_id", id)
			}
			// RequestPermissionOutcome::selected { optionId }
			b, _ := json.Marshal(map[string]any{
				"outcome": map[string]any{
					"type":     "selected",
					"optionId": optID,
				},
			})
			return b, nil

		default:
			return nil, fmt.Errorf("unsupported client method %s", method)
		}
	}
}

// CleanupWorkspace removes or archives the workspace per config.
func CleanupWorkspace(cfg *config.Config, path string) error {
	switch cfg.Scheduler.WorkspaceRetention {
	case "archive_on_session_close":
		ar := filepath.Join(filepath.Clean(cfg.Scheduler.WorkspacesRoot), "..", "archive")
		if err := os.MkdirAll(ar, 0o755); err != nil {
			return err
		}
		return workspace.Archive(path, ar)
	default:
		return workspace.RemoveAll(path)
	}
}

// DrainReader optionally drains an io.Reader (used in tests).
func DrainReader(r io.Reader) {
	go func() {
		_, _ = io.Copy(io.Discard, r)
	}()
}
