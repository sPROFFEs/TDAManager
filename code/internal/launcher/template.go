package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var crossCompilingLanguages = map[string]bool{
	"go": true, "rust": true, "node": true, "dotnet": true,
	"java": true, "ruby": true, "php": true,
}

func configureTemplate(projectDir, provider, lang, platform, runnerLinux, runnerWindows, binaryName string) error {
	if err := keepProviderWorkflow(projectDir, provider); err != nil {
		return err
	}
	if err := copyBuildTemplate(projectDir, lang, binaryName); err != nil {
		return err
	}
	if err := configureWorkflow(projectDir, provider, lang, platform, runnerLinux, runnerWindows); err != nil {
		return err
	}
	if err := configureBuildToggles(filepath.Join(projectDir, "build.sh"), platform); err != nil {
		return err
	}
	return os.MkdirAll(filepath.Join(projectDir, "code"), 0o755)
}

// cleanupTemplate removes files that are only relevant when using the template
// manually (publish.sh, build-templates/, BUILD_CONTRACT.md) and writes a
// ready-to-edit README so the user has a clean starting point.
// Must be called after configureTemplate — build-templates/ is still needed
// by copyBuildTemplate up until that point.
func cleanupTemplate(projectDir, projectName, binaryName string) error {
	for _, rel := range []string{
		"publish.sh",
		"BUILD_CONTRACT.md",
	} {
		_ = os.Remove(filepath.Join(projectDir, rel))
	}
	if err := removeAll(filepath.Join(projectDir, "build-templates")); err != nil {
		return fmt.Errorf("removing build-templates: %w", err)
	}
	return writeStarterReadme(projectDir, projectName, binaryName)
}

func writeStarterReadme(projectDir, projectName, binaryName string) error {
	content := "# " + projectName + "\n\n" +
		"> _Short description of what this tool does._\n\n" +
		"## Installation\n\n" +
		"Download the latest binary from the [Releases](../../releases) page " +
		"and place it somewhere on your `PATH`.\n\n" +
		"## Usage\n\n" +
		"```\n" + binaryName + " [options]\n```\n\n" +
		"## Development\n\n" +
		"Source code lives in the `code/` directory.\n\n" +
		"Update `CHANGELOG.md` before publishing a new release.\n"
	return os.WriteFile(filepath.Join(projectDir, "README.md"), []byte(content), 0o644)
}

func keepProviderWorkflow(projectDir, provider string) error {
	switch provider {
	case "github":
		return os.RemoveAll(filepath.Join(projectDir, ".gitea"))
	case "gitea":
		return os.RemoveAll(filepath.Join(projectDir, ".github"))
	default:
		return nil
	}
}

func copyBuildTemplate(projectDir, lang, binaryName string) error {
	group := "no-crosscompiling"
	if crossCompilingLanguages[lang] {
		group = "supports-crosscompiling"
	}
	source := filepath.Join(projectDir, "build-templates", group, "build."+lang+".sh")
	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("build template not found for %s: %w", lang, err)
	}

	content := string(data)
	// All build templates output to "dist". The user's code lives inside ./code,
	// so the build runs from there and writes to ../dist (= repo root /dist).
	content = strings.ReplaceAll(content, `OUTPUT_DIR="dist"`, `OUTPUT_DIR="../dist"`)
	if binaryName != "" {
		content = replaceShellValue(content, "APP_NAME", binaryName)
	}
	content = injectCodeDirGuard(content)

	target := filepath.Join(projectDir, "build.sh")
	return os.WriteFile(target, []byte(content), 0o755)
}

// UpdateBinaryName rewrites APP_NAME in the project's build.sh. Call this when
// the user edits the binary name from the dashboard.
func UpdateBinaryName(projectDir, binaryName string) error {
	path := filepath.Join(projectDir, "build.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("build.sh not found: %w", err)
	}
	updated := replaceShellValue(string(data), "APP_NAME", binaryName)
	return os.WriteFile(path, []byte(updated), 0o755)
}

