package cli

import (
	"io"

	"github.com/rwx-research/mint-cli/internal/client"
)

type Service struct {
	Config
}

func NewService(cfg Config) (Service, error) {
	if err := cfg.Validate(); err != nil {
		// TODO: Wrap
		return Service{}, err
	}

	return Service{cfg}, nil
}

func (s Service) InitiateRun(cfg InitiateRunConfig) error {
	if err := cfg.Validate(); err != nil {
		// TODO: Wrap
		return err
	}

	fd, err := s.FileSystem.Open(cfg.MintFilePath)
	if err != nil {
		// TODO: Wrap
		return err
	}
	defer fd.Close()

	fileContent, err := io.ReadAll(fd)
	if err != nil {
		// TODO: Wrap
		return err
	}

	// TODO: Wrap
	return s.Client.InitiateRun(client.InitiateRunConfig{TaskDefinitions: []client.TaskDefinition{{
		Path: cfg.MintFilePath,
		Body: string(fileContent),
	}}})
}
