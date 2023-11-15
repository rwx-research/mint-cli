package main

import (
	"github.com/rwx-research/mint-cli/cmd/mint/config"

	"github.com/spf13/cobra"
)

var (
	Debug bool

	rootCmd = &cobra.Command{
		Use:           "mint",
		Short:         "A CLI client from www.rwx.com/mint",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       config.Version,
	}
)

func init() {
	rootCmd.PersistentFlags().BoolVar(&Debug, "debug", false, "enable debug output")
	_ = rootCmd.PersistentFlags().MarkHidden("debug")

	rootCmd.AddCommand(runCmd)
}
