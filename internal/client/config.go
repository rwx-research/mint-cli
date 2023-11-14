package client

import "errors"

type Config struct {
	Host        string
	AccessToken string
}

func (c Config) Validate() error {
	if c.AccessToken == "" {
		// TODO: Custom error type here
		return errors.New("Missing access-token")
	}

	if c.Host == "" {
		// TODO: Custom error type here
		return errors.New("Missing host")
	}

	return nil
}

type InitiateRunConfig struct {
	TaskDefinitions []TaskDefinition
	UseCache        bool
}

func (c InitiateRunConfig) Validate() error {
	if len(c.TaskDefinitions) == 0 {
		// TODO: Custom error type here
		return errors.New("No task definitions")
	}

	return nil
}
