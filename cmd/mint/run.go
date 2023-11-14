package main

import (
	"fmt"
	"os"

	"github.com/rwx-research/mint-cli/internal/cli"
	"github.com/rwx-research/mint-cli/internal/client"
	"github.com/rwx-research/mint-cli/internal/fs"

	"github.com/spf13/cobra"
)

var (
	AccessToken   string
	MintDirectory string
	MintFilePath  string
	NoCache       bool

	mintHost string
	service  cli.Service

	runCmd = &cobra.Command{
		Args: cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New(client.Config{AccessToken: AccessToken, Host: mintHost})
			if err != nil {
				return err
			}

			service, err = cli.NewService(cli.Config{Client: c, FileSystem: fs.Local{}})
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			targetedTask := ""
			if len(args) == 1 {
				targetedTask = args[0]
			}

			runURL, err := service.InitiateRun(cli.InitiateRunConfig{
				MintDirectory: MintDirectory,
				MintFilePath:  MintFilePath,
				NoCache:       NoCache,
				TargetedTask:  targetedTask,
			})
			if err != nil {
				return err
			}

			fmt.Printf("Run is watchable at %s\n", runURL.String())
			return nil

		},
		Short: "Start a new run on Mint",
		Use:   "run [flags] --user-access-token=<token> [task]",
	}
)

func init() {
	mintHost = os.Getenv("MINT_HOST")
	if mintHost == "" {
		mintHost = "mint.rwx.com"
	}

	runCmd.Flags().StringVarP(&MintFilePath, "file", "f", "", "a Mint config file to use for sourcing task definitions")
	runCmd.Flags().StringVar(&MintDirectory, "dir", ".mint", "the directory containing your mint task definitions. By default, this is used to source task definitions")
	runCmd.Flags().BoolVar(&NoCache, "no-cache", false, "do not read or write to the cache")

	runCmd.Flags().StringVar(&AccessToken, "access-token", "", "the access token for Mint")
	runCmd.MarkFlagRequired("user-access-token")
}
