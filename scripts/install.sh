#!/usr/bin/env sh
set -eu

REPO="${POE_REPOSITORY:-oco-adam/panelofexperts}"
VERSION="${POE_VERSION:-}"
BASE_URL="${POE_BASE_URL:-}"
INSTALL_DIR="${POE_INSTALL_DIR:-$HOME/.local/bin}"
FORCE="${POE_FORCE_INSTALL:-0}"
NO_PROFILE="${POE_NO_PROFILE:-0}"

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Missing required command: $1" >&2
    exit 1
  }
}

download_to() {
  source="$1"
  destination="$2"
  case "$source" in
    http://*|https://*)
      require_cmd curl
      curl -fsSL "$source" -o "$destination"
      ;;
    file://*)
      cp "${source#file://}" "$destination"
      ;;
    *)
      cp "$source" "$destination"
      ;;
  esac
}

resolve_os() {
  case "$(uname -s)" in
    Darwin) echo "darwin" ;;
    Linux) echo "linux" ;;
    *) echo "Unsupported operating system" >&2; exit 1 ;;
  esac
}

resolve_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *) echo "Unsupported architecture" >&2; exit 1 ;;
  esac
}

latest_version() {
  require_cmd curl
  api_url="https://api.github.com/repos/$REPO/releases/latest"
  response_path="$(mktemp)"
  status="$(curl -sSL -o "$response_path" -w '%{http_code}' "$api_url" 2>/dev/null || true)"
  case "$status" in
    200)
      version="$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$response_path" | head -n 1)"
      rm -f "$response_path"
      [ -n "$version" ] || {
        echo "GitHub returned a latest release for $REPO, but no tag_name was present." >&2
        return 1
      }
      printf '%s\n' "$version"
      ;;
    404)
      rm -f "$response_path"
      echo "No published GitHub release found for $REPO. Quick install works only after a release is published." >&2
      echo "Build from source instead, or rerun with POE_VERSION and POE_BASE_URL pointed at release artifacts." >&2
      return 1
      ;;
    *)
      rm -f "$response_path"
      echo "Failed to resolve the latest GitHub release for $REPO (HTTP ${status:-000})." >&2
      return 1
      ;;
  esac
}

resolve_profile() {
  shell_name="$(basename "${SHELL:-sh}")"
  case "$shell_name" in
    zsh) echo "$HOME/.zprofile" ;;
    bash)
      if [ -f "$HOME/.bash_profile" ]; then
        echo "$HOME/.bash_profile"
      else
        echo "$HOME/.profile"
      fi
      ;;
    *) echo "$HOME/.profile" ;;
  esac
}

ensure_path_entry() {
  dir="$1"
  [ "$NO_PROFILE" = "1" ] && return 0
  case ":$PATH:" in
    *":$dir:"*) return 0 ;;
  esac
  profile="$(resolve_profile)"
  mkdir -p "$(dirname "$profile")"
  touch "$profile"
  marker="poe-installer-path"
  if grep -q "$marker" "$profile" 2>/dev/null; then
    return 0
  fi
  profile_dir="$dir"
  case "$dir" in
    "$HOME"/*) profile_dir="\$HOME/${dir#"$HOME"/}" ;;
  esac
  {
    printf '\n# %s\n' "$marker"
    printf 'export PATH="%s:$PATH"\n' "$profile_dir"
  } >>"$profile"
}

write_receipt() {
  install_path="$1"
  source_url="$2"
  if [ -n "${POE_HOME:-}" ]; then
    home_dir="$POE_HOME"
  elif [ -n "${XDG_CONFIG_HOME:-}" ]; then
    home_dir="$XDG_CONFIG_HOME/poe"
  elif [ "$(resolve_os)" = "darwin" ]; then
    home_dir="$HOME/Library/Application Support/poe"
  else
    home_dir="$HOME/.config/poe"
  fi
  mkdir -p "$home_dir"
  cat >"$home_dir/install-receipt.json" <<EOF
{
  "version": "$VERSION",
  "channel": "direct",
  "installed_at": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
  "install_path": "$install_path",
  "source_url": "$source_url",
  "repository": "$REPO"
}
EOF
}

verify_checksum() {
  archive_path="$1"
  checksums_path="$2"
  asset_name="$3"
  expected="$(awk -v name="$asset_name" '$2 == name { print $1 }' "$checksums_path")"
  [ -n "$expected" ] || {
    echo "Checksum for $asset_name not found" >&2
    exit 1
  }
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$archive_path" | awk '{print $1}')"
  else
    actual="$(shasum -a 256 "$archive_path" | awk '{print $1}')"
  fi
  [ "$expected" = "$actual" ] || {
    echo "Checksum verification failed for $asset_name" >&2
    exit 1
  }
}

OS="$(resolve_os)"
ARCH="$(resolve_arch)"
if [ -z "$VERSION" ]; then
  VERSION="$(latest_version)" || exit 1
fi
[ -n "$VERSION" ] || {
  echo "Unable to resolve a release version. Set POE_VERSION explicitly." >&2
  exit 1
}

if [ -n "$BASE_URL" ]; then
  RELEASE_ROOT="$BASE_URL"
else
  RELEASE_ROOT="https://github.com/$REPO/releases/download/$VERSION"
fi

archive_extension="tar.gz"
ASSET_VERSION="${VERSION#v}"
asset_name="poe_${ASSET_VERSION}_${OS}_${ARCH}.${archive_extension}"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT INT TERM
archive_path="$tmpdir/$asset_name"
checksums_path="$tmpdir/checksums.txt"

download_to "$RELEASE_ROOT/$asset_name" "$archive_path"
download_to "$RELEASE_ROOT/checksums.txt" "$checksums_path"
verify_checksum "$archive_path" "$checksums_path" "$asset_name"

mkdir -p "$INSTALL_DIR"
target_path="$INSTALL_DIR/poe"
existing_path="$(command -v poe 2>/dev/null || true)"
if [ -n "$existing_path" ] && [ "$existing_path" != "$target_path" ] && [ "$FORCE" != "1" ]; then
  echo "Found poe at $existing_path, which does not match $target_path. Re-run with POE_FORCE_INSTALL=1 to override." >&2
  exit 1
fi

extract_dir="$tmpdir/extract"
mkdir -p "$extract_dir"
tar -xzf "$archive_path" -C "$extract_dir"
install -m 0755 "$extract_dir/poe" "$target_path"

ensure_path_entry "$INSTALL_DIR"
write_receipt "$target_path" "$RELEASE_ROOT/$asset_name"

echo "Installed poe $VERSION to $target_path"
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    if [ "$NO_PROFILE" = "1" ]; then
      echo "Add $INSTALL_DIR to PATH before opening a new shell."
    else
      profile="$(resolve_profile)"
      echo "PATH updated in $profile. Start a new shell or run: export PATH=\"$INSTALL_DIR:\$PATH\""
    fi
    ;;
esac
