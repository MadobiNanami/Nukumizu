package telegram

import (
	"strings"
	"testing"
)

func TestSplitMessageWithinLimit(t *testing.T) {
	got := splitMessage("short message", maxMessageLen)
	if len(got) != 1 || got[0] != "short message" {
		t.Fatalf("expected single chunk, got %q", got)
	}
}

func TestSplitMessageChunks(t *testing.T) {
	// A message longer than the limit must be split, and the rune content must
	// be preserved exactly when the chunks are re-joined.
	text := strings.Repeat("a", maxMessageLen+100)
	got := splitMessage(text, maxMessageLen)
	if len(got) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(got))
	}
	for _, c := range got {
		if len([]rune(c)) > maxMessageLen {
			t.Fatalf("chunk exceeds %d runes: %d", maxMessageLen, len([]rune(c)))
		}
	}
	if strings.Join(got, "") != text {
		t.Fatalf("chunks do not reconstruct the original text")
	}
}

func TestSplitMessageKeepsLines(t *testing.T) {
	// Splitting must prefer a newline boundary over cutting mid-line.
	text := strings.Repeat("x", maxMessageLen-50) + "\n" + strings.Repeat("y", 200)
	got := splitMessage(text, maxMessageLen)
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(got))
	}
	if strings.HasSuffix(got[0], "\n") == false {
		t.Fatalf("first chunk should end at the newline, got %q...", got[0][len(got[0])-10:])
	}
	if !strings.HasPrefix(got[1], "y") {
		t.Fatalf("second chunk should start after the newline, got %q...", got[1][:10])
	}
}

func TestSplitMessageOverlongLine(t *testing.T) {
	// A single line longer than the limit falls back to a hard rune cut.
	text := strings.Repeat("x", maxMessageLen*2+10)
	got := splitMessage(text, maxMessageLen)
	if strings.Join(got, "") != text {
		t.Fatalf("chunks do not reconstruct the original text")
	}
}

func TestStripCommandBotSuffix(t *testing.T) {
	cases := map[string]string{
		"/list":            "/list",
		"/list@MyBot":      "/list",
		"/list@MyBot arg1": "/list arg1",
		"/status@Bot u1":   "/status u1",
		"not a command":    "not a command",
	}
	for in, want := range cases {
		if got := stripCommandBotSuffix(in); got != want {
			t.Errorf("stripCommandBotSuffix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTelegramChatType(t *testing.T) {
	cases := map[string]string{
		"private":   "private",
		"group":     "group",
		"supergroup": "group",
		"channel":   "",
		"":          "",
	}
	for in, want := range cases {
		if got := telegramChatType(in); got != want {
			t.Errorf("telegramChatType(%q) = %q, want %q", in, got, want)
		}
	}
}
