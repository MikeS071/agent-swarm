package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type TelegramNotifier struct {
	Token  string
	ChatID string
	client *http.Client
}

func NewTelegramNotifier(token, chatID string) *TelegramNotifier {
	return &TelegramNotifier{
		Token:  token,
		ChatID: chatID,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *TelegramNotifier) Alert(ctx context.Context, msg string) error {
	return n.send(ctx, "🚨 *Agent-Swarm Watchdog*\n"+msg)
}

func (n *TelegramNotifier) Info(ctx context.Context, msg string) error {
	return n.send(ctx, "🤖 *Agent-Swarm Watchdog*\n"+msg)
}

func (n *TelegramNotifier) send(_ context.Context, text string) error {
	if n.Token == "" || n.ChatID == "" {
		return nil
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.Token)
	body, _ := json.Marshal(map[string]string{
		"chat_id":    n.ChatID,
		"text":       text,
		"parse_mode": "Markdown",
	})
	resp, err := n.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("telegram: status %d", resp.StatusCode)
	}
	return nil
}