// injectCodeDirGuard wraps the template build.sh so it cd's into the code/
// folder before running the build, ensuring the user's source stays isolated
// from workflow/tooling files.
func injectCodeDirGuard(content string) string {
	marker := `OUTPUT_DIR="../dist"`
	guard := `CODE_DIR="code"

if [ ! -d "$CODE_DIR" ]; then
  echo "ERROR: expected project source code in ./$CODE_DIR"
  exit 1
fi

mkdir -p "dist"
cd "$CODE_DIR"`
	if strings.Contains(content, `cd "$CODE_DIR"`) {
		return content
	}
	return strings.Replace(content, marker, marker+"\n"+guard, 1)
}

func configureWorkflow(projectDir, provider, lang, platform, runnerLinux, runnerWindows string) error {
	workflow := workflowAbsPath(projectDir, provider)

	data, err := os.ReadFile(workflow)
	if err != nil {
		return err
	}
	content := string(data)
	// Scope replacements to the plan job so we don't overwrite the hardcoded
	// per-platform values in build_native_linux / build_native_windows.
	content = replaceEnvInJob(content, "plan", "PROJECT_LANG", lang)
	content = replaceEnvInJob(content, "plan", "BUILD_LINUX", boolFlag(platform != "windows"))
	content = replaceEnvInJob(content, "plan", "BUILD_WINDOWS", boolFlag(platform != "linux"))
	// FORCE_NATIVE=1 only when the user explicitly asked for native dual-runner
	// on a language that would otherwise cross-compile (e.g. Go+Fyne).
	content = replaceEnvInJob(content, "plan", "FORCE_NATIVE", boolFlag(platform == "both-native"))
	content = replaceRunners(content, runnerLinux, runnerWindows)
	return os.WriteFile(workflow, []byte(content), 0o644)
}

// UpdateRunners rewrites the workflow's runs-on values from the dashboard.
// Runs are split: every job uses runnerLinux except build_native_windows,
// which uses runnerWindows.
func UpdateRunners(projectDir, provider, runnerLinux, runnerWindows string) error {
	workflow := workflowAbsPath(projectDir, provider)
	data, err := os.ReadFile(workflow)
	if err != nil {
		return err
	}
	content := string(data)
	content = replaceRunners(content, runnerLinux, runnerWindows)
	return os.WriteFile(workflow, []byte(content), 0o644)
}

func workflowAbsPath(projectDir, provider string) string {
	if provider == "gitea" {
		return filepath.Join(projectDir, ".gitea", "workflows", "release.yaml")
	}
	return filepath.Join(projectDir, ".github", "workflows", "release.yaml")
}

// replaceRunners rewrites every `runs-on:` line. Lines inside the
// build_native_windows job get runnerWindows; everything else gets runnerLinux.
// If a runner value is empty, the corresponding lines are left untouched.
func replaceRunners(content, runnerLinux, runnerWindows string) string {
	lines := strings.Split(content, "\n")
	inWindowsJob := false
	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if isJobHeader(trimmed) {
			inWindowsJob = strings.HasPrefix(trimmed, "  build_native_windows:")
			continue
		}
		if !strings.Contains(line, "runs-on:") {
			continue
		}
		runner := runnerLinux
		if inWindowsJob {
			runner = runnerWindows
		}
		if strings.TrimSpace(runner) == "" {
			continue
		}
		idx := strings.Index(line, "runs-on:")
		prefix := line[:idx+len("runs-on:")]

		runnerVal := strings.TrimSpace(runner)
		if strings.Contains(runnerVal, ",") && !strings.HasPrefix(runnerVal, "[") {
			runnerVal = "[" + runnerVal + "]"
		}

		suffix := ""
		if strings.HasSuffix(line, "\r") {
			suffix = "\r"
		}
		lines[i] = prefix + " " + runnerVal + suffix
	}
	return strings.Join(lines, "\n")
}

