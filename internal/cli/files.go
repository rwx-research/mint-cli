package cli

import (
	"encoding/json"
	"fmt"
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
// from the working directory to find a .mint directory. If the found path is not
// a directory or is not readable, an error is returned.
func findAndValidateMintDirectoryPath(configuredDirectory string) (string, error) {
	foundPath, err := findMintDirectoryPath(configuredDirectory)
	if err != nil {
		return "", err
	}

	if foundPath != "" {
		mintDirInfo, err := os.Stat(foundPath)
		if err != nil {
			return foundPath, fmt.Errorf("unable to read the .mint directory at %q", foundPath)
		}

		if !mintDirInfo.IsDir() {
			return foundPath, fmt.Errorf(".mint directory at %q is not a directory", foundPath)
		}
	}

	return foundPath, nil
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
	if len(files) != 0 {
		return mintDirectoryEntriesFromPaths(files)
	} else if mintDir != "" {
		return mintDirectoryEntries(mintDir)
	}
	return make([]MintDirectoryEntry, 0), nil
}

// mintDirectoryEntriesFromPaths loads all the files in paths relative to the current working directory.
func mintDirectoryEntriesFromPaths(paths []string) ([]MintDirectoryEntry, error) {
	return readMintDirectoryEntries(paths, "")
}

// mintDirectoryEntries loads all the files in the given dotMintPath relative to the parent of dotMintPath.
func mintDirectoryEntries(dotMintPath string) ([]MintDirectoryEntry, error) {
	return readMintDirectoryEntries([]string{dotMintPath}, dotMintPath)
}

func readMintDirectoryEntries(paths []string, relativeTo string) ([]MintDirectoryEntry, error) {
	entries := make([]MintDirectoryEntry, 0)
	var totalSize int

	for _, path := range paths {
		err := filepath.WalkDir(path, func(subpath string, de os.DirEntry, err error) error {
			entry, entrySize, suberr := mintDirectoryEntry(subpath, de, relativeTo)
			if suberr != nil {
				return suberr
			}

			totalSize += entrySize
			entries = append(entries, entry)
			return nil
		})
		if err != nil {
			return nil, errors.Wrapf(err, "reading mint directory entries at %s", path)
		}
	}

	if totalSize > 5*1024*1024 {
		return nil, fmt.Errorf("the size of the these files exceed 5MiB: %s", strings.Join(paths, ", "))
	}

	return entries, nil
}

// mintDirectoryEntry finds the file at path and converts it to a MintDirectoryEntry.
func mintDirectoryEntry(path string, de os.DirEntry, makePathRelativeTo string) (MintDirectoryEntry, int, error) {
	if de == nil {
		return MintDirectoryEntry{}, 0, os.ErrNotExist
	}

	info, err := de.Info()
	if err != nil {
		return MintDirectoryEntry{}, 0, err
	}

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

// filterYAMLFiles finds any *.yml and *.yaml files in the given entries.
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

// filterFiles finds only files in the given entries.
func filterFiles(entries []MintDirectoryEntry) []MintDirectoryEntry {
	files := make([]MintDirectoryEntry, 0)

	for _, entry := range entries {
		if !entry.IsFile() {
			continue
		}

		files = append(files, entry)
	}

	return files
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

	doc, err := ParseYAMLDoc(string(content))
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
	return entry.IsFile() && (strings.HasSuffix(entry.OriginalPath, ".yml") || strings.HasSuffix(entry.OriginalPath, ".yaml"))
}

func resolveWd() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Return a consistent path, which can be an issue on macOS where
	// /var is symlinked to /private/var.
	return filepath.EvalSymlinks(wd)
}

func relativePathFromWd(path string) string {
	wd, err := resolveWd()
	if err != nil {
		return path
	}

	if rel, err := filepath.Rel(wd, path); err == nil {
		return rel
	}

	return path
}
