package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/rwx-research/mint-cli/cmd/mint/config"
	"github.com/rwx-research/mint-cli/internal/accesstoken"
	"github.com/rwx-research/mint-cli/internal/errors"
	"github.com/rwx-research/mint-cli/internal/messages"
	"github.com/rwx-research/mint-cli/internal/versions"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (rtf roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return rtf(r)
}

// Client is an API Client for Mint
type Client struct {
	http.RoundTripper
}

func NewClient(cfg Config) (Client, error) {
	if err := cfg.Validate(); err != nil {
		return Client{}, errors.Wrap(err, "validation failed")
	}

	roundTrip := func(req *http.Request) (*http.Response, error) {
		if req.URL.Scheme == "" {
			req.URL.Scheme = "https"
		}
		if req.URL.Host == "" {
			req.URL.Host = cfg.Host
		}
		req.Header.Set("User-Agent", fmt.Sprintf("mint-cli/%s", config.Version))

		token, err := accesstoken.Get(cfg.AccessTokenBackend, cfg.AccessToken)
		if err != nil {
			return nil, errors.Wrap(err, "unable to retrieve access token")
		}
		if token != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		}

		return http.DefaultClient.Do(req)
	}

	return NewClientWithRoundTrip(roundTrip), nil
}

func NewClientWithRoundTrip(rt func(*http.Request) (*http.Response, error)) Client {
	roundTripper := versions.NewRoundTripper(roundTripFunc(rt))
	return Client{roundTripper}
}

func (c Client) GetDebugConnectionInfo(debugKey string) (DebugConnectionInfo, error) {
	connectionInfo := DebugConnectionInfo{}

	if debugKey == "" {
		return connectionInfo, errors.New("missing debugKey")
	}

	endpoint := fmt.Sprintf("/mint/api/debug_connection_info?debug_key=%s", url.QueryEscape(debugKey))
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return connectionInfo, errors.Wrap(err, "unable to create new HTTP request")
	}

	resp, err := c.RoundTrip(req)
	if err != nil {
		return connectionInfo, errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
		break
	case 400:
		connectionError := DebugConnectionInfoError{}
		if err := json.NewDecoder(resp.Body).Decode(&connectionError); err == nil {
			return connectionInfo, errors.Wrap(errors.ErrBadRequest, connectionError.Error)
		}
		return connectionInfo, errors.ErrBadRequest
	case 404:
		return connectionInfo, errors.ErrNotFound
	case 410:
		connectionError := DebugConnectionInfoError{}
		if err := json.NewDecoder(resp.Body).Decode(&connectionError); err == nil {
			return connectionInfo, errors.Wrap(errors.ErrGone, connectionError.Error)
		}
		return connectionInfo, errors.ErrGone
	default:
		return connectionInfo, errors.New(fmt.Sprintf("Unable to call Mint API - %s", resp.Status))
	}

	if err := json.NewDecoder(resp.Body).Decode(&connectionInfo); err != nil {
		return connectionInfo, errors.Wrap(err, "unable to parse API response")
	}

	return connectionInfo, nil
}

// InitiateRun sends a request to Mint for starting a new run
func (c Client) InitiateRun(cfg InitiateRunConfig) (*InitiateRunResult, error) {
	endpoint := "/mint/api/runs"

	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	encodedBody, err := json.Marshal(struct {
		Run InitiateRunConfig `json:"run"`
	}{cfg})
	if err != nil {
		return nil, errors.Wrap(err, "unable to encode as JSON")
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(encodedBody))
	if err != nil {
		return nil, errors.Wrap(err, "unable to create new HTTP request")
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.RoundTrip(req)
	if err != nil {
		return nil, errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		msg := extractErrorMessage(resp.Body)
		if msg == "" {
			msg = fmt.Sprintf("Unable to call Mint API - %s", resp.Status)
		}

		return nil, errors.New(msg)
	}

	respBody := struct {
		SnakeRunId            string   `json:"run_id"`
		SnakeRunURL           string   `json:"run_url"`
		SnakeTargetedTaskKeys []string `json:"targeted_task_keys"`
		SnakeDefinitionPath   string   `json:"definition_path"`
		CamelRunId            string   `json:"runId"`
		CamelRunURL           string   `json:"runURL"`
		CamelTargetedTaskKeys []string `json:"targetedTaskKeys"`
		CamelDefinitionPath   string   `json:"definitionPath"`
	}{}

	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return nil, errors.Wrap(err, "unable to parse API response")
	}

	if respBody.CamelRunId != "" {
		return &InitiateRunResult{
			RunId:            respBody.CamelRunId,
			RunURL:           respBody.CamelRunURL,
			TargetedTaskKeys: respBody.CamelTargetedTaskKeys,
			DefinitionPath:   respBody.CamelDefinitionPath,
		}, nil
	} else {
		return &InitiateRunResult{
			RunId:            respBody.SnakeRunId,
			RunURL:           respBody.SnakeRunURL,
			TargetedTaskKeys: respBody.SnakeTargetedTaskKeys,
			DefinitionPath:   respBody.SnakeDefinitionPath,
		}, nil
	}
}

