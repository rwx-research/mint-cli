package api_test

import (
	"bytes"
	"encoding/json"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"net/http"
	"net/url"

	"github.com/rwx-research/mint-cli/internal/api"
)

var _ = Describe("API Client", func() {
	Describe("InitiateRun", func() {
		It("prefixes the endpoint with the base path and parses camelcase responses", func() {
			body := struct {
				RunID            string   `json:"runId"`
				RunURL           string   `json:"runUrl"`
				TargetedTaskKeys []string `json:"targetedTaskKeys"`
				DefinitionPath   string   `json:"definitionPath"`
			}{
				RunID:            "123",
				RunURL:           "https://cloud.rwx.com/mint/org/runs/123",
				TargetedTaskKeys: []string{},
				DefinitionPath:   "foo",
			}
			bodyBytes, _ := json.Marshal(body)

			roundTrip := func(req *http.Request) (*http.Response, error) {
				Expect(req.URL.Path).To(Equal("/mint/api/runs"))
				return &http.Response{
					Status:     "201 Created",
					StatusCode: 201,
					Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
					Header:     http.Header{"X-Mint-Cli-Latest-Version": []string{"1.2.3"}},
				}, nil
			}

			c := api.NewClientWithRoundTrip(roundTrip)

			initRunConfig := api.InitiateRunConfig{
				InitializationParameters: []api.InitializationParameter{},
				TaskDefinitions: []api.MintDirectoryEntry{
					{Path: "foo", FileContents: "echo 'bar'", Permissions: 0o644, Type: "file"},
				},
				TargetedTaskKeys: []string{},
				UseCache:         false,
			}

			result, err := c.InitiateRun(initRunConfig)
			Expect(err).To(BeNil())
			Expect(result.RunId).To(Equal("123"))
			Expect(c.LatestVersionNumber()).To(Equal("1.2.3"))
		})

		It("prefixes the endpoint with the base path and parses snakecase responses", func() {
			body := struct {
				RunID            string   `json:"run_id"`
				RunURL           string   `json:"run_url"`
				TargetedTaskKeys []string `json:"targeted_task_keys"`
				DefinitionPath   string   `json:"definition_path"`
			}{
				RunID:            "123",
				RunURL:           "https://cloud.rwx.com/mint/org/runs/123",
				TargetedTaskKeys: []string{},
				DefinitionPath:   "foo",
			}
			bodyBytes, _ := json.Marshal(body)

			roundTrip := func(req *http.Request) (*http.Response, error) {
				Expect(req.URL.Path).To(Equal("/mint/api/runs"))
				return &http.Response{
					Status:     "201 Created",
					StatusCode: 201,
					Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
				}, nil
			}

			c := api.NewClientWithRoundTrip(roundTrip)

			initRunConfig := api.InitiateRunConfig{
				InitializationParameters: []api.InitializationParameter{},
				TaskDefinitions: []api.MintDirectoryEntry{
					{Path: "foo", FileContents: "echo 'bar'", Permissions: 0o644, Type: "file"},
				},
				TargetedTaskKeys: []string{},
				UseCache:         false,
			}

			result, err := c.InitiateRun(initRunConfig)
			Expect(err).To(BeNil())
			Expect(result.RunId).To(Equal("123"))
		})
	})

	Describe("ObtainAuthCode", func() {
		It("builds the request", func() {
			body := struct {
				AuthorizationUrl string `json:"authorization_url"`
				TokenUrl         string `json:"token_url"`
			}{
				AuthorizationUrl: "https://cloud.rwx.com/_/auth/code?code=foobar",
				TokenUrl:         "https://cloud.rwx.com/api/auth/codes/code-uuid/token",
			}
			bodyBytes, _ := json.Marshal(body)

			roundTrip := func(req *http.Request) (*http.Response, error) {
				Expect(req.URL.Path).To(Equal("/api/auth/codes"))
				return &http.Response{
					Status:     "201 Created",
					StatusCode: 201,
					Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
				}, nil
			}

			c := api.NewClientWithRoundTrip(roundTrip)

			obtainAuthCodeConfig := api.ObtainAuthCodeConfig{
				Code: api.ObtainAuthCodeCode{
					DeviceName: "some-device",
				},
			}

			_, err := c.ObtainAuthCode(obtainAuthCodeConfig)
			Expect(err).To(BeNil())
		})
	})

	Describe("AcquireToken", func() {
		It("builds the request using the supplied url", func() {
			body := struct {
				State string `json:"state"`
				Token string `json:"token"`
			}{
				State: "authorized",
				Token: "some-token",
			}
			bodyBytes, _ := json.Marshal(body)

			roundTrip := func(req *http.Request) (*http.Response, error) {
				expected, err := url.Parse("https://cloud.rwx.com/api/auth/codes/some-uuid/token")
				Expect(err).NotTo(HaveOccurred())
				Expect(req.URL).To(Equal(expected))
				return &http.Response{
					Status:     "200 OK",
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
				}, nil
			}

			c := api.NewClientWithRoundTrip(roundTrip)

			_, err := c.AcquireToken("https://cloud.rwx.com/api/auth/codes/some-uuid/token")
			Expect(err).To(BeNil())
		})
	})

	Describe("Whoami", func() {
		It("makes the request", func() {
			email := "some-email@example.com"
			body := struct {
				OrganizationSlug string  `json:"organization_slug"`
				TokenKind        string  `json:"token_kind"`
				UserEmail        *string `json:"user_email,omitempty"`
			}{
				OrganizationSlug: "some-org",
				TokenKind:        "personal_access_token",
				UserEmail:        &email,
			}
			bodyBytes, _ := json.Marshal(body)

			roundTrip := func(req *http.Request) (*http.Response, error) {
				Expect(req.URL.Path).To(Equal("/api/auth/whoami"))
				return &http.Response{
					Status:     "200 OK",
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
					Header:     http.Header{"X-Mint-Cli-Latest-Version": []string{"1.2.3"}},
				}, nil
			}

			c := api.NewClientWithRoundTrip(roundTrip)

			_, err := c.Whoami()
			Expect(err).To(BeNil())
			Expect(c.LatestVersionNumber()).To(Equal("1.2.3"))
		})
	})

	Describe("SetSecretsInVault", func() {
		It("makes the request", func() {
			body := api.SetSecretsInVaultConfig{
				VaultName: "default",
				Secrets:   []api.Secret{{Name: "ABC", Secret: "123"}},
			}
			bodyBytes, _ := json.Marshal(body)

			roundTrip := func(req *http.Request) (*http.Response, error) {
				Expect(req.URL.Path).To(Equal("/mint/api/vaults/secrets"))
				return &http.Response{
					Status:     "200 OK",
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
					Header:     http.Header{"X-Mint-Cli-Latest-Version": []string{"1.2.3"}},
				}, nil
			}

			c := api.NewClientWithRoundTrip(roundTrip)

			_, err := c.SetSecretsInVault(body)
			Expect(err).To(BeNil())
			Expect(c.LatestVersionNumber()).To(Equal("1.2.3"))
		})
	})
})
