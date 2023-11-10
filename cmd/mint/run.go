package main

import (
	"os"

	"github.com/rwx-research/mint-cli/internal/cli"
	"github.com/rwx-research/mint-cli/internal/client"
	"github.com/rwx-research/mint-cli/internal/fs"

	"github.com/spf13/cobra"
)

var (
	MintFilePath string
	AccessToken  string

	mintHost string
	service  cli.Service

	runCmd = &cobra.Command{
		Args: cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New(client.Config{AccessToken: AccessToken, Host: mintHost})
			if err != nil {
				return err
			}

			service, err = cli.NewService(cli.Config{Client: c, FileSystem: fs.Local{}})
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return service.InitiateRun(cli.InitiateRunConfig{MintFilePath: MintFilePath})
		},
		Short: "Start a new run on Mint",
		Use:   "run [flags] --user-access-token=<token>",
	}
)

func init() {
	mintHost = os.Getenv("MINT_HOST")
	if mintHost == "" {
		mintHost = "mint.rwx.com"
	}

	runCmd.Flags().StringVarP(&MintFilePath, "file", "f", "", "a Mint config file to use for sourcing task definitions")

	runCmd.Flags().StringVar(&AccessToken, "access-token", "", "the access token for Mint")
	runCmd.MarkFlagRequired("user-access-token")
}
