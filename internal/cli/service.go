package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/rwx-research/mint-cli/internal/accesstoken"
	"github.com/rwx-research/mint-cli/internal/api"
	"github.com/rwx-research/mint-cli/internal/dotenv"
	"github.com/rwx-research/mint-cli/internal/errors"
	"github.com/rwx-research/mint-cli/internal/fs"
	"github.com/rwx-research/mint-cli/internal/messages"
	"github.com/rwx-research/mint-cli/internal/versions"

	"github.com/briandowns/spinner"
	"github.com/goccy/go-yaml"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

var HandledError = errors.New("handled error")

// Service holds the main business logic of the CLI.
type Service struct {
	Config
}

func NewService(cfg Config) (Service, error) {
	if err := cfg.Validate(); err != nil {
		return Service{}, errors.Wrap(err, "validation failed")
	}

	return Service{cfg}, nil
}

// DebugRunConfig will connect to a running task over SSH. Key exchange is facilitated over the Cloud API.
func (s Service) DebugTask(cfg DebugTaskConfig) error {
	err := cfg.Validate()
	if err != nil {
		return errors.Wrap(err, "validation failed")
	}

	connectionInfo, err := s.APIClient.GetDebugConnectionInfo(cfg.DebugKey)
	if err != nil {
		return err
	}

	if !connectionInfo.Debuggable {
		return errors.Wrap(errors.ErrRetry, "The task or run is not in a debuggable state")
	}

	privateUserKey, err := ssh.ParsePrivateKey([]byte(connectionInfo.PrivateUserKey))
	if err != nil {
		return errors.Wrap(err, "unable to parse key material retrieved from Cloud API")
	}

	rawPublicHostKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(connectionInfo.PublicHostKey))
	if err != nil {
		return errors.Wrap(err, "unable to parse host key retrieved from Cloud API")
	}

	publicHostKey, err := ssh.ParsePublicKey(rawPublicHostKey.Marshal())
	if err != nil {
		return errors.Wrap(err, "unable to parse host key retrieved from Cloud API")
	}

	sshConfig := ssh.ClientConfig{
		User:            "mint-cli", // TODO: Add version number
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(privateUserKey)},
		HostKeyCallback: ssh.FixedHostKey(publicHostKey),
		BannerCallback: func(message string) error {
			fmt.Println(message)
			return nil
		},
	}

	if err = s.SSHClient.Connect(connectionInfo.Address, sshConfig); err != nil {
		return errors.Wrap(err, "unable to establish SSH connection to remote host")
	}
	defer s.SSHClient.Close()

	if err := s.SSHClient.InteractiveSession(); err != nil {
		var exitErr *ssh.ExitError
		// 137 is the default exit code for SIGKILL. This happens if the agent is forcefully terminating
		// the SSH server due to a run or task cancellation.
		if errors.As(err, &exitErr) && exitErr.ExitStatus() == 137 {
			return errors.New("The task was cancelled. Please check the Web UI for further details.")
		}

		return errors.Wrap(err, "unable to start interactive session on remote host")
	}

	return nil
}

// InitiateRun will connect to the Cloud API and start a new run in Mint.
func (s Service) InitiateRun(cfg InitiateRunConfig) (*api.InitiateRunResult, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	mintDirectory := make([]api.MintDirectoryEntry, 0)
	taskDefinitionYamlPath := cfg.MintFilePath

	mintDirectoryPath, err := s.findMintDirectoryPath(cfg.MintDirectory)
	if err != nil {
		return nil, errors.Wrap(err, "unable to find .mint directory")
	}

	// It's possible (when no directory is specified) that there is no .mint directory found during traversal
	if mintDirectoryPath != "" {
		mintDirectoryEntries, err := s.mintDirectoryEntries(mintDirectoryPath)
		if err != nil {
			if errors.Is(err, errors.ErrFileNotExists) {
				return nil, fmt.Errorf("You specified --dir %q, but %q could not be found", cfg.MintDirectory, cfg.MintDirectory)
			}

			return nil, err
		}

		mintDirInfo, err := os.Stat(mintDirectoryPath)
		if err != nil {
			return nil, fmt.Errorf("Unable to read the .mint directory at %q", mintDirectoryPath)
		}

		if !mintDirInfo.IsDir() {
			return nil, fmt.Errorf("The .mint directory at %q is not a directory", mintDirectoryPath)
		}

		mintDirectory = mintDirectoryEntries
	}

	taskDefinitions, err := s.mintDirectoryEntriesFromPaths([]string{taskDefinitionYamlPath})
	if err != nil {
		return nil, errors.Wrap(err, "unable to read provided files")
	}
	if len(taskDefinitions) != 1 {
		return nil, fmt.Errorf("Expected exactly 1 run definition, got %d", len(taskDefinitions))
	}

	reloadRunDefinitions := func() error {
		// Reload run definitions after modifying the file
		taskDefinitions, err = s.mintDirectoryEntriesFromPaths([]string{taskDefinitionYamlPath})
		if err != nil {
			return errors.Wrapf(err, "unable to reload %q", taskDefinitionYamlPath)
		}
		if mintDirectoryPath != "" {
			mintDirectoryEntries, err := s.mintDirectoryEntries(mintDirectoryPath)
			if err != nil && !errors.Is(err, errors.ErrFileNotExists) {
				return errors.Wrapf(err, "unable to reload mint directory %q", mintDirectoryPath)
			}

			mintDirectory = mintDirectoryEntries
		}
		return nil
	}

	addBaseIfNeeded, err := s.resolveBaseForFiles(taskDefinitions, BaseLayerSpec{})
	s.outputLatestVersionMessage()
	if err != nil {
		return nil, errors.Wrap(err, "unable to resolve base")
	}

	if addBaseIfNeeded.HasChanges() {
		update := addBaseIfNeeded.UpdatedRunFiles[0]
		if update.ResolvedBase.Os == "" {
			return nil, errors.New("unable to determine OS")
		}

		fmt.Fprintf(s.Stderr, "Configured %q to run on %s\n\n", taskDefinitionYamlPath, update.ResolvedBase.Os)

		if err = reloadRunDefinitions(); err != nil {
			return nil, err
		}
	}

	res, err := s.resolveLeavesForFiles(taskDefinitions, ResolveLeavesConfig{
		DefaultDir:          cfg.MintDirectory,
		LatestVersionPicker: PickLatestMajorVersion,
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to resolve leaves")
	}
	if res.HasChanges() {
		for leaf, version := range res.ResolvedLeaves {
			fmt.Fprintf(s.Stderr, "Configured leaf %s to use version %s\n", leaf, version)
		}
		fmt.Fprintln(s.Stderr, "")

		if err = reloadRunDefinitions(); err != nil {
			return nil, err
		}
	}

	i := 0
	initializationParameters := make([]api.InitializationParameter, len(cfg.InitParameters))
	for key, value := range cfg.InitParameters {
		initializationParameters[i] = api.InitializationParameter{
			Key:   key,
			Value: value,
		}
		i++
	}

	runResult, err := s.APIClient.InitiateRun(api.InitiateRunConfig{
		InitializationParameters: initializationParameters,
		TaskDefinitions:          taskDefinitions,
		MintDirectory:            mintDirectory,
		TargetedTaskKeys:         cfg.TargetedTasks,
		Title:                    cfg.Title,
		UseCache:                 !cfg.NoCache,
	})
	if err != nil {
		return nil, errors.Wrap(err, "Failed to initiate run")
	}

	return runResult, nil
}

func (s Service) InitiateDispatch(cfg InitiateDispatchConfig) (*api.InitiateDispatchResult, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	dispatchResult, err := s.APIClient.InitiateDispatch(api.InitiateDispatchConfig{
		DispatchKey: cfg.DispatchKey,
		Params:      cfg.Params,
		Ref:         cfg.Ref,
		Title:       cfg.Title,
	})
	s.outputLatestVersionMessage()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to initiate dispatch")
	}

	return dispatchResult, nil
}

