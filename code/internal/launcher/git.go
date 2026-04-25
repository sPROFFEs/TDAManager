package launcher

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	ErrAuthRequired    = errors.New("authentication required")
	ErrProviderUnknown = errors.New("provider could not be auto-detected")
)

type commandResult struct {
	Output string
	Err    error
}

func runGit(dir string, args ...string) commandResult {
	cmd := exec.Command("git", args...)
	cmd.SysProcAttr = sysProcAttr()
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return commandResult{Output: strings.TrimSpace(buf.String()), Err: err}
}

func cloneTemplate(templateURL, targetDir string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return errors.New("git is not available in PATH")
	}
	if err := os.MkdirAll(filepath.Dir(targetDir), 0o755); err != nil {
		return err
	}
	if entries, err := os.ReadDir(targetDir); err == nil && len(entries) > 0 {
		return fmt.Errorf("target folder is not empty: %s", targetDir)
	}
	res := runGit("", "clone", "--depth", "1", templateURL, targetDir)
	if res.Err != nil {
		return fmt.Errorf("template clone failed: %s", res.Output)
	}
	return removeAll(filepath.Join(targetDir, ".git"))
}

// removeAll deletes a directory tree, resetting permissions on the way down so
// it works on Windows where `git clone` leaves read-only files inside .git.
func removeAll(path string) error {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			_ = os.Chmod(p, 0o755)
		} else {
			_ = os.Chmod(p, 0o644)
		}
		return nil
	})
	return os.RemoveAll(path)
}

func initRepository(projectDir, remoteURL string) error {
	if res := runGit(projectDir, "init"); res.Err != nil {
		return fmt.Errorf("git init failed: %s", res.Output)
	}
	_ = runGit(projectDir, "remote", "remove", "origin")
	if res := runGit(projectDir, "remote", "add", "origin", remoteURL); res.Err != nil {
		return fmt.Errorf("git remote add failed: %s", res.Output)
	}
	return nil
}

func testRemote(projectDir string) error {
	res := runGit(projectDir, "ls-remote", "origin")
	if res.Err != nil {
		return classifyGitError(res.Output)
	}
	return nil
}

func commitReadme(projectDir string) error {
	if res := runGit(projectDir, "add", "README.md"); res.Err != nil {
		return fmt.Errorf("git add README.md failed: %s", res.Output)
	}
	res := runGit(projectDir, "commit", "-m", "Initial template connectivity test")
	if res.Err != nil && !strings.Contains(res.Output, "nothing to commit") {
		return fmt.Errorf("test commit failed: %s", res.Output)
	}
	if head := runGit(projectDir, "rev-parse", "HEAD"); head.Err != nil {
		return fmt.Errorf("no commit was produced; check that README.md exists in the template")
	}
	if res := runGit(projectDir, "branch", "-M", "main"); res.Err != nil {
		return fmt.Errorf("branch setup failed: %s", res.Output)
	}
	if res := runGit(projectDir, "push", "-u", "origin", "main"); res.Err != nil {
		return classifyGitError(res.Output)
	}
	return nil
}

func gitStatus(projectDir string) string {
	res := runGit(projectDir, "status", "--short", "--branch")
	if res.Err != nil {
		return "Git status unavailable:\n" + res.Output
	}
	if res.Output == "" {
		return "Working tree clean."
	}
	return res.Output
}

func remoteStatus(projectDir string) string {
	res := runGit(projectDir, "ls-remote", "origin")
	if res.Err != nil {
		return "Remote unavailable:\n" + res.Output
	}
	return "Remote reachable."
}

// publish stages everything except files already covered by .gitignore, then
// commits, pushes, tags and pushes the tag. It refuses to commit if it detects
// build artefacts that should never reach the remote.
func publish(projectDir, version, message string) error {
	if err := assertNoForbiddenArtifacts(projectDir); err != nil {
		return err
	}
	if res := runGit(projectDir, "add", "."); res.Err != nil {
		return fmt.Errorf("git add failed: %s", res.Output)
	}
	if res := runGit(projectDir, "commit", "-m", "Release "+version+": "+message); res.Err != nil {
		if !strings.Contains(res.Output, "nothing to commit") {
			return fmt.Errorf("release commit failed: %s", res.Output)
		}
	}
	if res := runGit(projectDir, "push", "origin", "main"); res.Err != nil {
		return classifyGitError(res.Output)
	}
	if res := runGit(projectDir, "tag", "-a", version, "-m", message); res.Err != nil {
		return fmt.Errorf("tag creation failed: %s", res.Output)
	}
	if res := runGit(projectDir, "push", "origin", version); res.Err != nil {
		return classifyGitError(res.Output)
	}
	return nil
}

// assertNoForbiddenArtifacts mirrors the regex enforced by publish.sh in the
// template so the launcher cannot accidentally commit binaries.
func assertNoForbiddenArtifacts(projectDir string) error {
	res := runGit(projectDir, "ls-files", "--others", "--exclude-standard")
	if res.Err != nil {
		return nil
	}
	for _, line := range strings.Split(res.Output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isForbiddenArtifact(line) {
			return fmt.Errorf("refusing to publish: untracked artefact found that the policy forbids: %s", line)
		}
	}
	return nil
}

func isForbiddenArtifact(path string) bool {
	lower := strings.ToLower(path)
	if strings.HasPrefix(lower, "dist/") {
		return true
	}
	switch filepath.Ext(lower) {
	case ".exe", ".deb", ".o", ".so", ".bin", ".out":
		return true
	}
	return false
}

