# TDAManager — Tool Deployment Automated Manager

A desktop launcher for bootstrapping, configuring, and releasing pentesting /
DevSecOps tools through a standardised CI pipeline. Pick a language, point it
at a remote repository, and TDAManager wires up the Git remote, installs the
build template, configures the release workflow, and ships tagged releases
with one click.

Built with Go + [Fyne](https://fyne.io). Runs on Linux and Windows.

---

## What it does

- **Bootstrap** a new project from a Git template (defaults to
  [`autoworkflow-templater`](https://github.com/sPROFFEs/autoworkflow-templater))
- **Auto-detect** the provider (GitHub vs Gitea, including self-hosted Gitea
  via API probing) and keep only the workflow folder that matches
- **Configure** the build template for one of:
  `go · python · rust · node · java · dotnet · ruby · php · c · cpp`
- **Wire up** runner tags (`runs-on:`) for Linux and Windows separately,
  including comma-separated label sets for self-hosted runners
- **Sign in** to the remote with a Personal Access Token using a per-repo
  credential helper (no token leaks in `.git/config`)
- **Publish releases** by injecting a `CHANGELOG.md` entry, committing,
  tagging `vX.Y.Z`, and pushing — all atomically
- **Detect & block** commits that contain forbidden artefacts (`*.exe`,
  `*.deb`, `*.so`, `dist/`, …) before they reach the remote

The build template the launcher writes follows a strict three-zone contract
(see `BUILD_CONTRACT.md`): a fixed launcher contract block, an editable
project-config block, and a freely-customisable build-steps block.

---

## Installation

Download the binary for your platform from the
[Releases](../../releases) page.

### Linux

```bash
curl -L -o TDAManager https://.../TDAManager
chmod +x TDAManager
./TDAManager
```

Or install the `.deb` package if available:

```bash
sudo dpkg -i tdamanager_<version>_amd64.deb
```

### Windows

Download `TDAManager.exe` and run it. On first launch the binary extracts a
bundled Mesa `opengl32.dll` next to itself and re-exec's so VMs without a real
GPU still get OpenGL 3.3 via software rendering. No manual setup needed.

---

## Usage

1. Launch the app — the sidebar lists registered projects, the toolbar has a
   light/dark toggle and template-source settings.
2. Click **New project**, fill in:
   - Target folder (where the repo will live locally)
   - Remote URL (HTTPS clone URL of the empty repo on GitHub/Gitea)
   - Tool name (becomes the binary filename via `APP_NAME`)
   - Language and platforms
   - Runner tags for Linux and Windows
3. Bootstrap copies the template, configures the workflow, removes the
   manual-publish bits (`publish.sh`, `BUILD_CONTRACT.md`, `build-templates/`),
   and writes a starter README.
4. Sign in when prompted and confirm the initial connectivity push.
5. Drop your source code into `./code/` (the launcher's `build.sh` always
   `cd`s into that directory before compiling).
6. When ready, click **Publish release**, fill in `vX.Y.Z` and changelog
   notes, and the launcher commits, tags, and pushes.

State is persisted to your OS config dir
(`%AppData%\devsecops.flow.launcher\state.json` on Windows,
`~/.config/devsecops.flow.launcher/state.json` on Linux).

---

## Building from source

This project is a Fyne app, so CGO is required and cross-compiling from a
single host is fragile. The CI workflow uses **native dual-runner mode** —
one Linux runner builds the ELF, one Windows runner builds the EXE.

### Local development

```bash
cd code
make build-linux      # CGO=1, native gcc
make build-windows    # CGO=1, mingw-w64 (cross-compile, fragile)
make fyne-cross-windows   # uses Docker — most reliable for Windows builds
```

System dependencies on Debian/Ubuntu:

```bash
sudo apt-get install gcc pkg-config libgl1-mesa-dev xorg-dev
```

### CI builds

`build.sh` at the repo root is what the workflow runs. It `cd`s into `code/`,
installs Fyne dependencies if missing (Linux only), and compiles for the
runner's OS. See `BUILD_CONTRACT.md` for the contract details.

---

## Project layout

```
TDAManager/
├── build.sh                 # CI entry point (configured by the launcher itself)
├── CHANGELOG.md             # Keep-a-Changelog format, drives release notes
├── README.md                # this file
├── .github/workflows/       # GitHub Actions release pipeline
└── code/                    # Go module
    ├── cmd/flow-launcher/   # main package + Windows OpenGL bootstrap
    ├── internal/launcher/   # UI, state, git, template, theme
    ├── go.mod / go.sum
    ├── icon.png
    └── Makefile             # local-dev shortcuts
```

---

## Adding a new language

Three repos / files are involved when adding support for a new language:

1. **The build template** (in the `plantilla-flow` template repo,
   `build-templates/<group>/build.<lang>.sh`):
   - Pick `supports-crosscompiling/` if a single Linux job can produce both
     binaries (e.g. Go without CGO, Rust with mingw, .NET RIDs).
   - Pick `no-crosscompiling/` if each platform needs its own runner
     (Python/PyInstaller, C, C++, anything that links against system libs).
   - Copy the three-zone layout from any existing template — keep the
     🔒 LAUNCHER CONTRACT blocks intact, fill the ⚙️ PROJECT CONFIG, and
     write your build commands in the 🔨 block.

2. **The release workflow** (`plantilla-flow/.github/workflows/release.yaml`
   and `.gitea/workflows/release.yaml`, both at once):
   - Add `<lang>` to the `case "$LANG" in ... ) ;;` validation list in the
     `plan` job.
   - Add `<lang>` to the `CROSS_SUPPORTED` switch — under the cross-supported
     branch if it cross-compiles, under the no-cross branch otherwise.
   - Add a "Setup &lt;lang&gt; toolchain" step in the appropriate native job(s)
     (`build_native_linux` and/or `build_native_windows`) for installing the
     compiler / runtime — follow the pattern of the existing setup steps.

3. **The launcher** (this repo, `code/internal/launcher/`):
   - `template.go`: add `<lang>: true` to the `crossCompilingLanguages` map
     **only** if the build cross-compiles. Native-only languages must NOT be
     in the map.
   - `app.go`: add `"<lang>"` to the `langSelect` widget's options list in
     the bootstrap view (search for `widget.NewSelect([]string{...})`).

4. **Document it**: add a row to the languages table in
   `plantilla-flow/BUILD_CONTRACT.md`, then rebuild + tag a new TDAManager
   release so the binary picks up the new selector.

> **About the "both native runners" option**: a language being in
> `crossCompilingLanguages` only sets the *default* — the user can still pick
> "both native runners" in the bootstrap radio for any cross-compiling
> language. That writes `FORCE_NATIVE=1` to the workflow, which overrides the
> `CROSS_SUPPORTED` switch and routes to dual-runner mode. This is what
> TDAManager itself does, because Fyne needs CGO + system OpenGL/X11 libs
> that don't cross-compile reliably from a single host.

---

## Releasing

Use the launcher's **Publish release** button (preferred), or do it manually:

1. Add a `## [X.Y.Z] - YYYY-MM-DD` section to `CHANGELOG.md`.
2. Commit and push.
3. Tag `vX.Y.Z` and push the tag — the workflow builds, packages
   (`.deb` for Linux), and publishes a GitHub release with checksums.

The workflow refuses to publish if the changelog entry for the tag is missing
or empty.

---

## License

TBD.
