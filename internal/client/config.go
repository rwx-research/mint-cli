package client

import "github.com/pkg/errors"

type Config struct {
	Host        string
	AccessToken string
}

func (c Config) Validate() error {
	if c.AccessToken == "" {
		return errors.New("Missing access-token")
	}

	if c.Host == "" {
		return errors.New("Missing host")
	}

	return nil
}

type InitiateRunConfig struct {
	InitializationParameters map[string]string
	TaskDefinitions          []TaskDefinition
	TargetedTaskKey          string
	UseCache                 bool
}

func (c InitiateRunConfig) Validate() error {
	if len(c.TaskDefinitions) == 0 {
		return errors.New("No task definitions")
	}

	return nil
}
