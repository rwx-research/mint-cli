package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rwx-research/mint-cli/internal/cli"

	"github.com/pkg/errors"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
)

var (
	InitParameters []string
	MintDirectory  string
	MintFilePath   string
	NoCache        bool
	Open           bool

	runCmd = &cobra.Command{
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetedTask := ""
			if len(args) == 1 {
				targetedTask = args[0]
			}

			initParams, err := parseInitParameters(InitParameters)
			if err != nil {
				return errors.Wrap(err, "unable to parse init parameters")
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

			runURLString := runURL.String()
			fmt.Printf("Run is watchable at %s\n", runURLString)

			if Open {
				if err := open.Run(runURLString); err != nil {
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
	runCmd.Flags().BoolVar(&NoCache, "no-cache", false, "do not read or write to the cache")
	runCmd.Flags().StringArrayVar(&InitParameters, "init-parameter", []string{}, "initialization parameters for the run, available in the `init` context. Can be specified multiple times")
	runCmd.Flags().StringVarP(&MintFilePath, "file", "f", "", "a Mint config file to use for sourcing task definitions")
	runCmd.Flags().StringVar(&MintDirectory, "dir", ".mint", "the directory containing your mint task definitions. By default, this is used to source task definitions")
	runCmd.Flags().BoolVar(&Open, "open", false, "open the run in a browser")
}

// parseInitParameters converts a list of `key=value` pairs to a map. It also reads any `MINT_INIT_` variables from the
// environment
func parseInitParameters(params []string) (map[string]string, error) {
	parsedParams := make(map[string]string)

	parse := func(p string) error {
		fields := strings.Split(p, "=")
		if len(fields) != 2 {
			return errors.Errorf("unable to parse %q", p)
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
