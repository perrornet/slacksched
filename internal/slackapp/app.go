package slackapp

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"schduler/internal/config"
	"schduler/internal/messagefilter"
	"schduler/internal/scheduler"
	"schduler/internal/session"
	"schduler/internal/slackassistant"
	"schduler/internal/slackmrkdwn"
	"schduler/internal/slackthread"
)

// App wires Socket Mode to the scheduler.
type App struct {
	cfg            *config.Config
	api            *slack.Client
	sm             *socketmode.Client
	log            *slog.Logger
	sch            *scheduler.Scheduler
	filter         *messagefilter.Filter
	botToken       string
	selfMentionRE  *regexp.Regexp
	contextAPIURL  string // set when schduler exposes local Slack Context HTTP API
}

// New builds the Slack app. botToken is the xoxb- token for Web API calls such as assistant.threads.setStatus.
// botUserID is the bot member id from auth.test (used for filtering and stripping self-mentions).
func New(cfg *config.Config, log *slog.Logger, sch *scheduler.Scheduler, api *slack.Client, botToken, contextAPIURL, botUserID string) *App {
	if log == nil {
		log = slog.Default()
	}
	uid := strings.TrimSpace(botUserID)
	f := messagefilter.New(
		cfg.Slack.AllowedDMUserIDs,
		cfg.Slack.AllowedChannelIDs,
		cfg.Slack.RequireMentionInChannels,
		uid,
		sch.IsThreadBound,
	)
	return &App{
		cfg:           cfg,
		api:           api,
		log:           log,
		sch:           sch,
		filter:        f,
		botToken:      strings.TrimSpace(botToken),
		selfMentionRE: newSelfMentionRE(uid),
		contextAPIURL: strings.TrimSpace(contextAPIURL),
	}
}

// Run listens until the Socket Mode client stops.
func (a *App) Run(ctx context.Context) error {
	go func() {
		for evt := range a.sm.Events {
			a.handleSocketEvent(evt)
		}
	}()
	return a.sm.RunContext(ctx)
}

func (a *App) handleSocketEvent(evt socketmode.Event) {
	switch evt.Type {
	case socketmode.EventTypeConnecting:
		a.log.Info("connecting slack socket mode")
	case socketmode.EventTypeConnectionError:
		a.log.Info("slack socket connection error")
	case socketmode.EventTypeConnected:
		a.log.Info("slack socket connected")
	case socketmode.EventTypeEventsAPI:
		ev, ok := evt.Data.(slackevents.EventsAPIEvent)
		if !ok {
			return
		}
		a.sm.Ack(*evt.Request)
		a.handleEventsAPI(ev)
	case socketmode.EventTypeInteractive:
		a.sm.Ack(*evt.Request)
	case socketmode.EventTypeHello:
	default:
	}
}

func (a *App) handleEventsAPI(ev slackevents.EventsAPIEvent) {
	switch slackevents.EventsAPIType(ev.InnerEvent.Type) {
	case slackevents.Message:
		im, ok := ev.InnerEvent.Data.(*slackevents.MessageEvent)
		if !ok {
			return
		}
		a.onMessage(ev, im)
	case slackevents.AppMention:
		im, ok := ev.InnerEvent.Data.(*slackevents.AppMentionEvent)
		if !ok {
			return
		}
		a.onAppMention(ev, im)
	default:
	}
}

func (a *App) onAppMention(api slackevents.EventsAPIEvent, m *slackevents.AppMentionEvent) {
	root := session.RootThread(m.ThreadTimeStamp, m.TimeStamp)
	chType := channelTypeFromID(m.Channel)
	eid := callbackEventID(api)
	in := messagefilter.Input{
		TeamID:        api.TeamID,
		EventID:       eid,
		ClientMsgID:   "",
		UserID:        m.User,
		ChannelID:     m.Channel,
		Text:          m.Text,
		ThreadTS:      m.ThreadTimeStamp,
		MessageTS:     m.TimeStamp,
		Subtype:       "",
		IsIM:          chType == "im",
		UserIsBot:     false,
		MessageHasBot: strings.TrimSpace(m.BotID) != "",
	}
	if ok, why := a.filter.ShouldProcess(in); !ok {
		a.log.Info("slack_inbound_skipped", "reason", why, "channel", m.Channel, "user", m.User, "event_id", eid, "subtype", "app_mention")
		return
	}
	a.dispatch(api.TeamID, m.Channel, root, m.User, strings.TrimSpace(m.Text), eid, m.TimeStamp)
}

