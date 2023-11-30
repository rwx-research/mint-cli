package main

import (
	"os"

	"github.com/rwx-research/mint-cli/cmd/mint/config"
	"github.com/rwx-research/mint-cli/internal/cli"
	"github.com/rwx-research/mint-cli/internal/client"
	"github.com/rwx-research/mint-cli/internal/fs"
	"github.com/rwx-research/mint-cli/internal/ssh"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	AccessToken string
	Debug       bool

	mintHost string
	service  cli.Service

	// rootCmd represents the main `mint` command
	rootCmd = &cobra.Command{
		Use:           "mint",
		Short:         "A CLI client from www.rwx.com/mint",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       config.Version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New(client.Config{AccessToken: AccessToken, Host: mintHost})
			if err != nil {
				return errors.Wrap(err, "unable to initialize API client")
			}

			service, err = cli.NewService(cli.Config{APIClient: c, FileSystem: fs.Local{}, SSHClient: new(ssh.Client)})
			if err != nil {
				return errors.Wrap(err, "unable to initialize CLI")
			}

			return nil
		},
	}
)

func init() {
	// A different host can only be set over the environment
	mintHost = os.Getenv("MINT_HOST")
	if mintHost == "" {
		mintHost = "mint.rwx.com"
	}

	rootCmd.PersistentFlags().StringVar(&AccessToken, "access-token", os.Getenv("RWX_ACCESS_TOKEN"), "the access token for Mint")
	rootCmd.PersistentFlags().BoolVar(&Debug, "debug", false, "enable debug output")
	_ = rootCmd.PersistentFlags().MarkHidden("debug")

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(debugCmd)
}
