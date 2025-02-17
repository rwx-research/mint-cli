package versions

import (
	"os"
	"strings"
	"sync"

	semver "github.com/Masterminds/semver/v3"
	"github.com/rwx-research/mint-cli/cmd/mint/config"
)

var versionHolder *lockedVersions

type lockedVersions struct {
	currentVersion *semver.Version
	latestVersion  *semver.Version
	mu             sync.RWMutex
}

func init() {
	currentVersionStr := config.Version
	if currentVersionStr == "" {
		currentVersionStr = "0.0.0"
	}
	versionHolder = &lockedVersions{
		currentVersion: semver.MustParse(currentVersionStr),
		latestVersion:  semver.MustParse("0.0.0"),
	}
}

func GetCliCurrentVersion() *semver.Version {
	return versionHolder.currentVersion
}

func GetCliLatestVersion() *semver.Version {
	versionHolder.mu.RLock()
	defer versionHolder.mu.RUnlock()

	return versionHolder.latestVersion
}

func SetCliLatestVersion(versionStr string) error {
	version, err := semver.NewVersion(versionStr)
	if err != nil {
		return err
	}

	versionHolder.mu.Lock()
	versionHolder.latestVersion = version
	versionHolder.mu.Unlock()

	return nil
}

func NewVersionAvailable() bool {
	currentVersion := GetCliCurrentVersion()
	latestVersion := GetCliLatestVersion()

	return latestVersion.GreaterThan(currentVersion)
}

func InstalledWithHomebrew() bool {
	fname, err := os.Executable()
	if err != nil {
		return false
	}

	return strings.Contains(strings.ToLower(fname), "/homebrew/")
}
