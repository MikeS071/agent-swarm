package cmd

import (
	"fmt"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"io"
	"os"
	"strings"

	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/spf13/cobra"
)

func NewDoneCmd(d *dispatcher.Dispatcher, out io.Writer) *cobra.Command {
	if out == nil {
		out = os.Stdout
	}

	cmd := &cobra.Command{
		Use:   "done <ticket> [sha]",
		Short: "Mark a ticket as done",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(_ *cobra.Command, args []string) error {
			ticket := args[0]
			sha := ""
			if len(args) == 2 {
				sha = args[1]
			}

			sig, spawnable := d.MarkDone(ticket, sha)
			fmt.Fprintf(out, "marked %s done\n", ticket)
			return printDispatchResult(out, sig, spawnable)
		},
	}

	return cmd
}

func printDispatchResult(out io.Writer, sig dispatcher.Signal, spawnable []string) error {
	switch sig {
	case dispatcher.SignalSpawn:
		if len(spawnable) == 0 {
			_, err := fmt.Fprintln(out, "no spawnable tickets")
			return err
		}
		_, err := fmt.Fprintf(out, "WOULD spawn: %s\n", strings.Join(spawnable, ", "))
		return err
	case dispatcher.SignalPhaseGate:
		_, err := fmt.Fprintln(out, "phase gate reached; run `swarm go` to continue")
		return err
	case dispatcher.SignalAllDone:
		_, err := fmt.Fprintln(out, "all tickets complete")
		return err
	case dispatcher.SignalBlocked:
		_, err := fmt.Fprintln(out, "blocked: no spawnable tickets")
		return err
	default:
		return nil
	}
}

var doneCmd = &cobra.Command{
	Use:   "done <ticket> [sha]",
	Short: "Mark a ticket as done",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(_ *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		tr, err := tracker.Load(trackerPath)
		if err != nil {
			return err
		}
		d := dispatcher.New(cfg, tr)
		sha := ""
		if len(args) == 2 {
			sha = args[1]
		}
		sig, spawnable := d.MarkDone(args[0], sha)
		if err := tr.SaveTo(trackerPath); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "marked %s done\n", args[0])
		return printDispatchResult(os.Stdout, sig, spawnable)
	},
}

func init() {
	rootCmd.AddCommand(doneCmd)
}
