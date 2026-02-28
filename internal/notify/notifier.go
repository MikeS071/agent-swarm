package notify

import "context"

type Notifier interface {
	Alert(ctx context.Context, msg string) error
	Info(ctx context.Context, msg string) error
}
