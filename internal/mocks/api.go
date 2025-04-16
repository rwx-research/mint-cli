package mocks

import (
	"github.com/rwx-research/mint-cli/internal/api"
	"github.com/rwx-research/mint-cli/internal/errors"
)

type API struct {
	MockInitiateRun            func(api.InitiateRunConfig) (*api.InitiateRunResult, error)
	MockGetDebugConnectionInfo func(runID string) (api.DebugConnectionInfo, error)
	MockObtainAuthCode         func(api.ObtainAuthCodeConfig) (*api.ObtainAuthCodeResult, error)
	MockAcquireToken           func(tokenUrl string) (*api.AcquireTokenResult, error)
	MockWhoami                 func() (*api.WhoamiResult, error)
	MockSetSecretsInVault      func(api.SetSecretsInVaultConfig) (*api.SetSecretsInVaultResult, error)
	MockGetLeafVersions        func() (*api.LeafVersionsResult, error)
	MockLint                   func(api.LintConfig) (*api.LintResult, error)
	MockInitiateDispatch       func(api.InitiateDispatchConfig) (*api.InitiateDispatchResult, error)
	MockGetDispatch            func(api.GetDispatchConfig) (*api.GetDispatchResult, error)
	MockResolveBaseLayer       func(api.ResolveBaseLayerConfig) (api.ResolveBaseLayerResult, error)
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

func (c *API) Whoami() (*api.WhoamiResult, error) {
	if c.MockWhoami != nil {
		return c.MockWhoami()
	}

	return nil, errors.New("MockWhoami was not configured")
}

func (c *API) SetSecretsInVault(cfg api.SetSecretsInVaultConfig) (*api.SetSecretsInVaultResult, error) {
	if c.MockSetSecretsInVault != nil {
		return c.MockSetSecretsInVault(cfg)
	}

	return nil, errors.New("MockSetSecretsInVault was not configured")
}

func (c *API) GetLeafVersions() (*api.LeafVersionsResult, error) {
	if c.MockGetLeafVersions != nil {
		return c.MockGetLeafVersions()
	}

	return nil, errors.New("MockGetLeafVersions was not configured")
}

func (c *API) Lint(cfg api.LintConfig) (*api.LintResult, error) {
	if c.MockLint != nil {
		return c.MockLint(cfg)
	}

	return nil, errors.New("MockLint was not configured")
}

func (c *API) InitiateDispatch(cfg api.InitiateDispatchConfig) (*api.InitiateDispatchResult, error) {
	if c.MockInitiateDispatch != nil {
		return c.MockInitiateDispatch(cfg)
	}

	return nil, errors.New("MockInitiateDispatch was not configured")
}

func (c *API) GetDispatch(cfg api.GetDispatchConfig) (*api.GetDispatchResult, error) {
	if c.MockGetDispatch != nil {
		return c.MockGetDispatch(cfg)
	}

	return nil, errors.New("MockGetDispatch was not configured")
}

func (c *API) ResolveBaseLayer(cfg api.ResolveBaseLayerConfig) (api.ResolveBaseLayerResult, error) {
	if c.MockResolveBaseLayer != nil {
		return c.MockResolveBaseLayer(cfg)
	}

	return api.ResolveBaseLayerResult{}, errors.New("MockResolveBaseLayer was not configured")
}
