package cli

import (
	"io"

	"github.com/rwx-research/mint-cli/internal/accesstoken"
	"github.com/rwx-research/mint-cli/internal/api"
	"github.com/rwx-research/mint-cli/internal/errors"
)

type Config struct {
	APIClient APIClient
	SSHClient SSHClient
	Stdout    io.Writer
	Stderr    io.Writer
}

func (c Config) Validate() error {
	if c.APIClient == nil {
		return errors.New("missing Mint client")
	}

	if c.SSHClient == nil {
		return errors.New("missing SSH client constructor")
	}

	if c.Stdout == nil {
		return errors.New("missing Stdout")
	}

	if c.Stderr == nil {
		return errors.New("missing Stderr")
	}

	return nil
}

type DebugTaskConfig struct {
	DebugKey string
}

func (c DebugTaskConfig) Validate() error {
	if c.DebugKey == "" {
		return errors.New("you must specify a run ID, a task ID, or a Mint Cloud URL")
	}

	return nil
}

type InitiateRunConfig struct {
	InitParameters map[string]string
	Json           bool
	MintDirectory  string
	MintFilePath   string
	NoCache        bool
	TargetedTasks  []string
	Title          string
}

func (c InitiateRunConfig) Validate() error {
	if c.MintFilePath == "" {
		return errors.New("the path to a mint-file must be provided using the --file flag.")
	}

	return nil
}

type InitiateDispatchConfig struct {
	DispatchKey string
	Params      map[string]string
	Ref         string
	Json        bool
	Title       string
}

func (c InitiateDispatchConfig) Validate() error {
	if c.DispatchKey == "" {
		return errors.New("a dispatch key must be provided")
	}

	return nil
}

type GetDispatchConfig struct {
	DispatchId string
}

type GetDispatchRun struct {
	RunId  string
	RunUrl string
}

type LintOutputFormat int

const (
	LintOutputNone LintOutputFormat = iota
	LintOutputOneLine
	LintOutputMultiLine
)

type LintConfig struct {
	MintDirectory string
	MintFilePaths []string
	OutputFormat  LintOutputFormat
}

func (c LintConfig) Validate() error {
	return nil
}

func NewLintConfig(filePaths []string, mintDir string, formatString string) (LintConfig, error) {
	var format LintOutputFormat

	switch formatString {
	case "none":
		format = LintOutputNone
	case "oneline":
		format = LintOutputOneLine
	case "multiline":
		format = LintOutputMultiLine
	default:
		return LintConfig{}, errors.New("unknown output format, expected one of: none, oneline, multiline")
	}

	return LintConfig{
		MintDirectory: mintDir,
		MintFilePaths: filePaths,
		OutputFormat:  format,
	}, nil
}

type LoginConfig struct {
	DeviceName         string
	AccessTokenBackend accesstoken.Backend
	OpenUrl            func(url string) error
}

func (c LoginConfig) Validate() error {
	if c.DeviceName == "" {
		return errors.New("the device name must be provided")
	}

	return nil
}

type WhoamiConfig struct {
	Json bool
}

func (c WhoamiConfig) Validate() error {
	return nil
}

type SetSecretsInVaultConfig struct {
	Secrets []string
	Vault   string
	File    string
}

func (c SetSecretsInVaultConfig) Validate() error {
	if c.Vault == "" {
		return errors.New("the vault name must be provided")
	}

	if len(c.Secrets) == 0 && c.File == "" {
		return errors.New("the secrets to set must be provided")
	}

	return nil
}

type UpdateBaseConfig struct {
	DefaultDir               string
	Files                    []string
	ReplacementVersionPicker func(versions api.LeafVersionsResult, leaf string, major string) (string, error) // TODO: no
}

func (c UpdateBaseConfig) Validate() error {
	if len(c.Files) == 0 && c.DefaultDir == "" {
		return errors.New("a default directory must be provided if not specifying files explicitly")
	}

	if c.ReplacementVersionPicker == nil {
		return errors.New("a replacement version picker must be provided")
	}

	return nil
}

type UpdateLeavesConfig struct {
	DefaultDir               string
	Files                    []string
	ReplacementVersionPicker func(versions api.LeafVersionsResult, leaf string, major string) (string, error)
}

func (c UpdateLeavesConfig) Validate() error {
	if len(c.Files) == 0 && c.DefaultDir == "" {
		return errors.New("a default directory must be provided if not specifying files explicitly")
	}

	if c.ReplacementVersionPicker == nil {
		return errors.New("a replacement version picker must be provided")
	}

	return nil
}

type ResolveBaseConfig struct {
	DefaultDir string
	Files      []string
	Os         string
	Tag        string
	Arch       string
}

func (c ResolveBaseConfig) Validate() error {
	if c.DefaultDir == "" {
		return errors.New("a default directory must be provided")
	}

	return nil
}

type BaseLayerSpec struct {
	Os   string `yaml:"os"`
	Tag  string `yaml:"tag"`
	Arch string `yaml:"arch"`
}

func (b BaseLayerSpec) Merge(other BaseLayerSpec) BaseLayerSpec {
	os := b.Os
	if other.Os != "" {
		os = other.Os
	}

	tag := b.Tag
	if other.Tag != "" {
		tag = other.Tag
	}

	arch := b.Arch
	if other.Arch != "" {
		arch = other.Arch
	}

	return BaseLayerSpec{
		Os:   os,
		Tag:  tag,
		Arch: arch,
	}
}

type BaseLayerRunFile struct {
	Spec         BaseLayerSpec
	ResolvedBase BaseLayerSpec
	Filepath     string
	Error        error
	Updated      bool
}

type ResolveBaseResult struct {
	ErroredRunFiles []BaseLayerRunFile
	UpdatedRunFiles []BaseLayerRunFile
}

func (r ResolveBaseResult) HasChanges() bool {
	return len(r.ErroredRunFiles) > 0 || len(r.UpdatedRunFiles) > 0
}

type ResolveLeavesConfig struct {
	DefaultDir          string
	Files               []string
	LatestVersionPicker func(versions api.LeafVersionsResult, leaf string, _ string) (string, error)
}

func (c ResolveLeavesConfig) PickLatestVersion(versions api.LeafVersionsResult, leaf string) (string, error) {
	return c.LatestVersionPicker(versions, leaf, "")
}

func (c ResolveLeavesConfig) Validate() error {
	if c.LatestVersionPicker == nil {
		return errors.New("a latest version picker must be provided")
	}

	return nil
}

type ResolveLeavesResult struct {
	ResolvedLeaves map[string]string
}

func (r ResolveLeavesResult) HasChanges() bool {
	return len(r.ResolvedLeaves) > 0
}
