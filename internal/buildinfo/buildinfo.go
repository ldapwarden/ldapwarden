// Package buildinfo reports which build of the server is running.
//
// It answers the first question asked of any bug report — "which version?" —
// from the binary itself rather than from anything the client could have cached,
// so a stale browser bundle cannot misreport what it is talking to.
package buildinfo

import (
	"runtime/debug"
	"strings"
)

// Version is stamped at link time by the release build:
//
//	-ldflags "-X github.com/ldapwarden/ldapwarden/internal/buildinfo.Version=2026.08.29.120000"
//
// It is empty in every other build, which is why Get falls back to the VCS
// stamp Go records automatically.
var Version string

// revisionLength keeps the commit short enough to quote in a bug report and
// long enough to be unambiguous in a repository of this size.
const revisionLength = 12

// Info is what the server knows about its own build.
type Info struct {
	// Version is the release tag, or "dev" when the binary was not stamped.
	Version string `json:"version"`
	// Revision is the git commit, short. Empty when the build had no VCS
	// information — a container build, for one, since the image does not copy
	// .git — in which case Version is all there is.
	Revision string `json:"revision,omitempty"`
	// Modified reports that the working tree carried uncommitted changes. A
	// version alone would be a lie about such a build, so it is said out loud
	// rather than folded into the revision string.
	Modified bool `json:"modified,omitempty"`
}

func Get() Info {
	// go build records the VCS settings for any build made inside a repository.
	// The release binaries are built from a checkout, so they carry both stamp
	// and revision; the container build has no .git and carries only the stamp.
	var settings []debug.BuildSetting
	if build, ok := debug.ReadBuildInfo(); ok {
		settings = build.Settings
	}
	return get(Version, settings)
}

// get is Get with its two inputs handed in, because neither can be arranged
// from a test: Version is fixed at link time and the build settings come from
// the binary that is running the test.
func get(version string, settings []debug.BuildSetting) Info {
	info := Info{Version: version}
	if info.Version == "" {
		info.Version = "dev"
	}
	for _, setting := range settings {
		switch setting.Key {
		case "vcs.revision":
			info.Revision = shorten(setting.Value)
		case "vcs.modified":
			info.Modified = setting.Value == "true"
		}
	}
	return info
}

func shorten(revision string) string {
	revision = strings.TrimSpace(revision)
	if len(revision) > revisionLength {
		return revision[:revisionLength]
	}
	return revision
}
