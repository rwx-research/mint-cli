package main

import (
	"os"

	"github.com/rwx-research/mint-cli/internal/cli"
	"github.com/spf13/cobra"
)

var leavesCmd = &cobra.Command{
	Short: "Manage Mint leaves",
	Use:   "leaves",
}

var (
	Files []string

	leavesUpdateCmd = &cobra.Command{
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireAccessToken()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return service.UpdateLeaves(cli.UpdateLeavesConfig{
				Files:      args,
				DefaultDir: ".mint",
				Stdout:     os.Stdout,
				Stderr:     os.Stderr,
			})
		},
		Short: "Update all leaves to their latest major version",
		Long: "Update all leaves to their latest major version.\n" +
			"Takes a list of files as arguments, or updates all toplevel yaml files in .mint if no files are given.",
		Use: "update [flags] [file...]",
	}
)

func init() {
	leavesCmd.AddCommand(leavesUpdateCmd)
}
