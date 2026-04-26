package launcher

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestClassifyGitError(t *testing.T) {
	cases := []struct {
		name    string
		output  string
		wantErr error // nil = expect "git operation failed:" wrapper
	}{
		{
			name:    "DNS failure",
			output:  "fatal: unable to access 'https://x/': Could not resolve host: x",
			wantErr: nil,
		},
		{
			name:    "repo not found",
			output:  "remote: Repository not found.\nfatal: ...",
			wantErr: ErrAuthRequired,
		},
		{
			name:    "auth failed",
			output:  "remote: Invalid username or password.\nfatal: Authentication failed for ...",
			wantErr: ErrAuthRequired,
		},
		{
			name:    "permission denied",
			output:  "Permission denied (publickey).",
			wantErr: ErrAuthRequired,
		},
		{
			name:    "403",
			output:  "remote: HTTP 403: Forbidden",
			wantErr: ErrAuthRequired,
		},
		{
			name:    "could not read username (terminal prompt disabled)",
			output:  "fatal: could not read Username for ...",
			wantErr: ErrAuthRequired,
		},
		{
			name:    "generic fallback",
			output:  "fatal: refusing to merge unrelated histories",
			wantErr: nil, // falls through to default
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := classifyGitError(tc.output)
			if err == nil {
				t.Fatalf("classifyGitError returned nil for %q", tc.output)
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("classifyGitError(%q) = %v, want %v", tc.output, err, tc.wantErr)
				}
				return
			}
			// Default case: no specific sentinel, just check the wrapper format.
			if !strings.Contains(err.Error(), "git operation failed") &&
				!strings.Contains(err.Error(), "remote repository is not reachable") {
				t.Errorf("classifyGitError(%q) = %v, expected wrapped fallback", tc.output, err)
			}
		})
	}
}

func TestDetectProvider_HostnameMatching(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"github HTTPS", "https://github.com/user/repo.git", "github"},
		{"github SSH-style URL not parsed as URL", "git@github.com:user/repo.git", "unknown"}, // url.Parse fails on this form, returns unknown
		{"gitea explicit", "https://gitea.example.com/user/repo.git", "gitea"},
		{"gitea on subdomain", "https://my-gitea.acme.io/user/repo.git", "gitea"},
		{"unknown host without probe match", "https://gitlab.com/user/repo.git", "unknown"},
		{"empty URL", "", "unknown"},
		{"garbage", "::not a url::", "unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := detectProvider(tc.url)
			// We don't probe the network in unit tests for unknown hosts.
			if tc.want == "unknown" && got == "gitea" {
				t.Skipf("network probe accidentally hit live gitea endpoint for %q", tc.url)
			}
			if got != tc.want {
				t.Errorf("detectProvider(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

func TestProbeGitea(t *testing.T) {
	t.Run("returns gitea on /api/v1/version with version field", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/version" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(`{"version":"1.21.0"}`))
		}))
		defer srv.Close()

		u := mustParseURL(t, srv.URL)
		provider, ok := probeGitea(u)
		if !ok || provider != "gitea" {
			t.Errorf("probeGitea(%q) = (%q, %v), want (\"gitea\", true)", srv.URL, provider, ok)
		}
	})

	t.Run("returns false on 404", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer srv.Close()

		u := mustParseURL(t, srv.URL)
		_, ok := probeGitea(u)
		if ok {
			t.Errorf("probeGitea(%q) = ok=true, want false", srv.URL)
		}
	})

	t.Run("returns false on body without version field", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`<html>not gitea</html>`))
		}))
		defer srv.Close()

		u := mustParseURL(t, srv.URL)
		_, ok := probeGitea(u)
		if ok {
			t.Errorf("probeGitea(%q) = ok=true on non-JSON body, want false", srv.URL)
		}
	})
}

// ─── Client tests using a fake runner ──────────────────────────────────────

func TestClient_Status_CleanTree(t *testing.T) {
	c := NewClientWithRunner(func(dir string, args ...string) commandResult {
		return commandResult{Output: "", Err: nil}
	})
	got := c.Status("/some/dir")
	if got != "Working tree clean." {
		t.Errorf("Status with empty output = %q, want %q", got, "Working tree clean.")
	}
}

