package scheduler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/perrornet/slacksched/internal/config"
	"github.com/perrornet/slacksched/internal/contextapi"
	"github.com/perrornet/slacksched/internal/provider"
	"github.com/perrornet/slacksched/internal/session"
	"github.com/perrornet/slacksched/internal/workspace"
)

// Job is one queued Slack user message for a thread.
type Job struct {
	Key    session.Key
	UserID string
	Text   string
	// AfterBootstrapPrompt is the second user turn on a new provider session:
	// raw Slack message text only (same as Text). Ignored after the handshake.
	AfterBootstrapPrompt string
	// SlackContext is written into AGENTS.md before each Prompt (channel/thread metadata and optional transcript).
	SlackContext workspace.SlackRuntimeContext
	EventID      string
	Done         func(text string, err error)
	// OnStreamPhase is optional (assistant_live_status): Cursor stream-json and Codex item/* ; phase is "thinking" | "tool_call" | "idle".
	OnStreamPhase func(phase, tool string)
}

// PromptRunner is a live provider session (implemented by *provider.Handle).
type PromptRunner interface {
	SessionID() string
	Prompt(ctx context.Context, userText string) (text, stopReason string, err error)
	Close() error
}

// Factory constructs provider sessions (overridable in tests).
type Factory interface {
	Start(ctx context.Context, log *slog.Logger, prof config.ProviderProfile, absWorkspace string, extraEnv []string) (PromptRunner, error)
}

type defaultFactory struct {
	cfg *config.Config
}

func (d *defaultFactory) Start(ctx context.Context, log *slog.Logger, prof config.ProviderProfile, absWorkspace string, extraEnv []string) (PromptRunner, error) {
	initTO := 2 * time.Minute
	if d.cfg.Scheduler.PromptTimeout.Duration() < initTO {
		initTO = d.cfg.Scheduler.PromptTimeout.Duration()
	}
	return provider.Start(ctx, log, prof, absWorkspace, initTO, initTO, d.cfg.Logging.ACPTrace, extraEnv...)
}

// Scheduler binds one provider + ACP session per Slack thread key.
type Scheduler struct {
	cfg     *config.Config
	log     *slog.Logger
	profile string
	factory Factory

	ctxReg     *contextapi.Registry
	ctxBaseURL string
	sessionBot workspace.SessionBotIdentity

	mu      sync.Mutex
	workers map[string]*worker
}

// New validates config and builds a scheduler.
// ctxReg and ctxBaseURL enable the on-demand Slack thread HTTP API (empty base URL disables).
// sessionBot is written once into AGENTS.md when a new session workspace is created (UserID non-empty).
// Session constraints and Slack HTTP API docs are generated into AGENTS.md by workspace.CreateSessionWorkspace.
func New(cfg *config.Config, log *slog.Logger, fac Factory, ctxReg *contextapi.Registry, ctxBaseURL string, sessionBot workspace.SessionBotIdentity) (*Scheduler, error) {
	pName, err := cfg.DefaultProviderProfile()
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.Default()
	}
	if fac == nil {
		fac = &defaultFactory{cfg: cfg}
	}
	return &Scheduler{
		cfg:        cfg,
		log:        log,
		profile:    pName,
		factory:    fac,
		ctxReg:     ctxReg,
		ctxBaseURL: strings.TrimSpace(ctxBaseURL),
		sessionBot: sessionBot,
		workers:    make(map[string]*worker),
	}, nil
}

// IsThreadBound reports whether a worker exists for this Slack thread.
func (s *Scheduler) IsThreadBound(teamID, channelID, rootThreadTS string) bool {
	k := session.Key{TeamID: teamID, ChannelID: channelID, RootThreadTS: rootThreadTS}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.workers[k.String()]
	return ok
}

// Enqueue runs work on the dedicated worker for the job key.
func (s *Scheduler) Enqueue(j Job) {
	ks := j.Key.String()
	s.mu.Lock()
	w, ok := s.workers[ks]
	if !ok {
		w = newWorker(s, j.Key)
		s.workers[ks] = w
	}
	s.mu.Unlock()
	w.jobs <- j
}

type worker struct {
	s    *Scheduler
	key  session.Key
	jobs chan Job

	// contextBearerToken is registered with ctxReg for this worker (Context API); empty if disabled.
	contextBearerToken string

	// sessionBootstrapBody is the fixed first user turn (see workspace.BuildSessionBootstrapMarkdown).
	sessionBootstrapBody string
	// sessionBootstrapDone is true after the bootstrap prompt and the first real user message have been sent.
	sessionBootstrapDone bool
}

func newWorker(s *Scheduler, key session.Key) *worker {
	w := &worker{s: s, key: key, jobs: make(chan Job, 256)}
	go w.run()
	return w
}

func (w *worker) applySlackContext(ws string, job Job) error {
	if ws == "" {
		return nil
	}
	agentPath := filepath.Join(ws, w.s.cfg.Scheduler.AgentMDFilename)
	sc := job.SlackContext
	sc.AgentDoc = w.s.cfg.Scheduler.AgentMDFilename
	sc.ContextAPIBaseURL = w.s.ctxBaseURL
	return workspace.ReplaceSlackContextBody(agentPath, sc.BuildMarkdownBody())
}

func (w *worker) remove() {
	w.s.mu.Lock()
	delete(w.s.workers, w.key.String())
	w.s.mu.Unlock()
}

