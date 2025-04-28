package cli

import (
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
	"github.com/rwx-research/mint-cli/internal/messages"
	"github.com/rwx-research/mint-cli/internal/versions"

	"github.com/briandowns/spinner"
	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
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

	mintDirectoryPath, err := findMintDirectoryPath(cfg.MintDirectory)
	if err != nil {
		return nil, errors.Wrap(err, "unable to find .mint directory")
	}

	// It's possible (when no directory is specified) that there is no .mint directory found during traversal
	if mintDirectoryPath != "" {
		mintDirectoryEntries, err := mintDirectoryEntries(mintDirectoryPath)
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

	taskDefinitions, err := mintDirectoryEntriesFromPaths([]string{taskDefinitionYamlPath})
	if err != nil {
		return nil, errors.Wrap(err, "unable to read provided files")
	}
	if len(taskDefinitions) != 1 {
		return nil, fmt.Errorf("Expected exactly 1 run definition, got %d", len(taskDefinitions))
	}

	reloadRunDefinitions := func() error {
		// Reload run definitions after modifying the file
		taskDefinitions, err = mintDirectoryEntriesFromPaths([]string{taskDefinitionYamlPath})
		if err != nil {
			return errors.Wrapf(err, "unable to reload %q", taskDefinitionYamlPath)
		}
		if mintDirectoryPath != "" {
			mintDirectoryEntries, err := mintDirectoryEntries(mintDirectoryPath)
			if err != nil && !errors.Is(err, errors.ErrFileNotExists) {
				return errors.Wrapf(err, "unable to reload mint directory %q", mintDirectoryPath)
			}

			mintDirectory = mintDirectoryEntries
		}
		return nil
	}

	addBaseIfNeeded, err := s.resolveOrUpdateBaseForFiles(taskDefinitions, BaseLayerSpec{}, false)
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

	mintFiles := filterYAMLFilesForModification(taskDefinitions, func(doc *YAMLDoc) bool {
		return true
	})
	resolvedLeaves, err := s.resolveOrUpdateLeavesForFiles(mintFiles, false, PickLatestMajorVersion)
	if err != nil {
		return nil, err
	}
	if len(resolvedLeaves) > 0 {
		for leaf, version := range resolvedLeaves {
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
	mintDirectoryPath, err := findMintDirectoryPath(cfg.MintDirectory)
	if err != nil {
		return nil, errors.Wrap(err, "unable to find .mint directory")
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
			mintDirectory, err := mintDirectoryEntries(fileOrDir)
			if err != nil {
				return nil, errors.Wrap(err, "unable to find yaml files in directory")
			}

			for _, entry := range filterYAMLFiles(mintDirectory) {
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

	taskDefinitions, err := taskDefinitionsFromPaths(relativeTaskDefinitionYamlPaths)
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
	err := cfg.Validate()
	if err != nil {
		return errors.Wrap(err, "validation failed")
	}

	entries, err := getFileOrDirectoryYAMLEntries(cfg.Files, cfg.DefaultDir)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		return errors.New(fmt.Sprintf("no files provided, and no yaml files found in directory %s", cfg.DefaultDir))
	}

	mintFiles := filterYAMLFilesForModification(entries, func(doc *YAMLDoc) bool {
		return true
	})
	replacements, err := s.resolveOrUpdateLeavesForFiles(mintFiles, true, cfg.ReplacementVersionPicker)
	if err != nil {
		return err
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

var reLeafVersion = regexp.MustCompile(`([a-z0-9-]+\/[a-z0-9-]+)(?:\s+(([0-9]+)\.[0-9]+\.[0-9]+))?`)

type LeafVersion struct {
	Original     string
	Name         string
	Version      string
	MajorVersion string
}

func (s Service) parseLeafVersion(str string) LeafVersion {
	match := reLeafVersion.FindStringSubmatch(str)
	if len(match) == 0 {
		return LeafVersion{}
	}

	return LeafVersion{
		Original:     match[0],
		Name:         tryGetSliceAtIndex(match, 1, ""),
		Version:      tryGetSliceAtIndex(match, 2, ""),
		MajorVersion: tryGetSliceAtIndex(match, 3, ""),
	}
}

func (s Service) resolveOrUpdateLeavesForFiles(mintFiles []*MintYAMLFile, update bool, versionPicker func(versions api.LeafVersionsResult, leaf string, major string) (string, error)) (map[string]string, error) {
	leafVersions, err := s.APIClient.GetLeafVersions()
	s.outputLatestVersionMessage()
	if err != nil {
		return nil, errors.Wrap(err, "unable to fetch leaf versions")
	}

	docs := make(map[string]*YAMLDoc)
	replacements := make(map[string]string)

	for _, file := range mintFiles {
		hasChange := false
		err = file.Doc.ForEachNode("$.tasks[*].call", func(node ast.Node) error {
			leafVersion := s.parseLeafVersion(node.String())
			if !update && leafVersion.MajorVersion != "" {
				return nil
			}

			targetLeafVersion, err := versionPicker(*leafVersions, leafVersion.Name, leafVersion.MajorVersion)
			if err != nil {
				fmt.Fprintln(s.Stderr, err.Error())
				return nil
			}

			newLeaf := fmt.Sprintf("%s %s", leafVersion.Name, targetLeafVersion)
			if newLeaf == node.String() {
				return nil
			}

			if err = file.Doc.ReplaceAtPath(node.GetPath(), newLeaf); err != nil {
				return err
			}

			replacements[leafVersion.Original] = targetLeafVersion
			hasChange = true
			return nil
		})
		if err != nil {
			return nil, errors.Wrap(err, "unable to replace leaf references")
		}

		if hasChange {
			docs[file.Entry.OriginalPath] = file.Doc
		}
	}

	for path, doc := range docs {
		if !doc.HasChanges() {
			continue
		}

		err := doc.WriteFile(path)
		if err != nil {
			return replacements, err
		}
	}

	return replacements, nil
}

func (s Service) UpdateBase(cfg UpdateBaseConfig) (ResolveBaseResult, error) {
	err := cfg.Validate()
	if err != nil {
		return ResolveBaseResult{}, errors.Wrap(err, "validation failed")
	}

	yamlFiles, err := getFileOrDirectoryYAMLEntries(cfg.Files, cfg.DefaultDir)
	if err != nil {
		return ResolveBaseResult{}, err
	}

	if len(yamlFiles) == 0 {
		return ResolveBaseResult{}, fmt.Errorf("no files found in mint directory %q", cfg.DefaultDir)
	}

	result, err := s.resolveOrUpdateBaseForFiles(yamlFiles, BaseLayerSpec{}, true)
	s.outputLatestVersionMessage()
	if err != nil {
		return ResolveBaseResult{}, err
	}

	return result, nil
}

func (s Service) ResolveBase(cfg ResolveBaseConfig) (ResolveBaseResult, error) {
	err := cfg.Validate()
	if err != nil {
		return ResolveBaseResult{}, errors.Wrap(err, "validation failed")
	}

	yamlFiles, err := getFileOrDirectoryYAMLEntries(cfg.Files, cfg.DefaultDir)
	if err != nil {
		return ResolveBaseResult{}, err
	}

	if len(yamlFiles) == 0 {
		return ResolveBaseResult{}, fmt.Errorf("no files found in mint directory %q", cfg.DefaultDir)
	}

	requestedSpec := BaseLayerSpec{
		Os:   cfg.Os,
		Tag:  cfg.Tag,
		Arch: cfg.Arch,
	}

	result, err := s.resolveOrUpdateBaseForFiles(yamlFiles, requestedSpec, false)
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

func (s Service) getFilesForBaseResolveOrUpdate(mintFiles []api.MintDirectoryEntry, requestedSpec BaseLayerSpec, update bool) ([]BaseLayerRunFile, error) {
	runFiles := make([]BaseLayerRunFile, 0)

	for _, entry := range mintFiles {
		content, err := os.ReadFile(entry.OriginalPath)
		if err != nil {
			return nil, err
		}

		// JSON is valid YAML, but we don't support modifying it
		if isJSON(content) {
			continue
		}

		doc, err := ParseYamlDoc(string(content))
		if err != nil {
			// Skip files that are not valid YAML
			if isYAMLSyntaxError(err) {
				continue
			}
			return nil, err
		}

		// Skip files that don't have a 'tasks' key
		if !doc.HasTasks() {
			continue
		}

		// Skip files that already define a 'base' with at least 'os' and 'tag'
		if !update && doc.HasBase() && doc.TryReadStringAtPath("$.base.os") != "" && doc.TryReadStringAtPath("$.base.tag") != "" {
			continue
		}

		parsed := struct {
			Base BaseLayerSpec `yaml:"base"`
		}{}
		if err = yaml.Unmarshal(content, &parsed); err != nil {
			return nil, err
		}

		runFiles = append(runFiles, BaseLayerRunFile{
			Spec:     requestedSpec.Merge(parsed.Base),
			Filepath: entry.OriginalPath,
		})
	}

	return runFiles, nil
}

func (s Service) resolveOrUpdateBaseForFiles(mintFiles []api.MintDirectoryEntry, requestedSpec BaseLayerSpec, update bool) (ResolveBaseResult, error) {
	runFiles, err := s.getFilesForBaseResolveOrUpdate(mintFiles, requestedSpec, update)
	if err != nil {
		return ResolveBaseResult{}, err
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	errs, _ := errgroup.WithContext(ctx)
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

	doc, err := ParseYamlDoc(string(content))
	if err != nil {
		return err
	}

	if err := doc.InsertOrUpdateBase(resolvedBase); err != nil {
		return err
	}

	if !doc.HasChanges() {
		return nil
	}

	if err = file.Truncate(0); err != nil {
		return err
	}

	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return err
	}

	_, err = doc.WriteTo(file)
	return err
}

func (s Service) ResolveLeaves(cfg ResolveLeavesConfig) (ResolveLeavesResult, error) {
	err := cfg.Validate()
	if err != nil {
		return ResolveLeavesResult{}, errors.Wrap(err, "validation failed")
	}

	entries, err := getFileOrDirectoryYAMLEntries(cfg.Files, cfg.DefaultDir)
	if err != nil {
		return ResolveLeavesResult{}, err
	}

	if len(entries) == 0 {
		return ResolveLeavesResult{}, errors.New(fmt.Sprintf("no files provided, and no yaml files found in directory %s", cfg.DefaultDir))
	}

	mintFiles := filterYAMLFilesForModification(entries, func(doc *YAMLDoc) bool {
		return true
	})

	replacements, err := s.resolveOrUpdateLeavesForFiles(mintFiles, false, cfg.LatestVersionPicker)
	if err != nil {
		return ResolveLeavesResult{}, err
	}

	if len(replacements) == 0 {
		fmt.Fprintln(s.Stdout, "No leaves to resolve.")
	} else {
		fmt.Fprintln(s.Stdout, "Resolved the following leaves:")
		for leaf, version := range replacements {
			fmt.Fprintf(s.Stdout, "\t%s → %s\n", leaf, version)
		}
	}

	return ResolveLeavesResult{ResolvedLeaves: replacements}, nil
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

func tryGetSliceAtIndex[S ~[]E, E any](s S, index int, defaultValue E) E {
	if len(s) <= index {
		return defaultValue
	}
	return s[index]
}
