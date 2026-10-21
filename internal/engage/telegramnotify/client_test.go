package telegramnotify

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

// The bot token is part of every Bot API URL, and net/http quotes the URL in a
// transport error. The error text is persisted (subscription_matches.last_error)
// and logged, so it must never carry the token.
func TestClient_TransportErrorOmitsToken(t *testing.T) {
	srv := httptest.NewServer(nil)
	base := srv.URL
	srv.Close() // every request now fails at the transport

	const token = "123456:SECRET-bot-token"
	err := NewClientWithBase(token, base).SendMessage(context.Background(), 1, "hi")
	if err == nil {
		t.Fatal("want a transport error, got nil")
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("error leaks the bot token: %v", err)
	}
	if !strings.Contains(err.Error(), "sendMessage") {
		t.Fatalf("error should still name the method: %v", err)
	}
}
