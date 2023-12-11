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

	"golang.org/x/crypto/ssh"
)

var _ = Describe("CLI Service", func() {
	var (
		config     cli.Config
		service    cli.Service
		mockClient *mocks.Client
		mockFS     *mocks.FileSystem
		mockSSH    *mocks.SSH
	)

	BeforeEach(func() {
		mockClient = new(mocks.Client)
		mockFS = new(mocks.FileSystem)
		mockSSH = new(mocks.SSH)

		config = cli.Config{
			APIClient:  mockClient,
			FileSystem: mockFS,
			SSHClient:  mockSSH,
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
						RunId:            "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
						RunURL:           "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
						TargetedTaskKeys: []string{},
						DefinitionPath:   ".mint/mint.yml",
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
							RunId:            "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
							RunURL:           "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
							TargetedTaskKeys: []string{},
							DefinitionPath:   ".mint/mint.yml",
						}, nil
					}
				})

				It("instructs the API client to not use the cache", func() {})
			})

			Context("and an optional task key argument", func() {
				BeforeEach(func() {
					runConfig.TargetedTasks = []string{fmt.Sprintf("%d", GinkgoRandomSeed())}

					mockClient.MockInitiateRun = func(cfg client.InitiateRunConfig) (*client.InitiateRunResult, error) {
						Expect(cfg.TaskDefinitions).To(HaveLen(1))
						Expect(cfg.TaskDefinitions[0].Path).To(Equal(runConfig.MintFilePath))
						Expect(cfg.TargetedTaskKeys).To(Equal([]string{fmt.Sprintf("%d", GinkgoRandomSeed())}))
						receivedFileContent = cfg.TaskDefinitions[0].FileContents
						return &client.InitiateRunResult{
							RunId:            "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
							RunURL:           "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
							TargetedTaskKeys: []string{},
							DefinitionPath:   ".mint/mint.yml",
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
							RunId:            "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
							RunURL:           "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
							TargetedTaskKeys: []string{},
							DefinitionPath:   ".mint/mint.yml",
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

	Describe("debugging a task", func() {
		const (
			// The CLI will validate key material before connecting over SSH, hence we need some "real" keys here
			privateTestKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACDiyT6ht8Z2XBEJpLR4/xmNouq5KDdn5G++cUcTH4EhzwAAAJhIWxlBSFsZ
QQAAAAtzc2gtZWQyNTUxOQAAACDiyT6ht8Z2XBEJpLR4/xmNouq5KDdn5G++cUcTH4Ehzw
AAAEC6442PQKevgYgeT0SIu9zwlnEMl6MF59ZgM+i0ByMv4eLJPqG3xnZcEQmktHj/GY2i
6rkoN2fkb75xRxMfgSHPAAAAEG1pbnQgQ0xJIHRlc3RpbmcBAgMEBQ==
-----END OPENSSH PRIVATE KEY-----`
			publicTestKey = `ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOLJPqG3xnZcEQmktHj/GY2i6rkoN2fkb75xRxMfgSHP mint CLI testing`
		)

		var (
			debugConfig                                                          cli.DebugTaskConfig
			agentAddress, runID                                                  string
			connectedViaSSH, fetchedConnectionInfo, interactiveSSHSessionStarted bool
		)

		BeforeEach(func() {
			agentAddress = fmt.Sprintf("%d.example.org:1234", GinkgoRandomSeed())
			connectedViaSSH = false
			fetchedConnectionInfo = false
			interactiveSSHSessionStarted = false
			runID = fmt.Sprintf("run-%d", GinkgoRandomSeed())

			debugConfig = cli.DebugTaskConfig{
				RunURL: fmt.Sprintf("https://cloud.rwx.com/mint/rwx/runs/%s", runID),
			}

			mockClient.MockGetDebugConnectionInfo = func(runId string) (client.DebugConnectionInfo, error) {
				Expect(runID).To(Equal(runId))
				fetchedConnectionInfo = true
				// Note: This is returning a matching private & public key. The real API returns different ones
				return client.DebugConnectionInfo{PrivateUserKey: privateTestKey, PublicHostKey: publicTestKey, Address: agentAddress}, nil
			}

			mockSSH.MockConnect = func(addr string, _ ssh.ClientConfig) error {
				Expect(addr).To(Equal(agentAddress))
				connectedViaSSH = true
				return nil
			}

			mockSSH.MockInteractiveSession = func() error {
				interactiveSSHSessionStarted = true
				return nil
			}
		})

		JustBeforeEach(func() {
			Expect(service.DebugTask(debugConfig)).To(Succeed())
		})

		It("fetches the connection info from the API", func() {
			Expect(fetchedConnectionInfo).To(BeTrue())
		})

		It("connects to the agent over SSH", func() {
			Expect(connectedViaSSH).To(BeTrue())
		})

		It("starts an interactive SSH session", func() {
			Expect(interactiveSSHSessionStarted).To(BeTrue())
		})
	})
})
