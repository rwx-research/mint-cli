package cli

import "github.com/pkg/errors"

type Config struct {
	Client     MintClient
	FileSystem FileSystem
}

func (c Config) Validate() error {
	if c.Client == nil {
		return errors.New("missing Mint client")
	}

	if c.FileSystem == nil {
		return errors.New("missing file-system interface")
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
