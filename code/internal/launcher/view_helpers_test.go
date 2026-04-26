package launcher

import (
	"path/filepath"
	"testing"
)

func TestSanitizeBinaryName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"My-Tool", "my-tool"},
		{"My Tool", "my_tool"},
		{"CVE-Parser", "cve-parser"},
		{"foo/bar", "foo_bar"},
		{"foo.bar.exe", "foo_bar_exe"},
		{"  spaced  ", "spaced"},
		{"___leading_and_trailing___", "leading_and_trailing"},
		{"123abc", "123abc"},
		{"", ""},
		{"!@#$", ""},
		{"hello-world_42", "hello-world_42"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := sanitizeBinaryName(tc.in)
			if got != tc.want {
				t.Errorf("sanitizeBinaryName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseInt(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"0", 0},
		{"7", 7},
		{"123", 123},
		{"99", 99},
		{"", 0},     // empty string: loop doesn't run, returns 0
		{"-1", -1},  // minus sign is not a digit -> -1 sentinel
		{"abc", -1}, // non-digit
		{"1a2", -1},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := parseInt(tc.in)
			if got != tc.want {
				t.Errorf("parseInt(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestFirstNonEmpty(t *testing.T) {
	cases := []struct {
		name   string
		values []string
		want   string
	}{
		{"all empty falls back to release", []string{"", "  ", "\n"}, "release"},
		{"first wins", []string{"first", "second"}, "first"},
		{"skips blanks", []string{"", "  ", "third"}, "third"},
		{"trims whitespace and takes first line",
			[]string{"  hello\nworld\n"}, "hello"},
		{"empty input falls back", nil, "release"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := firstNonEmpty(tc.values...)
			if got != tc.want {
				t.Errorf("firstNonEmpty(%q) = %q, want %q", tc.values, got, tc.want)
			}
		})
	}
}

func TestPlatformValue(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"only linux", "linux"},
		{"only windows", "windows"},
		{"both native runners", "both-native"},
		{"cross-compiling", "both"},
		{"anything else", "both"}, // default branch
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := platformValue(tc.in)
			if got != tc.want {
				t.Errorf("platformValue(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestWorkflowFilePath(t *testing.T) {
	cases := []struct {
		provider string
		want     string
	}{
		{"github", filepath.Join(".github", "workflows", "release.yaml")},
		{"gitea", filepath.Join(".gitea", "workflows", "release.yaml")},
		{"unknown", filepath.Join(".github", "workflows", "release.yaml")}, // defaults to github
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			got := workflowFilePath(tc.provider)
			if got != tc.want {
				t.Errorf("workflowFilePath(%q) = %q, want %q", tc.provider, got, tc.want)
			}
		})
	}
}
