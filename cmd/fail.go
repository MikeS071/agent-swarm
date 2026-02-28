package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/spf13/cobra"
)

func NewFailCmd(d *dispatcher.Dispatcher, out io.Writer) *cobra.Command {
	if out == nil {
		out = os.Stdout
	}

	return &cobra.Command{
		Use:   "fail <ticket>",
		Short: "Mark a ticket as failed",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ticket := args[0]
			if err := d.MarkFailed(ticket); err != nil {
				return err
			}
			_, err := fmt.Fprintf(out, "marked %s failed\n", ticket)
			return err
		},
	}
}