func (s Service) GetDispatch(cfg GetDispatchConfig) ([]GetDispatchRun, error) {
	dispatchResult, err := s.APIClient.GetDispatch(api.GetDispatchConfig{
		DispatchId: cfg.DispatchId,
	})
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get dispatch")
	}

	if dispatchResult.Status == "not_ready" {
		return nil, errors.ErrRetry
	}

	if dispatchResult.Status == "error" {
		if dispatchResult.Error == "" {
			return nil, errors.New("Failed to get dispatch")
		}
		return nil, errors.New(dispatchResult.Error)
	}

	if len(dispatchResult.Runs) == 0 {
		return nil, errors.New("No runs were created as a result of this dispatch")
	}

	runs := make([]GetDispatchRun, len(dispatchResult.Runs))
	for i, run := range dispatchResult.Runs {
		runs[i] = GetDispatchRun{RunId: run.RunId, RunUrl: run.RunUrl}
	}

	return runs, nil
}

func (s Service) Lint(cfg LintConfig) (*api.LintResult, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	configFilePaths := cfg.MintFilePaths

	// Ensure both the provided paths and everything in the MintDirectory is loaded.
	mintDirectoryPath, err := s.findMintDirectoryPath(cfg.MintDirectory)
	if err != nil {
		return nil, fmt.Errorf("You specified a mint directory of %q, but %q could not be found", cfg.MintDirectory, cfg.MintDirectory)
	}
	if mintDirectoryPath != "" {
		configFilePaths = append(configFilePaths, mintDirectoryPath)
	}
	configFilePaths = removeDuplicateStrings(configFilePaths)

	taskDefinitionYamlPaths := make([]string, 0)
	for _, fileOrDir := range configFilePaths {
		fi, err := os.Stat(fileOrDir)
		if err != nil {
			if errors.Is(err, errors.ErrFileNotExists) {
				return nil, fmt.Errorf("you specified %q, but %q could not be found", fileOrDir, fileOrDir)
			}
			return nil, errors.Wrap(err, "unable to find file or directory")
		}

		if fi.IsDir() {
			mintDirectory, err := s.mintDirectoryEntries(fileOrDir)
			if err != nil {
				return nil, errors.Wrap(err, "unable to find yaml files in directory")
			}

			for _, entry := range s.yamlFilesInMintDirectory(mintDirectory) {
				taskDefinitionYamlPaths = append(taskDefinitionYamlPaths, entry.OriginalPath)
			}
		} else {
			taskDefinitionYamlPaths = append(taskDefinitionYamlPaths, fileOrDir)
		}
	}
	taskDefinitionYamlPaths = removeDuplicateStrings(taskDefinitionYamlPaths)

	wd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(err, "unable to get current working directory")
	}

	// Normalize paths to be relative to current working directory.
	relativeTaskDefinitionYamlPaths := make([]string, len(taskDefinitionYamlPaths))
	for i, yamlPath := range taskDefinitionYamlPaths {
		if relativePath, err := filepath.Rel(wd, yamlPath); err == nil {
			relativeTaskDefinitionYamlPaths[i] = relativePath
		} else {
			relativeTaskDefinitionYamlPaths[i] = yamlPath
		}
	}
	relativeTaskDefinitionYamlPaths = removeDuplicateStrings(relativeTaskDefinitionYamlPaths)

	taskDefinitions, err := s.taskDefinitionsFromPaths(relativeTaskDefinitionYamlPaths)
	if err != nil {
		return nil, errors.Wrap(err, "unable to read provided files")
	}
	taskDefinitions = removeDuplicates(taskDefinitions, func(td api.TaskDefinition) string {
		return td.Path
	})

	var targetPaths []string
	if len(cfg.MintFilePaths) > 0 {
		targetPaths = make([]string, len(cfg.MintFilePaths))

		for i, yamlPath := range cfg.MintFilePaths {
			if relativePath, err := filepath.Rel(wd, yamlPath); err == nil {
				targetPaths[i] = relativePath
			} else {
				targetPaths[i] = yamlPath
			}
		}
		targetPaths = removeDuplicateStrings(targetPaths)
		_, snippetFileNames := findSnippets(targetPaths)
		if len(snippetFileNames) > 0 {
			return nil, fmt.Errorf("You cannot target snippets for linting, but you targeted the following snippets: %s\n\nTo lint snippets, include them from a Mint run definition and lint the run definition.", strings.Join(snippetFileNames, ", "))
		}
	} else {
		targetPaths = relativeTaskDefinitionYamlPaths
		nonSnippetFileNames, _ := findSnippets(targetPaths)
		targetPaths = nonSnippetFileNames
	}

	lintResult, err := s.APIClient.Lint(api.LintConfig{
		TaskDefinitions: taskDefinitions,
		TargetPaths:     targetPaths,
	})
	s.outputLatestVersionMessage()
	if err != nil {
		return nil, errors.Wrap(err, "unable to lint files")
	}

	switch cfg.OutputFormat {
	case LintOutputOneLine:
		err = outputLintOneLine(s.Stdout, lintResult.Problems)
	case LintOutputMultiLine:
		err = outputLintMultiLine(s.Stdout, lintResult.Problems, len(targetPaths))
	}
	if err != nil {
		return nil, errors.Wrap(err, "unable to output lint results")
	}

	return lintResult, nil
}

