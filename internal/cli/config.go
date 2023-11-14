package cli

import "errors"

type Config struct {
	Client     MintClient
	FileSystem FileSystem
}

func (c Config) Validate() error {
	if c.Client == nil {
		// TODO: Custom error type here
		return errors.New("Missing Mint client")
	}

	if c.FileSystem == nil {
		// TODO: Custom error type here
		return errors.New("Missing file-system interface")
	}

	return nil
}

type InitiateRunConfig struct {
	InitParameters map[string]string
	MintDirectory  string
	MintFilePath   string
	NoCache        bool
}

func (c InitiateRunConfig) Validate() error {
	if c.MintDirectory == "" && c.MintFilePath == "" {
		// TODO: Custom error type here
		return errors.New("Either the mint directory or the mint config file path needs to be set")
	}

	return nil
}
