package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/rwx-research/mint-cli/internal/accesstoken"
	"github.com/rwx-research/mint-cli/internal/client"
	"github.com/rwx-research/mint-cli/internal/errors"

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
			if errors.Is(err, errors.ErrFileNotExists) {
				errMsg := "No run definitions provided!"

				if cfg.MintDirectory != ".mint" {
					errMsg = fmt.Sprintf("%s You specified --dir %s but the directory %s could not be found", errMsg, cfg.MintDirectory, cfg.MintDirectory)
				} else {
					errMsg = fmt.Sprintf("%s Add a run definition to your .mint directory, or use --file", errMsg)
				}

				return nil, errors.New(errMsg)
			}

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

// InitiateRun will connect to the Cloud API and start a new run in Mint.
func (s Service) Login(cfg LoginConfig) error {
	err := cfg.Validate()
	if err != nil {
		return errors.Wrap(err, "validation failed")
	}

	authCodeResult, err := s.APIClient.ObtainAuthCode(client.ObtainAuthCodeConfig{
		Code: client.ObtainAuthCodeCode{
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