func outputLintMultiLine(w io.Writer, problems []api.LintProblem, fileCount int) error {
	for _, lf := range problems {
		fmt.Fprintln(w)

		if len(lf.StackTrace) > 0 {
			fmt.Fprint(w, "[", lf.Severity, "] ")
			fmt.Fprintln(w, messages.FormatUserMessage(lf.Message, lf.Frame, lf.StackTrace, lf.Advice))
		} else {
			if fileLoc := lf.FileLocation(); len(fileLoc) > 0 {
				fmt.Fprint(w, fileLoc, "  ")
			}
			fmt.Fprint(w, "[", lf.Severity, "]")
			fmt.Fprintln(w)

			fmt.Fprint(w, lf.Message)

			if len(lf.Advice) > 0 {
				fmt.Fprint(w, "\n", lf.Advice)
			}

			fmt.Fprintln(w)
		}
	}

	pluralizedProblems := "problems"
	if len(problems) == 1 {
		pluralizedProblems = "problem"
	}

	pluralizedFiles := "files"
	if fileCount == 1 {
		pluralizedFiles = "file"
	}

	fmt.Fprintf(w, "\nChecked %d %s and found %d %s.\n", fileCount, pluralizedFiles, len(problems), pluralizedProblems)

	return nil
}

func outputLintOneLine(w io.Writer, lintedFiles []api.LintProblem) error {
	if len(lintedFiles) == 0 {
		return nil
	}

	for _, lf := range lintedFiles {
		fmt.Fprintf(w, "%-8s", lf.Severity)

		if fileLoc := lf.FileLocation(); len(fileLoc) > 0 {
			fmt.Fprint(w, fileLoc, " - ")
		}

		fmt.Fprint(w, strings.TrimSuffix(strings.ReplaceAll(lf.Message, "\n", " "), " "))
		fmt.Fprintln(w)
	}

	return nil
}

func (s Service) Login(cfg LoginConfig) error {
	err := cfg.Validate()
	if err != nil {
		return errors.Wrap(err, "validation failed")
	}

	authCodeResult, err := s.APIClient.ObtainAuthCode(api.ObtainAuthCodeConfig{
		Code: api.ObtainAuthCodeCode{
			DeviceName: cfg.DeviceName,
		},
	})
	if err != nil {
		return errors.Wrap(err, "unable to obtain an auth code")
	}

	// we print a nice message to handle the case where opening the browser fails, so we ignore this error
	cfg.OpenUrl(authCodeResult.AuthorizationUrl) //nolint:errcheck

	fmt.Fprintln(s.Stdout)
	fmt.Fprintln(s.Stdout, "To authorize this device, you'll need to login to RWX Cloud and choose an organization.")
	fmt.Fprintln(s.Stdout, "Your browser should automatically open. If it does not, you can visit this URL:")
	fmt.Fprintln(s.Stdout)
	fmt.Fprintf(s.Stdout, "\t%v\n", authCodeResult.AuthorizationUrl)
	fmt.Fprintln(s.Stdout)
	fmt.Fprintln(s.Stdout, "Once authorized, a personal access token will be generated and stored securely on this device.")
	fmt.Fprintln(s.Stdout)

	indicator := spinner.New(spinner.CharSets[11], 100*time.Millisecond, spinner.WithWriter(s.Stdout))
	indicator.Suffix = " Waiting for authorization..."
	indicator.Start()

	stop := func() {
		indicator.Stop()
		s.outputLatestVersionMessage()
	}

	for {
		tokenResult, err := s.APIClient.AcquireToken(authCodeResult.TokenUrl)
		if err != nil {
			stop()
			return errors.Wrap(err, "unable to acquire the token")
		}

		switch tokenResult.State {
		case "consumed":
			stop()
			return errors.New("The code has already been used. Try again.")
		case "expired":
			stop()
			return errors.New("The code has expired. Try again.")
		case "authorized":
			stop()
			if tokenResult.Token == "" {
				return errors.New("The code has been authorized, but there is no token. You can try again, but this is likely an issue with RWX Cloud. Please reach out at support@rwx.com.")
			} else {
				fmt.Fprint(s.Stdout, "Authorized!\n")
				return accesstoken.Set(cfg.AccessTokenBackend, tokenResult.Token)
			}
		case "pending":
			time.Sleep(1 * time.Second)
		default:
			stop()
			return errors.New("The code is in an unexpected state. You can try again, but this is likely an issue with RWX Cloud. Please reach out at support@rwx.com.")
		}
	}
}

