package launcher

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ErrNotPlantillaRepo is returned when the selected folder does not contain
// a recognisable plantilla-flow workflow.
var ErrNotPlantillaRepo = errors.New("not a plantilla-flow repository")

// DetectedProject holds every field that could be auto-detected from an
// existing repository. Fields that could not be read are left at their zero
// value and their names are appended to Warnings.
type DetectedProject struct {
	Project
	Warnings []string // human-readable list of fields that fell back to defaults
}

// DetectFromRepo inspects dir and returns a DetectedProject with as many
// fields populated as possible. Returns ErrNotPlantillaRepo when no
// recognisable workflow is found so the caller can show a targeted error.
func DetectFromRepo(dir string) (DetectedProject, error) {
	var det DetectedProject

	// ── Git sanity ────────────────────────────────────────────────────────
	if res := runGit(dir, "rev-parse", "--git-dir"); res.Err != nil {
		return det, fmt.Errorf("%w: the folder is not a git repository", ErrNotPlantillaRepo)
	}

	// ── Remote URL ────────────────────────────────────────────────────────
	remoteRes := runGit(dir, "remote", "get-url", "origin")
	remoteURL := strings.TrimSpace(remoteRes.Output)
	if remoteURL == "" {
		det.Warnings = append(det.Warnings, "no remote origin configured — you can add one later")
	}

	// ── Provider & workflow file ──────────────────────────────────────────
	provider, workflowContent, err := readProviderWorkflow(dir)
	if err != nil {
		return det, err // already wrapped with ErrNotPlantillaRepo
	}

	// ── Parse workflow env values (plan job) ──────────────────────────────
	lang := readWorkflowEnv(workflowContent, "PROJECT_LANG")
	if lang == "" {
		lang = "go"
		det.Warnings = append(det.Warnings, "PROJECT_LANG not found — defaulting to 'go'")
	}

	buildLinux := readWorkflowEnv(workflowContent, "BUILD_LINUX")
	if buildLinux == "" {
		buildLinux = "1"
	}
	buildWindows := readWorkflowEnv(workflowContent, "BUILD_WINDOWS")
	if buildWindows == "" {
		buildWindows = "1"
	}
	forceNative := readWorkflowEnv(workflowContent, "FORCE_NATIVE")
	if forceNative == "" {
		forceNative = "0"
	}

	// ── Runner labels ─────────────────────────────────────────────────────
	runnerLinux := readSectionRunsOn(workflowContent, "plan:")
	if runnerLinux == "" {
		runnerLinux = defaultRunnerLinux
		det.Warnings = append(det.Warnings, "Linux runner not detected — using default '"+defaultRunnerLinux+"'")
	}

	runnerWindows := readSectionRunsOn(workflowContent, "build_native_windows:")
	if runnerWindows == "" {
		runnerWindows = defaultRunnerWindows
	}

	// ── build.sh ─────────────────────────────────────────────────────────
	binaryName, releaseMode, releaseFiles := readBuildScript(dir, &det.Warnings)

	if binaryName == "" {
		binaryName = sanitizeBinaryName(filepath.Base(dir))
		det.Warnings = append(det.Warnings, "APP_NAME not found in build.sh — derived from folder name: '"+binaryName+"'")
	}

	// ── Platform ─────────────────────────────────────────────────────────
	platform := derivePlatform(lang, buildLinux, buildWindows, forceNative)

	// ── Assemble ─────────────────────────────────────────────────────────
	det.Project = Project{
		Name:          filepath.Base(dir),
		BinaryName:    binaryName,
		Path:          dir,
		RemoteURL:     remoteURL,
		Provider:      provider,
		Language:      lang,
		Platform:      platform,
		RunnerLinux:   runnerLinux,
		RunnerWindows: runnerWindows,
		ReleaseMode:   releaseMode,
		ReleaseFiles:  releaseFiles,
		CreatedAt:     time.Now(),
	}
	return det, nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// readProviderWorkflow detects the CI provider by checking which workflow
// directory exists and returns (provider, file content, error).
func readProviderWorkflow(dir string) (string, string, error) {
	candidates := []struct {
		provider string
		path     string
	}{
		{"github", filepath.Join(dir, ".github", "workflows", "release.yaml")},
		{"gitea", filepath.Join(dir, ".gitea", "workflows", "release.yaml")},
	}

	for _, c := range candidates {
		data, err := os.ReadFile(c.path)
		if err != nil {
			continue
		}
		content := string(data)
		// Verify it's our template and not just any workflow file.
		if !strings.Contains(content, "PROJECT_LANG") || !strings.Contains(content, "build_mode") {
			return "", "", fmt.Errorf(
				"%w: %s exists but does not look like a plantilla-flow workflow (missing PROJECT_LANG / build_mode fields)",
				ErrNotPlantillaRepo, c.path,
			)
		}
		return c.provider, content, nil
	}

	return "", "", fmt.Errorf(
		"%w: no release.yaml found in .github/workflows/ or .gitea/workflows/",
		ErrNotPlantillaRepo,
	)
}

// readWorkflowEnv extracts the plain value of an env variable from the plan
// job section. Handles both quoted ('1') and unquoted (go) values.
func readWorkflowEnv(content, key string) string {
	re := regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(key) + `:\s*['"]?([^'"\n\r]+?)['"]?\s*$`)
	if m := re.FindStringSubmatch(content); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// readSectionRunsOn extracts the runs-on value from the first occurrence of
// runs-on: after the given section header (e.g. "plan:" or "build_native_windows:").
// Returns the value as a comma-separated string, stripping YAML list brackets.
func readSectionRunsOn(content, section string) string {
	idx := strings.Index(content, section)
	if idx < 0 {
		return ""
	}
	sub := content[idx:]
	re := regexp.MustCompile(`(?m)^\s+runs-on:\s*(.+)$`)
	m := re.FindStringSubmatch(sub)
	if len(m) < 2 {
		return ""
	}
	raw := strings.TrimSpace(m[1])
	raw = strings.Trim(raw, "[]")
	return strings.TrimSpace(raw)
}

// readBuildScript reads APP_NAME, RELEASE_MODE and RELEASE_FILES defaults
// from build.sh. Appends to warnings when build.sh is absent.
func readBuildScript(dir string, warnings *[]string) (binaryName, releaseMode, releaseFiles string) {
	data, err := os.ReadFile(filepath.Join(dir, "build.sh"))
	if err != nil {
		*warnings = append(*warnings, "build.sh not found — binary name will be derived from folder name")
		return "", "archive", ""
	}
	content := string(data)
	binaryName = readShellDefault(content, "APP_NAME")
	releaseMode = readShellDefault(content, "RELEASE_MODE")
	if releaseMode == "" {
		releaseMode = "archive"
	}
	releaseFiles = readShellDefault(content, "RELEASE_FILES")
	return
}

// readShellDefault extracts the default value from KEY="${KEY:-value}" in a
// shell script. Returns an empty string when the pattern is not found.
func readShellDefault(content, key string) string {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `="\$\{` + regexp.QuoteMeta(key) + `:-([^}"]*)}"`)
	if m := re.FindStringSubmatch(content); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// derivePlatform reconstructs the platform string from workflow variable
// values, mirroring the logic in view_bootstrap.go / platformValue().
func derivePlatform(lang, buildLinux, buildWindows, forceNative string) string {
	wl := buildLinux == "1"
	ww := buildWindows == "1"

	switch {
	case lang == "script":
		return "only linux"
	case wl && !ww:
		return "only linux"
	case !wl && ww:
		return "only windows"
	case forceNative == "1" || !crossCompilingLanguages[lang]:
		return "both native runners"
	default:
		return "cross-compiling"
	}
}
