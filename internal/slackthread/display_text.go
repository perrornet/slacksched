package slackthread

import (
	"strings"

	"github.com/slack-go/slack"
)

// DisplayText returns the best human-readable body for prompts and the context API.
// If the top-level msg.Text is empty (common for Block Kit / legacy attachment cards
// from bots such as Opsgenie), text is gathered from blocks and attachments.
func DisplayText(m *slack.Message) string {
	if m == nil {
		return ""
	}
	if t := strings.TrimSpace(m.Text); t != "" {
		return t
	}
	var parts []string
	appendFromBlocks(&m.Msg.Blocks, &parts)
	for i := range m.Msg.Attachments {
		appendAttachmentText(&m.Msg.Attachments[i], &parts)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func appendFromBlocks(blocks *slack.Blocks, parts *[]string) {
	if blocks == nil {
		return
	}
	for _, b := range blocks.BlockSet {
		if b == nil {
			continue
		}
		appendFromBlock(b, parts)
	}
}

func appendFromBlock(b slack.Block, parts *[]string) {
	switch blk := b.(type) {
	case *slack.SectionBlock:
		appendTextObj(blk.Text, parts)
		for _, f := range blk.Fields {
			appendTextObj(f, parts)
		}
	case *slack.HeaderBlock:
		appendTextObj(blk.Text, parts)
	case *slack.ContextBlock:
		for _, e := range blk.ContextElements.Elements {
			switch e.MixedElementType() {
			case slack.MixedElementText:
				if t, ok := e.(*slack.TextBlockObject); ok {
					appendTextObj(t, parts)
				}
			case slack.MixedElementImage:
				if im, ok := e.(*slack.ImageBlockElement); ok {
					addTrimmed(im.AltText, parts)
				}
			}
		}
	case *slack.RichTextBlock:
		for _, e := range blk.Elements {
			appendRichTextElement(e, parts)
		}
	default:
		// divider, image, actions, input, file, video, unknown: no plain text to add
	}
}

func appendTextObj(t *slack.TextBlockObject, parts *[]string) {
	if t == nil {
		return
	}
	addTrimmed(t.Text, parts)
}

func addTrimmed(s string, parts *[]string) {
	s = strings.TrimSpace(s)
	if s != "" {
		*parts = append(*parts, s)
	}
}

func appendAttachmentText(a *slack.Attachment, parts *[]string) {
	if a == nil {
		return
	}
	addTrimmed(a.Pretext, parts)
	title := strings.TrimSpace(a.Title)
	if title != "" {
		if link := strings.TrimSpace(a.TitleLink); link != "" {
			*parts = append(*parts, title+" "+link)
		} else {
			*parts = append(*parts, title)
		}
	}
	addTrimmed(a.Text, parts)
	addTrimmed(a.Fallback, parts)
	for _, f := range a.Fields {
		ft := strings.TrimSpace(f.Title)
		fv := strings.TrimSpace(f.Value)
		switch {
		case ft != "" && fv != "":
			*parts = append(*parts, ft+": "+fv)
		case fv != "":
			*parts = append(*parts, fv)
		case ft != "":
			*parts = append(*parts, ft)
		}
	}
	appendFromBlocks(&a.Blocks, parts)
}

func appendRichTextElement(e slack.RichTextElement, parts *[]string) {
	switch el := e.(type) {
	case *slack.RichTextSection:
		for _, se := range el.Elements {
			appendRichTextSectionElement(se, parts)
		}
	case *slack.RichTextList:
		for _, child := range el.Elements {
			appendRichTextElement(child, parts)
		}
	case *slack.RichTextQuote:
		rts := slack.RichTextSection(*el)
		for _, se := range rts.Elements {
			appendRichTextSectionElement(se, parts)
		}
	case *slack.RichTextPreformatted:
		rts := el.RichTextSection
		for _, se := range rts.Elements {
			appendRichTextSectionElement(se, parts)
		}
	case *slack.RichTextUnknown:
		addTrimmed(el.Raw, parts)
	}
}

func appendRichTextSectionElement(e slack.RichTextSectionElement, parts *[]string) {
	switch el := e.(type) {
	case *slack.RichTextSectionTextElement:
		addTrimmed(el.Text, parts)
	case *slack.RichTextSectionLinkElement:
		if txt := strings.TrimSpace(el.Text); txt != "" {
			*parts = append(*parts, txt)
		} else {
			addTrimmed(el.URL, parts)
		}
	case *slack.RichTextSectionEmojiElement:
		if el.Name != "" {
			*parts = append(*parts, ":"+el.Name+":")
		}
	case *slack.RichTextSectionUserElement:
		if el.UserID != "" {
			*parts = append(*parts, "<@"+el.UserID+">")
		}
	case *slack.RichTextSectionChannelElement:
		if el.ChannelID != "" {
			*parts = append(*parts, "<#"+el.ChannelID+">")
		}
	}
}
