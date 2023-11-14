package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rwx-research/mint-cli/internal/cli"
	"github.com/rwx-research/mint-cli/internal/client"
	"github.com/rwx-research/mint-cli/internal/fs"

	"github.com/spf13/cobra"
)

var (
	AccessToken    string
	InitParameters []string
	MintDirectory  string
	MintFilePath   string
	NoCache        bool

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

			initParams, err := parseInitParameters(InitParameters)
			if err != nil {
				return err
			}

			runURL, err := service.InitiateRun(cli.InitiateRunConfig{
				InitParameters: initParams,
				MintDirectory:  MintDirectory,
				MintFilePath:   MintFilePath,
				NoCache:        NoCache,
				TargetedTask:   targetedTask,
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

	runCmd.Flags().BoolVar(&NoCache, "no-cache", false, "do not read or write to the cache")
	runCmd.Flags().StringArrayVar(&InitParameters, "init-parameters", []string{}, "initialization parameters for the run, available in the `init` context. Can be specified multiple times")
	runCmd.Flags().StringVarP(&MintFilePath, "file", "f", "", "a Mint config file to use for sourcing task definitions")
	runCmd.Flags().StringVar(&MintDirectory, "dir", ".mint", "the directory containing your mint task definitions. By default, this is used to source task definitions")

	runCmd.Flags().StringVar(&AccessToken, "access-token", "", "the access token for Mint")
	_ = runCmd.MarkFlagRequired("user-access-token")
}

func parseInitParameters(params []string) (map[string]string, error) {
	parsedParams := make(map[string]string)

	parse := func(p string) error {
		fields := strings.Split(p, "=")
		if len(fields) != 2 {
			// TODO: Custom error
			return fmt.Errorf("Unable to parse init-parameter %q", p)
		}

		parsedParams[fields[0]] = fields[1]
		return nil
	}

	const prefix = "MINT_INIT_"
	for _, envVar := range os.Environ() {
		if !strings.HasPrefix(envVar, prefix) {
			continue
		}

		if err := parse(strings.TrimPrefix(envVar, prefix)); err != nil {
			return nil, err
		}
	}

	// Parse flag parameters after the environment as they take precedence
	for _, param := range params {
		if err := parse(param); err != nil {
			return nil, err
		}
	}

	return parsedParams, nil
}
