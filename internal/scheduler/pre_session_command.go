package scheduler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/perrornet/slacksched/internal/session"
)

func runPreSessionCommand(ctx context.Context, command, workspacePath string, key session.Key) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}

	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	cmd.Dir = workspacePath
	cmd.Env = append(os.Environ(),
		"SCHDULER_SESSION_WORKSPACE="+workspacePath,
		"SCHDULER_SESSION_TEAM_ID="+key.TeamID,
		"SCHDULER_SESSION_CHANNEL_ID="+key.ChannelID,
		"SCHDULER_SESSION_ROOT_THREAD_TS="+key.RootThreadTS,
	)

	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		return fmt.Errorf("pre-session command: %w", err)
	}
	return fmt.Errorf("pre-session command: %w: %s", err, msg)
}
