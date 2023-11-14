package main

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:           "mint",
	Short:         "A CLI client from www.rwx.com/mint",
	SilenceErrors: true,
	SilenceUsage:  true,
	Version:       version,
}

func init() {
	rootCmd.AddCommand(runCmd)
}
