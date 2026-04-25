# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
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
