package notify

import "context"

type TelegramNotifier struct{}

func NewTelegramNotifier() *TelegramNotifier {
	return &TelegramNotifier{}
}

func (n *TelegramNotifier) Alert(_ context.Context, _ string) error {
	return nil
}

func (n *TelegramNotifier) Info(_ context.Context, _ string) error {
	return nil
}
