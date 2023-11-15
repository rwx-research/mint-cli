package main

import "github.com/spf13/cobra"

var (
	Debug bool

	rootCmd = &cobra.Command{
		Use:           "mint",
		Short:         "A CLI client from www.rwx.com/mint",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       version,
	}
)

func init() {
	rootCmd.PersistentFlags().BoolVar(&Debug, "debug", false, "enable debug output")
	_ = rootCmd.PersistentFlags().MarkHidden("debug")

	rootCmd.AddCommand(runCmd)
}
