package cli_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"bytes"
	"io"

	"github.com/rwx-research/mint-cli/internal/cli"
	"github.com/rwx-research/mint-cli/internal/client"
	"github.com/rwx-research/mint-cli/internal/fs"
	"github.com/rwx-research/mint-cli/internal/mocks"
)

var _ = Describe("CLI Service", func() {
	var (
		config     cli.Config
		service    cli.Service
		mockClient *mocks.Client
		mockFS     *mocks.FileSystem
	)

	BeforeEach(func() {
		mockClient = new(mocks.Client)
		mockFS = new(mocks.FileSystem)

		config = cli.Config{
			Client:     mockClient,
			FileSystem: mockFS,
		}
	})

	JustBeforeEach(func() {
		var err error
		service, err = cli.NewService(config)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("initiating runs", func() {
		var runConfig cli.InitiateRunConfig

		BeforeEach(func() {
			runConfig = cli.InitiateRunConfig{}
		})

		JustBeforeEach(func() {
			Expect(service.InitiateRun(runConfig)).To(Succeed())
		})

		Context("with a specific mint config file", func() {
			var originalFileContent string
			var receivedFileContent string

			BeforeEach(func() {
				originalFileContent = "tasks:\n  - key: foo\n    run: echo 'bar'\n"
				receivedFileContent = ""
				runConfig.MintFilePath = "mint.yml"

				mockFS.MockOpen = func(name string) (fs.File, error) {
					Expect(name).To(Equal("mint.yml"))
					return io.NopCloser(bytes.NewBufferString(originalFileContent)), nil
				}
				mockClient.MockInitiateRun = func(cfg client.InitiateRunConfig) error {
					Expect(cfg.TaskDefinitions).To(HaveLen(1))
					Expect(cfg.TaskDefinitions[0].Path).To(Equal(runConfig.MintFilePath))
					receivedFileContent = cfg.TaskDefinitions[0].Body
					return nil
				}
			})

			It("sends the file contents to cloud", func() {
				Expect(receivedFileContent).To(Equal(originalFileContent))
			})
		})
	})
})
