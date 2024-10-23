package cli

import (
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

	"github.com/briandowns/spinner"
	"golang.org/x/crypto/ssh"
)

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

	privateUserKey, err := ssh.ParsePrivateKey([]byte(connectionInfo.PrivateUserKey))
	if err != nil {
		return errors.Wrapf(err, "unable to parse key material retrieved from Cloud API")
	}

	rawPublicHostKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(connectionInfo.PublicHostKey))
	if err != nil {
		return errors.Wrapf(err, "unable to parse host key retrieved from Cloud API")
	}

	publicHostKey, err := ssh.ParsePublicKey(rawPublicHostKey.Marshal())
	if err != nil {
		return errors.Wrapf(err, "unable to parse host key retrieved from Cloud API")
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
	if err != nil {
		return nil, errors.Wrap(err, "unable to lint files")
	}

	switch cfg.OutputFormat {
	case LintOutputOneLine:
		err = outputLintOneLine(cfg.Output, lintResult.Problems)
	case LintOutputMultiLine:
		err = outputLintMultiLine(cfg.Output, lintResult.Problems, len(targetPaths))
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

// InitiateRun will connect to the Cloud API and start a new run in Mint.
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

	fmt.Fprintln(cfg.Stdout)
	fmt.Fprintln(cfg.Stdout, "To authorize this device, you'll need to login to RWX Cloud and choose an organization.")
	fmt.Fprintln(cfg.Stdout, "Your browser should automatically open. If it does not, you can visit this URL:")
	fmt.Fprintln(cfg.Stdout)
	fmt.Fprintf(cfg.Stdout, "\t%v\n", authCodeResult.AuthorizationUrl)
	fmt.Fprintln(cfg.Stdout)
	fmt.Fprintln(cfg.Stdout, "Once authorized, a personal access token will be generated and stored securely on this device.")
	fmt.Fprintln(cfg.Stdout)

	indicator := spinner.New(spinner.CharSets[11], 100*time.Millisecond, spinner.WithWriter(cfg.Stdout))
	indicator.Suffix = " Waiting for authorization..."
	indicator.Start()

	for {
		tokenResult, err := s.APIClient.AcquireToken(authCodeResult.TokenUrl)
		if err != nil {
			indicator.Stop()
			return errors.Wrap(err, "unable to acquire the token")
		}

		switch tokenResult.State {
		case "consumed":
			indicator.Stop()
			return errors.New("The code has already been used. Try again.")
		case "expired":
			indicator.Stop()
			return errors.New("The code has expired. Try again.")
		case "authorized":
			indicator.Stop()
			if tokenResult.Token == "" {
				return errors.New("The code has been authorized, but there is no token. You can try again, but this is likely an issue with RWX Cloud. Please reach out at support@rwx.com.")
			} else {
				fmt.Fprint(cfg.Stdout, "Authorized!\n")
				return accesstoken.Set(cfg.AccessTokenBackend, tokenResult.Token)
			}
		case "pending":
			time.Sleep(1 * time.Second)
		default:
			indicator.Stop()
			return errors.New("The code is in an unexpected state. You can try again, but this is likely an issue with RWX Cloud. Please reach out at support@rwx.com.")
		}
	}
}

func (s Service) Whoami(cfg WhoamiConfig) error {
	result, err := s.APIClient.Whoami()
	if err != nil {
		return errors.Wrap(err, "unable to determine details about the access token")
	}

	if cfg.Json {
		encoded, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return errors.Wrap(err, "unable to JSON encode the result")
		}

		fmt.Fprint(cfg.Stdout, string(encoded))
	} else {
		fmt.Fprintf(cfg.Stdout, "Token Kind: %v\n", strings.ReplaceAll(result.TokenKind, "_", " "))
		fmt.Fprintf(cfg.Stdout, "Organization: %v\n", result.OrganizationSlug)
		if result.UserEmail != nil {
			fmt.Fprintf(cfg.Stdout, "User: %v\n", *result.UserEmail)
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

	if result != nil && len(result.SetSecrets) > 0 {
		fmt.Fprintln(cfg.Stdout)
		fmt.Fprintf(cfg.Stdout, "Successfully set the following secrets: %s", strings.Join(result.SetSecrets, ", "))
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
	if err != nil {
		return errors.Wrap(err, "unable to fetch leaf versions")
	}

	replacements := make(map[string]string)
	for leaf, majorVersions := range leafReferences {
		for majorVersion, references := range majorVersions {
			targetLeafVersion, err := cfg.ReplacementVersionPicker(*leafVersions, leaf, majorVersion)
			if err != nil {
				fmt.Fprintln(cfg.Stderr, err.Error())
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

	err = s.replaceInFiles(files, replacements)
	if err != nil {
		return errors.Wrap(err, "unable to replace leaf references")
	}

	if len(replacements) == 0 {
		fmt.Fprintln(cfg.Stdout, "No leaves to update.")
	} else {
		fmt.Fprintln(cfg.Stdout, "Updated the following leaves:")
		for original, replacement := range replacements {
			replacementParts := strings.Split(replacement, " ")
			if len(replacementParts) == 2 {
				fmt.Fprintf(cfg.Stdout, "\t%s → %s\n", original, replacementParts[1])
			} else {
				fmt.Fprintf(cfg.Stdout, "\t%s → %s\n", original, replacement)
			}
		}
	}

	return nil
}

var reLeaf = regexp.MustCompile(`([a-z0-9-]+\/[a-z0-9-]+) ([0-9]+)\.[0-9]+\.[0-9]+`)

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
			fullMatch := string(match[0])
			leaf := string(match[1])
			majorVersion := string(match[2])

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

func (s Service) replaceInFiles(files []string, replacements map[string]string) error {
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
