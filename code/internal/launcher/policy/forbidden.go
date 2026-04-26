// Package policy holds the project-wide build/release policies.
//
// The forbidden-artifact list is the single source of truth for what must
// never reach a remote release. Both the Go publish flow (git.go) and the
// manual publish.sh in the template repo enforce this — keep the two in
// sync until publish.sh learns to read the JSON file directly.
package policy

import (
	"path/filepath"
	"strings"
)

// ForbiddenPrefixes lists path prefixes that are never publishable.
// Any tracked or untracked file whose path begins with one of these is
// treated as a build artefact.
var ForbiddenPrefixes = []string{
	"dist/",
}

// ForbiddenExtensions lists file extensions (with leading dot) that must
// never be committed. Compiled binaries, packaged installers and shared
// libraries belong here.
var ForbiddenExtensions = []string{
	".exe",
	".deb",
	".o",
	".so",
	".bin",
	".out",
	".a",
	".dylib",
}

// ForbiddenBaseNames lists exact filenames that are placeholders or known
// extensionless build outputs. Add new placeholders here so they are blocked
// in both the Go and bash code paths.
var ForbiddenBaseNames = []string{
	"a.out",
	"my_tool",
	"cve-parser",
}

// IsForbidden reports whether path matches any forbidden rule.
// Comparison is case-insensitive on the path components, matching the
// behaviour of the previous inline check in git.go.
func IsForbidden(path string) bool {
	lower := strings.ToLower(path)

	for _, prefix := range ForbiddenPrefixes {
		if strings.HasPrefix(lower, strings.ToLower(prefix)) {
			return true
		}
	}

	ext := filepath.Ext(lower)
	for _, e := range ForbiddenExtensions {
		if ext == e {
			return true
		}
	}

	base := filepath.Base(lower)
	for _, name := range ForbiddenBaseNames {
		if base == strings.ToLower(name) {
			return true
		}
	}
	return false
}
