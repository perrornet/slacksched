package provider

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/perrornet/slacksched/internal/config"
)

func startCursorCLI(log *slog.Logger, prof config.ProviderProfile, absWorkspace string, acpTrace bool, extraEnv []string) (*Handle, error) {
	if log == nil {
		log = slog.Default()
	}
	return &Handle{
		transport: "cursor_cli",
		workspace: absWorkspace,
		log:       log,
		prof:      prof,
		acpTrace:  acpTrace,
		sessionID: "",
		extraEnv:  append([]string(nil), extraEnv...),
	}, nil
}

func (h *Handle) promptCursorCLI(ctx context.Context, userText string) (string, string, error) {
	model := strings.TrimSpace(h.prof.Model)
	args := buildCursorCLIArgs(userText, h.workspace, h.sessionID, model, h.prof.Args, h.log)
	if h.log != nil {
		h.log.Info("cursor_cli command", "exec", h.prof.Command, "args", args)
	}
	cmd := exec.CommandContext(ctx, h.prof.Command, args...)
	cmd.Dir = h.workspace
	cmd.Env = append(os.Environ(), mergeProviderEnv(h.prof.Env, h.extraEnv)...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", "", err
	}

	var stderrWg sync.WaitGroup
	stderrWg.Add(1)
	go func() {
		defer stderrWg.Done()
		buf := make([]byte, 4096)
		for {
			n, e := stderr.Read(buf)
			if n > 0 && h.log != nil {
				h.log.Info("provider stderr", "text", strings.TrimSpace(string(buf[:n])))
			}
			if e != nil {
				return
			}
		}
	}()

	if err := cmd.Start(); err != nil {
		stderrWg.Wait()
		return "", "", fmt.Errorf("start cursor-agent: %w", err)
	}

	finalText, newSID, asstBuf, rerr := readCursorStreamJSON(ctx, stdout, h.log, h.acpTrace)
	stderrWg.Wait()
	waitErr := cmd.Wait()
	if rerr != nil {
		return "", "", rerr
	}
	if waitErr != nil {
		return "", "", fmt.Errorf("cursor-agent: %w", waitErr)
	}
	if strings.TrimSpace(newSID) != "" {
		h.sessionID = newSID
	}
	out := strings.TrimSpace(finalText)
	if out == "" {
		out = strings.TrimSpace(asstBuf)
	}
	if out == "" {
		out = "(no text in result)"
	}
	return out, "end_turn", nil
}
