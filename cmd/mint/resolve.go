package main

import (
	"fmt"

	"github.com/rwx-research/mint-cli/internal/cli"
	"github.com/spf13/cobra"
)

var resolveCmd = &cobra.Command{
	Short: "Resolve and add versions for base layers and Mint leaves",
	Use:   "resolve",
	RunE: func(cmd *cobra.Command, args []string) error {
		base, err := service.ResolveBase(cli.ResolveBaseConfig{
			DefaultDir: ".mint",
		})
		if err != nil {
			return err
		}
		if base.HasChanges() {
			fmt.Println()
		}

		_, err = service.ResolveLeaves(cli.ResolveLeavesConfig{
			DefaultDir:          ".mint",
			LatestVersionPicker: cli.PickLatestMajorVersion,
		})

		return err
	},
}

var (
	resolveBaseOs   string
	resolveBaseTag  string
	resolveBaseArch string

	resolveBaseCmd = &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.ResolveBase(cli.ResolveBaseConfig{
				DefaultDir: ".mint",
				Os:         resolveBaseOs,
				Tag:        resolveBaseTag,
				Arch:       resolveBaseArch,
			})
			return err
		},
		Short: "Add a base layer to Mint run configurations that do not have one",
		Long: "Add a base layer to Mint run configurations that do not have one.\n" +
			"Updates all top-level YAML files in .mint that are missing a 'base' to include one.\n" +
			"Mint will find the best match based on the provided flags. If no flags are provided,\n" +
			"it will use the current default base layer.",
		Use: "base [flags]",
	}

	resolveLeavesCmd = &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := service.ResolveLeaves(cli.ResolveLeavesConfig{
				DefaultDir:          ".mint",
				LatestVersionPicker: cli.PickLatestMajorVersion,
			})
			return err
		},
		Short: "Add the latest version to all leaf invocations that do not have one",
		Long: "Add the latest version to all leaf invocations that do not have one.\n" +
			"Updates all top-level YAML files in .mint that 'call' a leaf without a version\n" +
			"to use the latest version.",
		Use: "leaves",
	}
)

func init() {
	resolveBaseCmd.Flags().StringVar(&resolveBaseOs, "os", "", "target operating system")
	resolveBaseCmd.Flags().StringVar(&resolveBaseTag, "tag", "", "target base layer tag")
	resolveBaseCmd.Flags().StringVar(&resolveBaseArch, "arch", "", "target architecture")
	resolveCmd.AddCommand(resolveBaseCmd)
	resolveCmd.AddCommand(resolveLeavesCmd)
}
