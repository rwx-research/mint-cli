package client_test

import (
	"bytes"
	"encoding/json"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"net/http"
	"net/url"

	"github.com/rwx-research/mint-cli/internal/client"
)

var _ = Describe("Client", func() {
	Describe("InitiateRun", func() {
		It("prefixes the endpoint with the base path", func() {
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
				}, nil
			}

			c := client.Client{roundTrip}

			initRunConfig := client.InitiateRunConfig{
				InitializationParameters: map[string]string{},
				TaskDefinitions: []client.TaskDefinition{
					{Path: "foo", FileContents: "echo 'bar'"},
				},
				TargetedTaskKeys: []string{},
				UseCache:         false,
			}

			_, err := c.InitiateRun(initRunConfig)
			Expect(err).To(BeNil())
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

			c := client.Client{roundTrip}

			obtainAuthCodeConfig := client.ObtainAuthCodeConfig{
				Code: client.ObtainAuthCodeCode{
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

			c := client.Client{roundTrip}

			_, err := c.AcquireToken("https://cloud.rwx.com/api/auth/codes/some-uuid/token")
			Expect(err).To(BeNil())
		})
	})
})