func (s Service) Whoami(cfg WhoamiConfig) error {
	result, err := s.APIClient.Whoami()
	s.outputLatestVersionMessage()
	if err != nil {
		return errors.Wrap(err, "unable to determine details about the access token")
	}

	if cfg.Json {
		encoded, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return errors.Wrap(err, "unable to JSON encode the result")
		}

		fmt.Fprint(s.Stdout, string(encoded))
	} else {
		fmt.Fprintf(s.Stdout, "Token Kind: %v\n", strings.ReplaceAll(result.TokenKind, "_", " "))
		fmt.Fprintf(s.Stdout, "Organization: %v\n", result.OrganizationSlug)
		if result.UserEmail != nil {
			fmt.Fprintf(s.Stdout, "User: %v\n", *result.UserEmail)
		}
	}

	return nil
}

func (s Service) SetSecretsInVault(cfg SetSecretsInVaultConfig) error {
	err := cfg.Validate()
	if err != nil {
		return errors.Wrap(err, "validation failed")
	}

	secrets := []api.Secret{}
	for i := range cfg.Secrets {
		key, value, found := strings.Cut(cfg.Secrets[i], "=")
		if !found {
			return errors.New(fmt.Sprintf("Invalid secret '%s'. Secrets must be specified in the form 'KEY=value'.", cfg.Secrets[i]))
		}
		secrets = append(secrets, api.Secret{
			Name:   key,
			Secret: value,
		})
	}

	if cfg.File != "" {
		fd, err := os.Open(cfg.File)
		if err != nil {
			return errors.Wrapf(err, "error while opening %q", cfg.File)
		}
		defer fd.Close()

		fileContent, err := io.ReadAll(fd)
		if err != nil {
			return errors.Wrapf(err, "error while reading %q", cfg.File)
		}

		dotenvMap := make(map[string]string)
		err = dotenv.ParseBytes(fileContent, dotenvMap)
		if err != nil {
			return errors.Wrapf(err, "error while parsing %q", cfg.File)
		}

		for key, value := range dotenvMap {
			secrets = append(secrets, api.Secret{
				Name:   key,
				Secret: value,
			})
		}
	}

	result, err := s.APIClient.SetSecretsInVault(api.SetSecretsInVaultConfig{
		VaultName: cfg.Vault,
		Secrets:   secrets,
	})
	s.outputLatestVersionMessage()

	if result != nil && len(result.SetSecrets) > 0 {
		fmt.Fprintln(s.Stdout)
		fmt.Fprintf(s.Stdout, "Successfully set the following secrets: %s", strings.Join(result.SetSecrets, ", "))
	}

	if err != nil {
		return errors.Wrap(err, "unable to set secrets")
	}

	return nil
}

func (s Service) UpdateLeaves(cfg UpdateLeavesConfig) error {
	var files []string

	err := cfg.Validate()
	if err != nil {
		return errors.Wrap(err, "validation failed")
	}

	if len(cfg.Files) > 0 {
		files = cfg.Files
	} else {
		mintDirectory, err := s.mintDirectoryEntries(cfg.DefaultDir)
		if err != nil {
			return err
		}

		yamlFiles := make([]string, 0)
		for _, entry := range s.yamlFilesInMintDirectory(mintDirectory) {
			yamlFiles = append(yamlFiles, entry.OriginalPath)
		}

		files = yamlFiles
	}

	if len(files) == 0 {
		return errors.New(fmt.Sprintf("no files provided, and no yaml files found in directory %s", cfg.DefaultDir))
	}

	leafReferences, err := s.findLeafReferences(files)
	if err != nil {
		return err
	}

	leafVersions, err := s.APIClient.GetLeafVersions()
	s.outputLatestVersionMessage()
	if err != nil {
		return errors.Wrap(err, "unable to fetch leaf versions")
	}

	replacements := make(map[string]string)
	for leaf, majorVersions := range leafReferences {
		for majorVersion, references := range majorVersions {
			targetLeafVersion, err := cfg.ReplacementVersionPicker(*leafVersions, leaf, majorVersion)
			if err != nil {
				fmt.Fprintln(s.Stderr, err.Error())
				continue
			}

			replacement := fmt.Sprintf("%s %s", leaf, targetLeafVersion)
			for _, reference := range references {
				if reference != replacement {
					replacements[reference] = replacement
				}
			}
		}
	}

	err = s.replaceStringInFiles(files, replacements)
	if err != nil {
		return errors.Wrap(err, "unable to replace leaf references")
	}

	if len(replacements) == 0 {
		fmt.Fprintln(s.Stdout, "No leaves to update.")
	} else {
		fmt.Fprintln(s.Stdout, "Updated the following leaves:")
		for original, replacement := range replacements {
			replacementParts := strings.Split(replacement, " ")
			if len(replacementParts) == 2 {
				fmt.Fprintf(s.Stdout, "\t%s → %s\n", original, replacementParts[1])
			} else {
				fmt.Fprintf(s.Stdout, "\t%s → %s\n", original, replacement)
			}
		}
	}

	return nil
}

var reLeaf = regexp.MustCompile(`call:\s+(([a-z0-9-]+\/[a-z0-9-]+)(?:\s+([0-9]+)\.[0-9]+\.[0-9]+)?)`)

// findLeafReferences returns a map indexed with the leaf names. Each key is another map, this time indexed by
// the major version number. Finally, the value is an array of version strings as they appeared in the source
// file
func (s Service) findLeafReferences(files []string) (map[string]map[string][]string, error) {
	matches := make(map[string]map[string][]string)

	for _, path := range files {
		fd, err := os.Open(path)
		if err != nil {
			return nil, errors.Wrapf(err, "error while opening %q", path)
		}
		defer fd.Close()

		fileContent, err := io.ReadAll(fd)
		if err != nil {
			return nil, errors.Wrapf(err, "error while reading %q", path)
		}

		for _, match := range reLeaf.FindAllSubmatch(fileContent, -1) {
			fullMatch := string(match[1])
			leaf := string(match[2])
			majorVersion := string(match[3])

			majorVersions, ok := matches[leaf]
			if !ok {
				majorVersions = make(map[string][]string)
			}

			if _, ok := majorVersions[majorVersion]; !ok {
				majorVersions[majorVersion] = []string{fullMatch}
			} else {
				majorVersions[majorVersion] = append(majorVersions[majorVersion], fullMatch)
			}

			matches[leaf] = majorVersions
		}
	}

	return matches, nil
}

