package notify

import (
	"context"
	"fmt"
	"io"
	"os"
)

const (
	colorRed   = "\033[31m"
	colorCyan  = "\033[36m"
	colorReset = "\033[0m"
)

type StdoutNotifier struct {
	out io.Writer
}

func NewStdoutNotifier(w io.Writer) *StdoutNotifier {
	if w == nil {
		w = os.Stdout
	}
	return &StdoutNotifier{out: w}
}

func (n *StdoutNotifier) Alert(_ context.Context, msg string) error {
	_, err := fmt.Fprintf(n.out, "%s[ALERT]%s %s\n", colorRed, colorReset, msg)
	return err
}

func (n *StdoutNotifier) Info(_ context.Context, msg string) error {
	_, err := fmt.Fprintf(n.out, "%s[INFO]%s %s\n", colorCyan, colorReset, msg)
	return err
}
