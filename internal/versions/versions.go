package versions

import (
	"os"
	"strings"
	"sync"

	semver "github.com/Masterminds/semver/v3"
	"github.com/rwx-research/mint-cli/cmd/mint/config"
)

var versionHolder *lockedVersions
var emptyVersion = semver.MustParse("0.0.0")

type lockedVersions struct {
	currentVersion *semver.Version
	latestVersion  *semver.Version
	mu             sync.RWMutex
}

func init() {
	currentVersion, err := semver.NewVersion(config.Version)
	if err != nil {
		// Assume this is a development build and it is newer than any release.
		currentVersion = semver.MustParse("9999+" + config.Version)
	}

	versionHolder = &lockedVersions{
		currentVersion: currentVersion,
		latestVersion:  emptyVersion,
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

func HasCliLatestVersion() bool {
	return !GetCliLatestVersion().Equal(emptyVersion)
}

func InstalledWithHomebrew() bool {
	fname, err := os.Executable()
	if err != nil {
		return false
	}

	return strings.Contains(strings.ToLower(fname), "/homebrew/")
}
