package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rwx-research/mint-cli/internal/api"
	"github.com/rwx-research/mint-cli/internal/errors"
	"github.com/rwx-research/mint-cli/internal/fs"
)

type MintDirectoryEntry = api.MintDirectoryEntry
type TaskDefinition = api.TaskDefinition

type MintYAMLFile struct {
	Entry MintDirectoryEntry
	Doc   *YAMLDoc
}

// findMintDirectoryPath returns a configured directory, if it exists, or walks up
// from the working directory to find a .mint directory.
func findMintDirectoryPath(configuredDirectory string) (string, error) {
	if configuredDirectory != "" {
		return configuredDirectory, nil
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", errors.Wrap(err, "unable to determine the working directory")
	}

	// otherwise, walk up the working directory looking at each basename
	for {
		workingDirHasMintDir, err := fs.Exists(filepath.Join(workingDirectory, ".mint"))
		if err != nil {
			return "", errors.Wrapf(err, "unable to determine if .mint exists in %q", workingDirectory)
		}

		if workingDirHasMintDir {
			return filepath.Join(workingDirectory, ".mint"), nil
		}

		if workingDirectory == string(os.PathSeparator) {
			return "", nil
		}

		parentDir, _ := filepath.Split(workingDirectory)
		workingDirectory = filepath.Clean(parentDir)
	}
}

// getFileOrDirectoryYAMLEntries gets a MintDirectoryEntry for every given YAML file, or all YAML files in mintDir when no files are provided.
func getFileOrDirectoryYAMLEntries(files []string, mintDir string) ([]MintDirectoryEntry, error) {
	entries, err := getFileOrDirectoryEntries(files, mintDir)
	if err != nil {
		return nil, err
	}
	return filterYAMLFiles(entries), nil
}

// getFileOrDirectoryEntries gets a MintDirectoryEntry for every given file, or all files in mintDir when no files are provided.
func getFileOrDirectoryEntries(files []string, mintDir string) ([]MintDirectoryEntry, error) {
	if len(files) > 0 {
		return mintDirectoryEntriesFromPaths(files)
	} else if mintDir != "" {
		return mintDirectoryEntries(mintDir)
	}
	return make([]MintDirectoryEntry, 0), nil
}

// mintDirectoryEntries reads all files in the given dir and ensures the total size in within limits.
func mintDirectoryEntries(dir string) ([]MintDirectoryEntry, error) {
	entries := make([]MintDirectoryEntry, 0)
	var totalSize int

	err := filepath.Walk(dir, func(pathInDir string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error reading %q: %w", pathInDir, err)
		}

		entry, entrySize, err := mintDirectoryEntry(pathInDir, info, dir)
		if err != nil {
			return err
		}

		totalSize += entrySize
		entries = append(entries, entry)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve the entire contents of the .mint directory %q: %w", dir, err)
	}
	if totalSize > 5*1024*1024 {
		return nil, fmt.Errorf("the size of the .mint directory at %q exceeds 5MiB", dir)
	}

	return entries, nil
}

// mintDirectoryEntriesFromPaths reads given file paths and ensures the total size in within limits.
func mintDirectoryEntriesFromPaths(paths []string) ([]MintDirectoryEntry, error) {
	entries := make([]MintDirectoryEntry, 0)
	var totalSize int

	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			return nil, errors.Wrapf(err, "error while stating %q", path)
		}

		entry, entrySize, err := mintDirectoryEntry(path, info, "")
		if err != nil {
			return nil, err
		}

		totalSize += entrySize
		entries = append(entries, entry)
	}
	if totalSize > 5*1024*1024 {
		return nil, fmt.Errorf("the size of the these files exceed 5MiB: %s", strings.Join(paths, ", "))
	}

	return entries, nil
}

