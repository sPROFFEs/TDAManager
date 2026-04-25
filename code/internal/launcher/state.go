package launcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	appID                = "devsecops.flow.launcher"
	defaultTemplateURL   = "https://github.com/sPROFFEs/autoworkflow-templater.git"
	defaultRunnerLinux   = "self-hosted, debian-12"
	defaultRunnerWindows = "windows-latest"
	currentSchema        = 2
)

type Project struct {
	Name          string    `json:"name"`
	BinaryName    string    `json:"binary_name,omitempty"`
	Path          string    `json:"path"`
	RemoteURL     string    `json:"remote_url"`
	Provider      string    `json:"provider"`
	Language      string    `json:"language"`
	Platform      string    `json:"platform"`
	RunnerLinux   string    `json:"runner_linux"`
	RunnerWindows string    `json:"runner_windows"`
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
