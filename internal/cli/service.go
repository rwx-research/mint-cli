package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rwx-research/mint-cli/internal/accesstoken"
	"github.com/rwx-research/mint-cli/internal/api"
	"github.com/rwx-research/mint-cli/internal/dotenv"
	"github.com/rwx-research/mint-cli/internal/errors"
	"github.com/rwx-research/mint-cli/internal/messages"
	"github.com/rwx-research/mint-cli/internal/versions"

	"github.com/briandowns/spinner"
	"github.com/goccy/go-yaml/ast"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

const DefaultArch = "x86_64"

var HandledError = errors.New("handled error")
var hasOutputVersionMessage atomic.Bool

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
	defer s.outputLatestVersionMessage()
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
	defer s.outputLatestVersionMessage()
	err := cfg.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	var mintDirectory []MintDirectoryEntry
	runDefinitionPath := cfg.MintFilePath

	mintDirectoryPath, err := findAndValidateMintDirectoryPath(cfg.MintDirectory)
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

		mintDirectory = mintDirectoryEntries
	}

	runDefinition, err := mintDirectoryEntriesFromPaths([]string{runDefinitionPath})
	if err != nil {
		return nil, errors.Wrap(err, "unable to read provided files")
	}
	runDefinition = filterFiles(runDefinition)
	if len(runDefinition) != 1 {
		return nil, fmt.Errorf("expected exactly 1 run definition, got %d", len(runDefinition))
	}

	// reloadRunDefinitions reloads run definitions after modifying the file.
	reloadRunDefinitions := func() error {
		runDefinition, err = mintDirectoryEntriesFromPaths([]string{runDefinitionPath})
		if err != nil {
			return errors.Wrapf(err, "unable to reload %q", runDefinitionPath)
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

	addBaseIfNeeded, err := s.resolveOrUpdateBaseForFiles(runDefinition, BaseLayerSpec{}, false)
	if err != nil {
		return nil, errors.Wrap(err, "unable to resolve base")
	}

	if addBaseIfNeeded.HasChanges() {
		update := addBaseIfNeeded.UpdatedRunFiles[0]
		if update.ResolvedBase.Os == "" {
			return nil, errors.New("unable to determine OS")
		}

		fmt.Fprintf(s.Stderr, "Configured %q to run on %s\n\n", runDefinitionPath, update.ResolvedBase.Os)

		if err = reloadRunDefinitions(); err != nil {
			return nil, err
		}
	}

	mintFiles := filterYAMLFilesForModification(runDefinition, func(doc *YAMLDoc) bool {
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
		TaskDefinitions:          runDefinition,
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
	defer s.outputLatestVersionMessage()
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
	if err != nil {
		return nil, errors.Wrap(err, "Failed to initiate dispatch")
	}

	return dispatchResult, nil
}

func (s Service) GetDispatch(cfg GetDispatchConfig) ([]GetDispatchRun, error) {
	defer s.outputLatestVersionMessage()
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
	defer s.outputLatestVersionMessage()
	err := cfg.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	targetedEntries, err := mintDirectoryEntriesFromPaths(cfg.MintFilePaths)
	if err != nil {
		return nil, err
	}
	targetedEntries = filterYAMLFiles(targetedEntries)
	targetedEntries = removeDuplicates(targetedEntries, func(entry MintDirectoryEntry) string {
		return entry.Path
	})

	mintDirectoryPath, err := findAndValidateMintDirectoryPath(cfg.MintDirectory)
	if err != nil {
		return nil, errors.Wrap(err, "unable to find .mint directory")
	}

	var mintDirEntries []MintDirectoryEntry
	if mintDirectoryPath != "" {
		mdEntries, err := mintDirectoryEntries(mintDirectoryPath)
		if err != nil {
			return nil, err
		}

		// Ensure both the provided paths and everything in the MintDirectory is loaded.
		mdEntries = filterYAMLFiles(mdEntries)
		mdEntries = removeDuplicates(mdEntries, func(entry MintDirectoryEntry) string {
			return entry.Path
		})

		for _, entry := range mdEntries {
			// Don't duplicate targeted files that are also in .mint
			if slices.ContainsFunc(targetedEntries, func(te MintDirectoryEntry) bool {
				return te.Path == entry.Path
			}) {
				continue
			}
			mintDirEntries = append(mintDirEntries, entry)
		}
	}

	definitionEntries := append(targetedEntries, mintDirEntries...)

	// When no files are targeted, lint all .mint files
	if len(cfg.MintFilePaths) == 0 && len(mintDirEntries) > 0 {
		targetedEntries = mintDirEntries
	}

	taskDefinitions := Map(definitionEntries, func(entry MintDirectoryEntry) TaskDefinition {
		return TaskDefinition{
			Path:         entry.Path,
			FileContents: entry.FileContents,
		}
	})

	targetedPaths := Map(targetedEntries, func(entry MintDirectoryEntry) string {
		return entry.Path
	})

	if len(cfg.MintFilePaths) > 0 {
		_, snippetFileNames := findSnippets(targetedPaths)
		if len(snippetFileNames) > 0 {
			return nil, fmt.Errorf("You cannot target snippets for linting, but you targeted the following snippets: %s\n\nTo lint snippets, include them from a Mint run definition and lint the run definition.", strings.Join(snippetFileNames, ", "))
		}
	} else {
		nonSnippetFileNames, _ := findSnippets(targetedPaths)
		targetedPaths = nonSnippetFileNames
	}

	lintResult, err := s.APIClient.Lint(api.LintConfig{
		TaskDefinitions: taskDefinitions,
		TargetPaths:     targetedPaths,
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to lint files")
	}

	switch cfg.OutputFormat {
	case LintOutputOneLine:
		err = outputLintOneLine(s.Stdout, lintResult.Problems)
	case LintOutputMultiLine:
		err = outputLintMultiLine(s.Stdout, lintResult.Problems, len(targetedPaths))
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
	defer s.outputLatestVersionMessage()
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

	if result != nil && len(result.SetSecrets) > 0 {
		fmt.Fprintln(s.Stdout)
		fmt.Fprintf(s.Stdout, "Successfully set the following secrets: %s", strings.Join(result.SetSecrets, ", "))
	}

	if err != nil {
		return errors.Wrap(err, "unable to set secrets")
	}

	return nil
}

func (s Service) ResolveLeaves(cfg ResolveLeavesConfig) (ResolveLeavesResult, error) {
	err := cfg.Validate()
	if err != nil {
		return ResolveLeavesResult{}, errors.Wrap(err, "validation failed")
	}

	mintDirectoryPath, err := findAndValidateMintDirectoryPath(cfg.MintDirectory)
	if err != nil {
		return ResolveLeavesResult{}, errors.Wrap(err, "unable to find .mint directory")
	}

	yamlFiles, err := getFileOrDirectoryYAMLEntries(cfg.Files, mintDirectoryPath)
	if err != nil {
		return ResolveLeavesResult{}, err
	}

	if len(yamlFiles) == 0 {
		return ResolveLeavesResult{}, fmt.Errorf("no files provided, and no yaml files found in directory %s", mintDirectoryPath)
	}

	mintFiles := filterYAMLFilesForModification(yamlFiles, func(doc *YAMLDoc) bool {
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

func (s Service) UpdateLeaves(cfg UpdateLeavesConfig) error {
	defer s.outputLatestVersionMessage()
	err := cfg.Validate()
	if err != nil {
		return errors.Wrap(err, "validation failed")
	}

	mintDirectoryPath, err := findAndValidateMintDirectoryPath(cfg.MintDirectory)
	if err != nil {
		return errors.Wrap(err, "unable to find .mint directory")
	}

	yamlFiles, err := getFileOrDirectoryYAMLEntries(cfg.Files, mintDirectoryPath)
	if err != nil {
		return err
	}

	if len(yamlFiles) == 0 {
		return errors.New(fmt.Sprintf("no files provided, and no yaml files found in directory %s", mintDirectoryPath))
	}

	mintFiles := filterYAMLFilesForModification(yamlFiles, func(doc *YAMLDoc) bool {
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
	if err != nil {
		return nil, errors.Wrap(err, "unable to fetch leaf versions")
	}

	docs := make(map[string]*YAMLDoc)
	replacements := make(map[string]string)

	for _, file := range mintFiles {
		hasChange := false
		err = file.Doc.ForEachNode("$.tasks[*].call", func(node ast.Node) error {
			leafVersion := s.parseLeafVersion(node.String())
			if leafVersion.Name == "" {
				// Leaves won't be found for eg. embedded runs, call: ${{ run.mint-dir }}/embed.yml
				return nil
			} else if !update && leafVersion.MajorVersion != "" {
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

func (s Service) ResolveBase(cfg ResolveBaseConfig) (ResolveBaseResult, error) {
	defer s.outputLatestVersionMessage()
	err := cfg.Validate()
	if err != nil {
		return ResolveBaseResult{}, errors.Wrap(err, "validation failed")
	}

	mintDirectoryPath, err := findAndValidateMintDirectoryPath(cfg.MintDirectory)
	if err != nil {
		return ResolveBaseResult{}, errors.Wrap(err, "unable to find .mint directory")
	}

	yamlFiles, err := getFileOrDirectoryYAMLEntries(cfg.Files, mintDirectoryPath)
	if err != nil {
		return ResolveBaseResult{}, err
	}

	if len(yamlFiles) == 0 {
		return ResolveBaseResult{}, fmt.Errorf("no files provided, and no yaml files found in directory %s", mintDirectoryPath)
	}

	requestedSpec := BaseLayerSpec{
		Os:   cfg.Os,
		Tag:  cfg.Tag,
		Arch: cfg.Arch,
	}

	result, err := s.resolveOrUpdateBaseForFiles(yamlFiles, requestedSpec, false)
	if err != nil {
		return ResolveBaseResult{}, err
	}

	if len(yamlFiles) == 0 {
		fmt.Fprintf(s.Stdout, "No run files found in %q.\n", cfg.MintDirectory)
	} else if !result.HasChanges() {
		fmt.Fprintln(s.Stdout, "No run files were missing base.")
	} else {
		if len(result.UpdatedRunFiles) > 0 {
			fmt.Fprintln(s.Stdout, "Added base to the following run definitions:")
			for _, runFile := range result.UpdatedRunFiles {
				fmt.Fprintf(s.Stdout, "\t%s → %s, tag %s\n", relativePathFromWd(runFile.OriginalPath), runFile.ResolvedBase.Os, runFile.ResolvedBase.Tag)
			}
			if len(result.ErroredRunFiles) > 0 {
				fmt.Fprintln(s.Stdout)
			}
		}

		if len(result.ErroredRunFiles) > 0 {
			fmt.Fprintln(s.Stdout, "Failed to add base to the following run definitions:")
			for _, runFile := range result.ErroredRunFiles {
				fmt.Fprintf(s.Stdout, "\t%s → %s\n", relativePathFromWd(runFile.OriginalPath), runFile.Error)
			}
		}
	}

	return result, nil
}

func (s Service) UpdateBase(cfg UpdateBaseConfig) (ResolveBaseResult, error) {
	defer s.outputLatestVersionMessage()
	err := cfg.Validate()
	if err != nil {
		return ResolveBaseResult{}, errors.Wrap(err, "validation failed")
	}

	mintDirectoryPath, err := findAndValidateMintDirectoryPath(cfg.MintDirectory)
	if err != nil {
		return ResolveBaseResult{}, errors.Wrap(err, "unable to find .mint directory")
	}

	yamlFiles, err := getFileOrDirectoryYAMLEntries(cfg.Files, mintDirectoryPath)
	if err != nil {
		return ResolveBaseResult{}, err
	}

	if len(yamlFiles) == 0 {
		errmsg := "no files provided, and no yaml files found"
		if mintDirectoryPath != "" {
			errmsg = fmt.Sprintf("%s in directory %s", errmsg, mintDirectoryPath)
		}

		return ResolveBaseResult{}, errors.New(errmsg)
	}

	result, err := s.resolveOrUpdateBaseForFiles(yamlFiles, BaseLayerSpec{}, true)
	if err != nil {
		return ResolveBaseResult{}, err
	}

	if !result.HasChanges() {
		fmt.Fprintln(s.Stdout, "No run bases to update.")
	} else {
		if len(result.UpdatedRunFiles) > 0 {
			fmt.Fprintln(s.Stdout, "Updated base for the following run definitions:")
			for _, runFile := range result.UpdatedRunFiles {
				if runFile.Spec.Tag != "" {
					fmt.Fprintf(s.Stdout, "\t%s tag %s → tag %s\n", relativePathFromWd(runFile.OriginalPath), runFile.OriginalBase.Tag, runFile.ResolvedBase.Tag)
				} else {
					fmt.Fprintf(s.Stdout, "\t%s → tag %s\n", relativePathFromWd(runFile.OriginalPath), runFile.ResolvedBase.Tag)
				}
				if len(result.ErroredRunFiles) > 0 {
					fmt.Println()
				}
			}
		}

		if len(result.ErroredRunFiles) > 0 {
			fmt.Fprintln(s.Stdout, "Failed to updated base for the following run definitions:")
			for _, runFile := range result.ErroredRunFiles {
				fmt.Fprintf(s.Stdout, "\t%s → %s\n", relativePathFromWd(runFile.OriginalPath), runFile.Error)
			}
		}
	}

	return result, nil
}

func (s Service) resolveOrUpdateBaseForFiles(mintFiles []MintDirectoryEntry, requestedSpec BaseLayerSpec, update bool) (ResolveBaseResult, error) {
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
		resolvedBase, found := specToResolved[runFile.Spec]
		if !found {
			continue
		}
		runFile.ResolvedBase = resolvedBase

		err := s.writeRunFileWithBase(runFile)
		if err != nil {
			runFile.Error = err
			erroredRunFiles = append(erroredRunFiles, runFile)
		} else if runFile.HasChanges() {
			updatedRunFiles = append(updatedRunFiles, runFile)
		}
	}

	return ResolveBaseResult{
		ErroredRunFiles: erroredRunFiles,
		UpdatedRunFiles: updatedRunFiles,
	}, nil
}

func (s Service) getFilesForBaseResolveOrUpdate(entries []MintDirectoryEntry, requestedSpec BaseLayerSpec, update bool) ([]BaseLayerRunFile, error) {
	yamlFiles := filterYAMLFilesForModification(entries, func(doc *YAMLDoc) bool {
		if !doc.HasTasks() {
			return false
		}

		// Skip files that already define a 'base' with at least 'os' and 'tag'
		if !update && doc.HasBase() && doc.TryReadStringAtPath("$.base.os") != "" && doc.TryReadStringAtPath("$.base.tag") != "" {
			return false
		}

		return true
	})

	runFiles := make([]BaseLayerRunFile, 0)
	for _, yamlFile := range yamlFiles {
		spec := BaseLayerSpec{
			Os:   yamlFile.Doc.TryReadStringAtPath("$.base.os"),
			Tag:  yamlFile.Doc.TryReadStringAtPath("$.base.tag"),
			Arch: yamlFile.Doc.TryReadStringAtPath("$.base.arch"),
		}

		runFiles = append(runFiles, BaseLayerRunFile{
			OriginalBase: spec,
			Spec:         requestedSpec.Merge(spec),
			OriginalPath: yamlFile.Entry.OriginalPath,
		})
	}

	return runFiles, nil
}

func extractMajorVersion(v string) string {
	parts := strings.Split(v, ".")
	if len(parts) > 1 {
		return parts[0]
	}
	return v
}

func flattenPathMap(pathMap map[string][]string) []string {
	var result []string
	for _, paths := range pathMap {
		result = append(result, paths...)
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func (s Service) logUnknownBaseTag(tag string, paths []string) {
	paths = Map(paths, func(p string) string {
		return relativePathFromWd(p)
	})
	fmt.Fprintf(s.Stderr, "Unknown base tag %s for run definitions: %s\n",
		tag, strings.Join(paths, ", "))
}

func (s Service) resolveBaseSpecs(runFiles []BaseLayerRunFile) (map[BaseLayerSpec]BaseLayerSpec, error) {
	// Group run files by unique specs to minimize API calls
	type specGroup struct {
		OriginalBases map[BaseLayerSpec]struct{}
		RunFilePaths  map[string][]string
		ResolvedSpec  BaseLayerSpec
	}

	// Maps normalized specs (what we'll resolve) to their group data
	specGroups := make(map[BaseLayerSpec]*specGroup)

	// Maps original specs to their normalized form for lookup.
	// This is the original _spec_, not the _original base_, which is
	// an important distinction when defaults are provided via CLI args.
	originalToNormalized := make(map[BaseLayerSpec]BaseLayerSpec)

	// Group by normalized specs
	for _, runFile := range runFiles {
		normalizedSpec := runFile.Spec
		normalizedSpec.Tag = extractMajorVersion(normalizedSpec.Tag)

		originalToNormalized[runFile.Spec] = normalizedSpec

		// Update or create the spec group
		group, exists := specGroups[normalizedSpec]
		if !exists {
			group = &specGroup{
				OriginalBases: make(map[BaseLayerSpec]struct{}),
				RunFilePaths:  make(map[string][]string),
			}
			specGroups[normalizedSpec] = group
		}

		// Add the original base
		group.OriginalBases[runFile.OriginalBase] = struct{}{}

		// Group paths by original tag for better error reporting
		originalTag := runFile.OriginalBase.Tag
		group.RunFilePaths[originalTag] = append(group.RunFilePaths[originalTag], runFile.OriginalPath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	errs, ctx := errgroup.WithContext(ctx)
	errs.SetLimit(3)

	// Process each unique spec
	var mu sync.Mutex
	for normalizedSpec, group := range specGroups {
		errs.Go(func() error {
			result, err := s.APIClient.ResolveBaseLayer(api.ResolveBaseLayerConfig{
				Os:   normalizedSpec.Os,
				Arch: normalizedSpec.Arch,
				Tag:  normalizedSpec.Tag,
			})

			if err != nil {
				if errors.Is(err, api.ErrNotFound) {
					// For not found errors, we report all paths in the group but don't error out
					allPaths := flattenPathMap(group.RunFilePaths)
					s.logUnknownBaseTag(normalizedSpec.Tag, allPaths)
					return nil
				}
				return errors.Wrapf(err, "unable to resolve base layer %+v", normalizedSpec)
			}

			resolvedSpec := BaseLayerSpec{
				Os:   result.Os,
				Tag:  result.Tag,
				Arch: result.Arch,
			}

			mu.Lock()
			defer mu.Unlock()
			group.ResolvedSpec = resolvedSpec

			// Check each original base against the resolved version
			for origBase := range maps.Keys(group.OriginalBases) {
				// Only compare versions if they're in the same major version group
				if extractMajorVersion(origBase.Tag) == extractMajorVersion(resolvedSpec.Tag) {
					if origBase.TagVersion().GreaterThan(resolvedSpec.TagVersion()) {
						// Report the specific tag that wasn't found
						paths := group.RunFilePaths[origBase.Tag]
						s.logUnknownBaseTag(origBase.Tag, paths)

						// Don't modify the resolved base (eg. don't downgrade 1.2 -> 1.1)
						delete(group.OriginalBases, origBase)
						originalToNormalized[origBase] = origBase
					}
				}
			}

			return nil
		})
	}

	if err := errs.Wait(); err != nil {
		return nil, err
	}

	originalToResolved := make(map[BaseLayerSpec]BaseLayerSpec, len(runFiles))
	for originalBase, normalizedSpec := range originalToNormalized {
		group := specGroups[normalizedSpec]
		// If resolution failed, don't add to the result
		if group != nil && group.ResolvedSpec != (BaseLayerSpec{}) {
			originalToResolved[originalBase] = group.ResolvedSpec
		}
	}

	return originalToResolved, nil
}

func (s Service) writeRunFileWithBase(runFile BaseLayerRunFile) error {
	doc, err := ParseYAMLFile(runFile.OriginalPath)
	if err != nil {
		return err
	}

	resolvedBase := runFile.ResolvedBase
	base := map[string]any{
		"os": resolvedBase.Os,
	}

	// Prevent unnecessary quoting of float-like tags, eg. 1.2
	if strings.Count(resolvedBase.Tag, ".") == 1 {
		parsedTag, err := strconv.ParseFloat(resolvedBase.Tag, 64)
		if err != nil {
			return err
		}
		base["tag"] = parsedTag
	} else {
		base["tag"] = resolvedBase.Tag
	}

	if resolvedBase.Arch != "" && resolvedBase.Arch != DefaultArch {
		base["arch"] = resolvedBase.Arch
	}

	if !doc.HasBase() {
		err = doc.InsertBefore("$.tasks", map[string]any{
			"base": base,
		})
		if err != nil {
			return err
		}
	} else {
		if err = doc.MergeAtPath("$.base", base); err != nil {
			return err
		}
	}

	if !doc.HasChanges() {
		return nil
	}

	return doc.WriteFile(runFile.OriginalPath)
}

func (s Service) outputLatestVersionMessage() {
	if !versions.HasCliLatestVersion() {
		return
	}

	if !hasOutputVersionMessage.CompareAndSwap(false, true) {
		return
	}

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

func Map[T any, R any](input []T, transformer func(T) R) []R {
	result := make([]R, len(input))
	for i, item := range input {
		result[i] = transformer(item)
	}
	return result
}

func tryGetSliceAtIndex[S ~[]E, E any](s S, index int, defaultValue E) E {
	if len(s) <= index {
		return defaultValue
	}
	return s[index]
}
