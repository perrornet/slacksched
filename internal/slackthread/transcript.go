// Package slackthread fetches and formats Slack thread history for prompts and the context API.
package slackthread

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/slack-go/slack"
)

const (
	DefaultMaxMessages = 100
	DefaultMaxChars    = 12000
)

// CollectReplies returns thread messages oldest-first (Slack API order), up to maxMsgs (clamped 1..200 per request batch internally).
func CollectReplies(ctx context.Context, api *slack.Client, channelID, rootTS string, maxMsgs int) ([]slack.Message, error) {
	if maxMsgs <= 0 {
		maxMsgs = DefaultMaxMessages
	}
	var collected []slack.Message
	cursor := ""
	for len(collected) < maxMsgs {
		batch := maxMsgs - len(collected)
		if batch > 200 {
			batch = 200
		}
		msgs, hasMore, next, err := api.GetConversationRepliesContext(ctx, &slack.GetConversationRepliesParameters{
			ChannelID: channelID,
			Timestamp: rootTS,
			Limit:     batch,
			Cursor:    cursor,
			Inclusive: true,
		})
		if err != nil {
			return nil, err
		}
		collected = append(collected, msgs...)
		if !hasMore || next == "" {
			break
		}
		cursor = next
		if len(msgs) == 0 {
			break
		}
	}
	if len(collected) > maxMsgs {
		collected = collected[:maxMsgs]
	}
	return collected, nil
}

// BuildPromptWithThreadContext prepends prior thread lines to currentText (excluding triggerTS line), capped by maxChars.
// On failure or empty history, returns currentText unchanged.
func BuildPromptWithThreadContext(ctx context.Context, api *slack.Client, log *slog.Logger, channelID, rootTS, triggerTS, currentText string, maxMsgs, maxChars int) string {
	transcript, err := TranscriptExcluding(ctx, api, channelID, rootTS, triggerTS, maxMsgs, maxChars)
	if err != nil {
		if log != nil {
			log.Warn("slack_thread_replies", "err", err, "channel_id", channelID, "root_thread_ts", rootTS)
		}
		return currentText
	}
	if strings.TrimSpace(transcript) == "" {
		return currentText
	}
	var b strings.Builder
	b.WriteString("[Slack 线程 — 以下为先前消息，来源 conversations.replies]\n")
	b.WriteString(transcript)
	b.WriteString("\n---\n本条待回答的消息：\n")
	b.WriteString(currentText)
	return b.String()
}

// TranscriptExcluding formats thread messages except the one with triggerTS, obeying char budget.
func TranscriptExcluding(ctx context.Context, api *slack.Client, channelID, rootTS, triggerTS string, maxMsgs, maxChars int) (string, error) {
	if maxChars <= 0 {
		maxChars = DefaultMaxChars
	}
	collected, err := CollectReplies(ctx, api, channelID, rootTS, maxMsgs)
	if err != nil {
		return "", err
	}
	var lines []string
	charBudget := maxChars
	for _, m := range collected {
		if strings.TrimSpace(m.Timestamp) == strings.TrimSpace(triggerTS) {
			continue
		}
		line := FormatLine(&m)
		if line == "" {
			continue
		}
		if len(line)+1 > charBudget {
			lines = append(lines, "…（已截断：超过字数上限）")
			break
		}
		lines = append(lines, line)
		charBudget -= len(line) + 1
	}
	return strings.Join(lines, "\n"), nil
}

// FormatLine turns a Slack message into a single log-style line.
func FormatLine(m *slack.Message) string {
	if m == nil {
		return ""
	}
	st := strings.TrimSpace(m.SubType)
	if st != "" && st != "bot_message" && m.Text == "" {
		return ""
	}
	t := strings.TrimSpace(m.Text)
	if t == "" {
		return ""
	}
	speaker := m.User
	if speaker == "" && m.BotID != "" {
		speaker = "机器人:" + m.BotID
	}
	if speaker == "" {
		speaker = "?"
	}
	return fmt.Sprintf("[时间戳 %s] %s: %s", m.Timestamp, speaker, t)
}
