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
	MintFilePath string
}

func (s InitiateRunConfig) Validate() error {
	if s.MintFilePath == "" {
		// TODO: Custom error type here
		return errors.New("Missing mint-file")
	}

	return nil
}
