package cmd

import "github.com/spf13/cobra"

var ticketsCmd = &cobra.Command{
	Use:   "tickets",
	Short: "Ticket readiness and quality commands",
}

func init() {
	ticketsCmd.AddCommand(ticketsLintCmd)
	rootCmd.AddCommand(ticketsCmd)
}
