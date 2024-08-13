package cli_test

import (
	"bytes"
	"io"
	"os"
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"

	"fmt"
	"strings"

	"github.com/rwx-research/mint-cli/internal/accesstoken"
	"github.com/rwx-research/mint-cli/internal/api"
	"github.com/rwx-research/mint-cli/internal/cli"
	"github.com/rwx-research/mint-cli/internal/errors"
	"github.com/rwx-research/mint-cli/internal/fs"
	"github.com/rwx-research/mint-cli/internal/memoryfs"
	"github.com/rwx-research/mint-cli/internal/mocks"

	"golang.org/x/crypto/ssh"
)

var _ = Describe("CLI Service", func() {
	var (
		config  cli.Config
		service cli.Service
		mockAPI *mocks.API
		mockFS  *mocks.FileSystem
		mockSSH *mocks.SSH
	)

	BeforeEach(func() {
		mockAPI = new(mocks.API)
		mockFS = new(mocks.FileSystem)
		mockSSH = new(mocks.SSH)

		config = cli.Config{
			APIClient:  mockAPI,
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

		Context("with a specific mint file and no specific directory", func() {
			Context("when a directory with files is found", func() {
				var originalSpecifiedFileContent string
				var originalMintDirFileContent string
				var receivedSpecifiedFileContent string
				var receivedMintDirFileContent string

				BeforeEach(func() {
					originalSpecifiedFileContent = "tasks:\n  - key: foo\n    run: echo 'bar'\n"
					originalMintDirFileContent = "tasks:\n  - key: mintdir\n    run: echo 'mintdir'\n"
					receivedSpecifiedFileContent = ""
					receivedMintDirFileContent = ""
					runConfig.MintFilePath = "mint.yml"
					runConfig.MintDirectory = ""

					mockFS.MockGetwd = func() (string, error) {
						return "/some/path/to/working/directory", nil
					}
					mockFS.MockExists = func(name string) (bool, error) {
						return name == "/some/path/to/.mint", nil
					}
					mockFS.MockReadDir = func(name string) ([]fs.DirEntry, error) {
						Expect(name).To(Equal("/some/path/to/.mint"))
						return []fs.DirEntry{
							mocks.DirEntry{FileName: "mintdir-tasks.yml"},
							mocks.DirEntry{FileName: "mintdir-tasks.json"},
						}, nil
					}
					mockFS.MockOpen = func(name string) (fs.File, error) {
						if name == "mint.yml" {
							file := mocks.NewFile(originalSpecifiedFileContent)
							return file, nil
						} else if name == "/some/path/to/.mint/mintdir-tasks.yml" {
							file := mocks.NewFile(originalMintDirFileContent)
							return file, nil
						} else {
							Expect(name).To(BeNil())
							return nil, errors.New("file does not exist")
						}
					}
					mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
						Expect(cfg.TaskDefinitions).To(HaveLen(1))
						Expect(cfg.TaskDefinitions[0].Path).To(Equal(runConfig.MintFilePath))
						Expect(cfg.MintDirectory).To(HaveLen(1))
						Expect(cfg.MintDirectory[0].Path).To(Equal(".mint/mintdir-tasks.yml"))
						Expect(cfg.UseCache).To(BeTrue())
						receivedSpecifiedFileContent = cfg.TaskDefinitions[0].FileContents
						receivedMintDirFileContent = cfg.MintDirectory[0].FileContents
						return &api.InitiateRunResult{
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
					Expect(receivedSpecifiedFileContent).To(Equal(originalSpecifiedFileContent))
					Expect(receivedMintDirFileContent).To(Equal(originalMintDirFileContent))
				})
			})

			Context("when the specified file is invalid", func() {
				var originalSpecifiedFileContent string
				var originalMintDirFileContent string

				BeforeEach(func() {
					originalSpecifiedFileContent = "NOT YAML!!!"
					originalMintDirFileContent = "tasks:\n  - key: mintdir\n    run: echo 'mintdir'\n"
					runConfig.MintFilePath = "mint.yml"
					runConfig.MintDirectory = ""

					mockFS.MockGetwd = func() (string, error) {
						return "/some/path/to/working/directory", nil
					}
					mockFS.MockExists = func(name string) (bool, error) {
						return name == "/some/path/to/.mint", nil
					}
					mockFS.MockReadDir = func(name string) ([]fs.DirEntry, error) {
						Expect(name).To(Equal("/some/path/to/.mint"))
						return []fs.DirEntry{
							mocks.DirEntry{FileName: "mintdir-tasks.yml"},
							mocks.DirEntry{FileName: "mintdir-tasks.json"},
						}, nil
					}
					mockFS.MockOpen = func(name string) (fs.File, error) {
						if name == "mint.yml" {
							file := mocks.NewFile(originalSpecifiedFileContent)
							return file, nil
						} else if name == "/some/path/to/.mint/mintdir-tasks.yml" {
							file := mocks.NewFile(originalMintDirFileContent)
							return file, nil
						} else {
							Expect(name).To(BeNil())
							return nil, errors.New("file does not exist")
						}
					}
				})

				It("returns an error", func() {
					_, err := service.InitiateRun(runConfig)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("unable to parse"))
				})
			})

			Context("when a directory with invalid files is found", func() {
				var originalSpecifiedFileContent string
				var originalMintDirFileContent string

				BeforeEach(func() {
					originalSpecifiedFileContent = "tasks:\n  - key: foo\n    run: echo 'bar'\n"
					originalMintDirFileContent = "NOT YAML!!!!!!!!"
					runConfig.MintFilePath = "mint.yml"
					runConfig.MintDirectory = ""

					mockFS.MockGetwd = func() (string, error) {
						return "/some/path/to/working/directory", nil
					}
					mockFS.MockExists = func(name string) (bool, error) {
						return name == "/some/path/to/.mint", nil
					}
					mockFS.MockReadDir = func(name string) ([]fs.DirEntry, error) {
						Expect(name).To(Equal("/some/path/to/.mint"))
						return []fs.DirEntry{
							mocks.DirEntry{FileName: "mintdir-tasks.yml"},
							mocks.DirEntry{FileName: "mintdir-tasks.json"},
						}, nil
					}
					mockFS.MockOpen = func(name string) (fs.File, error) {
						if name == "mint.yml" {
							file := mocks.NewFile(originalSpecifiedFileContent)
							return file, nil
						} else if name == "/some/path/to/.mint/mintdir-tasks.yml" {
							file := mocks.NewFile(originalMintDirFileContent)
							return file, nil
						} else {
							Expect(name).To(BeNil())
							return nil, errors.New("file does not exist")
						}
					}
				})

				It("returns an error", func() {
					_, err := service.InitiateRun(runConfig)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("unable to parse"))
				})
			})

			Context("when an empty directory is found", func() {
				var originalSpecifiedFileContent string
				var receivedSpecifiedFileContent string

				BeforeEach(func() {
					originalSpecifiedFileContent = "tasks:\n  - key: foo\n    run: echo 'bar'\n"
					receivedSpecifiedFileContent = ""
					runConfig.MintFilePath = "mint.yml"
					runConfig.MintDirectory = ""

					mockFS.MockGetwd = func() (string, error) {
						return "/some/path/to/working/directory", nil
					}
					mockFS.MockExists = func(name string) (bool, error) {
						return name == "/some/path/to/.mint", nil
					}
					mockFS.MockReadDir = func(name string) ([]fs.DirEntry, error) {
						Expect(name).To(Equal("/some/path/to/.mint"))
						return []fs.DirEntry{}, nil
					}
					mockFS.MockOpen = func(name string) (fs.File, error) {
						if name == "mint.yml" {
							file := mocks.NewFile(originalSpecifiedFileContent)
							return file, nil
						} else {
							Expect(name).To(BeNil())
							return nil, errors.New("file does not exist")
						}
					}
					mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
						Expect(cfg.TaskDefinitions).To(HaveLen(1))
						Expect(cfg.TaskDefinitions[0].Path).To(Equal(runConfig.MintFilePath))
						Expect(cfg.MintDirectory).To(HaveLen(0))
						Expect(cfg.UseCache).To(BeTrue())
						receivedSpecifiedFileContent = cfg.TaskDefinitions[0].FileContents
						return &api.InitiateRunResult{
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
					Expect(receivedSpecifiedFileContent).To(Equal(originalSpecifiedFileContent))
				})
			})

			Context("when a directory is not found", func() {
				var originalSpecifiedFileContent string
				var receivedSpecifiedFileContent string

				BeforeEach(func() {
					originalSpecifiedFileContent = "tasks:\n  - key: foo\n    run: echo 'bar'\n"
					receivedSpecifiedFileContent = ""
					runConfig.MintFilePath = "mint.yml"
					runConfig.MintDirectory = ""

					mockFS.MockGetwd = func() (string, error) {
						return "/some/path/to/working/directory", nil
					}
					mockFS.MockExists = func(name string) (bool, error) {
						return false, nil
					}
					mockFS.MockOpen = func(name string) (fs.File, error) {
						if name == "mint.yml" {
							file := mocks.NewFile(originalSpecifiedFileContent)
							return file, nil
						} else {
							Expect(name).To(BeNil())
							return nil, errors.New("file does not exist")
						}
					}
					mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
						Expect(cfg.TaskDefinitions).To(HaveLen(1))
						Expect(cfg.TaskDefinitions[0].Path).To(Equal(runConfig.MintFilePath))
						Expect(cfg.MintDirectory).To(HaveLen(0))
						Expect(cfg.UseCache).To(BeTrue())
						receivedSpecifiedFileContent = cfg.TaskDefinitions[0].FileContents
						return &api.InitiateRunResult{
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
					Expect(receivedSpecifiedFileContent).To(Equal(originalSpecifiedFileContent))
				})
			})
		})

		Context("with no specific mint file and no specific directory", func() {
			BeforeEach(func() {
				runConfig.MintFilePath = ""
				runConfig.MintDirectory = ""
			})

			It("returns an error", func() {
				_, err := service.InitiateRun(runConfig)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("the path to a mint-file must be provided"))
			})
		})

		Context("with a specific mint file and a specific directory", func() {
			Context("when a directory with files is found", func() {
				var originalSpecifiedFileContent string
				var originalMintDirFileContent string
				var receivedSpecifiedFileContent string
				var receivedMintDirFileContent string

				BeforeEach(func() {
					originalSpecifiedFileContent = "tasks:\n  - key: foo\n    run: echo 'bar'\n"
					originalMintDirFileContent = "tasks:\n  - key: mintdir\n    run: echo 'mintdir'\n"
					receivedSpecifiedFileContent = ""
					receivedMintDirFileContent = ""
					runConfig.MintFilePath = "mint.yml"
					runConfig.MintDirectory = "some-dir"

					mockFS.MockReadDir = func(name string) ([]fs.DirEntry, error) {
						Expect(name).To(Equal("some-dir"))
						return []fs.DirEntry{
							mocks.DirEntry{FileName: "mintdir-tasks.yml"},
							mocks.DirEntry{FileName: "mintdir-tasks.json"},
						}, nil
					}
					mockFS.MockOpen = func(name string) (fs.File, error) {
						if name == "mint.yml" {
							file := mocks.NewFile(originalSpecifiedFileContent)
							return file, nil
						} else if name == "some-dir/mintdir-tasks.yml" {
							file := mocks.NewFile(originalMintDirFileContent)
							return file, nil
						} else {
							Expect(name).To(BeNil())
							return nil, errors.New("file does not exist")
						}
					}
					mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
						Expect(cfg.TaskDefinitions).To(HaveLen(1))
						Expect(cfg.TaskDefinitions[0].Path).To(Equal(runConfig.MintFilePath))
						Expect(cfg.MintDirectory).To(HaveLen(1))
						Expect(cfg.MintDirectory[0].Path).To(Equal(".mint/mintdir-tasks.yml"))
						Expect(cfg.UseCache).To(BeTrue())
						receivedSpecifiedFileContent = cfg.TaskDefinitions[0].FileContents
						receivedMintDirFileContent = cfg.MintDirectory[0].FileContents
						return &api.InitiateRunResult{
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
					Expect(receivedSpecifiedFileContent).To(Equal(originalSpecifiedFileContent))
					Expect(receivedMintDirFileContent).To(Equal(originalMintDirFileContent))
				})
			})

			Context("when the specified file is invalid", func() {
				var originalSpecifiedFileContent string
				var originalMintDirFileContent string

				BeforeEach(func() {
					originalSpecifiedFileContent = "NOT YAML!!!"
					originalMintDirFileContent = "tasks:\n  - key: mintdir\n    run: echo 'mintdir'\n"
					runConfig.MintFilePath = "mint.yml"
					runConfig.MintDirectory = "some-dir"

					mockFS.MockReadDir = func(name string) ([]fs.DirEntry, error) {
						Expect(name).To(Equal("some-dir"))
						return []fs.DirEntry{
							mocks.DirEntry{FileName: "mintdir-tasks.yml"},
							mocks.DirEntry{FileName: "mintdir-tasks.json"},
						}, nil
					}
					mockFS.MockOpen = func(name string) (fs.File, error) {
						if name == "mint.yml" {
							file := mocks.NewFile(originalSpecifiedFileContent)
							return file, nil
						} else if name == "some-dir/mintdir-tasks.yml" {
							file := mocks.NewFile(originalMintDirFileContent)
							return file, nil
						} else {
							Expect(name).To(BeNil())
							return nil, errors.New("file does not exist")
						}
					}
				})

				It("returns an error", func() {
					_, err := service.InitiateRun(runConfig)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("unable to parse"))
				})
			})

			Context("when a directory with invalid files is found", func() {
				var originalSpecifiedFileContent string
				var originalMintDirFileContent string

				BeforeEach(func() {
					originalSpecifiedFileContent = "tasks:\n  - key: foo\n    run: echo 'bar'\n"
					originalMintDirFileContent = "NOT YAML!!!!!!!!"
					runConfig.MintFilePath = "mint.yml"
					runConfig.MintDirectory = "some-dir"

					mockFS.MockReadDir = func(name string) ([]fs.DirEntry, error) {
						Expect(name).To(Equal("some-dir"))
						return []fs.DirEntry{
							mocks.DirEntry{FileName: "mintdir-tasks.yml"},
							mocks.DirEntry{FileName: "mintdir-tasks.json"},
						}, nil
					}
					mockFS.MockOpen = func(name string) (fs.File, error) {
						if name == "mint.yml" {
							file := mocks.NewFile(originalSpecifiedFileContent)
							return file, nil
						} else if name == "some-dir/mintdir-tasks.yml" {
							file := mocks.NewFile(originalMintDirFileContent)
							return file, nil
						} else {
							Expect(name).To(BeNil())
							return nil, errors.New("file does not exist")
						}
					}
				})

				It("returns an error", func() {
					_, err := service.InitiateRun(runConfig)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("unable to parse"))
				})
			})

			Context("when an empty directory is found", func() {
				var originalSpecifiedFileContent string
				var receivedSpecifiedFileContent string

				BeforeEach(func() {
					originalSpecifiedFileContent = "tasks:\n  - key: foo\n    run: echo 'bar'\n"
					receivedSpecifiedFileContent = ""
					runConfig.MintFilePath = "mint.yml"
					runConfig.MintDirectory = "some-dir"

					mockFS.MockReadDir = func(name string) ([]fs.DirEntry, error) {
						Expect(name).To(Equal("some-dir"))
						return []fs.DirEntry{}, nil
					}
					mockFS.MockOpen = func(name string) (fs.File, error) {
						if name == "mint.yml" {
							file := mocks.NewFile(originalSpecifiedFileContent)
							return file, nil
						} else {
							Expect(name).To(BeNil())
							return nil, errors.New("file does not exist")
						}
					}
					mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
						Expect(cfg.TaskDefinitions).To(HaveLen(1))
						Expect(cfg.TaskDefinitions[0].Path).To(Equal(runConfig.MintFilePath))
						Expect(cfg.MintDirectory).To(HaveLen(0))
						Expect(cfg.UseCache).To(BeTrue())
						receivedSpecifiedFileContent = cfg.TaskDefinitions[0].FileContents
						return &api.InitiateRunResult{
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
					Expect(receivedSpecifiedFileContent).To(Equal(originalSpecifiedFileContent))
				})
			})

			Context("when the directory is not found", func() {
				var originalSpecifiedFileContent string

				BeforeEach(func() {
					originalSpecifiedFileContent = "tasks:\n  - key: foo\n    run: echo 'bar'\n"
					runConfig.MintFilePath = "mint.yml"
					runConfig.MintDirectory = "some-dir"

					mockFS.MockReadDir = func(name string) ([]fs.DirEntry, error) {
						Expect(name).To(Equal("some-dir"))
						return nil, os.ErrNotExist
					}
					mockFS.MockOpen = func(name string) (fs.File, error) {
						if name == "mint.yml" {
							file := mocks.NewFile(originalSpecifiedFileContent)
							return file, nil
						} else {
							Expect(name).To(BeNil())
							return nil, errors.New("file does not exist")
						}
					}
				})

				It("returns an error", func() {
					_, err := service.InitiateRun(runConfig)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("could not be found"))
				})
			})
		})

		Context("with no specific mint file and a specific directory", func() {
			BeforeEach(func() {
				runConfig.MintFilePath = ""
				runConfig.MintDirectory = "some-dir"
			})

			It("returns an error", func() {
				_, err := service.InitiateRun(runConfig)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("the path to a mint-file must be provided"))
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
				DebugKey: runID,
			}

			mockAPI.MockGetDebugConnectionInfo = func(runId string) (api.DebugConnectionInfo, error) {
				Expect(runID).To(Equal(runId))
				fetchedConnectionInfo = true
				// Note: This is returning a matching private & public key. The real API returns different ones
				return api.DebugConnectionInfo{PrivateUserKey: privateTestKey, PublicHostKey: publicTestKey, Address: agentAddress}, nil
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

	Describe("logging in", func() {
		var (
			tokenBackend accesstoken.Backend
			stdout       strings.Builder
		)

		BeforeEach(func() {
			var err error
			tokenBackend, err = accesstoken.NewMemoryBackend()
			Expect(err).NotTo(HaveOccurred())

			stdout = strings.Builder{}
		})

		Context("when unable to obtain an auth code", func() {
			BeforeEach(func() {
				mockAPI.MockObtainAuthCode = func(oacc api.ObtainAuthCodeConfig) (*api.ObtainAuthCodeResult, error) {
					Expect(oacc.Code.DeviceName).To(Equal("some-device"))
					return nil, errors.New("error in obtain auth code")
				}
			})

			It("returns an error", func() {
				err := service.Login(cli.LoginConfig{
					DeviceName:         "some-device",
					AccessTokenBackend: tokenBackend,
					Stdout:             &stdout,
					OpenUrl: func(url string) error {
						Expect(url).To(Equal("https://cloud.local/_/auth/code?code=your-code"))
						return nil
					},
				})

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error in obtain auth code"))
			})
		})

		Context("with an auth code created", func() {
			BeforeEach(func() {
				mockAPI.MockObtainAuthCode = func(oacc api.ObtainAuthCodeConfig) (*api.ObtainAuthCodeResult, error) {
					Expect(oacc.Code.DeviceName).To(Equal("some-device"))
					return &api.ObtainAuthCodeResult{
						AuthorizationUrl: "https://cloud.local/_/auth/code?code=your-code",
						TokenUrl:         "https://cloud.local/api/auth/codes/code-uuid/token",
					}, nil
				}
			})

			Context("when polling results in authorized", func() {
				BeforeEach(func() {
					pollCounter := 0
					mockAPI.MockAcquireToken = func(tokenUrl string) (*api.AcquireTokenResult, error) {
						Expect(tokenUrl).To(Equal("https://cloud.local/api/auth/codes/code-uuid/token"))

						if pollCounter > 1 {
							pollCounter++
							return &api.AcquireTokenResult{State: "authorized", Token: "your-token"}, nil
						} else {
							pollCounter++
							return &api.AcquireTokenResult{State: "pending"}, nil
						}
					}
				})

				It("does not error", func() {
					err := service.Login(cli.LoginConfig{
						DeviceName:         "some-device",
						AccessTokenBackend: tokenBackend,
						Stdout:             &stdout,
						OpenUrl: func(url string) error {
							Expect(url).To(Equal("https://cloud.local/_/auth/code?code=your-code"))
							return nil
						},
					})

					Expect(err).NotTo(HaveOccurred())
				})

				It("stores the token", func() {
					err := service.Login(cli.LoginConfig{
						DeviceName:         "some-device",
						AccessTokenBackend: tokenBackend,
						Stdout:             &stdout,
						OpenUrl: func(url string) error {
							Expect(url).To(Equal("https://cloud.local/_/auth/code?code=your-code"))
							return nil
						},
					})
					Expect(err).NotTo(HaveOccurred())

					token, err := tokenBackend.Get()
					Expect(err).NotTo(HaveOccurred())
					Expect(token).To(Equal("your-token"))
				})

				It("indicates success and help in case the browser does not open", func() {
					err := service.Login(cli.LoginConfig{
						DeviceName:         "some-device",
						AccessTokenBackend: tokenBackend,
						Stdout:             &stdout,
						OpenUrl: func(url string) error {
							Expect(url).To(Equal("https://cloud.local/_/auth/code?code=your-code"))
							return nil
						},
					})
					Expect(err).NotTo(HaveOccurred())

					Expect(stdout.String()).To(ContainSubstring("https://cloud.local/_/auth/code?code=your-code"))
					Expect(stdout.String()).To(ContainSubstring("Authorized!"))
				})

				It("attempts to open the authorization URL, but doesn't care if it fails", func() {
					err := service.Login(cli.LoginConfig{
						DeviceName:         "some-device",
						AccessTokenBackend: tokenBackend,
						Stdout:             &stdout,
						OpenUrl: func(url string) error {
							Expect(url).To(Equal("https://cloud.local/_/auth/code?code=your-code"))
							return errors.New("couldn't open it")
						},
					})
					Expect(err).NotTo(HaveOccurred())

					Expect(stdout.String()).To(ContainSubstring("https://cloud.local/_/auth/code?code=your-code"))
					Expect(stdout.String()).To(ContainSubstring("Authorized!"))
				})
			})

			Context("when polling results in consumed", func() {
				BeforeEach(func() {
					pollCounter := 0
					mockAPI.MockAcquireToken = func(tokenUrl string) (*api.AcquireTokenResult, error) {
						Expect(tokenUrl).To(Equal("https://cloud.local/api/auth/codes/code-uuid/token"))

						if pollCounter > 1 {
							pollCounter++
							return &api.AcquireTokenResult{State: "consumed"}, nil
						} else {
							pollCounter++
							return &api.AcquireTokenResult{State: "pending"}, nil
						}
					}
				})

				It("errors", func() {
					err := service.Login(cli.LoginConfig{
						DeviceName:         "some-device",
						AccessTokenBackend: tokenBackend,
						Stdout:             &stdout,
						OpenUrl: func(url string) error {
							Expect(url).To(Equal("https://cloud.local/_/auth/code?code=your-code"))
							return nil
						},
					})

					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("already been used"))
				})

				It("does not store the token", func() {
					err := service.Login(cli.LoginConfig{
						DeviceName:         "some-device",
						AccessTokenBackend: tokenBackend,
						Stdout:             &stdout,
						OpenUrl: func(url string) error {
							Expect(url).To(Equal("https://cloud.local/_/auth/code?code=your-code"))
							return nil
						},
					})
					Expect(err).To(HaveOccurred())

					token, err := tokenBackend.Get()
					Expect(err).NotTo(HaveOccurred())
					Expect(token).To(Equal(""))
				})

				It("does not indicate success, but still helps in case the browser does not open", func() {
					err := service.Login(cli.LoginConfig{
						DeviceName:         "some-device",
						AccessTokenBackend: tokenBackend,
						Stdout:             &stdout,
						OpenUrl: func(url string) error {
							Expect(url).To(Equal("https://cloud.local/_/auth/code?code=your-code"))
							return nil
						},
					})
					Expect(err).To(HaveOccurred())

					Expect(stdout.String()).To(ContainSubstring("https://cloud.local/_/auth/code?code=your-code"))
					Expect(stdout.String()).NotTo(ContainSubstring("Authorized!"))
				})
			})

			Context("when polling results in expired", func() {
				BeforeEach(func() {
					pollCounter := 0
					mockAPI.MockAcquireToken = func(tokenUrl string) (*api.AcquireTokenResult, error) {
						Expect(tokenUrl).To(Equal("https://cloud.local/api/auth/codes/code-uuid/token"))

						if pollCounter > 1 {
							pollCounter++
							return &api.AcquireTokenResult{State: "expired"}, nil
						} else {
							pollCounter++
							return &api.AcquireTokenResult{State: "pending"}, nil
						}
					}
				})

				It("errors", func() {
					err := service.Login(cli.LoginConfig{
						DeviceName:         "some-device",
						AccessTokenBackend: tokenBackend,
						Stdout:             &stdout,
						OpenUrl: func(url string) error {
							Expect(url).To(Equal("https://cloud.local/_/auth/code?code=your-code"))
							return nil
						},
					})

					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("has expired"))
				})

				It("does not store the token", func() {
					err := service.Login(cli.LoginConfig{
						DeviceName:         "some-device",
						AccessTokenBackend: tokenBackend,
						Stdout:             &stdout,
						OpenUrl: func(url string) error {
							Expect(url).To(Equal("https://cloud.local/_/auth/code?code=your-code"))
							return nil
						},
					})
					Expect(err).To(HaveOccurred())

					token, err := tokenBackend.Get()
					Expect(err).NotTo(HaveOccurred())
					Expect(token).To(Equal(""))
				})

				It("does not indicate success, but still helps in case the browser does not open", func() {
					err := service.Login(cli.LoginConfig{
						DeviceName:         "some-device",
						AccessTokenBackend: tokenBackend,
						Stdout:             &stdout,
						OpenUrl: func(url string) error {
							Expect(url).To(Equal("https://cloud.local/_/auth/code?code=your-code"))
							return nil
						},
					})
					Expect(err).To(HaveOccurred())

					Expect(stdout.String()).To(ContainSubstring("https://cloud.local/_/auth/code?code=your-code"))
					Expect(stdout.String()).NotTo(ContainSubstring("Authorized!"))
				})
			})

			Context("when polling results in something else", func() {
				BeforeEach(func() {
					pollCounter := 0
					mockAPI.MockAcquireToken = func(tokenUrl string) (*api.AcquireTokenResult, error) {
						Expect(tokenUrl).To(Equal("https://cloud.local/api/auth/codes/code-uuid/token"))

						if pollCounter > 1 {
							pollCounter++
							return &api.AcquireTokenResult{State: "unexpected-state-here-uh-oh"}, nil
						} else {
							pollCounter++
							return &api.AcquireTokenResult{State: "pending"}, nil
						}
					}
				})

				It("errors", func() {
					err := service.Login(cli.LoginConfig{
						DeviceName:         "some-device",
						AccessTokenBackend: tokenBackend,
						Stdout:             &stdout,
						OpenUrl: func(url string) error {
							Expect(url).To(Equal("https://cloud.local/_/auth/code?code=your-code"))
							return nil
						},
					})

					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("is in an unexpected state"))
				})

				It("does not store the token", func() {
					err := service.Login(cli.LoginConfig{
						DeviceName:         "some-device",
						AccessTokenBackend: tokenBackend,
						Stdout:             &stdout,
						OpenUrl: func(url string) error {
							Expect(url).To(Equal("https://cloud.local/_/auth/code?code=your-code"))
							return nil
						},
					})
					Expect(err).To(HaveOccurred())

					token, err := tokenBackend.Get()
					Expect(err).NotTo(HaveOccurred())
					Expect(token).To(Equal(""))
				})

				It("does not indicate success, but still helps in case the browser does not open", func() {
					err := service.Login(cli.LoginConfig{
						DeviceName:         "some-device",
						AccessTokenBackend: tokenBackend,
						Stdout:             &stdout,
						OpenUrl: func(url string) error {
							Expect(url).To(Equal("https://cloud.local/_/auth/code?code=your-code"))
							return nil
						},
					})
					Expect(err).To(HaveOccurred())

					Expect(stdout.String()).To(ContainSubstring("https://cloud.local/_/auth/code?code=your-code"))
					Expect(stdout.String()).NotTo(ContainSubstring("Authorized!"))
				})
			})
		})
	})

	Describe("whoami", func() {
		var (
			stdout strings.Builder
		)

		BeforeEach(func() {
			stdout = strings.Builder{}
		})

		Context("when outputting json", func() {
			Context("when the request fails", func() {
				BeforeEach(func() {
					mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
						return nil, errors.New("uh oh can't figure out who you are")
					}
				})

				It("returns an error", func() {
					err := service.Whoami(cli.WhoamiConfig{
						Json:   true,
						Stdout: &stdout,
					})

					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("unable to determine details about the access token"))
					Expect(err.Error()).To(ContainSubstring("can't figure out who you are"))
				})
			})

			Context("when there is an email", func() {
				BeforeEach(func() {
					mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
						email := "someone@rwx.com"
						return &api.WhoamiResult{
							TokenKind:        "personal_access_token",
							OrganizationSlug: "rwx",
							UserEmail:        &email,
						}, nil
					}
				})

				It("writes the token kind, organization, and user", func() {
					err := service.Whoami(cli.WhoamiConfig{
						Json:   true,
						Stdout: &stdout,
					})

					Expect(err).NotTo(HaveOccurred())
					Expect(stdout.String()).To(ContainSubstring(`"token_kind": "personal_access_token"`))
					Expect(stdout.String()).To(ContainSubstring(`"organization_slug": "rwx"`))
					Expect(stdout.String()).To(ContainSubstring(`"user_email": "someone@rwx.com"`))
				})
			})

			Context("when there is not an email", func() {
				BeforeEach(func() {
					mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
						return &api.WhoamiResult{
							TokenKind:        "organization_access_token",
							OrganizationSlug: "rwx",
						}, nil
					}
				})

				It("writes the token kind and organization", func() {
					err := service.Whoami(cli.WhoamiConfig{
						Json:   true,
						Stdout: &stdout,
					})

					Expect(err).NotTo(HaveOccurred())
					Expect(stdout.String()).To(ContainSubstring(`"token_kind": "organization_access_token"`))
					Expect(stdout.String()).To(ContainSubstring(`"organization_slug": "rwx"`))
					Expect(stdout.String()).NotTo(ContainSubstring(`"user_email"`))
				})
			})
		})

		Context("when outputting plaintext", func() {
			Context("when the request fails", func() {
				BeforeEach(func() {
					mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
						return nil, errors.New("uh oh can't figure out who you are")
					}
				})

				It("returns an error", func() {
					err := service.Whoami(cli.WhoamiConfig{
						Json:   false,
						Stdout: &stdout,
					})

					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("unable to determine details about the access token"))
					Expect(err.Error()).To(ContainSubstring("can't figure out who you are"))
				})
			})

			Context("when there is an email", func() {
				BeforeEach(func() {
					mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
						email := "someone@rwx.com"
						return &api.WhoamiResult{
							TokenKind:        "personal_access_token",
							OrganizationSlug: "rwx",
							UserEmail:        &email,
						}, nil
					}
				})

				It("writes the token kind, organization, and user", func() {
					err := service.Whoami(cli.WhoamiConfig{
						Json:   false,
						Stdout: &stdout,
					})

					Expect(err).NotTo(HaveOccurred())
					Expect(stdout.String()).To(ContainSubstring("Token Kind: personal access token"))
					Expect(stdout.String()).To(ContainSubstring("Organization: rwx"))
					Expect(stdout.String()).To(ContainSubstring("User: someone@rwx.com"))
				})
			})

			Context("when there is not an email", func() {
				BeforeEach(func() {
					mockAPI.MockWhoami = func() (*api.WhoamiResult, error) {
						return &api.WhoamiResult{
							TokenKind:        "organization_access_token",
							OrganizationSlug: "rwx",
						}, nil
					}
				})

				It("writes the token kind and organization", func() {
					err := service.Whoami(cli.WhoamiConfig{
						Json:   false,
						Stdout: &stdout,
					})

					Expect(err).NotTo(HaveOccurred())
					Expect(stdout.String()).To(ContainSubstring("Token Kind: organization access token"))
					Expect(stdout.String()).To(ContainSubstring("Organization: rwx"))
					Expect(stdout.String()).NotTo(ContainSubstring("User:"))
				})
			})
		})
	})

	Describe("setting secrets", func() {
		var (
			stdout strings.Builder
		)

		BeforeEach(func() {
			var err error
			Expect(err).NotTo(HaveOccurred())

			stdout = strings.Builder{}
		})

		Context("when unable to set secrets", func() {
			BeforeEach(func() {
				mockAPI.MockSetSecretsInVault = func(ssivc api.SetSecretsInVaultConfig) (*api.SetSecretsInVaultResult, error) {
					Expect(ssivc.VaultName).To(Equal("default"))
					Expect(ssivc.Secrets[0].Name).To(Equal("ABC"))
					Expect(ssivc.Secrets[0].Secret).To(Equal("123"))
					return nil, errors.New("error setting secret")
				}
			})

			It("returns an error", func() {
				err := service.SetSecretsInVault(cli.SetSecretsInVaultConfig{
					Vault:   "default",
					Secrets: []string{"ABC=123"},
					Stdout:  &stdout,
				})

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error setting secret"))
			})
		})

		Context("with secrets set", func() {
			BeforeEach(func() {
				mockAPI.MockSetSecretsInVault = func(ssivc api.SetSecretsInVaultConfig) (*api.SetSecretsInVaultResult, error) {
					Expect(ssivc.VaultName).To(Equal("default"))
					Expect(ssivc.Secrets[0].Name).To(Equal("ABC"))
					Expect(ssivc.Secrets[0].Secret).To(Equal("123"))
					Expect(ssivc.Secrets[1].Name).To(Equal("DEF"))
					Expect(ssivc.Secrets[1].Secret).To(Equal("\"xyz\""))
					return &api.SetSecretsInVaultResult{
						SetSecrets: []string{"ABC", "DEF"},
					}, nil
				}
			})

			It("is successful", func() {
				err := service.SetSecretsInVault(cli.SetSecretsInVaultConfig{
					Vault:   "default",
					Secrets: []string{"ABC=123", "DEF=\"xyz\""},
					Stdout:  &stdout,
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(stdout.String()).To(Equal("\nSuccessfully set the following secrets: ABC, DEF"))
			})
		})

		Context("when reading secrets from a file", func() {
			BeforeEach(func() {
				mockAPI.MockSetSecretsInVault = func(ssivc api.SetSecretsInVaultConfig) (*api.SetSecretsInVaultResult, error) {
					sort.Slice(ssivc.Secrets, func(i, j int) bool {
						return ssivc.Secrets[i].Name < ssivc.Secrets[j].Name
					})
					Expect(ssivc.VaultName).To(Equal("default"))
					Expect(ssivc.Secrets[0].Name).To(Equal("A"))
					Expect(ssivc.Secrets[0].Secret).To(Equal("123"))
					Expect(ssivc.Secrets[1].Name).To(Equal("B"))
					Expect(ssivc.Secrets[1].Secret).To(Equal("xyz"))
					Expect(ssivc.Secrets[2].Name).To(Equal("C"))
					Expect(ssivc.Secrets[2].Secret).To(Equal("q\\nqq"))
					Expect(ssivc.Secrets[3].Name).To(Equal("D"))
					Expect(ssivc.Secrets[3].Secret).To(Equal("a multiline\nstring\nspanning lines"))
					return &api.SetSecretsInVaultResult{
						SetSecrets: []string{"A", "B", "C", "D"},
					}, nil
				}

				mockFS.MockOpen = func(name string) (fs.File, error) {
					Expect(name).To(Equal("secrets.txt"))
					file := mocks.NewFile("A=123\nB=\"xyz\"\nC='q\\nqq'\nD=\"a multiline\nstring\nspanning lines\"")
					return file, nil
				}
			})

			It("is successful", func() {
				err := service.SetSecretsInVault(cli.SetSecretsInVaultConfig{
					Vault:   "default",
					Secrets: []string{},
					File:    "secrets.txt",
					Stdout:  &stdout,
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(stdout.String()).To(Equal("\nSuccessfully set the following secrets: A, B, C, D"))
			})
		})
	})

	Describe("updating leaves", func() {
		var (
			stdout strings.Builder
			stderr strings.Builder
		)

		BeforeEach(func() {
			stdout = strings.Builder{}
			stderr = strings.Builder{}
		})

		Context("when no files provided", func() {
			Context("when no yaml files found in the default directory", func() {
				BeforeEach(func() {
					mockFS.MockReadDir = func(name string) ([]fs.DirEntry, error) {
						return []fs.DirEntry{
							mocks.DirEntry{FileName: "foo.txt"},
							mocks.DirEntry{FileName: "bar.json"},
						}, nil
					}
				})

				It("returns an error", func() {
					err := service.UpdateLeaves(cli.UpdateLeavesConfig{
						Stdout:                   &stdout,
						Stderr:                   &stderr,
						Files:                    []string{},
						DefaultDir:               ".mint",
						ReplacementVersionPicker: cli.PickLatestMajorVersion,
					})

					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("no files provided, and no yaml files found in directory .mint"))
				})
			})

			Context("when yaml files are found in the specified directory", func() {
				var openedFiles []string

				BeforeEach(func() {
					openedFiles = []string{}

					mockFS.MockReadDir = func(name string) ([]fs.DirEntry, error) {
						return []fs.DirEntry{
							mocks.DirEntry{FileName: "foo.txt"},
							mocks.DirEntry{FileName: "bar.yaml"},
							mocks.DirEntry{FileName: "baz.yml"},
						}, nil
					}
					mockFS.MockOpen = func(path string) (fs.File, error) {
						openedFiles = append(openedFiles, path)
						file := mocks.NewFile("")
						return file, nil
					}
					mockFS.MockCreate = func(path string) (fs.File, error) {
						openedFiles = append(openedFiles, path)
						file := mocks.NewFile("")
						return file, nil
					}

					mockAPI.MockGetLeafVersions = func() (*api.LeafVersionsResult, error) {
						return &api.LeafVersionsResult{
							LatestMajor: map[string]string{},
						}, nil
					}
				})

				It("uses the default directory", func() {
					err := service.UpdateLeaves(cli.UpdateLeavesConfig{
						Stdout:                   &stdout,
						Stderr:                   &stderr,
						Files:                    []string{},
						DefaultDir:               ".mint",
						ReplacementVersionPicker: cli.PickLatestMajorVersion,
					})

					Expect(err).NotTo(HaveOccurred())
					Expect(openedFiles).To(ContainElement(".mint/bar.yaml"))
					Expect(openedFiles).To(ContainElement(".mint/baz.yml"))
				})
			})
		})

		Context("with files", func() {
			var originalFiles map[string]string
			var writtenFiles map[string]*mocks.File
			var majorLeafVersions map[string]string
			var minorLeafVersions map[string]map[string]string
			var leafError error

			BeforeEach(func() {
				originalFiles = make(map[string]string)
				writtenFiles = make(map[string]*mocks.File)
				majorLeafVersions = make(map[string]string)
				minorLeafVersions = make(map[string]map[string]string)
				leafError = nil

				mockFS.MockOpen = func(path string) (fs.File, error) {
					content, ok := originalFiles[path]
					if !ok {
						return nil, errors.New("file not found")
					}
					file := mocks.NewFile(content)
					return file, nil
				}
				mockFS.MockCreate = func(path string) (fs.File, error) {
					file := mocks.NewFile("")
					writtenFiles[path] = file
					return file, nil
				}
				mockAPI.MockGetLeafVersions = func() (*api.LeafVersionsResult, error) {
					return &api.LeafVersionsResult{
						LatestMajor: majorLeafVersions,
						LatestMinor: minorLeafVersions,
					}, leafError
				}
			})

			Context("when the leaf versions cannot be retrieved", func() {
				BeforeEach(func() {
					leafError = errors.New("cannot get leaf versions")
					originalFiles["foo.yaml"] = ""
				})

				It("returns an error", func() {
					err := service.UpdateLeaves(cli.UpdateLeavesConfig{
						Stdout:                   &stdout,
						Stderr:                   &stderr,
						Files:                    []string{"foo.yaml"},
						ReplacementVersionPicker: cli.PickLatestMajorVersion,
					})

					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("cannot get leaf versions"))
				})
			})

			Context("when all leaves are already up-to-date", func() {
				BeforeEach(func() {
					majorLeafVersions["mint/setup-node"] = "1.2.3"
					originalFiles["foo.yaml"] = `
					tasks:
						- key: foo
							call: mint/setup-node 1.2.3
					`
				})

				It("does not change the file content", func() {
					err := service.UpdateLeaves(cli.UpdateLeavesConfig{
						Stdout:                   &stdout,
						Stderr:                   &stderr,
						Files:                    []string{"foo.yaml"},
						ReplacementVersionPicker: cli.PickLatestMajorVersion,
					})

					Expect(err).NotTo(HaveOccurred())
					Expect(writtenFiles["foo.yaml"].Buffer.String()).To(Equal(`
					tasks:
						- key: foo
							call: mint/setup-node 1.2.3
					`))
				})

				It("indicates no leaves were updated", func() {
					err := service.UpdateLeaves(cli.UpdateLeavesConfig{
						Stdout:                   &stdout,
						Stderr:                   &stderr,
						Files:                    []string{"foo.yaml"},
						ReplacementVersionPicker: cli.PickLatestMajorVersion,
					})

					Expect(err).NotTo(HaveOccurred())
					Expect(stdout.String()).To(ContainSubstring("No leaves to update."))
				})
			})

			Context("when there are leaves to update across multiple files", func() {
				BeforeEach(func() {
					majorLeafVersions["mint/setup-node"] = "1.2.3"
					majorLeafVersions["mint/setup-ruby"] = "1.0.1"
					originalFiles["foo.yaml"] = `
					tasks:
						- key: foo
							call: mint/setup-node 1.0.1
						- key: bar
							call: mint/setup-ruby 0.0.1
					`
					originalFiles["bar.yaml"] = `
					tasks:
						- key: foo
							call: mint/setup-ruby 1.0.0
					`
				})

				It("updates all files", func() {
					err := service.UpdateLeaves(cli.UpdateLeavesConfig{
						Stdout:                   &stdout,
						Stderr:                   &stderr,
						Files:                    []string{"foo.yaml", "bar.yaml"},
						ReplacementVersionPicker: cli.PickLatestMajorVersion,
					})

					Expect(err).NotTo(HaveOccurred())
					Expect(writtenFiles["foo.yaml"].Buffer.String()).To(Equal(`
					tasks:
						- key: foo
							call: mint/setup-node 1.2.3
						- key: bar
							call: mint/setup-ruby 1.0.1
					`))
					Expect(writtenFiles["bar.yaml"].Buffer.String()).To(Equal(`
					tasks:
						- key: foo
							call: mint/setup-ruby 1.0.1
					`))
				})

				It("indicates leaves were updated", func() {
					err := service.UpdateLeaves(cli.UpdateLeavesConfig{
						Stdout:                   &stdout,
						Stderr:                   &stderr,
						Files:                    []string{"foo.yaml", "bar.yaml"},
						ReplacementVersionPicker: cli.PickLatestMajorVersion,
					})

					Expect(err).NotTo(HaveOccurred())
					Expect(stdout.String()).To(ContainSubstring("Updated the following leaves:"))
					Expect(stdout.String()).To(ContainSubstring("mint/setup-node 1.0.1 → 1.2.3"))
					Expect(stdout.String()).To(ContainSubstring("mint/setup-ruby 0.0.1 → 1.0.1"))
					Expect(stdout.String()).To(ContainSubstring("mint/setup-ruby 1.0.0 → 1.0.1"))
				})
			})

			Context("when a leaf cannot be found", func() {
				BeforeEach(func() {
					originalFiles["foo.yaml"] = `
					tasks:
						- key: foo
							call: mint/setup-node 1.0.1
					`
				})

				It("does not modify the file", func() {
					err := service.UpdateLeaves(cli.UpdateLeavesConfig{
						Stdout:                   &stdout,
						Stderr:                   &stderr,
						Files:                    []string{"foo.yaml"},
						ReplacementVersionPicker: cli.PickLatestMajorVersion,
					})

					Expect(err).NotTo(HaveOccurred())
					Expect(writtenFiles["foo.yaml"].Buffer.String()).To(Equal(`
					tasks:
						- key: foo
							call: mint/setup-node 1.0.1
					`))
				})

				It("indicates a leaf could not be found", func() {
					err := service.UpdateLeaves(cli.UpdateLeavesConfig{
						Stdout:                   &stdout,
						Stderr:                   &stderr,
						Files:                    []string{"foo.yaml"},
						ReplacementVersionPicker: cli.PickLatestMajorVersion,
					})

					Expect(err).NotTo(HaveOccurred())
					Expect(stderr.String()).To(ContainSubstring(`Unable to find the leaf "mint/setup-node"; skipping it.`))
				})
			})

			Context("when a leaf reference is a later version than the latest major", func() {
				BeforeEach(func() {
					majorLeafVersions["mint/setup-node"] = "1.0.3"
					originalFiles["foo.yaml"] = `
					tasks:
						- key: foo
							call: mint/setup-node 1.1.1
					`
				})

				It("updates the leaf", func() {
					err := service.UpdateLeaves(cli.UpdateLeavesConfig{
						Stdout:                   &stdout,
						Stderr:                   &stderr,
						Files:                    []string{"foo.yaml"},
						ReplacementVersionPicker: cli.PickLatestMajorVersion,
					})

					Expect(err).NotTo(HaveOccurred())
					Expect(writtenFiles["foo.yaml"].Buffer.String()).To(Equal(`
					tasks:
						- key: foo
							call: mint/setup-node 1.0.3
					`))
				})
			})

			Context("when a leaf reference is a major version behind the latest", func() {
				BeforeEach(func() {
					majorLeafVersions["mint/setup-node"] = "2.0.3"
					minorLeafVersions["mint/setup-node"] = make(map[string]string)
					minorLeafVersions["mint/setup-node"]["2"] = "2.0.3"
					minorLeafVersions["mint/setup-node"]["1"] = "1.1.1"
				})

				JustBeforeEach(func() {
					Expect(service.UpdateLeaves(cli.UpdateLeavesConfig{
						Stdout:                   &stdout,
						Stderr:                   &stderr,
						Files:                    []string{"foo.yaml"},
						ReplacementVersionPicker: cli.PickLatestMinorVersion,
					})).To(Succeed())
				})

				Context("while referencing the latest minor version", func() {
					BeforeEach(func() {
						originalFiles["foo.yaml"] = `
					tasks:
						- key: foo
							call: mint/setup-node 1.1.1
						`
					})

					It("does not modify the file", func() {
						Expect(writtenFiles["foo.yaml"].Buffer.String()).To(Equal(`
					tasks:
						- key: foo
							call: mint/setup-node 1.1.1
						`))
					})

					It("indicates no leaves were updated", func() {
						Expect(stdout.String()).To(ContainSubstring("No leaves to update."))
					})
				})

				Context("while not referencing the latest minor version", func() {
					BeforeEach(func() {
						originalFiles["foo.yaml"] = `
					tasks:
						- key: foo
							call: mint/setup-node 1.0.9
						`
					})

					It("updates the file", func() {
						Expect(writtenFiles["foo.yaml"].Buffer.String()).To(Equal(`
					tasks:
						- key: foo
							call: mint/setup-node 1.1.1
						`))
					})

					It("indicates that a leaf was updated", func() {
						Expect(stdout.String()).To(ContainSubstring("Updated the following leaves:"))
						Expect(stdout.String()).To(ContainSubstring("mint/setup-node 1.0.9 → 1.1.1"))
					})
				})
			})
		})
	})

	Describe("linting", func() {
		var truncatedDiff bool
		var lintConfig cli.LintConfig
		var stdout bytes.Buffer
		var memfs *memoryfs.MemoryFS

		BeforeEach(func() {
			truncatedDiff = format.TruncatedDiff
			format.TruncatedDiff = false

			memfs = memoryfs.NewFS()
			Expect(memfs.MkdirAll("/some/path/to")).NotTo(HaveOccurred())
			Expect(memfs.Chdir("/some/path/to")).NotTo(HaveOccurred())
			config.FileSystem = memfs

			stdout.Reset()
			lintConfig = cli.LintConfig{Output: io.Writer(&stdout), OutputFormat: cli.LintOutputNone}
		})

		AfterEach(func() {
			format.TruncatedDiff = truncatedDiff
		})

		Context("with multiple errors", func() {
			BeforeEach(func() {
				Expect(memfs.WriteFileString("mint1.yml", "mint1 contents")).NotTo(HaveOccurred())
				Expect(memfs.WriteFileString(".mint/base.yml", ".mint/base.yml contents")).NotTo(HaveOccurred())
				Expect(memfs.WriteFileString(".mint/base.json", ".mint/base.json contents")).NotTo(HaveOccurred())

				lintConfig.MintFilePaths = []string{"mint1.yml", ".mint/base.yml"}

				mockAPI.MockLint = func(cfg api.LintConfig) (*api.LintResult, error) {
					Expect(cfg.TaskDefinitions).To(HaveLen(2))
					return &api.LintResult{
						Problems: []api.LintProblem{
							{Severity: "error", Message: "message 1\nmessage 1a", FileName: "mint1.yml", Line: api.NewNullInt(11), Column: api.NewNullInt(22), Advice: "advice 1\nadvice 1a"},
							{Severity: "error", Message: "message 2\nmessage 2a", FileName: "mint1.yml", Line: api.NewNullInt(15), Column: api.NewNullInt(4)},
							{Severity: "warning", Message: "message 3", FileName: "mint1.yml", Line: api.NewNullInt(2), Column: api.NewNullInt(6), Advice: "advice 3\nadvice 3a"},
							{Severity: "warning", Message: "message 4", FileName: "mint1.yml", Line: api.NullInt{IsNull: true}, Column: api.NullInt{IsNull: true}},
						},
					}, nil
				}
			})

			Context("using oneline output", func() {
				BeforeEach(func() {
					lintConfig.OutputFormat = cli.LintOutputOneLine
				})

				It("orders and lists only files", func() {
					_, err := service.Lint(lintConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(stdout.String()).To(Equal(`warning mint1.yml - message 4
warning mint1.yml:2:6 - message 3
error   mint1.yml:11:22 - message 1 message 1a
error   mint1.yml:15:4 - message 2 message 2a
`))
				})
			})

			Context("using multiline output", func() {
				BeforeEach(func() {
					lintConfig.OutputFormat = cli.LintOutputMultiLine
				})

				It("orders and lists only files", func() {
					_, err := service.Lint(lintConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(stdout.String()).To(Equal(`mint1.yml  [warning]
message 4

mint1.yml:2:6  [warning]
message 3
advice 3
advice 3a

mint1.yml:11:22  [error]
message 1
message 1a
advice 1
advice 1a

mint1.yml:15:4  [error]
message 2
message 2a
`))
				})
			})

			Context("using none output", func() {
				BeforeEach(func() {
					lintConfig.OutputFormat = cli.LintOutputNone
				})

				It("doesn't output", func() {
					_, err := service.Lint(lintConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(stdout.String()).To(Equal(""))
				})
			})
		})

		Context("with no errors", func() {
			BeforeEach(func() {
				Expect(memfs.WriteFileString("mint1.yml", "mint1 contents")).NotTo(HaveOccurred())
				Expect(memfs.WriteFileString(".mint/base.yml", ".mint/base.yml contents")).NotTo(HaveOccurred())
				Expect(memfs.WriteFileString(".mint/base.json", ".mint/base.json contents")).NotTo(HaveOccurred())

				lintConfig.MintFilePaths = []string{"mint1.yml", ".mint/base.yml"}

				mockAPI.MockLint = func(cfg api.LintConfig) (*api.LintResult, error) {
					Expect(cfg.TaskDefinitions).To(HaveLen(2))
					return &api.LintResult{}, nil
				}
			})

			Context("using oneline output", func() {
				BeforeEach(func() {
					lintConfig.OutputFormat = cli.LintOutputOneLine
				})

				It("doesn't output", func() {
					_, err := service.Lint(lintConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(stdout.String()).To(Equal(""))
				})
			})

			Context("using multiline output", func() {
				BeforeEach(func() {
					lintConfig.OutputFormat = cli.LintOutputMultiLine
				})

				It("doesn't output", func() {
					_, err := service.Lint(lintConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(stdout.String()).To(Equal(""))
				})
			})

			Context("using none output", func() {
				BeforeEach(func() {
					lintConfig.OutputFormat = cli.LintOutputNone
				})

				It("doesn't output", func() {
					_, err := service.Lint(lintConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(stdout.String()).To(Equal(""))
				})
			})
		})
	})
})