var reTasks = regexp.MustCompile(`(?m)^tasks:`)

func (s Service) ResolveBase(cfg ResolveBaseConfig) (ResolveBaseResult, error) {
	err := cfg.Validate()
	if err != nil {
		return ResolveBaseResult{}, errors.Wrap(err, "validation failed")
	}

	mintDirectory, err := s.mintDirectoryEntries(cfg.DefaultDir)
	if err != nil {
		return ResolveBaseResult{}, err
	}

	yamlFiles := s.yamlFilesInMintDirectory(mintDirectory)
	if len(yamlFiles) == 0 {
		return ResolveBaseResult{}, fmt.Errorf("no files found in mint directory %q", cfg.DefaultDir)
	}

	requestedSpec := BaseLayerSpec{
		Os:   cfg.Os,
		Tag:  cfg.Tag,
		Arch: cfg.Arch,
	}

	result, err := s.resolveBaseForFiles(yamlFiles, requestedSpec)
	s.outputLatestVersionMessage()
	if err != nil {
		return ResolveBaseResult{}, err
	}

	pluralizeFiles := func(files []BaseLayerRunFile) string {
		if len(files) == 1 {
			return "1 file"
		}
		return fmt.Sprintf("%d files", len(files))
	}

	if len(yamlFiles) == 0 {
		fmt.Fprintf(s.Stdout, "No run files found in %q.\n", cfg.DefaultDir)
	} else if !result.HasChanges() {
		fmt.Fprintln(s.Stdout, "No run files were missing base.")
	} else {
		if len(result.UpdatedRunFiles) > 0 {
			fmt.Fprintf(s.Stdout, "Added base to %s:\n", pluralizeFiles(result.UpdatedRunFiles))
			for _, runFile := range result.UpdatedRunFiles {
				fmt.Fprintf(s.Stdout, "\t%s\n", runFile.Filepath)
			}
			if len(result.ErroredRunFiles) > 0 {
				fmt.Fprintln(s.Stdout)
			}
		}

		if len(result.ErroredRunFiles) > 0 {
			fmt.Fprintf(s.Stdout, "Failed to add base to %s:\n", pluralizeFiles(result.ErroredRunFiles))
			for _, runFile := range result.ErroredRunFiles {
				fmt.Fprintf(s.Stdout, "\t%s → %s\n", runFile.Filepath, runFile.Error)
			}
		}
	}

	return result, nil
}

func (s Service) resolveBaseForFiles(mintFiles []api.MintDirectoryEntry, requestedSpec BaseLayerSpec) (ResolveBaseResult, error) {
	runFiles := make([]BaseLayerRunFile, 0)

	for _, entry := range mintFiles {
		content, err := os.ReadFile(entry.OriginalPath)
		if err != nil {
			return ResolveBaseResult{}, err
		}

		parsed := struct {
			Base BaseLayerSpec `yaml:"base"`
		}{}
		if err = yaml.Unmarshal(content, &parsed); err == nil {
			// Skip any files that already define a 'base' with at least 'os' and 'tag'
			if parsed.Base.Os != "" && parsed.Base.Tag != "" {
				continue
			}
		}

		// Skip files that don't have a 'tasks' key
		if !reTasks.Match(content) {
			continue
		}

		runFiles = append(runFiles, BaseLayerRunFile{
			Spec:     requestedSpec.Merge(parsed.Base),
			Filepath: entry.OriginalPath,
		})
	}

	if len(runFiles) == 0 {
		return ResolveBaseResult{}, nil
	}

	specToResolved, err := s.resolveBaseSpecs(runFiles)
	if err != nil {
		return ResolveBaseResult{}, errors.Wrap(err, "unable to resolve base specs")
	}

	// Inject base config in file
	erroredRunFiles := make([]BaseLayerRunFile, 0, len(runFiles))
	updatedRunFiles := make([]BaseLayerRunFile, 0, len(runFiles))
	for _, runFile := range runFiles {
		resolvedBase := specToResolved[runFile.Spec]
		if (BaseLayerSpec{}) == resolvedBase {
			return ResolveBaseResult{}, fmt.Errorf("unable to resolve base spec %+v", runFile.Spec)
		}
		runFile.ResolvedBase = resolvedBase

		err := s.writeRunFileWithBase(runFile, resolvedBase)
		if err != nil {
			runFile.Error = err
			erroredRunFiles = append(erroredRunFiles, runFile)
		} else {
			runFile.Updated = true
			updatedRunFiles = append(updatedRunFiles, runFile)
		}
	}

	return ResolveBaseResult{
		ErroredRunFiles: erroredRunFiles,
		UpdatedRunFiles: updatedRunFiles,
	}, nil
}

func (s Service) resolveBaseSpecs(runFiles []BaseLayerRunFile) (map[BaseLayerSpec]BaseLayerSpec, error) {
	// Get unique base layer specs to resolve from the server.
	specToResolved := make(map[BaseLayerSpec]BaseLayerSpec)
	for _, runFile := range runFiles {
		specToResolved[runFile.Spec] = runFile.Spec
	}

	errs, _ := errgroup.WithContext(context.Background())
	errs.SetLimit(3)

	for spec := range specToResolved {
		errs.Go(func() error {
			resolvedSpec, err := s.APIClient.ResolveBaseLayer(api.ResolveBaseLayerConfig{
				Os:   spec.Os,
				Arch: spec.Arch,
				Tag:  spec.Tag,
			})
			if err != nil {
				return errors.Wrap(err, fmt.Sprintf("unable to resolve base layer %+v", spec))
			}

			specToResolved[spec] = BaseLayerSpec{
				Os:   resolvedSpec.Os,
				Tag:  resolvedSpec.Tag,
				Arch: resolvedSpec.Arch,
			}
			return nil
		})
	}

	if err := errs.Wait(); err != nil {
		return nil, err
	}

	return specToResolved, nil
}

