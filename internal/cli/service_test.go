package cli_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"fmt"
	"io"
	"strings"

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

	Describe("initiating a run", func() {
		var runConfig cli.InitiateRunConfig

		BeforeEach(func() {
			runConfig = cli.InitiateRunConfig{}
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
					return io.NopCloser(strings.NewReader(originalFileContent)), nil
				}
				mockClient.MockInitiateRun = func(cfg client.InitiateRunConfig) (*client.InitiateRunResult, error) {
					Expect(cfg.TaskDefinitions).To(HaveLen(1))
					Expect(cfg.TaskDefinitions[0].Path).To(Equal(runConfig.MintFilePath))
					Expect(cfg.UseCache).To(BeTrue())
					receivedFileContent = cfg.TaskDefinitions[0].FileContents
					return &client.InitiateRunResult{
						RunId: "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
						RunURL: "https://mint.rwx.com/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
						TargetedTaskKey: "",
						DefinitionPath: ".mint/mint.yml",
					}, nil
				}
			})

			JustBeforeEach(func() {
				_, err := service.InitiateRun(runConfig)
				Expect(err).ToNot(HaveOccurred())
			})

			It("sends the file contents to cloud", func() {
				Expect(receivedFileContent).To(Equal(originalFileContent))
			})

			Context("and the `--no-cache` flag", func() {
				BeforeEach(func() {
					runConfig.NoCache = true

					mockClient.MockInitiateRun = func(cfg client.InitiateRunConfig) (*client.InitiateRunResult, error) {
						Expect(cfg.TaskDefinitions).To(HaveLen(1))
						Expect(cfg.TaskDefinitions[0].Path).To(Equal(runConfig.MintFilePath))
						Expect(cfg.UseCache).To(BeFalse())
						receivedFileContent = cfg.TaskDefinitions[0].FileContents
						return &client.InitiateRunResult{
							RunId: "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
							RunURL: "https://mint.rwx.com/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
							TargetedTaskKey: "",
							DefinitionPath: ".mint/mint.yml",
						}, nil
					}
				})

				It("instructs the API client to not use the cache", func() {})
			})

			Context("and an optional task key argument", func() {
				BeforeEach(func() {
					runConfig.TargetedTask = fmt.Sprintf("%d", GinkgoRandomSeed())

					mockClient.MockInitiateRun = func(cfg client.InitiateRunConfig) (*client.InitiateRunResult, error) {
						Expect(cfg.TaskDefinitions).To(HaveLen(1))
						Expect(cfg.TaskDefinitions[0].Path).To(Equal(runConfig.MintFilePath))
						Expect(cfg.TargetedTaskKey).To(Equal(fmt.Sprintf("%d", GinkgoRandomSeed())))
						receivedFileContent = cfg.TaskDefinitions[0].FileContents
						return &client.InitiateRunResult{
							RunId: "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
							RunURL: "https://mint.rwx.com/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
							TargetedTaskKey: "",
							DefinitionPath: ".mint/mint.yml",
						}, nil
					}
				})

				It("instructs the API client to target the specified task key", func() {})
			})
		})

		Context("with a specific mint directory", func() {
			BeforeEach(func() {
				runConfig.MintDirectory = "test"
			})

			Context("which is empty", func() {
				var err error

				BeforeEach(func() {
					err = nil
					mockFS.MockReadDir = func(name string) ([]fs.DirEntry, error) {
						return []fs.DirEntry{}, nil
					}
				})

				JustBeforeEach(func() {
					_, err = service.InitiateRun(runConfig)
				})

				It("returns an error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("No run definitions provided"))
				})
			})

			Context("which contains an invalid yaml file", func() {
				var originalFileContent string
				var err error

				BeforeEach(func() {
					err = nil
					originalFileContent = "tasks:\n  - key:\n      run: - echo 'bar'\n"

					mockFS.MockReadDir = func(name string) ([]fs.DirEntry, error) {
						file := mocks.DirEntry{FileName: "mint.yaml"}
						return []fs.DirEntry{file}, nil
					}
					mockFS.MockOpen = func(path string) (fs.File, error) {
						return io.NopCloser(strings.NewReader(originalFileContent)), nil
					}
				})

				JustBeforeEach(func() {
					_, err = service.InitiateRun(runConfig)
				})

				It("returns an error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("unable to parse"))
				})
			})

			Context("which contains a mixture of files", func() {
				var originalFileContents map[string]string
				var receivedFileContents map[string]string

				BeforeEach(func() {
					receivedFileContents = make(map[string]string)
					originalFileContents = make(map[string]string)
					originalFileContents["test/foobar.yaml"] = "tasks:\n  - key: foo\n    run: echo 'bar'\n"
					originalFileContents["test/onetwo.yml"] = "tasks:\n  - key: one\n    run: echo 'two'\n"
					originalFileContents["test/helloworld.json"] = "tasks:\n  - key: hello\n    run: echo 'world'\n"

					mockClient.MockInitiateRun = func(cfg client.InitiateRunConfig) (*client.InitiateRunResult, error) {
						for _, def := range cfg.TaskDefinitions {
							receivedFileContents[def.Path] = def.FileContents
						}
						return &client.InitiateRunResult{
							RunId: "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
							RunURL: "https://mint.rwx.com/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
							TargetedTaskKey: "",
							DefinitionPath: ".mint/mint.yml",
						}, nil
					}
					mockFS.MockReadDir = func(name string) ([]fs.DirEntry, error) {
						foobar := mocks.DirEntry{FileName: "foobar.yaml"}
						onetwo := mocks.DirEntry{FileName: "onetwo.yml"}
						subdir := mocks.DirEntry{FileName: "directory.yaml", IsDirectory: true}
						return []fs.DirEntry{foobar, onetwo, subdir}, nil
					}
					mockFS.MockOpen = func(path string) (fs.File, error) {
						Expect(originalFileContents).To(HaveKey(path))
						return io.NopCloser(strings.NewReader(originalFileContents[path])), nil
					}
				})

				JustBeforeEach(func() {
					_, err := service.InitiateRun(runConfig)
					Expect(err).ToNot(HaveOccurred())
				})

				It("sends the file contents of *.yml and *.yaml files", func() {
					for path, content := range originalFileContents {
						if strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml") {
							Expect(receivedFileContents[path]).To(Equal(content))
						}
					}
				})

				It("doesn't send yaml files from sub-directories", func() {
					Expect(receivedFileContents).ToNot(HaveKey("test/directory.yaml"))
				})
			})
		})
	})
})