// mintDirectoryEntry finds the file at path and converts it to a MintDirectoryEntry.
func mintDirectoryEntry(path string, info os.FileInfo, makePathRelativeTo string) (MintDirectoryEntry, int, error) {
	mode := info.Mode()
	permissions := mode.Perm()

	var entryType string
	switch mode.Type() {
	case os.ModeDir:
		entryType = "dir"
	case os.ModeSymlink:
		entryType = "symlink"
	case os.ModeNamedPipe:
		entryType = "named-pipe"
	case os.ModeSocket:
		entryType = "socket"
	case os.ModeDevice:
		entryType = "device"
	case os.ModeCharDevice:
		entryType = "char-device"
	case os.ModeIrregular:
		entryType = "irregular"
	default:
		if mode.IsRegular() {
			entryType = "file"
		} else {
			entryType = "unknown"
		}
	}

	var fileContents string
	var contentLength int
	if entryType == "file" {
		contents, err := os.ReadFile(path)
		if err != nil {
			return MintDirectoryEntry{}, contentLength, fmt.Errorf("unable to read file %q: %w", path, err)
		}

		contentLength = len(contents)
		fileContents = string(contents)
	}

	relPath := path
	if makePathRelativeTo != "" {
		rel, err := filepath.Rel(makePathRelativeTo, path)
		if err != nil {
			return MintDirectoryEntry{}, contentLength, fmt.Errorf("unable to determine relative path of %q: %w", path, err)
		}
		relPath = filepath.ToSlash(filepath.Join(".mint", rel)) // Mint only supports unix-style path separators
	}

	return MintDirectoryEntry{
		Type:         entryType,
		OriginalPath: path,
		Path:         relPath,
		Permissions:  uint32(permissions),
		FileContents: fileContents,
	}, contentLength, nil
}

// taskDefinitionsFromPaths opens each file specified in `paths` and reads their content as a string.
// No validation takes place here.
func taskDefinitionsFromPaths(paths []string) ([]TaskDefinition, error) {
	taskDefinitions := make([]api.TaskDefinition, 0)

	for _, path := range paths {
		fd, err := os.Open(path)
		if err != nil {
			return nil, errors.Wrapf(err, "error while opening %q", path)
		}
		defer fd.Close()

		fileContent, err := io.ReadAll(fd)
		if err != nil {
			return nil, errors.Wrapf(err, "error while reading %q", path)
		}

		taskDefinitions = append(taskDefinitions, TaskDefinition{
			Path:         path,
			FileContents: string(fileContent),
		})
	}

	return taskDefinitions, nil
}

// filterYAMLFiles finds any *.yml and *.yaml files in the given entries.
// No further validation is made.
func filterYAMLFiles(entries []MintDirectoryEntry) []MintDirectoryEntry {
	yamlFiles := make([]MintDirectoryEntry, 0)

	for _, entry := range entries {
		if !isYAMLFile(entry) {
			continue
		}

		yamlFiles = append(yamlFiles, entry)
	}

	return yamlFiles
}

// filterYAMLFilesForModification finds any *.yml and *.yaml files in the given entries
// and reads and parses them. Entries that cannot be modified, such as JSON files
// masquerading as YAML, will not be included.
func filterYAMLFilesForModification(entries []MintDirectoryEntry, filter func(doc *YAMLDoc) bool) []*MintYAMLFile {
	yamlFiles := make([]*MintYAMLFile, 0)

	for _, entry := range entries {
		yamlFile := validateYAMLFileForModification(entry, filter)
		if yamlFile == nil {
			continue
		}

		yamlFiles = append(yamlFiles, yamlFile)
	}

	return yamlFiles
}

// validateYAMLFileForModification reads and parses the given file entry. If it cannot
// be modified, this method will return nil.
func validateYAMLFileForModification(entry MintDirectoryEntry, filter func(doc *YAMLDoc) bool) *MintYAMLFile {
	if !isYAMLFile(entry) {
		return nil
	}

	content, err := os.ReadFile(entry.OriginalPath)
	if err != nil {
		return nil
	}

	// JSON is valid YAML, but we don't support modifying it
	if isJSON(content) {
		return nil
	}

	doc, err := ParseYamlDoc(string(content))
	if err != nil {
		return nil
	}

	if !filter(doc) {
		return nil
	}

	return &MintYAMLFile{
		Entry: entry,
		Doc:   doc,
	}
}

func isJSON(content []byte) bool {
	var jsonContent any
	return len(content) > 0 && content[0] == '{' && json.Unmarshal(content, &jsonContent) == nil
}

func isYAMLFile(entry MintDirectoryEntry) bool {
	return entry.Type == "file" && (strings.HasSuffix(entry.OriginalPath, ".yml") || strings.HasSuffix(entry.OriginalPath, ".yaml"))
}