func (s Service) ensureBaseSection(input []byte, base BaseLayerSpec) ([]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(input))
	var lines []string
	tasksLineIndex := -1 // 0-based index of the root 'tasks:' line
	baseStartIndex := -1 // 0-based index of the root 'base:' line

	// Read all lines to find root 'tasks:' and 'base:' indices
	lineIdx := 0
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)

		// Check if the line is a root key (no leading whitespace)
		trimmedLine := strings.TrimLeft(line, " \t")
		isRootKey := len(trimmedLine) > 0 && trimmedLine == line

		if isRootKey {
			if line == "tasks:" && tasksLineIndex == -1 {
				tasksLineIndex = lineIdx
			}
			if line == "base:" && baseStartIndex == -1 {
				baseStartIndex = lineIdx
			}
		}
		lineIdx++
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning input data: %w", err)
	}

	// Ensure the root 'tasks:' key was found
	if tasksLineIndex == -1 {
		if len(lines) == 0 {
			return nil, fmt.Errorf("input is empty, required 'tasks:' key not found")
		}
		return nil, fmt.Errorf("root 'tasks:' key not found in input YAML")
	}

	// If there is already a 'base' field, find the end of it.
	// If 'arch' is already there, keep the entire line (eg. trailing comments).
	existingArchLine := ""
	baseEndIndex := -1 // Index of the line after the base field

	if baseStartIndex != -1 {
		// Find the end of the base field and look for 'arch' within it
		baseEndIndex = len(lines) // Default if base is the last element in the file
		for i := baseStartIndex + 1; i < len(lines); i++ {
			line := lines[i]

			// A root base field ends when a line is encountered that is not indented.
			// An empty line does not necessarily end the field.
			if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
				baseEndIndex = i
				break
			}

			// Look for the 'arch' key within the field
			trimmedLine := strings.TrimSpace(line)
			if strings.HasPrefix(trimmedLine, "arch:") {
				// Verify it's likely a direct key under base:
				// It must start with whitespace, and 'arch:' must follow that whitespace.
				if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
					// Find where 'arch:' starts
					archIndex := strings.Index(line, "arch:")
					if archIndex > 0 {
						// Check if the part before 'arch:' is only whitespace
						if strings.TrimSpace(line[:archIndex]) == "" {
							existingArchLine = line // Preserve original line with indentation
							// Continue searching until baseEndIndex is determined accurately
						}
					}
				}
			}
		}
	}

	// Construct the lines for the new/updated base field.
	// We always add os and tag using standard two-space ("  ") indentation.
	// If an original arch line was found, we append it to the new field.
	newBaseLines := []string{"base:", fmt.Sprintf("  os: %s", base.Os), fmt.Sprintf("  tag: %s", base.Tag)}
	if existingArchLine != "" {
		newBaseLines = append(newBaseLines, existingArchLine)
	} else if base.Arch != "" && base.Arch != "x86_64" {
		newBaseLines = append(newBaseLines, fmt.Sprintf("  arch: %s", base.Arch))
	}
	newBaseLines = append(newBaseLines, "")

	var outputLines []string

	if baseStartIndex != -1 {
		// Base exists: Replace the old base field range with the new lines
		if baseStartIndex > 0 {
			outputLines = append(outputLines, lines[0:baseStartIndex]...)
		}

		outputLines = append(outputLines, newBaseLines...)

		if baseEndIndex < len(lines) {
			outputLines = append(outputLines, lines[baseEndIndex:]...)
		}
	} else {
		// Base doesn't exist: Insert new base field before the 'tasks' field
		if tasksLineIndex > 0 {
			outputLines = append(outputLines, lines[0:tasksLineIndex]...)
		}

		outputLines = append(outputLines, newBaseLines...)
		outputLines = append(outputLines, lines[tasksLineIndex:]...)
	}

	finalOutput := strings.Join(outputLines, "\n")

	// Add a trailing newline if the original data had content
	if len(input) > 0 && len(finalOutput) > 0 && !strings.HasSuffix(finalOutput, "\n") {
		finalOutput += "\n"
	}

	return []byte(finalOutput), nil
}

func (s Service) writeRunFileWithBase(runFile BaseLayerRunFile, resolvedBase BaseLayerSpec) error {
	fi, err := os.Stat(runFile.Filepath)
	if err != nil {
		return fmt.Errorf("error getting file info for %q: %w", runFile.Filepath, err)
	}

	fileMode := fi.Mode()
	file, err := os.OpenFile(runFile.Filepath, os.O_RDWR|os.O_CREATE, fileMode)
	if err != nil {
		return err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	newContent, err := s.ensureBaseSection(content, resolvedBase)
	if err != nil {
		return err
	}

	if bytes.Equal(content, newContent) {
		return nil
	}

	if err = file.Truncate(0); err != nil {
		return err
	}

	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return err
	}

	_, err = file.Write(newContent)
	return err
}

func (s Service) ResolveLeaves(cfg ResolveLeavesConfig) (ResolveLeavesResult, error) {
	err := cfg.Validate()
	if err != nil {
		return ResolveLeavesResult{}, errors.Wrap(err, "validation failed")
	}

	mintDirectory, err := s.mintDirectoryEntries(cfg.DefaultDir)
	if err != nil {
		return ResolveLeavesResult{}, err
	}

	mintFiles := s.yamlFilesInMintDirectory(mintDirectory)
	result, err := s.resolveLeavesForFiles(mintFiles, cfg)
	if err != nil {
		return result, err
	}

	if len(result.ResolvedLeaves) == 0 {
		fmt.Fprintln(s.Stdout, "No leaves to resolve.")
	} else {
		fmt.Fprintln(s.Stdout, "Resolved the following leaves:")
		for leaf, version := range result.ResolvedLeaves {
			fmt.Fprintf(s.Stdout, "\t%s → %s\n", leaf, version)
		}
	}

	return result, nil
}

