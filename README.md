# Panel of Experts (`poe`)

`poe` is a terminal app that runs a structured AI discussion between a manager model and a panel of expert models. It is designed for planning and document work inside a real project directory, then saves the full run history and final output to disk.

In practice, you launch `poe`, pick which providers should act as manager and experts, describe the task, answer any manager follow-up questions, and then let the panel debate until a final proposal or Markdown deliverable is ready.

## What The App Does

- Detects which supported AI CLIs are installed locally: Codex, Claude, and Gemini.
- Lets you choose a manager and 2-3 experts from the providers you have available.
- Builds a planning brief before the expert discussion starts.
- Runs expert reviews and manager merges across multiple rounds.
- Saves the full run state, proposal history, review artifacts, and final Markdown output.
- Writes the final document to disk automatically when your task targets a Markdown file such as `README.md` or `docs/plan.md`.

## Supported Platforms

Prebuilt release binaries are published for:

- macOS `amd64` and `arm64`
- Linux `amd64` and `arm64`
- Windows `amd64` and `arm64`

If you prefer, you can also build from source on any platform with Go installed.

## Before You Install

You need:

1. A terminal with normal ANSI/UTF-8 support.
2. At least one supported provider CLI on your `PATH`.
3. An authenticated session for each provider you want to use.
4. Network access for the provider CLIs.

Supported providers:

- `codex`
- `claude`
- `gemini`

`poe doctor` will tell you which providers are currently:

- `missing`
- `available`
- `ready`

`ready` means the binary is present and the app found a local auth state for that provider.

## Install