// isJobHeader detects a line like `  jobname:` (exactly 2 leading spaces,
// then identifier, then colon, optional trailing space).
func isJobHeader(line string) bool {
	re := regexp.MustCompile(`^  [a-zA-Z_][a-zA-Z0-9_-]*:\s*$`)
	return re.MatchString(line)
}

// replaceEnvInJob locates the env: block of a specific job and replaces the
// value of `key` inside it. Lines outside that env block are untouched.
func replaceEnvInJob(content, jobName, key, value string) string {
	lines := strings.Split(content, "\n")
	inTargetJob := false
	inEnv := false
	jobHeader := "  " + jobName + ":"
	envIndent := "    env:"

	for i, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		suffix := ""
		if strings.HasSuffix(raw, "\r") {
			suffix = "\r"
		}
		if isJobHeader(line) {
			inTargetJob = strings.HasPrefix(line, jobHeader)
			inEnv = false
			continue
		}
		if !inTargetJob {
			continue
		}
		if strings.HasPrefix(line, envIndent) {
			inEnv = true
			continue
		}
		if inEnv && strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") && strings.TrimSpace(line) != "" {
			inEnv = false
		}
		if !inEnv {
			continue
		}
		keyPrefix := "      " + key + ":"
		if !strings.HasPrefix(line, keyPrefix) {
			continue
		}
		lines[i] = "      " + key + ": '" + value + "'" + suffix
	}
	return strings.Join(lines, "\n")
}

func configureBuildToggles(path, platform string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	content = replaceShellValue(content, "BUILD_LINUX", boolFlag(platform != "windows"))
	content = replaceShellValue(content, "BUILD_WINDOWS", boolFlag(platform != "linux"))
	return os.WriteFile(path, []byte(content), 0o755)
}

func injectChangelog(projectDir, version, added, changed, fixed string) error {
	path := filepath.Join(projectDir, "CHANGELOG.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	date := time.Now().Format("2006-01-02")
	section := "\n## [" + strings.TrimPrefix(version, "v") + "] - " + date + "\n\n"
	if strings.TrimSpace(added) != "" {
		section += "### Added\n" + bulletLines(added) + "\n"
	}
	if strings.TrimSpace(changed) != "" {
		section += "### Changed\n" + bulletLines(changed) + "\n"
	}
	if strings.TrimSpace(fixed) != "" {
		section += "### Fixed\n" + bulletLines(fixed) + "\n"
	}

	content := string(data)
	if strings.Contains(content, "## [Unreleased]") {
		content = strings.Replace(content, "## [Unreleased]", "## [Unreleased]"+section, 1)
	} else {
		content += section
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// readLatestVersion scans CHANGELOG.md for the most recent released version tag
// (lines matching "## [X.Y.Z]"). Returns "none" when the file is absent or has
// no tagged releases yet.
func readLatestVersion(projectDir string) string {
	data, err := os.ReadFile(filepath.Join(projectDir, "CHANGELOG.md"))
	if err != nil {
		return "none"
	}
	re := regexp.MustCompile(`(?m)^##\s+\[([0-9]+\.[0-9]+\.[0-9]+)\]`)
	matches := re.FindStringSubmatch(string(data))
	if len(matches) < 2 {
		return "none"
	}
	return matches[1]
}

func bulletLines(value string) string {
	var out []string
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, "- "+line)
		}
	}
	return strings.Join(out, "\n") + "\n"
}

func replaceShellValue(content, key, value string) string {
	re := regexp.MustCompile(`(?m)^(` + regexp.QuoteMeta(key) + `="\$\{` + regexp.QuoteMeta(key) + `:-)[^}]+(\}")`)
	return re.ReplaceAllString(content, "${1}"+value+"${2}")
}

func boolFlag(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
