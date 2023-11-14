package cli

import (
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/rwx-research/mint-cli/internal/client"

	"gopkg.in/yaml.v3"
)

type Service struct {
	Config
}

func NewService(cfg Config) (Service, error) {
	if err := cfg.Validate(); err != nil {
		// TODO: Wrap
		return Service{}, err
	}

	return Service{cfg}, nil
}

func (s Service) InitiateRun(cfg InitiateRunConfig) (*url.URL, error) {
	err := cfg.Validate()
	if err != nil {
		// TODO: Wrap
		return nil, err
	}

	paths := []string{cfg.MintFilePath}
	if cfg.MintFilePath == "" {
		paths, err = s.yamlFilePathsInDirectory(cfg.MintDirectory)
		if err != nil {
			// TODO: Wrap
			return nil, err
		}
	}

	if len(paths) == 0 {
		// TODO: Custom error
		return nil, fmt.Errorf("No run definitions provided! Add a run definition to your %s directory, or use `--file`", cfg.MintDirectory)
	}

	taskDefinitions, err := s.taskDefinitionsFromPaths(paths)
	if err != nil {
		// TODO: Wrap
		return nil, err
	}

	for _, taskDefinition := range taskDefinitions {
		if err := validateYAML(taskDefinition.FileContents); err != nil {
			// TODO: Custom error
			return nil, fmt.Errorf("Parsing error encountered within the definitions at %s:\n\n%s", taskDefinition.Path, err.Error())
		}
	}

	runURL, err := s.Client.InitiateRun(client.InitiateRunConfig{
		InitializationParameters: cfg.InitParameters,
		TaskDefinitions:          taskDefinitions,
		TargetedTaskKey:          cfg.TargetedTask,
		UseCache:                 !cfg.NoCache,
	})
	if err != nil {
		// TODO: Wrap
		return nil, err
	}

	return runURL, nil
}

func (s Service) taskDefinitionsFromPaths(paths []string) ([]client.TaskDefinition, error) {
	taskDefinitions := make([]client.TaskDefinition, 0)

	for _, path := range paths {
		fd, err := s.FileSystem.Open(path)
		if err != nil {
			// TODO: Wrap
			return nil, err
		}
		defer fd.Close()

		fileContent, err := io.ReadAll(fd)
		if err != nil {
			// TODO: Wrap
			return nil, err
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
		return nil, err
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
	// TODO: Wrap
	return yaml.Unmarshal([]byte(body), &contentMap)
}