Release downloads: [GitHub Releases](https://github.com/oco-adam/panelofexperts/releases)

### macOS

Fastest install:

```sh
curl -fsSL https://raw.githubusercontent.com/oco-adam/panelofexperts/main/scripts/install.sh | sh
```

Quick install requires at least one published GitHub release. If the installer says no published release exists yet, use the source-build path below until the first release is cut.

What this does:

- Downloads the latest macOS release for your CPU architecture.
- Verifies the archive checksum.
- Installs `poe` into `~/.local/bin` by default.
- Adds that directory to your shell profile if needed.

Then open a new shell and run:

```sh
poe version
poe doctor
```

Manual install:

1. Download the correct macOS archive from GitHub Releases.
2. Extract it.
3. Move `poe` into a directory on your `PATH`, such as `~/.local/bin`.
4. Run `poe version`.
5. Run `poe doctor`.

### Linux

Fastest install:

```sh
curl -fsSL https://raw.githubusercontent.com/oco-adam/panelofexperts/main/scripts/install.sh | sh
```

Quick install requires at least one published GitHub release. If the installer says no published release exists yet, use the source-build path below until the first release is cut.

Then verify:

```sh
poe version
poe doctor
```

Manual install:

1. Download the correct Linux archive from GitHub Releases.
2. Extract it.
3. Move `poe` into a directory on your `PATH`, such as `~/.local/bin`.
4. Run `poe version`.
5. Run `poe doctor`.

### Windows

Open PowerShell and run:

```powershell
irm https://raw.githubusercontent.com/oco-adam/panelofexperts/main/scripts/install.ps1 | iex
```

Quick install requires at least one published GitHub release. If the installer says no published release exists yet, use the source-build path below until the first release is cut.

Then verify:

```powershell
poe version
poe doctor
```

Manual install:

1. Download the correct Windows `.zip` archive from GitHub Releases.
2. Extract it.
3. Move `poe.exe` into a directory on your `PATH`.
4. Open a new PowerShell session.
5. Run `poe version`.
6. Run `poe doctor`.

### Build From Source

If you want a local development build instead of a release binary, or you are running from `main` before the first published release exists:

1. Install Go `1.25.1` or newer.
2. Clone this repository.
3. Build or install the CLI.

Build in place on macOS or Linux:

```sh
go build -o poe ./cmd/poe
```

Build in place on Windows:

```powershell
go build -o poe.exe ./cmd/poe
```

Install to your Go bin directory:

```sh
go install ./cmd/poe
```

After that, run:

```sh
poe version
poe doctor
```

Note: source builds report build metadata like `version: dev` until you cut a release build.

## Installer Options

The bundled install scripts support a few useful overrides:

- `POE_VERSION`: install a specific release tag such as `v1.2.3`
- `POE_INSTALL_DIR`: install to a custom directory
- `POE_BASE_URL`: use a custom release artifact location
- `POE_FORCE_INSTALL=1`: allow overwrite when another `poe` binary is found first on `PATH`
- `POE_HOME`: override the app config directory used for install receipts

Unix installer only:

- `POE_NO_PROFILE=1`: do not edit your shell profile

Windows installer flags:

- `-Version`
- `-InstallDir`
- `-BaseUrl`
- `-Force`
- `-NoPathUpdate`

More installer detail is in [`docs/install.md`](docs/install.md).

## Provider Setup

`poe` does not bundle model runtimes itself. It orchestrates the provider CLIs you already use locally.

For each provider you want to use:

1. Install that provider's CLI.
2. Make sure the CLI command is on your `PATH`.
3. Log in with that CLI.
4. Run `poe doctor` to confirm the provider shows as `ready`.

If a provider shows `available` but not `ready`, the binary was found but authentication is still missing or incomplete.

## Quick Start

From your project directory:

```sh
poe
```

Then follow this flow:

1. In the Setup screen, choose the manager provider.
2. Choose 2 or 3 expert providers.
3. Set `Max rounds` and `Merge mode`.
4. Leave the workspace and output paths as-is unless you want a custom location.
5. Type your initial request in `Initial Intent`.
6. Press `Enter` to create the run.
7. Review the manager brief.
8. Answer any follow-up questions, or press `Ctrl+S` to start the discussion.
9. Watch the live discussion in the monitor.
10. Press `r` when the final result is ready.

Example requests:

- `Draft a rollout plan for the current release pipeline`
- `Write docs/architecture.md explaining how the orchestrator works`
- `Add a comprehensive README.md that explains installation and usage`

If your request clearly targets a Markdown file, `poe` treats it as a document task and writes the final deliverable to that file when the discussion finishes.

## Screen-By-Screen Usage

### 1. Setup

This is where you configure the run:

- Manager provider
- Expert providers
- Expert count: `2` or `3`
- Max rounds
- Merge mode: `together` or `sequential`
- Workspace path
- Output root
- Initial intent

Controls:

- `Tab`, `Shift+Tab`, `Up`, `Down`, `j`, `k`: move between fields
- `Left`, `Right`, `h`, `l`: change the selected value
- `Enter`: create the run
- `q` or `Ctrl+C`: quit

### 2. Manager Brief

Before the manager asks follow-up questions, `poe` collects a repo-grounding snapshot from high-signal files in the current workspace and shows that summary in the brief screen. The manager is expected to use that grounding for repo facts and reserve follow-up questions for intent, scope, constraints, and tradeoffs the repo cannot answer.

The manager then turns your request into a structured brief. You can:

- answer manager follow-up questions
- clarify scope
- provide target file paths or constraints
- start the discussion once the brief is good enough

Controls:

- `Enter`: send your next reply to the manager
- `Ctrl+S`: start the expert discussion
- `q` or `Ctrl+C`: quit

### 3. Discussion Monitor

This screen shows:

- current run status
- current phase
- current round
- per-agent status
- live timeline events

Controls:

- `r`: open the final results once the run is finished
- `q` or `Ctrl+C`: quit

### 4. Results

The results screen shows the final Markdown and saved output paths.

Controls:

- `Up`, `Down`, `j`, `k`: scroll
- `m`: return to the monitor
- `q` or `Ctrl+C`: quit

## Commands

Start the interactive app:

```sh
poe
```

Run against a different workspace:

```sh
poe --cwd /path/to/workspace
```

Save run artifacts somewhere else:

```sh
poe --output-root /path/to/output
```

Browse saved runs in the current repository:

```sh
poe runs
```

Resume a failed or interrupted final deliverable phase in place:

```sh
poe retry --run 20260329-224313-myrepo --deliverable-timeout 2h
```

Start a fresh rerun from a saved brief with config overrides:

```sh
poe rerun --run 20260329-224313-myrepo --manager claude --experts gemini,codex --max-rounds 7 --merge sequential
```

Show build info:

```sh
poe version
```

Check installation, provider detection, and update status:

```sh
poe doctor
```

Enable Bubble Tea debug logging:

```sh
poe --debug
```

The final deliverable drafting phase now defaults to a `1h` timeout. Use `--deliverable-timeout` on `poe retry` or `poe rerun` to override it for that recovered run.

## Output Files

By default, run artifacts are written under:

```text
.panel-of-experts/runs/
```

Each run gets its own timestamped directory. Common files include:

- `state.json`: current run state
- `repo-grounding.json`: structured repo grounding snapshot used by the manager and panel
- `repo-grounding.md`: human-readable grounding summary shown in the brief flow
- `brief.json`: structured brief
- `brief.md`: rendered brief
- `proposal-vNNN.json`: structured manager proposals
- `proposal-vNNN.md`: rendered proposals
- `reviews/round-N/*.json`: expert review outputs
- `events.log`: timeline events
- `final.md`: final Markdown shown in the UI
- `deliverable.json`: final document metadata for Markdown-writing tasks
- `deliverable.md`: saved copy of the final document for Markdown-writing tasks

For document tasks, the app also writes the final content to the target file path chosen for the task.

`poe runs` reads these saved directories, `poe retry` resumes a saved run in place from a retryable final deliverable phase, and `poe rerun` creates a fresh run directory from a saved brief.

## Merge Modes

`poe` supports two merge strategies:

- `together`: the manager reconciles the full expert review bundle in one merge step
- `sequential`: the manager incorporates expert reviews one at a time

Use `together` for faster convergence and `sequential` when you want a more incremental merge trail.

## Troubleshooting

### `poe` command not found

- Open a new shell after installing.
- Confirm the install directory is on your `PATH`.
- Re-run the installer with `POE_INSTALL_DIR` if you want a different location.

### A provider is `missing`

- Install that provider CLI.
- Make sure its executable name is on your `PATH`.
- Run `poe doctor` again.

### A provider is `available` but not `ready`

- The CLI exists, but its login state was not detected.
- Authenticate with that provider's CLI, then re-run `poe doctor`.

### I want run files outside the repository

Start the app with:

```sh
poe --output-root /your/preferred/path
```

### I want to inspect the current environment quickly

Use:

```sh
poe doctor
```

It reports:

- build information
- current workspace
- output root
- app home
- install receipt path
- provider availability
- release update status

## Development

Run the test suite:

```sh
go test ./...
```

Create a local release snapshot:

```sh
goreleaser release --snapshot --clean
```

## License

No license file is currently present in this repository.