func (a *App) onMessage(api slackevents.EventsAPIEvent, m *slackevents.MessageEvent) {
	root := session.RootThread(m.ThreadTimeStamp, m.TimeStamp)
	eid := callbackEventID(api)
	in := messagefilter.Input{
		TeamID:        api.TeamID,
		EventID:       eid,
		ClientMsgID:   m.ClientMsgID,
		UserID:        m.User,
		ChannelID:     m.Channel,
		Text:          m.Text,
		ThreadTS:      m.ThreadTimeStamp,
		MessageTS:     m.TimeStamp,
		Subtype:       m.SubType,
		IsIM:          m.ChannelType == "im",
		UserIsBot:     strings.TrimSpace(m.SubType) == "bot_message",
		MessageHasBot: m.BotID != "",
	}
	if ok, why := a.filter.ShouldProcess(in); !ok {
		a.log.Info("slack_inbound_skipped", "reason", why, "channel", m.Channel, "user", m.User, "event_id", eid, "subtype", m.SubType)
		return
	}
	a.dispatch(api.TeamID, m.Channel, root, m.User, strings.TrimSpace(m.Text), eid, m.TimeStamp)
}

func channelTypeFromID(id string) string {
	if strings.HasPrefix(id, "D") {
		return "im"
	}
	return "channel"
}

func callbackEventID(ev slackevents.EventsAPIEvent) string {
	if cb, ok := ev.Data.(*slackevents.EventsAPICallbackEvent); ok {
		return cb.EventID
	}
	return ""
}

