package policy

import "testing"

func TestIsForbidden(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		// Forbidden by extension
		{"exe at root", "tool.exe", true},
		{"exe nested", "build/win/tool.exe", true},
		{"deb", "release-1.0.0.deb", true},
		{"shared lib", "libfoo.so", true},
		{"object file", "main.o", true},
		{"static archive", "libfoo.a", true},
		{"dylib", "libfoo.dylib", true},
		{"raw bin", "firmware.bin", true},
		{"out", "a.out", true},

		// Forbidden by prefix
		{"dist root", "dist/main", true},
		{"dist nested", "dist/linux/main", true},
		{"DIST uppercase", "DIST/main", true},

		// Forbidden by basename
		{"my_tool", "subdir/my_tool", true},
		{"cve-parser", "tools/cve-parser", true},

		// Allowed
		{"go source", "main.go", false},
		{"readme", "README.md", false},
		{"yaml workflow", ".github/workflows/release.yaml", false},
		{"shell script", "build.sh", false},
		{"icon", "assets/icon.png", false},
		{"empty", "", false},
		{"nested allowed", "code/internal/launcher/app.go", false},

		// Edge cases — extension match must be on the actual extension
		{"file with .exe in name but other ext", "myexe.txt", false},
		{"path with dist substring not prefix", "production/dist-config.yaml", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsForbidden(tc.path)
			if got != tc.want {
				t.Errorf("IsForbidden(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestForbiddenLists_NonEmpty(t *testing.T) {
	// Smoke test: if someone deletes a list, the regression here will be loud.
	if len(ForbiddenPrefixes) == 0 {
		t.Error("ForbiddenPrefixes is empty")
	}
	if len(ForbiddenExtensions) == 0 {
		t.Error("ForbiddenExtensions is empty")
	}
	if len(ForbiddenBaseNames) == 0 {
		t.Error("ForbiddenBaseNames is empty")
	}
}
