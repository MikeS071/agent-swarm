package cmd

import (
	"errors"
	"io"
	"os"

	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/spf13/cobra"
)

func NewGoCmd(d *dispatcher.Dispatcher, out io.Writer) *cobra.Command {
	if out == nil {
		out = os.Stdout
	}

	return &cobra.Command{
		Use:   "go [project]",
		Short: "Approve current phase gate",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			sig, _ := d.Evaluate()
			if sig != dispatcher.SignalPhaseGate {
				return errors.New("phase gate not reached")
			}
			sig, spawnable := d.ApprovePhaseGate()
			return printDispatchResult(out, sig, spawnable)
		},
	}
}
