# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
## [1.0.9] - 2026-04-26


## [1.0.8] - 2026-04-26

### Fixed
- fixed icon on compiling


## [1.0.7] - 2026-04-26

### Changed
- 1. policy/ sub-package (internal/launcher/policy/forbidden.go) — listas declarativas de prefijos, extensiones y nombres prohibidos. IsForbidden(path) es la única implementación; git.go la llama vía policy.IsForbidden. Tests pasan: ok flow-launcher/internal/launcher/policy 0.378s.
- 2. log/ sub-package (internal/launcher/log/log.go) — slog apuntando a <config-dir>/devsecops.flow.launcher/launcher.log, rotación a .1 cuando supera 2 MiB, fallback a stderr si el directorio no es escribible, no-op logger antes de Init() para que sea seguro llamarlo desde otros paquetes en init time. app.go Run() lo inicializa primero y defer log.Close(). State save/error path ya logea.
- 3. Store struct (en state.go) — wrapper con sync.RWMutex y métodos Open, Snapshot, Projects, TemplateURL, DarkMode, SetDarkMode, SetTemplateURL, UpsertProject, TouchProject, RemoveProject, FindProject. persistLocked se llama tras cada mutación, eliminando el patrón frágil u.state.X = Y; saveState(u.state) (ahora _ = u.store.SetX(...)). ui mantiene un state cacheado que se refresca con u.refresh().
- 4. Client struct (en git.go) — todas las operaciones git (Clone, Init, LsRemote, CommitReadme, Status, RemoteStatus, Publish, AssertNoForbiddenArtifacts, SetCredentials, VerifyAuth, IsAuthConfigured, RevokeAuth) son métodos. El runner field es inyectable (NewClient() usa el real, NewClientWithRunner(fn) para tests). Top-level functions (cloneTemplate, initRepository, etc.) delegan a defaultClient, así no rompí ningún call site.
- 5. Split de app.go — de 1224 líneas a 245. Siete archivos nuevos en el mismo package:
- view_welcome.go (43 LOC)
- view_bootstrap.go (303 LOC, incluye la pipeline bootstrap)
- view_dashboard.go (285 LOC)
- view_publish.go (153 LOC)
- view_signin.go (158 LOC, los tres dialogs bloqueantes)
- view_settings.go (27 LOC, template settings)
- view_helpers.go (134 LOC, providerColor/chip/heading/muted/sectionPanel/sanitizeBinaryName/etc.)
- 6. Tests — 4 archivos:
- policy/forbidden_test.go — 24 casos de IsForbidden, +smoke test de no-vacío. Verificado pasando.
- git_test.go — classifyGitError (7 casos), detectProvider (7 casos hostnames), probeGitea (3 casos con httptest), Client.Status/RemoteStatus/AssertNoForbiddenArtifacts con runner falso, TokenCreationURL. Estos no los puedo correr aquí (Fyne+CGO+OpenGL en esta VM), pero son sintácticamente correctos y aprovechan NewClientWithRunner.
- template_test.go — replaceShellValue (5 casos, incluyendo el caso real con command substitution) y readLatestVersion (5 + missing-file).
- view_helpers_test.go — sanitizeBinaryName (11 casos), parseInt (8), firstNonEmpty (5), platformValue (5), workflowFilePath (3 con filepath.Join para portabilidad)


## [1.0.6] - 2026-04-26

### Fixed
- name on .deb


## [1.0.5] - 2026-04-26

### Fixed
- go win


## [1.0.4] - 2026-04-26

### Fixed
- go compiler win fix


## [1.0.3] - 2026-04-26

### Fixed
- Languange selector color fix


## [1.0.2] - 2026-04-26

### Added
- fix


## [1.0.1] - 2026-04-26

### Added
- fix


## [1.0.0] - 2026-04-26

### Added
- first release



### Added
- _Nothing yet._

### Changed
- _Nothing yet._

### Fixed
- _Nothing yet._

## [1.0.0] - 2026-04-25

### Added
- Project bootstrap from a Git template (autoworkflow-templater by default),
  with auto-detection of GitHub vs Gitea (including self-hosted Gitea via
  `/api/v1/version` probing).
- Build template selection for ten languages: go, python, rust, node, java,
  dotnet, ruby, php, c, cpp.
- Per-platform runner tag configuration (Linux + Windows independently),
  including comma-separated label sets for self-hosted runner pools.
- Personal Access Token sign-in flow with a per-repo credential helper —
  remote URL stays clean, credentials live in `.git/credentials` (mode 0600).
- Authenticated remote verification (`git ls-remote`) before bootstrap
  completes.
- Publish flow that injects a `CHANGELOG.md` entry, commits, tags `vX.Y.Z`,
  pushes the branch and tag, and refuses to publish if untracked artefacts
  (`*.exe`, `*.deb`, `*.so`, `dist/`, …) are present.
- Light/dark mode toggle persisted in the app state.
- Auto-cleanup of template-only files (`publish.sh`, `BUILD_CONTRACT.md`,
  `build-templates/`) after bootstrap, with a starter README scaffolded for
  the new project.
- Editable tool name and runner configuration on the dashboard, synced back
  to `build.sh` and the workflow on save.
- Windows OpenGL self-extraction (Mesa llvmpipe `opengl32.dll` embedded via
  `go:embed` and re-execed on first launch) for VM / no-GPU environments.

### Changed
- Project moved to a native dual-runner CI strategy because Fyne requires
  CGO + system OpenGL/X11 libraries that don't cross-compile reliably from a
  single host.
