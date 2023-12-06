package cli

import (
	"io"
	"path/filepath"
	"strings"

	"github.com/rwx-research/mint-cli/internal/client"

	"github.com/pkg/errors"
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

	runId := filepath.Base(cfg.RunURL)

	connectionInfo, err := s.APIClient.GetDebugConnectionInfo(runId)
	if err != nil {
		return errors.Wrapf(err, "unable to fetch connection info for run %s", runId)
	}

	privateUserKey, err := ssh.ParsePrivateKey(connectionInfo.PrivateUserKey)
	if err != nil {
		return errors.Wrapf(err, "unable to parse key material retrieved from Cloud API")
	}

	publicHostKey, err := ssh.NewPublicKey(connectionInfo.PublicHostKey)
	if err != nil {
		return errors.Wrapf(err, "unable to parse host key retrieved from Cloud API")
	}

	sshConfig := ssh.ClientConfig{
		User:            "mint-cli", // TODO: Add version number
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(privateUserKey)},
		HostKeyCallback: ssh.FixedHostKey(publicHostKey),
	}

	if err = s.SSHClient.Connect(connectionInfo.Address, sshConfig); err != nil {
		return errors.Wrap(err, "unable to establish SSH connection to remote host")
	}
	defer s.SSHClient.Close()

	if err := s.SSHClient.InteractiveSession(); err != nil {
		return errors.Wrap(err, "unable to start interactive session on remote host")
	}

	return nil
}

// InitiateRun will connect to the Cloud API and start a new run in Mint.
func (s Service) InitiateRun(cfg InitiateRunConfig) (*client.InitiateRunResult, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	// If a specific mint-file wasn't supplied over the CLI flags, fall back to reading the mint directory
	paths := []string{cfg.MintFilePath}
	if cfg.MintFilePath == "" {
		paths, err = s.yamlFilePathsInDirectory(cfg.MintDirectory)
		if err != nil {
			return nil, errors.Wrap(err, "unable to find yaml files in directory")
		}
	}

	if len(paths) == 0 {
		return nil, errors.Errorf("No run definitions provided! Add a run definition to your %s directory, or use `--file`", cfg.MintDirectory)
	}

	taskDefinitions, err := s.taskDefinitionsFromPaths(paths)
	if err != nil {
		return nil, errors.Wrap(err, "unable to read provided files")
	}

	for _, taskDefinition := range taskDefinitions {
		if err := validateYAML(taskDefinition.FileContents); err != nil {
			return nil, errors.Wrapf(err, "unable to parse %q", taskDefinition.Path)
		}
	}

	runResult, err := s.APIClient.InitiateRun(client.InitiateRunConfig{
		InitializationParameters: cfg.InitParameters,
		TaskDefinitions:          taskDefinitions,
		TargetedTaskKeys:         cfg.TargetedTasks,
		UseCache:                 !cfg.NoCache,
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to initiate run")
	}

	return runResult, nil
}

// taskDefinitionsFromPaths opens each file specified in `paths` and reads their content as a string.
// No validation takes place here.
func (s Service) taskDefinitionsFromPaths(paths []string) ([]client.TaskDefinition, error) {
	taskDefinitions := make([]client.TaskDefinition, 0)

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

		taskDefinitions = append(taskDefinitions, client.TaskDefinition{
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

// validateYAML checks whether a string can be parsed as YAML
func validateYAML(body string) error {
	contentMap := make(map[string]any)
	if err := yaml.Unmarshal([]byte(body), &contentMap); err != nil {
		return errors.WithStack(err)
	}

	return nil
}
