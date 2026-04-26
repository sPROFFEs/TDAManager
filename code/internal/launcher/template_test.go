package launcher

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceShellValue(t *testing.T) {
	cases := []struct {
		name    string
		content string
		key     string
		value   string
		want    string
	}{
		{
			name:    "simple literal default",
			content: `APP_NAME="${APP_NAME:-my_tool}"`,
			key:     "APP_NAME",
			value:   "my-app",
			want:    `APP_NAME="${APP_NAME:-my-app}"`,
		},
		{
			name:    "complex default with command substitution",
			content: `APP_NAME="${APP_NAME:-$(basename "$PWD" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9_-]/_/g')}"`,
			key:     "APP_NAME",
			value:   "my-app",
			want:    `APP_NAME="${APP_NAME:-my-app}"`,
		},
		{
			name:    "BUILD_LINUX boolean replacement",
			content: `BUILD_LINUX="${BUILD_LINUX:-1}"`,
			key:     "BUILD_LINUX",
			value:   "0",
			want:    `BUILD_LINUX="${BUILD_LINUX:-0}"`,
		},
		{
			name: "leaves unrelated lines untouched",
			content: "APP_VERSION=\"${APP_VERSION:-dev-local}\"\n" +
				"OTHER=value\n" +
				"BUILD_LINUX=\"${BUILD_LINUX:-1}\"\n",
			key:   "BUILD_LINUX",
			value: "0",
			want: "APP_VERSION=\"${APP_VERSION:-dev-local}\"\n" +
				"OTHER=value\n" +
				"BUILD_LINUX=\"${BUILD_LINUX:-0}\"\n",
		},
		{
			name:    "no match leaves content unchanged",
			content: `APP_NAME="my_tool"`, // no ${VAR:-...} pattern
			key:     "APP_NAME",
			value:   "x",
			want:    `APP_NAME="my_tool"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := replaceShellValue(tc.content, tc.key, tc.value)
			if got != tc.want {
				t.Errorf("replaceShellValue(\n%q, %q, %q) =\n%q\nwant\n%q", tc.content, tc.key, tc.value, got, tc.want)
			}
		})
	}
}

func TestReadLatestVersion(t *testing.T) {
	cases := []struct {
		name      string
		changelog string
		want      string
	}{
		{
			name: "single tagged release",
			changelog: "# Changelog\n\n" +
				"## [Unreleased]\n\n" +
				"## [1.0.0] - 2026-04-25\n\n" +
				"### Added\n- first\n",
			want: "1.0.0",
		},
		{
			name: "picks the topmost tagged release",
			changelog: "## [Unreleased]\n\n" +
				"## [2.3.4] - 2026-05-01\n\n" +
				"## [2.3.3] - 2026-04-20\n\n" +
				"## [2.3.2] - 2026-04-10\n\n",
			want: "2.3.4",
		},
		{
			name: "no version tag yet",
			changelog: "# Changelog\n\n" +
				"## [Unreleased]\n\n" +
				"### Added\n- nothing\n",
			want: "none",
		},
		{
			name:      "empty changelog",
			changelog: "",
			want:      "none",
		},
		{
			name: "ignores non-semver bracketed entries",
			changelog: "## [Unreleased]\n\n" +
				"## [draft] - 2026-04-01\n\n" +
				"## [1.0.0] - 2026-03-01\n\n",
			want: "1.0.0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.changelog != "" {
				if err := os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte(tc.changelog), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got := readLatestVersion(dir)
			if got != tc.want {
				t.Errorf("readLatestVersion = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("missing file returns none", func(t *testing.T) {
		dir := t.TempDir()
		if got := readLatestVersion(dir); got != "none" {
			t.Errorf("readLatestVersion(missing) = %q, want %q", got, "none")
		}
	})
}
