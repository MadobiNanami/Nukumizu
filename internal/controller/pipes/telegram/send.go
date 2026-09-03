package telegram

import (
	"context"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"nukumizu-backend/internal/controller"
)

// maxMessageLen is the safe chunk size for outbound messages. Telegram's hard
// limit is 4096 characters; staying under it leaves headroom for encoding.
const maxMessageLen = 4000

// sendMessage sends a text message to a chat, splitting it into chunks that fit
// Telegram's 4096-character limit. All messages are sent with
// parse_mode=Markdown so fenced code blocks and inline formatting render as
// rich text. Templates must stay valid under Telegram's legacy Markdown:
// unpaired '*' or '_' characters (e.g. a lone '*Event: ...' label) make the
// API reject the whole message.
func (t *TelegramController) sendMessage(message controller.Message) error {
	if t.client == nil {
		return nil
	}
	if strings.TrimSpace(message.Content) == "" {
		return nil
	}

	for _, chunk := range splitMessage(message.Content, maxMessageLen) {
		if err := t.sendMessageChunk(message.ChatID, chunk); err != nil {
			return err
		}
	}
	return nil
}

// sendMessageChunk issues a single sendMessage API call for one chunk.
func (t *TelegramController) sendMessageChunk(chatID int64, text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
	defer cancel()

	_, err := t.client.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeMarkdownV1, // Telegram legacy Markdown
	})
	return err
}

// splitMessage splits text into chunks of at most maxLen runes, keeping
// complete lines when possible.
func splitMessage(text string, maxLen int) []string {
	if maxLen <= 0 {
		return []string{text}
	}

	remaining := []rune(text)
	if len(remaining) <= maxLen {
		return []string{text}
	}

	var chunks []string
	for len(remaining) > maxLen {
		// Prefer the last newline within the window so messages aren't cut
		// mid-line; fall back to a hard rune cut for overlong lines.
		cut := maxLen
		if nl := lastIndexRune(remaining[:maxLen], '\n'); nl > 0 {
			cut = nl + 1
		}
		chunks = append(chunks, string(remaining[:cut]))
		remaining = remaining[cut:]
	}
	if len(remaining) > 0 {
		chunks = append(chunks, string(remaining))
	}
	return chunks
}

func lastIndexRune(s []rune, r rune) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == r {
			return i
		}
	}
	return -1
}
