package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"

	"github.com/perrornet/slacksched/internal/config"
	"github.com/perrornet/slacksched/internal/contextapi"
	"github.com/perrornet/slacksched/internal/scheduler"
	"github.com/perrornet/slacksched/internal/slackapp"
	"github.com/perrornet/slacksched/internal/workspace"
)

func main() {
	cfgPath := flag.String("config", "configs/example.yaml", "path to YAML config")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	log, closeLog, err := newLogger(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: %v\n", err)
		os.Exit(1)
	}
	if closeLog != nil {
		defer closeLog()
	}

	log.Info("starting", "log_level", cfg.Logging.Level, "acp_trace", cfg.Logging.ACPTrace, "log_file", strings.TrimSpace(cfg.Logging.FilePath))
	botTok, err := cfg.SlackBotToken()
	if err != nil {
		log.Error("slack bot token", "err", err)
		os.Exit(1)
	}
	appTok, err := cfg.SlackAppToken()
	if err != nil {
		log.Error("slack app token", "err", err)
		os.Exit(1)
	}

	api := slack.New(botTok, slack.OptionAppLevelToken(appTok))
	auth, err := api.AuthTest()
	if err != nil {
		log.Error("auth.test", "err", err)
		os.Exit(1)
	}
	log.Info("connected slack app", "team", auth.Team, "user", auth.User)

	sessionBot := workspace.SessionBotIdentity{
		UserID:   auth.UserID,
		BotID:    auth.BotID,
		UserName: strings.TrimSpace(auth.User),
	}
	sessionBot.DisplayName = firstNonEmpty(slackBotProfileName(api, auth.UserID), sessionBot.UserName)
	log.Info("slack bot identity", "bot_user_id", sessionBot.UserID, "slack_bot_id", sessionBot.BotID, "bot_display_name", sessionBot.DisplayName, "bot_username", sessionBot.UserName)

	var ctxReg *contextapi.Registry
	var ctxAPIURL string
	listen := strings.TrimSpace(cfg.Slack.ContextAPIListen)
	if listen != "" {
		ctxReg = contextapi.NewRegistry()
		ln, err := net.Listen("tcp", listen)
		if err != nil {
			log.Error("context api listen", "addr", listen, "err", err)
			os.Exit(1)
		}
		srv := &http.Server{
			Handler:           contextapi.NewHandler(api, ctxReg, log),
			ReadHeaderTimeout: 10 * time.Second,
		}
		go func() {
			if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
				log.Error("context api server", "err", err)
			}
		}()
		ctxAPIURL = "http://" + ln.Addr().String()
		log.Info("slack context api", "listen_cfg", listen, "addr", ln.Addr().String(), "base_url", ctxAPIURL)
	}

	sch, err := scheduler.New(cfg, log, nil, ctxReg, ctxAPIURL, sessionBot)
	if err != nil {
		log.Error("scheduler", "err", err)
		os.Exit(1)
	}

	app := slackapp.New(cfg, log, sch, api, botTok, ctxAPIURL, sessionBot.UserID)
	sm := socketmode.New(api)
	app.SetSocketClient(sm)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := app.Run(ctx); err != nil && err != context.Canceled {
		log.Error("run", "err", err)
		os.Exit(1)
	}
}

func slackBotProfileName(api *slack.Client, userID string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	u, err := api.GetUserInfoContext(ctx, userID)
	if err != nil || u == nil {
		return ""
	}
	for _, s := range []string{
		strings.TrimSpace(u.Profile.DisplayName),
		strings.TrimSpace(u.Profile.RealName),
		strings.TrimSpace(u.RealName),
		strings.TrimSpace(u.Name),
	} {
		if s != "" {
			return s
		}
	}
	return ""
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return strings.TrimSpace(b)
}

// newLogger returns a logger writing to stdout, and optionally duplicating to logging.file_path.
// close must be called on shutdown so the file handle is flushed and closed.
func newLogger(cfg *config.Config) (*slog.Logger, func(), error) {
	opts := &slog.HandlerOptions{Level: cfg.SlogLevel()}
	path := strings.TrimSpace(cfg.Logging.FilePath)
	if path == "" {
		return slog.New(slog.NewTextHandler(os.Stdout, opts)), nil, nil
	}
	if d := filepath.Dir(path); d != "." && d != "" {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, nil, fmt.Errorf("mkdir log dir %s: %w", d, err)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file %s: %w", path, err)
	}
	mw := io.MultiWriter(os.Stdout, f)
	log := slog.New(slog.NewTextHandler(mw, opts))
	return log, func() { _ = f.Close() }, nil
}
