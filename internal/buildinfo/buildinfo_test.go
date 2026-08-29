package buildinfo

import (
	"runtime/debug"
	"testing"
)

func settings(pairs ...string) []debug.BuildSetting {
	out := make([]debug.BuildSetting, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, debug.BuildSetting{Key: pairs[i], Value: pairs[i+1]})
	}
	return out
}

func TestGetPrefersTheStamp(t *testing.T) {
	info := get("2026.08.29.120000", settings("vcs.revision", "0123456789abcdef0123", "vcs.modified", "false"))

	if info.Version != "2026.08.29.120000" {
		t.Errorf("Version = %q, want the linker stamp", info.Version)
	}
	if info.Revision != "0123456789ab" {
		t.Errorf("Revision = %q, want it shortened to %d characters", info.Revision, revisionLength)
	}
	if info.Modified {
		t.Error("Modified is true on a clean tree")
	}
}

// An unstamped binary is every build that is not a release — `go run .` most
// often. It must say so rather than claim a version it does not have.
func TestGetWithoutAStampSaysDev(t *testing.T) {
	if got := get("", nil).Version; got != "dev" {
		t.Errorf("Version = %q, want %q", got, "dev")
	}
}

// A container build carries the stamp and no VCS information, since the image
// does not copy .git. The revision is then absent, not wrong.
func TestGetWithoutVCS(t *testing.T) {
	info := get("2026.08.29.120000", nil)
	if info.Revision != "" {
		t.Errorf("Revision = %q, want empty", info.Revision)
	}
	if info.Version != "2026.08.29.120000" {
		t.Errorf("Version = %q, want the stamp", info.Version)
	}
}

// A build from a dirty tree is not the commit it names, and saying only the
// commit would be a lie about which code is running.
func TestGetReportsAModifiedTree(t *testing.T) {
	if !get("", settings("vcs.modified", "true")).Modified {
		t.Error("Modified is false although the tree was dirty")
	}
}

func TestShortenLeavesAShortRevisionAlone(t *testing.T) {
	if got := shorten("abc123"); got != "abc123" {
		t.Errorf("shorten(%q) = %q", "abc123", got)
	}
}

// The link-time path itself, which the unit tests above cannot reach: this one
// only asserts anything when the package is built with the stamp, which is what
// the Makefile's `version-stamp-check` target does.
func TestVersionVariableIsLinkable(t *testing.T) {
	if Version == "" {
		t.Skip("not a stamped build; run: go test -ldflags \"-X github.com/ldapwarden/ldapwarden/internal/buildinfo.Version=test\" ./internal/buildinfo/")
	}
	if Get().Version != Version {
		t.Errorf("Get().Version = %q, want the stamped %q", Get().Version, Version)
	}
}