func (s Service) resolveLeavesForFiles(mintFiles []api.MintDirectoryEntry, cfg ResolveLeavesConfig) (ResolveLeavesResult, error) {
	if len(mintFiles) == 0 {
		return ResolveLeavesResult{}, errors.New(fmt.Sprintf("no files provided, and no yaml files found in directory %s", cfg.DefaultDir))
	}

	files := make([]string, len(mintFiles))
	for i, entry := range mintFiles {
		files[i] = entry.OriginalPath
	}

	leafReferences, err := s.findLeafReferences(files)
	if err != nil {
		return ResolveLeavesResult{}, err
	}

	leavesWithoutVersion := make([]string, 0)
	for leaf, majorToMinorVersions := range leafReferences {
		if _, found := majorToMinorVersions[""]; found {
			leavesWithoutVersion = append(leavesWithoutVersion, leaf)
		}
	}

	leafVersions, err := s.APIClient.GetLeafVersions()
	s.outputLatestVersionMessage()
	if err != nil {
		return ResolveLeavesResult{}, errors.Wrap(err, "unable to fetch leaf versions")
	}

	replacements := make(map[*regexp.Regexp]string)
	resolved := make(map[string]string)
	for _, leaf := range leavesWithoutVersion {
		targetLeafVersion, err := cfg.PickLatestVersion(*leafVersions, leaf)
		if err != nil {
			fmt.Fprintln(s.Stderr, err.Error())
			continue
		}

		re, err := regexp.Compile(fmt.Sprintf(`(?m)^([ \t]*)call:[ \t]+%s([ \t]+#[^\r\n]+)?$`, regexp.QuoteMeta(leaf)))
		if err != nil {
			fmt.Fprintln(s.Stderr, err.Error())
			continue
		}

		replacement := fmt.Sprintf("${1}call: %s %s${2}", leaf, targetLeafVersion)
		replacements[re] = replacement
		resolved[leaf] = targetLeafVersion
	}

	err = s.replaceRegexpInFiles(files, replacements)
	if err != nil {
		return ResolveLeavesResult{}, errors.Wrap(err, "unable to replace leaf references")
	}

	return ResolveLeavesResult{ResolvedLeaves: resolved}, nil
}

func (s Service) replaceRegexpInFiles(files []string, replacements map[*regexp.Regexp]string) error {
	for _, path := range files {
		fd, err := os.Open(path)
		if err != nil {
			return errors.Wrapf(err, "error while opening %q", path)
		}
		defer fd.Close()

		fileContent, err := io.ReadAll(fd)
		if err != nil {
			return errors.Wrapf(err, "error while reading %q", path)
		}
		fileContentStr := string(fileContent)

		for re, new := range replacements {
			fileContentStr = re.ReplaceAllString(fileContentStr, new)
		}

		fd, err = os.Create(path)
		if err != nil {
			return errors.Wrapf(err, "error while opening %q", path)
		}
		defer fd.Close()

		_, err = io.WriteString(fd, fileContentStr)
		if err != nil {
			return errors.Wrapf(err, "error while writing %q", path)
		}
	}

	return nil
}

func (s Service) replaceStringInFiles(files []string, replacements map[string]string) error {
	for _, path := range files {
		fd, err := os.Open(path)
		if err != nil {
			return errors.Wrapf(err, "error while opening %q", path)
		}
		defer fd.Close()

		fileContent, err := io.ReadAll(fd)
		if err != nil {
			return errors.Wrapf(err, "error while reading %q", path)
		}
		fileContentStr := string(fileContent)

		for old, new := range replacements {
			fileContentStr = strings.ReplaceAll(fileContentStr, old, new)
		}

		fd, err = os.Create(path)
		if err != nil {
			return errors.Wrapf(err, "error while opening %q", path)
		}
		defer fd.Close()

		_, err = io.WriteString(fd, fileContentStr)
		if err != nil {
			return errors.Wrapf(err, "error while writing %q", path)
		}
	}

	return nil
}

// taskDefinitionsFromPaths opens each file specified in `paths` and reads their content as a string.
// No validation takes place here.
func (s Service) taskDefinitionsFromPaths(paths []string) ([]api.TaskDefinition, error) {
	taskDefinitions := make([]api.TaskDefinition, 0)

	for _, path := range paths {
		fd, err := os.Open(path)
		if err != nil {
			return nil, errors.Wrapf(err, "error while opening %q", path)
		}
		defer fd.Close()

		fileContent, err := io.ReadAll(fd)
		if err != nil {
			return nil, errors.Wrapf(err, "error while reading %q", path)
		}

		taskDefinitions = append(taskDefinitions, api.TaskDefinition{
			Path:         path,
			FileContents: string(fileContent),
		})
	}

	return taskDefinitions, nil
}

func (s Service) mintDirectoryEntries(dir string) ([]api.MintDirectoryEntry, error) {
	mintDirectoryEntries := make([]api.MintDirectoryEntry, 0)
	contentLength := 0

	err := filepath.Walk(dir, func(pathInDir string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error reading %q: %w", pathInDir, err)
		}

		entry, entryContentLength, err := s.mintDirectoryEntry(pathInDir, info, dir)
		if err != nil {
			return err
		}
		contentLength += entryContentLength
		mintDirectoryEntries = append(mintDirectoryEntries, entry)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve the entire contents of the .mint directory %q: %w", dir, err)
	}
	if contentLength > 5*1024*1024 {
		return nil, fmt.Errorf("the size of the .mint directory at %q exceeds 5MiB", dir)
	}

	return mintDirectoryEntries, nil
}

// taskDefinitionsFromPaths opens each file specified in `paths` and reads their content as a string.
// No validation takes place here.
func (s Service) mintDirectoryEntriesFromPaths(paths []string) ([]api.MintDirectoryEntry, error) {
	mintDirectoryEntries := make([]api.MintDirectoryEntry, 0)

	for _, path := range paths {
		fd, err := os.Open(path)
		if err != nil {
			return nil, errors.Wrapf(err, "error while opening %q", path)
		}
		defer fd.Close()

		info, err := os.Lstat(path)
		if err != nil {
			return nil, errors.Wrapf(err, "error while stating %q", path)
		}

		entry, _, err := s.mintDirectoryEntry(path, info, "")
		if err != nil {
			return nil, err
		}

		mintDirectoryEntries = append(mintDirectoryEntries, entry)
	}

	return mintDirectoryEntries, nil
}

