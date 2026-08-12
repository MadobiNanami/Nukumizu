package telegram

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"nukumizu-backend/internal/netproxy"
	"nukumizu-backend/postLog"
)

const apiBase = "https://api.telegram.org/bot"

// User mirrors a Telegram user.
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

// Chat mirrors a Telegram chat.
type Chat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"` // "private", "group", "supergroup", "channel"
	Title string `json:"title"`
}

// Message mirrors a Telegram message. Only the fields used for command
// handling are modeled.
type Message struct {
	MessageID int64           `json:"message_id"`
	From      *User           `json:"from"`
	Chat      Chat            `json:"chat"`
	Date      int64           `json:"date"`
	Text      string          `json:"text"`
	Entities  []MessageEntity `json:"entities"`
}

// MessageEntity mirrors a Telegram message entity (bot_command, mention, ...).
type MessageEntity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
}

// Update mirrors a Telegram update.
type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

// apiResponse is the Telegram Bot API response envelope.
type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
}

// maxMessageLen is the safe chunk size for outbound messages. Telegram's hard
// limit is 4096 characters; staying under it leaves headroom for encoding.
const maxMessageLen = 4000

// pollTimeout and pollLimit are the long-polling parameters passed to
// getUpdates. The server holds the request open for pollTimeout seconds, so the
// HTTP client timeout below must exceed it.
const (
	pollTimeout = 30
	pollLimit   = 100
)

// retryDelay is the pause between getUpdates attempts after an error.
const retryDelay = 5 * time.Second

// Client is a thin Telegram Bot API client. It both polls for incoming updates
// (long polling) and issues outbound Bot API calls (sendMessage).
type Client struct {
	token      string
	useProxy   bool
	httpClient *http.Client

	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewClient creates a Telegram Bot API client. When useProxy is set, all API
// calls are routed through the system-wide network proxy.
func NewClient(token string, useProxy bool) *Client {
	return &Client{
		token:      token,
		useProxy:   useProxy,
		httpClient: netproxy.HTTPClient(useProxy, 5*time.Minute),
		stopCh:     make(chan struct{}),
	}
}

func (c *Client) baseURL() string {
	return apiBase + c.token
}

// call performs an API call and unmarshals the result field into result (when
// non-nil). Non-OK responses are wrapped as errors, so the 409 Conflict body
// raised when another poller uses the same token surfaces to the caller.
func (c *Client) call(method string, params url.Values, result interface{}) error {
	endpoint := c.baseURL() + "/" + method

	var req *http.Request
	var err error
	if len(params) > 0 {
		req, err = http.NewRequest(http.MethodPost, endpoint, strings.NewReader(params.Encode()))
		if err != nil {
			return fmt.Errorf("failed to create telegram request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req, err = http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return fmt.Errorf("failed to create telegram request: %w", err)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("telegram API %s failed: %w", method, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read telegram response: %w", err)
	}

	var apiResp apiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("failed to unmarshal telegram response (%s): %w\n%s", method, err, string(body))
	}
	if !apiResp.OK {
		return fmt.Errorf("telegram API %s error: %d %s", method, apiResp.ErrorCode, apiResp.Description)
	}

	if result != nil {
		if err := json.Unmarshal(apiResp.Result, result); err != nil {
			return fmt.Errorf("failed to unmarshal telegram %s result: %w", method, err)
		}
	}

	return nil
}

// GetMe verifies the bot token and returns the bot's own user object.
func (c *Client) GetMe() (*User, error) {
	var u User
	if err := c.call("getMe", nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// getUpdates fetches incoming updates. offset is the first update to return;
// pass the previous last update_id + 1 to acknowledge the received updates.
func (c *Client) getUpdates(offset int64, limit, timeout int) ([]Update, error) {
	params := url.Values{
		"limit":   {strconv.Itoa(limit)},
		"timeout": {strconv.Itoa(timeout)},
	}
	if offset > 0 {
		params.Set("offset", strconv.FormatInt(offset, 10))
	}

	var updates []Update
	if err := c.call("getUpdates", params, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

// SendMessage sends a text message to a chat, splitting it into chunks that
// fit Telegram's 4096-character limit. The text is sent without a parse mode so
// template asterisks render literally.
func (c *Client) SendMessage(chatID int64, text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	for _, chunk := range splitMessage(text, maxMessageLen) {
		if err := c.sendMessageChunk(chatID, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) sendMessageChunk(chatID int64, text string) error {
	params := url.Values{
		"chat_id": {strconv.FormatInt(chatID, 10)},
		"text":    {text},
	}
	return c.call("sendMessage", params, nil)
}

// Listen runs the long-polling loop, calling onUpdate for every incoming
// update. It blocks until Stop is called, retrying on errors (e.g. the 409
// Conflict raised when another poller uses the same token). Run it in a
// goroutine.
func (c *Client) Listen(onUpdate func(Update)) {
	var offset int64 // Next getUpdates offset: previous last update_id + 1.

	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		updates, err := c.getUpdates(offset, pollLimit, pollTimeout)
		if err != nil {
			if strings.Contains(err.Error(), "409") || strings.Contains(err.Error(), "Conflict") {
				postLog.Error("Telegram getUpdates conflict (409): another poller is using the same bot token. Ensure only one instance is polling.")
			} else {
				postLog.Warning("Telegram getUpdates failed: " + err.Error())
			}

			select {
			case <-c.stopCh:
				return
			case <-time.After(retryDelay):
			}
			continue
		}

		for _, u := range updates {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			onUpdate(u)
		}
	}
}

// Stop closes the stop channel and unblocks the Listen loop.
func (c *Client) Stop() {
	c.stopOnce.Do(func() { close(c.stopCh) })
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
