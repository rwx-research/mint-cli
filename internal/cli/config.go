package cli

import (
	"io"

	"github.com/rwx-research/mint-cli/internal/accesstoken"
	"github.com/rwx-research/mint-cli/internal/errors"
	"github.com/rwx-research/mint-cli/internal/fs"
)

type Config struct {
	APIClient  APIClient
	FileSystem fs.FileSystem
	SSHClient  SSHClient
}

func (c Config) Validate() error {
	if c.APIClient == nil {
		return errors.New("missing Mint client")
	}

	if c.FileSystem == nil {
		return errors.New("missing file-system interface")
	}

	if c.SSHClient == nil {
		return errors.New("missing SSH client constructor")
	}

	return nil
}

type DebugTaskConfig struct {
	RunURL string
}

func (c DebugTaskConfig) Validate() error {
	if c.RunURL == "" {
		return errors.New("missing Mint run URL")
	}

	return nil
}

type InitiateRunConfig struct {
	InitParameters map[string]string
	Json           bool
	MintDirectory  string
	MintFilePath   string
	NoCache        bool
	TargetedTasks  []string
	Title          string
}

func (c InitiateRunConfig) Validate() error {
	return nil
}

type LoginConfig struct {
	DeviceName         string
	AccessTokenBackend accesstoken.Backend
	Stdout             io.Writer
	OpenUrl            func(url string) error
}

func (c LoginConfig) Validate() error {
	if c.DeviceName == "" {
		return errors.New("the device name must be provided")
	}

	return nil
}

type WhoamiConfig struct {
	Json   bool
	Stdout io.Writer
}

func (c WhoamiConfig) Validate() error {
	return nil
}

type SetSecretsInVaultConfig struct {
	Secrets []string
	Vault   string
	File    string
	Stdout  io.Writer
}

func (c SetSecretsInVaultConfig) Validate() error {
	if c.Vault == "" {
		return errors.New("the vault name must be provided")
	}

	if len(c.Secrets) == 0 && c.File == "" {
		return errors.New("the secrets to set must be provided")
	}

	return nil
}
