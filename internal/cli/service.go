package cli

import (
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/rwx-research/mint-cli/internal/client"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

type Service struct {
	Config
}

func NewService(cfg Config) (Service, error) {
	if err := cfg.Validate(); err != nil {
		return Service{}, errors.Wrap(err, "validation failed")
	}

	return Service{cfg}, nil
}

func (s Service) InitiateRun(cfg InitiateRunConfig) (*url.URL, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	paths := []string{cfg.MintFilePath}
	if cfg.MintFilePath == "" {
		paths, err = s.yamlFilePathsInDirectory(cfg.MintDirectory)
		if err != nil {
			return nil, errors.Wrap(err, "unable to find yaml files in directory")
		}
	}

	if len(paths) == 0 {
		return nil, errors.Errorf("No run definitions provided! Add a run definition to your %s directory, or use `--file`", cfg.MintDirectory)
	}

	taskDefinitions, err := s.taskDefinitionsFromPaths(paths)
	if err != nil {
		return nil, errors.Wrap(err, "unable to read provided files")
	}

	for _, taskDefinition := range taskDefinitions {
		if err := validateYAML(taskDefinition.FileContents); err != nil {
			return nil, errors.Wrapf(err, "unable to parse %q", taskDefinition.Path)
		}
	}

	runURL, err := s.Client.InitiateRun(client.InitiateRunConfig{
		InitializationParameters: cfg.InitParameters,
		TaskDefinitions:          taskDefinitions,
		TargetedTaskKey:          cfg.TargetedTask,
		UseCache:                 !cfg.NoCache,
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to initiate run")
	}

	return runURL, nil
}

func (s Service) taskDefinitionsFromPaths(paths []string) ([]client.TaskDefinition, error) {
	taskDefinitions := make([]client.TaskDefinition, 0)

	for _, path := range paths {
		fd, err := s.FileSystem.Open(path)
		if err != nil {
			return nil, errors.Wrapf(err, "error while opening %q", path)
		}
		defer fd.Close()

		fileContent, err := io.ReadAll(fd)
		if err != nil {
			return nil, errors.Wrapf(err, "error while reading %q", path)
		}

		taskDefinitions = append(taskDefinitions, client.TaskDefinition{
			Path:         path,
			FileContents: string(fileContent),
		})
	}

	return taskDefinitions, nil
}

func (s Service) yamlFilePathsInDirectory(dir string) ([]string, error) {
	paths := make([]string, 0)

	files, err := s.FileSystem.ReadDir(dir)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to read %q", dir)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if !strings.HasSuffix(file.Name(), ".yml") && !strings.HasSuffix(file.Name(), ".yaml") {
			continue
		}

		paths = append(paths, filepath.Join(dir, file.Name()))
	}

	return paths, nil
}

func validateYAML(body string) error {
	contentMap := make(map[string]any)
	if err := yaml.Unmarshal([]byte(body), &contentMap); err != nil {
		return errors.WithStack(err)
	}

	return nil
}