func (s Service) mintDirectoryEntry(path string, info os.FileInfo, makePathRelativeTo string) (api.MintDirectoryEntry, int, error) {
	mode := info.Mode()
	permissions := mode.Perm()

	var entryType string
	switch mode.Type() {
	case os.ModeDir:
		entryType = "dir"
	case os.ModeSymlink:
		entryType = "symlink"
	case os.ModeNamedPipe:
		entryType = "named-pipe"
	case os.ModeSocket:
		entryType = "socket"
	case os.ModeDevice:
		entryType = "device"
	case os.ModeCharDevice:
		entryType = "char-device"
	case os.ModeIrregular:
		entryType = "irregular"
	default:
		if mode.IsRegular() {
			entryType = "file"
		} else {
			entryType = "unknown"
		}
	}

	var fileContents string
	var contentLength int
	if entryType == "file" {
		contents, err := os.ReadFile(path)
		if err != nil {
			return api.MintDirectoryEntry{}, contentLength, fmt.Errorf("unable to read file %q: %w", path, err)
		}

		contentLength = len(contents)
		fileContents = string(contents)
	}

	relPath := path
	if makePathRelativeTo != "" {
		rel, err := filepath.Rel(makePathRelativeTo, path)
		if err != nil {
			return api.MintDirectoryEntry{}, contentLength, fmt.Errorf("unable to determine relative path of %q: %w", path, err)
		}
		relPath = filepath.ToSlash(filepath.Join(".mint", rel)) // Mint only supports unix-style path separators
	}

	return api.MintDirectoryEntry{
		Type:         entryType,
		OriginalPath: path,
		Path:         relPath,
		Permissions:  uint32(permissions),
		FileContents: fileContents,
	}, contentLength, nil
}

// yamlFilesInMintDirectory returns any *.yml and *.yaml files in a given mint directory
func (s Service) yamlFilesInMintDirectory(mintDirectory []api.MintDirectoryEntry) []api.MintDirectoryEntry {
	yamlEntries := make([]api.MintDirectoryEntry, 0)

	for _, entry := range mintDirectory {
		if entry.Type != "file" {
			continue
		}

		if !strings.HasSuffix(entry.OriginalPath, ".yml") && !strings.HasSuffix(entry.OriginalPath, ".yaml") {
			continue
		}

		yamlEntries = append(yamlEntries, entry)
	}

	return yamlEntries
}

func (s Service) findMintDirectoryPath(configuredDirectory string) (string, error) {
	if configuredDirectory != "" {
		return configuredDirectory, nil
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", errors.Wrap(err, "unable to determine the working directory")
	}

	// otherwise, walk up the working directory looking at each basename
	for {
		workingDirHasMintDir, err := fs.Exists(filepath.Join(workingDirectory, ".mint"))
		if err != nil {
			return "", errors.Wrapf(err, "unable to determine if .mint exists in %q", workingDirectory)
		}

		if workingDirHasMintDir {
			return filepath.Join(workingDirectory, ".mint"), nil
		}

		if workingDirectory == string(os.PathSeparator) {
			return "", nil
		}

		parentDir, _ := filepath.Split(workingDirectory)
		workingDirectory = filepath.Clean(parentDir)
	}
}

func (s Service) outputLatestVersionMessage() {
	showLatestVersion := os.Getenv("MINT_HIDE_LATEST_VERSION") == ""

	if !showLatestVersion || !versions.NewVersionAvailable() {
		return
	}

	w := s.Stderr
	fmt.Fprintln(w, "========================================")
	fmt.Fprintln(w, "A new version of Mint is available!")
	fmt.Fprintf(w, "You are currently on version %s\n", versions.GetCliCurrentVersion())
	fmt.Fprintf(w, "The latest version is %s\n", versions.GetCliLatestVersion())

	if versions.InstalledWithHomebrew() {
		fmt.Fprintln(w, "\nYou can update to the latest version with:")
		fmt.Fprintln(w, "    brew upgrade rwx-research/tap/mint")
	}

	fmt.Fprintln(w, "========================================")
	fmt.Fprintln(w)
}

func PickLatestMajorVersion(versions api.LeafVersionsResult, leaf string, _ string) (string, error) {
	latestVersion, ok := versions.LatestMajor[leaf]
	if !ok {
		return "", fmt.Errorf("Unable to find the leaf %q; skipping it.", leaf)
	}

	return latestVersion, nil
}

func PickLatestMinorVersion(versions api.LeafVersionsResult, leaf string, major string) (string, error) {
	majorVersions, ok := versions.LatestMinor[leaf]
	if !ok {
		return "", fmt.Errorf("Unable to find the leaf %q; skipping it.", leaf)
	}

	latestVersion, ok := majorVersions[major]
	if !ok {
		return "", fmt.Errorf("Unable to find major version %q for leaf %q; skipping it.", major, leaf)
	}

	return latestVersion, nil
}

func removeDuplicateStrings(list []string) []string {
	slices.Sort(list)
	return slices.Compact(list)
}

func findSnippets(fileNames []string) (nonSnippetFileNames []string, snippetFileNames []string) {
	for _, fileName := range fileNames {
		if strings.HasPrefix(path.Base(fileName), "_") {
			snippetFileNames = append(snippetFileNames, fileName)
		} else {
			nonSnippetFileNames = append(nonSnippetFileNames, fileName)
		}
	}
	return nonSnippetFileNames, snippetFileNames
}

func removeDuplicates[T any, K comparable](list []T, identity func(t T) K) []T {
	seen := make(map[K]bool)
	var ts []T

	for _, t := range list {
		id := identity(t)
		if _, found := seen[id]; !found {
			seen[id] = true
			ts = append(ts, t)
		}
	}
	return ts
}