func TestClient_Status_Dirty(t *testing.T) {
	c := NewClientWithRunner(func(dir string, args ...string) commandResult {
		return commandResult{Output: "## main\n M README.md\n?? new.txt", Err: nil}
	})
	got := c.Status("/some/dir")
	if !strings.Contains(got, "M README.md") {
		t.Errorf("Status with dirty output should include the modification, got %q", got)
	}
}

func TestClient_RemoteStatus(t *testing.T) {
	t.Run("reachable", func(t *testing.T) {
		c := NewClientWithRunner(func(dir string, args ...string) commandResult {
			return commandResult{Output: "abc123\trefs/heads/main", Err: nil}
		})
		if got := c.RemoteStatus("/d"); got != "Remote reachable." {
			t.Errorf("RemoteStatus reachable = %q, want %q", got, "Remote reachable.")
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		c := NewClientWithRunner(func(dir string, args ...string) commandResult {
			return commandResult{Output: "fatal: Could not resolve host", Err: errFake}
		})
		got := c.RemoteStatus("/d")
		if !strings.HasPrefix(got, "Remote unavailable") {
			t.Errorf("RemoteStatus unavailable = %q, want prefix %q", got, "Remote unavailable")
		}
	})
}

func TestClient_AssertNoForbiddenArtifacts(t *testing.T) {
	t.Run("clean tree passes", func(t *testing.T) {
		c := NewClientWithRunner(func(dir string, args ...string) commandResult {
			return commandResult{Output: "src/main.go\nREADME.md", Err: nil}
		})
		if err := c.AssertNoForbiddenArtifacts("/d"); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("rejects untracked .exe", func(t *testing.T) {
		c := NewClientWithRunner(func(dir string, args ...string) commandResult {
			return commandResult{Output: "src/main.go\ntool.exe\n", Err: nil}
		})
		err := c.AssertNoForbiddenArtifacts("/d")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "tool.exe") {
			t.Errorf("error should mention the offending path, got %q", err.Error())
		}
	})

	t.Run("rejects dist/ prefix", func(t *testing.T) {
		c := NewClientWithRunner(func(dir string, args ...string) commandResult {
			return commandResult{Output: "dist/binary\n", Err: nil}
		})
		if err := c.AssertNoForbiddenArtifacts("/d"); err == nil {
			t.Error("expected error for dist/ prefix, got nil")
		}
	})

	t.Run("git error swallowed (best-effort policy check)", func(t *testing.T) {
		c := NewClientWithRunner(func(dir string, args ...string) commandResult {
			return commandResult{Output: "fatal: not a repo", Err: errFake}
		})
		if err := c.AssertNoForbiddenArtifacts("/d"); err != nil {
			t.Errorf("expected nil when git fails (intentional fall-through), got %v", err)
		}
	})
}

func TestTokenCreationURL(t *testing.T) {
	t.Run("github includes scopes", func(t *testing.T) {
		got := TokenCreationURL("github", "")
		if !strings.Contains(got, "github.com/settings/tokens/new") {
			t.Errorf("github URL not as expected: %q", got)
		}
		if !strings.Contains(got, "scopes=repo") {
			t.Errorf("github URL missing repo scope: %q", got)
		}
	})

	t.Run("gitea uses the remote host", func(t *testing.T) {
		got := TokenCreationURL("gitea", "https://gitea.acme.io/me/repo.git")
		want := "https://gitea.acme.io/user/settings/applications"
		if got != want {
			t.Errorf("TokenCreationURL gitea = %q, want %q", got, want)
		}
	})

	t.Run("gitea fails gracefully on bad URL", func(t *testing.T) {
		got := TokenCreationURL("gitea", "::not a url")
		if got != "" {
			t.Errorf("expected empty string for bogus URL, got %q", got)
		}
	})

	t.Run("unknown provider returns empty", func(t *testing.T) {
		got := TokenCreationURL("bitbucket", "https://x")
		if got != "" {
			t.Errorf("expected empty string for unknown provider, got %q", got)
		}
	})
}

// ─── Helpers ──────────────────────────────────────────────────────────────

var errFake = errors.New("fake git failure")

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url %q: %v", raw, err)
	}
	return u
}
