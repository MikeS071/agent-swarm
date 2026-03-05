package cmd

import (
	"fmt"
	"os"

	"github.com/MikeS071/agent-swarm/internal/version"
	"github.com/spf13/cobra"
)

var cfgFile string
var showVersion bool

var rootCmd = &cobra.Command{
	Use:   "swarm",
	Short: "agent-swarm multi-agent orchestration CLI",
	RunE: func(cmd *cobra.Command, args []string) error {
		if showVersion {
			fmt.Println("agent-swarm " + version.String())
			return nil
		}
		return cmd.Help()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "swarm.toml", "config file path")
	rootCmd.Flags().BoolVar(&showVersion, "version", false, "print version and exit")
}