package main

import (
	"github.com/rwx-research/mint-cli/internal/cli"

	"github.com/spf13/cobra"
)

var debugCmd = &cobra.Command{
	Args: cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return requireAccessToken()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return service.DebugTask(cli.DebugTaskConfig{RunURL: args[0]})
	},
	Short: "Debug a task on Mint",
	Use:   "debug [flags] [runURL]",
}

func init() {
}