func classifyGitError(output string) error {
	lower := strings.ToLower(output)
	switch {
	case strings.Contains(lower, "could not resolve host"):
		return fmt.Errorf("remote repository is not reachable. Make sure your internet connection is active and the URL is correct.\n\n%s", output)
	case strings.Contains(lower, "repository not found"),
		strings.Contains(lower, "not found"),
		strings.Contains(lower, "authentication failed"),
		strings.Contains(lower, "permission denied"),
		strings.Contains(lower, "not permitted"),
		strings.Contains(lower, "403"),
		strings.Contains(lower, "could not read username"),
		strings.Contains(lower, "could not read from remote"):
		return ErrAuthRequired
	default:
		return fmt.Errorf("git operation failed:\n\n%s", output)
	}
}

// detectProvider performs a best-effort guess based on the hostname. For
// self-hosted instances or unrecognised hostnames it returns "unknown" and the
// caller is expected to ask the user.
func detectProvider(remote string) string {
	u, err := url.Parse(remote)
	if err != nil {
		return "unknown"
	}
	host := strings.ToLower(u.Host)
	switch {
	case strings.Contains(host, "github.com"):
		return "github"
	case strings.Contains(host, "gitea"):
		return "gitea"
	}
	if probed, ok := probeGitea(u); ok {
		return probed
	}
	return "unknown"
}

func probeGitea(remote *url.URL) (string, bool) {
	if remote == nil || remote.Host == "" {
		return "", false
	}
	probe := url.URL{Scheme: remote.Scheme, Host: remote.Host, Path: "/api/v1/version"}
	if probe.Scheme == "" {
		probe.Scheme = "https"
	}
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get(probe.String())
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	buf := make([]byte, 256)
	n, _ := resp.Body.Read(buf)
	body := strings.ToLower(string(buf[:n]))
	if strings.Contains(body, `"version"`) {
		return "gitea", true
	}
	return "", false
}

func openBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	case "darwin":
		cmd = exec.Command("open", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	cmd.SysProcAttr = sysProcAttr()
	return cmd.Start()
}

// TokenCreationURL returns a deep link to the token creation/management page
// for the relevant provider. For GitHub it pre-fills the description and the
// `repo` scope. For Gitea it points to the user applications screen.
func TokenCreationURL(provider, remoteURL string) string {
	switch provider {
	case "github":
		return "https://github.com/settings/tokens/new?description=Flow%20Launcher&scopes=repo,workflow"
	case "gitea":
		u, err := url.Parse(remoteURL)
		if err != nil || u.Host == "" {
			return ""
		}
		return u.Scheme + "://" + u.Host + "/user/settings/applications"
	}
	return ""
}

// SetRemoteAuth wires up a per-repo credential store so future `git fetch`,
// `git pull` and `git push` calls authenticate transparently.
//
// Implementation detail: we configure `credential.helper "store --file=..."`
// scoped to the repository (`git config --local`) and write the credentials
// to that file with mode 0600. The remote URL itself stays clean — no
// `oauth2:token@host` leaks in `.git/config`.
//
// The helper is keyed by host, so reusing the same token for a different repo
// on the same host works without re-entering it.
func SetRemoteAuth(projectDir, remoteURL, username, token string) error {
	u, err := url.Parse(remoteURL)
	if err != nil {
		return err
	}
	if u.Host == "" {
		return fmt.Errorf("remote URL has no host: %s", remoteURL)
	}
	if strings.TrimSpace(username) == "" {
		// "oauth2" works for GitHub PATs and Gitea >= 1.13.
		username = "oauth2"
	}

	credPath := filepath.Join(projectDir, ".git", "credentials")
	if err := os.MkdirAll(filepath.Dir(credPath), 0o755); err != nil {
		return err
	}
	scheme := u.Scheme
	if scheme == "" {
		scheme = "https"
	}
	line := scheme + "://" + url.QueryEscape(username) + ":" + url.QueryEscape(token) + "@" + u.Host + "\n"
	if err := os.WriteFile(credPath, []byte(line), 0o600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}

	// Use a path syntax that git's credential-store accepts on both Linux and
	// Windows (forward slashes).
	helperValue := "store --file=" + filepath.ToSlash(credPath)
	if res := runGit(projectDir, "config", "--local", "credential.helper", helperValue); res.Err != nil {
		return fmt.Errorf("set credential helper: %s", res.Output)
	}
	// Ensure the remote URL is clean (no embedded creds from older versions).
	if res := runGit(projectDir, "remote", "set-url", "origin", remoteURL); res.Err != nil {
		return fmt.Errorf("clean remote URL: %s", res.Output)
	}
	return nil
}

// VerifyRemoteAuth performs a zero-cost authenticated check (`git ls-remote`)
// after credentials have been set. Returns nil on success.
func VerifyRemoteAuth(projectDir string) error {
	res := runGit(projectDir, "ls-remote", "origin")
	if res.Err != nil {
		return classifyGitError(res.Output)
	}
	return nil
}

// IsAuthConfigured reports whether a credential file has been written for this
// repository. It is a fast, local-only check — no network call is made.
func IsAuthConfigured(projectDir string) bool {
	credPath := filepath.Join(projectDir, ".git", "credentials")
	info, err := os.Stat(credPath)
	return err == nil && info.Size() > 0
}

// RevokeAuth removes the stored credential file and clears the local
// credential.helper setting, effectively logging the user out of this repo.
func RevokeAuth(projectDir string) error {
	credPath := filepath.Join(projectDir, ".git", "credentials")
	if err := os.Remove(credPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove credentials: %w", err)
	}
	// Best-effort: ignore error if the key was never set
	_ = runGit(projectDir, "config", "--local", "--unset", "credential.helper")
	return nil
}
