package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/rwx-research/mint-cli/internal/cli"
	"github.com/rwx-research/mint-cli/internal/client"
	"github.com/rwx-research/mint-cli/internal/fs"

	"github.com/pkg/errors"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
)

const flagInit = "init"

var (
	AccessToken    string
	InitParameters []string
	Json           bool
	MintDirectory  string
	MintFilePath   string
	NoCache        bool
	Open           bool

	mintHost string
	service  cli.Service

	runCmd = &cobra.Command{
		PreRunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New(client.Config{AccessToken: AccessToken, Host: mintHost})
			if err != nil {
				return errors.Wrap(err, "unable to initialize API client")
			}

			service, err = cli.NewService(cli.Config{Client: c, FileSystem: fs.Local{}})
			if err != nil {
				return errors.Wrap(err, "unable to initialize CLI")
			}

			for _, arg := range args {
				if strings.Contains(arg, "=") {
					initParam := strings.Split(arg, "=")[0]
					return fmt.Errorf(
						"You have specified a task target with an equals sign: \"%s\".\n"+
							"Are you trying to specify an init parameter \"%s\"?\n"+
							"You can define multiple init parameters by specifying --%s multiple times.\n"+
							"You may have meant to specify --%s \"%s\".",
						arg,
						initParam,
						flagInit,
						flagInit,
						arg,
					)
				}
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var targetedTasks []string
			if len(args) >= 0 {
				targetedTasks = args
			}

			initParams, err := ParseInitParameters(InitParameters)
			if err != nil {
				return errors.Wrap(err, "unable to parse init parameters")
			}

			runResult, err := service.InitiateRun(cli.InitiateRunConfig{
				InitParameters: initParams,
				Json:           Json,
				MintDirectory:  MintDirectory,
				MintFilePath:   MintFilePath,
				NoCache:        NoCache,
				TargetedTasks:  targetedTasks,
			})
			if err != nil {
				return err
			}

			if Json {
				runResultJson, err := json.Marshal(runResult)
				if err != nil {
					return err
				}
				fmt.Println(string(runResultJson))
			} else {
				fmt.Printf("Run is watchable at %s\n", runResult.RunURL)
			}

			if Open {
				if err := open.Run(runResult.RunURL); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to open browser.\n")
				}
			}

			return nil

		},
		Short: "Start a new run on Mint",
		Use:   "run [flags] --user-access-token=<token> [task]",
	}
)

func init() {
	// A different host can only be set over the environment
	mintHost = os.Getenv("MINT_HOST")
	if mintHost == "" {
		mintHost = "cloud.rwx.com"
	}

	runCmd.Flags().BoolVar(&NoCache, "no-cache", false, "do not read or write to the cache")
	runCmd.Flags().StringArrayVar(&InitParameters, flagInit, []string{}, "initialization parameters for the run, available in the `init` context. Can be specified multiple times")
	runCmd.Flags().StringVarP(&MintFilePath, "file", "f", "", "a Mint config file to use for sourcing task definitions")
	runCmd.Flags().StringVar(&AccessToken, "access-token", os.Getenv("RWX_ACCESS_TOKEN"), "the access token for Mint")
	runCmd.Flags().StringVar(&MintDirectory, "dir", ".mint", "the directory containing your mint task definitions. By default, this is used to source task definitions")
	runCmd.Flags().BoolVar(&Open, "open", false, "open the run in a browser")
	runCmd.Flags().BoolVar(&Json, "json", false, "output json data to stdout")
}

// parseInitParameters converts a list of `key=value` pairs to a map. It also reads any `MINT_INIT_` variables from the
// environment
func ParseInitParameters(params []string) (map[string]string, error) {
	parsedParams := make(map[string]string)

	parse := func(p string) error {
		fields := strings.Split(p, "=")
		if len(fields) < 2 {
			return errors.Errorf("unable to parse %q", p)
		}

		parsedParams[fields[0]] = strings.Join(fields[1:], "=")
		return nil
	}

	const prefix = "MINT_INIT_"
	for _, envVar := range os.Environ() {
		if !strings.HasPrefix(envVar, prefix) {
			continue
		}

		if err := parse(strings.TrimPrefix(envVar, prefix)); err != nil {
			return nil, errors.WithStack(err)
		}
	}

	// Parse flag parameters after the environment as they take precedence
	for _, param := range params {
		if err := parse(param); err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return parsedParams, nil
}