func (a *App) dispatch(teamID, channelID, rootTS, userID, text, eventID, triggerTS string) {
	if text == "" {
		return
	}
	userText := text
	if strings.TrimSpace(a.contextAPIURL) != "" {
		userText = fmt.Sprintf("[Slack 线程上下文 API — 详见本会话工作区 `references/slack-context-api.md`]\nteam_id=%s\nchannel_id=%s\nroot_thread_ts=%s\ntrigger_message_ts=%s\n---\n%s",
			teamID, channelID, rootTS, triggerTS, text)
	}
	promptText := userText
	if a.cfg.Slack.ThreadRepliesInPrompt {
		promptText = slackthread.BuildPromptWithThreadContext(context.Background(), a.api, a.log, channelID, rootTS, triggerTS, userText,
			a.cfg.Slack.ThreadRepliesMaxMessages, a.cfg.Slack.ThreadRepliesMaxChars)
	}
	if a.cfg.Slack.AssistantStatus && a.botToken != "" {
		statusLine := buildLiveStatusLine(defaultAssistantStatusSuffix(&a.cfg.Slack), nil)
		var loading []string
		if a.cfg.Slack.AssistantLiveStatus {
			loading = buildAssistantLoadingCarousel("bootstrap", "")
		} else {
			loading = a.cfg.Slack.AssistantLoadingMessages
		}
		if err := slackassistant.ThreadStatus(context.Background(), nil, a.botToken, slackassistant.ThreadStatusParams{
			ChannelID:       channelID,
			ThreadTS:        rootTS,
			Status:          statusLine,
			LoadingMessages: loading,
		}); err != nil {
			a.log.Warn("assistant_threads_set_status", "err", err, "channel_id", channelID, "root_thread_ts", rootTS)
		}
	}

	inbound := []any{
		"team_id", teamID,
		"channel_id", channelID,
		"user_id", userID,
		"root_thread_ts", rootTS,
		"event_id", eventID,
		"text_len", runeLen(text),
		"thread_replies_in_prompt", a.cfg.Slack.ThreadRepliesInPrompt && promptText != text,
		"provider_prompt_len", runeLen(promptText),
	}
	if a.cfg.Logging.SlackTrace {
		inbound = append(inbound, "text_preview", previewRunes(text, maxSlackTextPreviewRunes))
	}
	a.log.Info("slack_inbound", inbound...)

	var lastPhaseKey string
	var seenTool map[string]bool
	var toolsUsed []string
	var onStreamPhase func(string, string)
	if a.cfg.Slack.AssistantStatus && a.botToken != "" && a.cfg.Slack.AssistantLiveStatus {
		onStreamPhase = func(phase, tool string) {
			tool = strings.TrimSpace(tool)
			if phase == "tool_call" && tool != "" {
				if seenTool == nil {
					seenTool = make(map[string]bool)
				}
				if !seenTool[tool] {
					seenTool[tool] = true
					toolsUsed = append(toolsUsed, tool)
				}
			}
			key := phase + "\x00" + tool
			if key == lastPhaseKey {
				return
			}
			lastPhaseKey = key
			statusLine := buildLiveStatusLine(defaultAssistantStatusSuffix(&a.cfg.Slack), toolsUsed)
			loading := buildAssistantLoadingCarousel(phase, tool)
			if err := slackassistant.ThreadStatus(context.Background(), nil, a.botToken, slackassistant.ThreadStatusParams{
				ChannelID:       channelID,
				ThreadTS:        rootTS,
				Status:          statusLine,
				LoadingMessages: loading,
			}); err != nil {
				a.log.Warn("assistant_threads_live_status", "err", err,
					"phase", phase, "tool", tool, "channel_id", channelID, "root_thread_ts", rootTS)
			}
		}
	}

	done := func(reply string, err error) {
		msg := reply
		if err != nil {
			msg = fmt.Sprintf("抱歉，处理出错（%v）。", err)
		}
		if strings.TrimSpace(msg) == "" {
			msg = "未能生成回复。"
		}
		msg = stripSelfMentions(a.selfMentionRE, msg)
		if a.cfg.Slack.ConvertOutboundMarkdownEnabled() {
			msg = slackmrkdwn.CommonMarkdownToMrkdwn(msg)
		}
		ch, ts, postErr := a.api.PostMessageContext(context.Background(), channelID,
			slack.MsgOptionText(msg, false),
			slack.MsgOptionTS(rootTS),
		)
		out := []any{
			"channel_id", channelID,
			"root_thread_ts", rootTS,
			"event_id", eventID,
			"provider_err", err != nil,
			"outbound_text_len", runeLen(msg),
		}
		if a.cfg.Logging.SlackTrace {
			out = append(out, "outbound_text_preview", previewRunes(msg, maxSlackTextPreviewRunes))
		}
		if postErr != nil {
			if a.cfg.Slack.AssistantStatus && a.botToken != "" {
				if clr := slackassistant.ThreadStatus(context.Background(), nil, a.botToken, slackassistant.ThreadStatusParams{
					ChannelID: channelID,
					ThreadTS:  rootTS,
					Status:    "",
				}); clr != nil {
					a.log.Warn("assistant_threads_clear_status", "err", clr, "channel_id", channelID, "root_thread_ts", rootTS)
				}
			}
			a.log.Info("slack_outbound", append(out, "err", postErr)...)
			return
		}
		if a.cfg.Slack.AssistantStatus && a.botToken != "" {
			if clr := slackassistant.ThreadStatus(context.Background(), nil, a.botToken, slackassistant.ThreadStatusParams{
				ChannelID: channelID,
				ThreadTS:  rootTS,
				Status:    "",
			}); clr != nil {
				a.log.Warn("assistant_threads_clear_status", "err", clr, "channel_id", channelID, "root_thread_ts", rootTS)
			}
		}
		a.log.Info("slack_outbound", append(out, "response_channel", ch, "posted_ts", ts)...)
	}
	a.sch.Enqueue(scheduler.Job{
		Key: session.Key{
			TeamID:       teamID,
			ChannelID:    channelID,
			RootThreadTS: rootTS,
		},
		UserID:        userID,
		Text:          promptText,
		EventID:       eventID,
		Done:          done,
		OnStreamPhase: onStreamPhase,
	})
}

// SetSocketClient attaches Socket Mode client created from api.
func (a *App) SetSocketClient(sm *socketmode.Client) {
	a.sm = sm
}

