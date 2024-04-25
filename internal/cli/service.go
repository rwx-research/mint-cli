package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rwx-research/mint-cli/internal/accesstoken"
	"github.com/rwx-research/mint-cli/internal/api"
	"github.com/rwx-research/mint-cli/internal/dotenv"
	"github.com/rwx-research/mint-cli/internal/errors"

	"github.com/briandowns/spinner"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"
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

	mintDirectoryYamlPaths := make([]string, 0)
	taskDefinitionYamlPaths := make([]string, 0)

	mintDirectoryPath, err := s.findMintDirectoryPath(cfg.MintDirectory)
	if err != nil {
		return nil, errors.Wrap(err, "unable to find .mint directory")
	}

	// It's possible (when no directory is specified) that there is no .mint directory found during traversal
	if mintDirectoryPath != "" {
		paths, err := s.yamlFilePathsInDirectory(mintDirectoryPath)
		if err != nil {
			if errors.Is(err, errors.ErrFileNotExists) && cfg.MintDirectory != "" {
				return nil, fmt.Errorf("You specified --dir %q, but %q could not be found", cfg.MintDirectory, cfg.MintDirectory)
			}

			return nil, errors.Wrap(err, "unable to find yaml files in directory")
		}
		mintDirectoryYamlPaths = paths
	}

	// If a file is not specified, we need to use whatever the .mint directory is as the task definitions
	// (whether it was specified or found via traversal)
	if cfg.MintFilePath == "" {
		taskDefinitionYamlPaths = mintDirectoryYamlPaths
	} else {
		taskDefinitionYamlPaths = append(taskDefinitionYamlPaths, cfg.MintFilePath)
	}

	if len(taskDefinitionYamlPaths) == 0 {
		if cfg.MintDirectory != "" {
			return nil, fmt.Errorf("No run definitions provided! Add a run definition to %q, or use --file", cfg.MintDirectory)
		} else {
			return nil, errors.New("No run definitions provided! Add a run definition to your .mint directory, or use --file")
		}
	}

	mintDirectory, err := s.taskDefinitionsFromPaths(mintDirectoryYamlPaths)
	if err != nil {
		return nil, errors.Wrap(err, "unable to read provided files")
	}

	taskDefinitions, err := s.taskDefinitionsFromPaths(taskDefinitionYamlPaths)
	if err != nil {
		return nil, errors.Wrap(err, "unable to read provided files")
	}

	for _, taskDefinition := range mintDirectory {
		if err := validateYAML(taskDefinition.FileContents); err != nil {
			return nil, errors.Wrapf(err, "unable to parse %q", taskDefinition.Path)
		}
	}

	for _, taskDefinition := range taskDefinitions {
		if err := validateYAML(taskDefinition.FileContents); err != nil {
			return nil, errors.Wrapf(err, "unable to parse %q", taskDefinition.Path)
		}
	}

	// mintDirectory task definitions must have their paths relative to the .mint directory
	for i, taskDefinition := range mintDirectory {
		relPath, err := filepath.Rel(mintDirectoryPath, taskDefinition.Path)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to determine relative path of %q", taskDefinition.Path)
		}
		taskDefinition.Path = filepath.Join(".mint", relPath)
		mintDirectory[i] = taskDefinition
	}

	// A fully implicit invocation results in traversing the working directory for a .mint directory
	// When we find one, regardless of the distance, we use it as the task definitions
	// In that case, we want to make the paths relative to the working directory so it's clear where the
	// definitions are defined
	if cfg.MintDirectory == "" && cfg.MintFilePath == "" {
		wd, err := s.FileSystem.Getwd()
		if err != nil {
			return nil, errors.Wrap(err, "unable to determine the working directory")
		}

		for i, taskDefinition := range taskDefinitions {
			relPath, err := filepath.Rel(wd, taskDefinition.Path)
			if err != nil {
				return nil, errors.Wrapf(err, "unable to determine relative path of %q", taskDefinition.Path)
			}
			taskDefinition.Path = relPath
			taskDefinitions[i] = taskDefinition
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
		return nil, errors.Wrap(err, "unable to initiate run")
	}

	return runResult, nil
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
		fd, err := s.FileSystem.Open(cfg.File)
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
		yamlFilePathsInDirectory, err := s.yamlFilePathsInDirectory(cfg.DefaultDir)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("unable to find yaml files in directory %s", cfg.DefaultDir))
		}
		files = yamlFilePathsInDirectory
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
		fd, err := s.FileSystem.Open(path)
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
		fd, err := s.FileSystem.Open(path)
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

		fd, err = s.FileSystem.Create(path)
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
		fd, err := s.FileSystem.Open(path)
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

// yamlFilePathsInDirectory returns any *.yml and *.yaml files in a given directory, ignoring any sub-directories.
func (s Service) yamlFilePathsInDirectory(dir string) ([]string, error) {
	paths := make([]string, 0)

	files, err := s.FileSystem.ReadDir(dir)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to read %q", dir)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if !strings.HasSuffix(file.Name(), ".yml") && !strings.HasSuffix(file.Name(), ".yaml") {
			continue
		}

		paths = append(paths, filepath.Join(dir, file.Name()))
	}

	return paths, nil
}

// yamlFilePathsInDirectory returns any *.yml and *.yaml files in a given directory, ignoring any sub-directories.
func (s Service) findMintDirectoryPath(configuredDirectory string) (string, error) {
	if configuredDirectory != "" {
		return configuredDirectory, nil
	}

	workingDirectory, err := s.FileSystem.Getwd()
	if err != nil {
		return "", errors.Wrap(err, "unable to determine the working directory")
	}

	// otherwise, walk up the working directory looking at each basename
	for {
		workingDirHasMintDir, err := s.FileSystem.Exists(filepath.Join(workingDirectory, ".mint"))
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

// validateYAML checks whether a string can be parsed as YAML
func validateYAML(body string) error {
	contentMap := make(map[string]any)
	if err := yaml.Unmarshal([]byte(body), &contentMap); err != nil {
		return errors.WithStack(err)
	}

	return nil
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
