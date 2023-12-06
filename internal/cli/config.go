package cli

import "github.com/pkg/errors"

type Config struct {
	APIClient  APIClient
	FileSystem FileSystem
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
}

func (c InitiateRunConfig) Validate() error {
	if c.MintDirectory == "" && c.MintFilePath == "" {
		return errors.New("either the mint directory or the mint config file path needs to be set")
	}

	return nil
}