func (c Client) Lint(cfg LintConfig) (*LintResult, error) {
	endpoint := "/mint/api/lints"

	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	encodedBody, err := json.Marshal(struct {
		Lint LintConfig `json:"lint"`
	}{cfg})
	if err != nil {
		return nil, errors.Wrap(err, "unable to encode as JSON")
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(encodedBody))
	if err != nil {
		return nil, errors.Wrap(err, "unable to create new HTTP request")
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.RoundTrip(req)
	if err != nil {
		return nil, errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		msg := extractErrorMessage(resp.Body)
		if msg == "" {
			msg = fmt.Sprintf("Unable to call Mint API - %s", resp.Status)
		}

		return nil, errors.New(msg)
	}

	lintResult := LintResult{}
	if err := json.NewDecoder(resp.Body).Decode(&lintResult); err != nil {
		return nil, errors.Wrap(err, "unable to parse API response")
	}

	return &lintResult, nil
}

// ObtainAuthCode requests a new one-time-use code to login on a device
func (c Client) ObtainAuthCode(cfg ObtainAuthCodeConfig) (*ObtainAuthCodeResult, error) {
	endpoint := "/api/auth/codes"

	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	encodedBody, err := json.Marshal(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "unable to encode as JSON")
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(encodedBody))
	if err != nil {
		return nil, errors.Wrap(err, "unable to create new HTTP request")
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.RoundTrip(req)
	if err != nil {
		return nil, errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		return nil, errors.New(fmt.Sprintf("Unable to call Mint API - %s", resp.Status))
	}

	respBody := ObtainAuthCodeResult{}
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return nil, errors.Wrap(err, "unable to parse API response")
	}

	return &respBody, nil
}

// AcquireToken consumes the one-time-use code once authorized
func (c Client) AcquireToken(tokenUrl string) (*AcquireTokenResult, error) {
	req, err := http.NewRequest(http.MethodGet, tokenUrl, bytes.NewBuffer(make([]byte, 0)))
	if err != nil {
		return nil, errors.Wrap(err, "unable to create new HTTP request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.RoundTrip(req)
	if err != nil {
		return nil, errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, errors.New(fmt.Sprintf("Unable to query the token URL - %s", resp.Status))
	}

	respBody := AcquireTokenResult{}
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return nil, errors.Wrap(err, "unable to parse API response")
	}

	return &respBody, nil
}

// Whoami provides details about the authenticated token
func (c Client) Whoami() (*WhoamiResult, error) {
	endpoint := "/api/auth/whoami"

	req, err := http.NewRequest(http.MethodGet, endpoint, bytes.NewBuffer([]byte{}))
	if err != nil {
		return nil, errors.Wrap(err, "unable to create new HTTP request")
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.RoundTrip(req)
	if err != nil {
		return nil, errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, errors.New(fmt.Sprintf("Unable to call Mint API - %s", resp.Status))
	}

	respBody := WhoamiResult{}
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return nil, errors.Wrap(err, "unable to parse API response")
	}

	return &respBody, nil
}

func (c Client) SetSecretsInVault(cfg SetSecretsInVaultConfig) (*SetSecretsInVaultResult, error) {
	endpoint := "/mint/api/vaults/secrets"

	encodedBody, err := json.Marshal(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "unable to encode as JSON")
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(encodedBody))
	if err != nil {
		return nil, errors.Wrap(err, "unable to create new HTTP request")
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.RoundTrip(req)
	if err != nil {
		return nil, errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		msg := extractErrorMessage(resp.Body)
		if msg == "" {
			msg = fmt.Sprintf("Unable to call Mint API - %s", resp.Status)
		}

		return nil, errors.New(msg)
	}

	respBody := SetSecretsInVaultResult{}
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return nil, errors.Wrap(err, "unable to parse API response")
	}

	return &respBody, nil
}

func (c Client) GetLeafVersions() (*LeafVersionsResult, error) {
	endpoint := "/mint/api/leaves"

	req, err := http.NewRequest(http.MethodGet, endpoint, bytes.NewBuffer([]byte{}))
	if err != nil {
		return nil, errors.Wrap(err, "unable to create new HTTP request")
	}

	resp, err := c.RoundTrip(req)
	if err != nil {
		return nil, errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		msg := extractErrorMessage(resp.Body)
		if msg == "" {
			msg = fmt.Sprintf("Unable to call Mint API - %s", resp.Status)
		}
		return nil, errors.New(msg)
	}

	respBody := LeafVersionsResult{}
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return nil, errors.Wrap(err, "unable to parse API response")
	}

	return &respBody, nil
}

type ErrorMessage struct {
	Message    string                `json:"message"`
	StackTrace []messages.StackEntry `json:"stack_trace,omitempty"`
	Frame      string                `json:"frame"`
	Advice     string                `json:"advice"`
}

// extractErrorMessage is a small helper function for parsing an API error message
func extractErrorMessage(reader io.Reader) string {
	errorStruct := struct {
		Error         string         `json:"error,omitempty"`
		ErrorMessages []ErrorMessage `json:"error_messages,omitempty"`
	}{}

	if err := json.NewDecoder(reader).Decode(&errorStruct); err != nil {
		return ""
	}

	if len(errorStruct.ErrorMessages) > 0 {
		var message strings.Builder
		for _, errorMessage := range errorStruct.ErrorMessages {
			message.WriteString("\n\n")
			message.WriteString(messages.FormatUserMessage(errorMessage.Message, errorMessage.Frame, errorMessage.StackTrace, errorMessage.Advice))
		}

		return message.String()
	}

	// Fallback to Error field
	if errorStruct.Error != "" {
		return errorStruct.Error
	}

	// Fallback to an empty string
	return ""
}
