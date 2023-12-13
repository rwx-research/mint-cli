package api

import (
	"github.com/pkg/errors"
	"github.com/rwx-research/mint-cli/internal/accesstoken"
)

type Config struct {
	Host               string
	AccessToken        string
	AccessTokenBackend accesstoken.Backend
}

func (c Config) Validate() error {
	if c.Host == "" {
		return errors.New("missing host")
	}

	return nil
}

type InitiateRunConfig struct {
	InitializationParameters map[string]string
	TaskDefinitions          []TaskDefinition
	TargetedTaskKeys         []string `json:",omitempty"`
	UseCache                 bool
}

type InitiateRunResult struct {
	RunId            string
	RunURL           string
	TargetedTaskKeys []string
	DefinitionPath   string
}

func (c InitiateRunConfig) Validate() error {
	if len(c.TaskDefinitions) == 0 {
		return errors.New("no task definitions")
	}

	return nil
}

type ObtainAuthCodeConfig struct {
	Code ObtainAuthCodeCode `json:"code"`
}

type ObtainAuthCodeCode struct {
	DeviceName string `json:"device_name"`
}

type ObtainAuthCodeResult struct {
	AuthorizationUrl string `json:"authorization_url"`
	TokenUrl         string `json:"token_url"`
}

func (c ObtainAuthCodeConfig) Validate() error {
	if c.Code.DeviceName == "" {
		return errors.New("device name must be provided")
	}

	return nil
}

type AcquireTokenResult struct {
	State string `json:"state"` // consumed, expired, authorized, pending
	Token string `json:"token,omitempty"`
}
