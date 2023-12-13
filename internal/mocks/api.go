package mocks

import (
	"github.com/rwx-research/mint-cli/internal/api"

	"github.com/pkg/errors"
)

type API struct {
	MockInitiateRun            func(api.InitiateRunConfig) (*api.InitiateRunResult, error)
	MockGetDebugConnectionInfo func(runID string) (api.DebugConnectionInfo, error)
	MockObtainAuthCode         func(api.ObtainAuthCodeConfig) (*api.ObtainAuthCodeResult, error)
	MockAcquireToken           func(tokenUrl string) (*api.AcquireTokenResult, error)
}

func (c *API) InitiateRun(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
	if c.MockInitiateRun != nil {
		return c.MockInitiateRun(cfg)
	}

	return nil, errors.New("MockInitiateRun was not configured")
}

func (c *API) GetDebugConnectionInfo(runID string) (api.DebugConnectionInfo, error) {
	if c.MockGetDebugConnectionInfo != nil {
		return c.MockGetDebugConnectionInfo(runID)
	}

	return api.DebugConnectionInfo{}, errors.New("MockGetDebugConnectionInfo was not configured")
}

func (c *API) ObtainAuthCode(cfg api.ObtainAuthCodeConfig) (*api.ObtainAuthCodeResult, error) {
	if c.MockObtainAuthCode != nil {
		return c.MockObtainAuthCode(cfg)
	}

	return nil, errors.New("MockObtainAuthCode was not configured")
}

func (c *API) AcquireToken(tokenUrl string) (*api.AcquireTokenResult, error) {
	if c.MockAcquireToken != nil {
		return c.MockAcquireToken(tokenUrl)
	}

	return nil, errors.New("MockAcquireToken was not configured")
}
