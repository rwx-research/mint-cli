package main

import (
	"github.com/rwx-research/mint-cli/internal/cli"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Short: "Update versions for base layers and Mint leaves",
	Use:   "update [flags] [files...]",
	RunE: func(cmd *cobra.Command, args []string) error {
		updateLeaves := func(files []string) error {
			replacementVersionPicker := cli.PickLatestMinorVersion
			if AllowMajorVersionChange {
				replacementVersionPicker = cli.PickLatestMajorVersion
			}

			return service.UpdateLeaves(cli.UpdateLeavesConfig{
				Files:                    files,
				DefaultDir:               ".mint",
				ReplacementVersionPicker: replacementVersionPicker,
			})
		}

		if len(args) > 0 && args[0] == "leaves" {
			return updateLeaves(args[1:])
		}

		return updateLeaves(args)
	},
}

var (
	AllowMajorVersionChange bool

	updateLeavesCmd = &cobra.Command{
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
		Use: "leaves [flags] [files...]",
	}
)

func init() {
	updateLeavesCmd.Flags().BoolVar(&AllowMajorVersionChange, "allow-major-version-change", false, "update leaves to the latest major version")
	updateCmd.Flags().BoolVar(&AllowMajorVersionChange, "allow-major-version-change", false, "update to the latest major version")
	updateCmd.AddCommand(updateLeavesCmd)
}
