package launcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"flow-launcher/internal/launcher/log"
)

const (
	appID                = "devsecops.flow.launcher"
	defaultTemplateURL   = "https://github.com/sPROFFEs/autoworkflow-templater.git"
	defaultRunnerLinux   = "self-hosted, debian-12"
	defaultRunnerWindows = "windows-latest"
	currentSchema        = 2
)

type Project struct {
	Name          string `json:"name"`
	BinaryName    string `json:"binary_name,omitempty"`
	Path          string `json:"path"`
	RemoteURL     string `json:"remote_url"`
	Provider      string `json:"provider"`
	Language      string `json:"language"`
	Platform      string `json:"platform"`
	RunnerLinux   string `json:"runner_linux"`
	RunnerWindows string `json:"runner_windows"`
	// LegacyRunner is the field used by schema v1; kept here only so the
	// JSON unmarshaler can read it on first launch and migrate it into
	// RunnerLinux. The save path always strips it.
	LegacyRunner string    `json:"runner,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	LastOpened   time.Time `json:"last_opened"`
}

type AppState struct {
	Schema      int       `json:"schema"`
	DarkMode    bool      `json:"dark_mode,omitempty"`
	TemplateURL string    `json:"template_url"`
	Projects    []Project `json:"projects"`
}

func loadState() AppState {
	path := statePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return AppState{Schema: currentSchema, TemplateURL: defaultTemplateURL}
	}

	var state AppState
	if err := json.Unmarshal(data, &state); err != nil {
		return AppState{Schema: currentSchema, TemplateURL: defaultTemplateURL}
	}
	if state.TemplateURL == "" {
		state.TemplateURL = defaultTemplateURL
	}
	migrateProjects(&state)
	state.Schema = currentSchema
	return state
}

// migrateProjects upgrades older Project records in-place.
func migrateProjects(state *AppState) {
	for i := range state.Projects {
		p := &state.Projects[i]
		if p.RunnerLinux == "" {
			if p.LegacyRunner != "" {
				p.RunnerLinux = p.LegacyRunner
			} else {
				p.RunnerLinux = defaultRunnerLinux
			}
		}
		if p.RunnerWindows == "" {
			p.RunnerWindows = defaultRunnerWindows
		}
		p.LegacyRunner = ""
	}
}

func saveState(state AppState) error {
	path := statePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	state.Schema = currentSchema
	// Strip the legacy field on save so the file converges to schema v2.
	for i := range state.Projects {
		state.Projects[i].LegacyRunner = ""
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func upsertProject(state *AppState, p Project) {
	p.LastOpened = time.Now()
	for i := range state.Projects {
		if filepath.Clean(state.Projects[i].Path) == filepath.Clean(p.Path) {
			state.Projects[i] = p
			return
		}
	}
	state.Projects = append(state.Projects, p)
}

func touchProject(state *AppState, path string) {
	for i := range state.Projects {
		if filepath.Clean(state.Projects[i].Path) == filepath.Clean(path) {
			state.Projects[i].LastOpened = time.Now()
			return
		}
	}
}

func removeProject(state *AppState, path string) {
	out := state.Projects[:0]
	for _, p := range state.Projects {
		if filepath.Clean(p.Path) != filepath.Clean(path) {
			out = append(out, p)
		}
	}
	state.Projects = out
}

func projectsSortedByRecent(state AppState) []Project {
	cp := make([]Project, len(state.Projects))
	copy(cp, state.Projects)
	sort.SliceStable(cp, func(i, j int) bool {
		ti := cp[i].LastOpened
		if ti.IsZero() {
			ti = cp[i].CreatedAt
		}
		tj := cp[j].LastOpened
		if tj.IsZero() {
			tj = cp[j].CreatedAt
		}
		return ti.After(tj)
	})
	return cp
}

func statePath() string {
	if cfg, err := os.UserConfigDir(); err == nil {
		return filepath.Join(cfg, appID, "state.json")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "."+appID, "state.json")
	}
	return filepath.Join(".", ".flow-launcher.json")
}

// ─── Store ────────────────────────────────────────────────────────────────────
//
// Store wraps AppState mutations and persists them automatically. Every code
// path that needs to change app state goes through a Store method, which
// guarantees the in-memory state and the JSON file on disk never diverge.
//
// The mutex is intentionally coarse-grained — state changes are infrequent
// and typing-safe atomic operations would just add boilerplate.

type Store struct {
	mu    sync.RWMutex
	state AppState
}

// Open loads state from disk and returns a Store ready for use.
func Open() *Store {
	return &Store{state: loadState()}
}

// Snapshot returns a deep-enough copy for read-only consumers. Project slice
// is copied so callers cannot mutate the live state by accident.
func (s *Store) Snapshot() AppState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := s.state
	cp.Projects = append([]Project(nil), s.state.Projects...)
	return cp
}

// Projects returns a sorted-by-recent copy of the project list.
func (s *Store) Projects() []Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return projectsSortedByRecent(s.state)
}

// TemplateURL returns the configured template repository URL.
func (s *Store) TemplateURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.TemplateURL
}

// DarkMode reports whether dark theme is active.
func (s *Store) DarkMode() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.DarkMode
}

// SetDarkMode persists the new theme preference.
func (s *Store) SetDarkMode(dark bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.DarkMode = dark
	return s.persistLocked("set dark mode")
}

// SetTemplateURL persists a new template repository URL.
func (s *Store) SetTemplateURL(url string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.TemplateURL = url
	return s.persistLocked("set template url")
}

// UpsertProject inserts or updates a project (matched by Path) and persists.
// LastOpened is refreshed by upsertProject before save.
func (s *Store) UpsertProject(p Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	upsertProject(&s.state, p)
	return s.persistLocked("upsert project")
}

// TouchProject bumps a project's LastOpened timestamp and persists.
func (s *Store) TouchProject(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	touchProject(&s.state, path)
	return s.persistLocked("touch project")
}

// RemoveProject removes a project (matched by Path) and persists.
func (s *Store) RemoveProject(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	removeProject(&s.state, path)
	return s.persistLocked("remove project")
}

// FindProject returns a copy of the project with the given path, or nil.
func (s *Store) FindProject(path string) *Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.state.Projects {
		if filepath.Clean(s.state.Projects[i].Path) == filepath.Clean(path) {
			cp := s.state.Projects[i]
			return &cp
		}
	}
	return nil
}

// persistLocked must be called with s.mu already held in write mode.
func (s *Store) persistLocked(op string) error {
	if err := saveState(s.state); err != nil {
		log.Default().Error("state save failed", "op", op, "err", err.Error())
		return err
	}
	log.Default().Debug("state saved", "op", op)
	return nil
}