func (w *worker) run() {
	defer w.remove()
	defer func() {
		if w.contextBearerToken != "" && w.s.ctxReg != nil {
			w.s.ctxReg.Unregister(w.contextBearerToken)
		}
	}()

	var prov PromptRunner
	var ws string
	var idle *time.Timer
	prof := w.s.cfg.Providers.Profiles[w.s.profile]

	stopIdle := func() {
		if idle != nil {
			idle.Stop()
			idle = nil
		}
	}
	idleDuration := func() time.Duration {
		a := w.s.cfg.Scheduler.ProviderIdleTimeout.Duration()
		b := w.s.cfg.Scheduler.SessionIdleTimeout.Duration()
		if b < a && b > 0 {
			return b
		}
		return a
	}
	resetIdle := func() {
		stopIdle()
		idle = time.NewTimer(idleDuration())
	}

	for {
		var idleCh <-chan time.Time
		if idle != nil {
			idleCh = idle.C
		}
		select {
		case <-idleCh:
			if prov != nil {
				_ = prov.Close()
				prov = nil
			}
			if ws != "" {
				if err := provider.CleanupWorkspace(w.s.cfg, ws); err != nil {
					w.s.log.Info("workspace cleanup", "err", err, "path", ws)
				}
				ws = ""
			}
			return

		case job := <-w.jobs:
			stopIdle()
			ctx := context.Background()
			if prov == nil {
				suffix := randomSuffix()
				bootRaw := workspace.BuildSessionBootstrapMarkdown()
				path, err := workspace.CreateSessionWorkspace(
					w.s.cfg.Scheduler.WorkspacesRoot,
					w.key.TeamID, w.key.ChannelID, w.key.RootThreadTS,
					suffix,
					w.s.cfg.Scheduler.AgentMDTemplatePath,
					w.s.cfg.Scheduler.AgentMDFilename,
					w.s.cfg.Scheduler.SlackMrkdwnGuidePath,
					w.s.ctxBaseURL,
					w.s.sessionBot,
				)
				if err != nil {
					job.Done("", fmt.Errorf("workspace: %w", err))
					resetIdle()
					continue
				}
				ws = path

				if err := runPreSessionCommand(ctx, w.s.cfg.Scheduler.PreSessionCommand, path, w.key); err != nil {
					_ = provider.CleanupWorkspace(w.s.cfg, ws)
					ws = ""
					job.Done("", err)
					resetIdle()
					continue
				}

				var extraEnv []string
				var bearer string
				if w.s.ctxReg != nil && w.s.ctxBaseURL != "" {
					bearer = contextapi.NewToken()
					w.s.ctxReg.Register(bearer, w.key)
					extraEnv = []string{
						"SCHDULER_CONTEXT_API_URL=" + w.s.ctxBaseURL,
						"SCHDULER_CONTEXT_API_TOKEN=" + bearer,
					}
				}

				h, err := w.s.factory.Start(ctx, w.s.log, prof, path, extraEnv)
				if err != nil {
					if bearer != "" && w.s.ctxReg != nil {
						w.s.ctxReg.Unregister(bearer)
					}
					_ = provider.CleanupWorkspace(w.s.cfg, ws)
					ws = ""
					job.Done("", fmt.Errorf("provider: %w", err))
					resetIdle()
					continue
				}
				prov = h
				w.contextBearerToken = bearer
				w.sessionBootstrapBody = bootRaw
				w.sessionBootstrapDone = false

				w.s.log.Info("provider ready",
					"slack_session_key", w.key.String(),
					"workspace_path", path,
					"acp_session_id", h.SessionID(),
					"slack_event_id", job.EventID,
					"context_api", bearer != "",
					"session_bootstrap", bootRaw != "",
				)
			}
			if ws != "" {
				if err := w.applySlackContext(ws, job); err != nil {
					w.s.log.Info("slack_context_agent_md", "err", err, "workspace_path", ws)
					job.Done("", fmt.Errorf("slack context: %w", err))
					resetIdle()
					continue
				}
			}
			runPrompt := func(pctx context.Context, user string) (string, error) {
				pctx = provider.ContextWithStreamPhaseCallback(pctx, job.OnStreamPhase)
				text, _, err := prov.Prompt(pctx, user)
				return text, err
			}
			pctx, cancel := context.WithTimeout(ctx, w.s.cfg.Scheduler.PromptTimeout.Duration())
			var text string
			var err error
			if w.sessionBootstrapBody != "" && !w.sessionBootstrapDone {
				// First turn: bootstrap only; assistant text is not posted to Slack. Only the second turn is user-visible.
				_, err = runPrompt(pctx, w.sessionBootstrapBody)
				cancel()
				if err != nil {
					w.s.log.Info("session_bootstrap_prompt_failed",
						"slack_session_key", w.key.String(),
						"slack_event_id", job.EventID,
						"err", err,
					)
					job.Done("", err)
					resetIdle()
					continue
				}
				w.sessionBootstrapDone = true
				userTurn := strings.TrimSpace(job.AfterBootstrapPrompt)
				if userTurn == "" {
					userTurn = job.Text
				}
				pctx2, cancel2 := context.WithTimeout(ctx, w.s.cfg.Scheduler.PromptTimeout.Duration())
				text, err = runPrompt(pctx2, userTurn)
				cancel2()
			} else {
				text, err = runPrompt(pctx, job.Text)
				cancel()
			}
			if err != nil {
				w.s.log.Info("prompt failed",
					"slack_session_key", w.key.String(),
					"slack_event_id", job.EventID,
					"err", err,
				)
				job.Done("", err)
			} else {
				job.Done(text, nil)
			}
			resetIdle()
		}
	}
}

func randomSuffix() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
