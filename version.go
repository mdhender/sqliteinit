package sqliteinit

import (
	"github.com/maloquacious/semver"
)

var (
	version = semver.Version{
		Major: 0,
		Minor: 10,
		Patch: 1,
		Build: semver.Commit(),
	}
)

func Version() semver.Version {
	return version
}
