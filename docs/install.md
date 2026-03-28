# Installing `poe`

`poe` is distributed as prebuilt archives on GitHub Releases, with first-party installers for macOS, Linux, and Windows.

## Quick Install

macOS and Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/oco-adam/panelofexperts/main/scripts/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/oco-adam/panelofexperts/main/scripts/install.ps1 | iex
```

Quick install requires at least one published GitHub release. If the installer says no published release exists yet, clone the repo and use the source-build flow from the README until the first release is published.

The installers:

- install `poe` onto your user PATH
- verify the downloaded archive checksum
- write a minimal install receipt for `poe doctor`
- support version pinning with `POE_VERSION`

## Manual Fallback

1. Download the archive for your platform from [GitHub Releases](https://github.com/oco-adam/panelofexperts/releases).
2. Extract the archive.
3. Move `poe` or `poe.exe` into a directory on your PATH.
4. Run `poe version`.
5. Run `poe doctor`.

## Installer Controls

- `POE_VERSION`: install a specific release tag such as `v1.2.3`
- `POE_INSTALL_DIR`: override the destination directory
- `POE_BASE_URL`: override the artifact source for local smoke tests or mirrors
- `POE_FORCE_INSTALL=1`: allow overwrite when another `poe` binary is already ahead of the chosen install path
- `POE_HOME`: override the config directory used for the install receipt
