package main

import (
	"errors"
	"os"
	"slices"

	"github.com/rwx-research/mint-cli/internal/api"
	"github.com/rwx-research/mint-cli/internal/cli"

	"github.com/spf13/cobra"
)

var (
	LintFailure = errors.New("lint failure")

	LintMintDirectory    string
	LintWarningsAsErrors bool
	LintOutputFormat     string

	lintCmd = &cobra.Command{
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireAccessToken()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var targetedFiles []string
			if len(args) >= 0 {
				targetedFiles = args
			}

			lintConfig, err := cli.NewLintConfig(
				targetedFiles,
				LintMintDirectory,
				os.Stdout,
				LintOutputFormat,
			)
			if err != nil {
				return err
			}

			lintResult, err := service.Lint(lintConfig)
			if err != nil {
				return err
			}

			if len(lintResult.Problems) == 0 {
				return nil
			}

			hasError := slices.ContainsFunc(lintResult.Problems, func(lf api.LintProblem) bool {
				return lf.Severity == "error"
			})

			if hasError || LintWarningsAsErrors {
				return LintFailure
			}

			return nil
		},
		Short:  "Lint Mint configuration files",
		Use:    "lint [flags] [files...]",
		Hidden: true,
	}
)

func init() {
	lintCmd.Flags().BoolVar(&LintWarningsAsErrors, "warnings-as-errors", false, "treat warnings as errors")
	lintCmd.Flags().StringVarP(&LintMintDirectory, "dir", "d", "", "the directory your Mint files are located in, typically `.mint`. By default, the CLI traverses up until it finds a `.mint` directory.")
	lintCmd.Flags().StringVarP(&LintOutputFormat, "output", "o", "multiline", "output format: multiline, oneline, none")
}
