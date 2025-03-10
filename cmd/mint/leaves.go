package main

import (
	"github.com/rwx-research/mint-cli/internal/cli"
	"github.com/spf13/cobra"
)

var leavesCmd = &cobra.Command{
	Short: "Manage Mint leaves",
	Use:   "leaves",
}

var (
	Files                   []string
	AllowMajorVersionChange bool

	leavesUpdateCmd = &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			replacementVersionPicker := cli.PickLatestMinorVersion
			if AllowMajorVersionChange {
				replacementVersionPicker = cli.PickLatestMajorVersion
			}

			return service.UpdateLeaves(cli.UpdateLeavesConfig{
				Files:                    args,
				DefaultDir:               ".mint",
				ReplacementVersionPicker: replacementVersionPicker,
			})
		},
		Short: "Update all leaves to their latest (minor) version",
		Long: "Update all leaves to their latest (minor) version.\n" +
			"Takes a list of files as arguments, or updates all toplevel YAML files in .mint if no files are given.",
		Use: "update [flags] [file...]",
	}
)

func init() {
	leavesUpdateCmd.Flags().BoolVar(&AllowMajorVersionChange, "allow-major-version-change", false, "update leaves to the latest major version")
	leavesCmd.AddCommand(leavesUpdateCmd)
}
