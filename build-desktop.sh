#!/usr/bin/env bash
set -euo pipefail

project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$project_root"
version="${1:-0.12.1}"
package_version="$(node -p 'require("./desktop/package.json").version')"
[[ "$version" == "$package_version" ]] || { echo "Version mismatch: package.json is $package_version, requested build is $version" >&2; exit 1; }
export DEEP_LEGENDS_KEY_MODE="${DEEP_LEGENDS_KEY_MODE:-private}"
case "$DEEP_LEGENDS_KEY_MODE" in public|private) ;; *) echo "DEEP_LEGENDS_KEY_MODE must be public or private" >&2; exit 1 ;; esac
rm -f "$project_root/dist/desktop/release-build.json"
find "$project_root/dist/desktop" -maxdepth 1 -type f \( -name 'Deep Legends*.exe' -o -name 'Deep Legends*.zip' -o -name 'SHA256SUMS.txt' \) -delete 2>/dev/null || true
rm -rf "$project_root/dist/desktop/win-unpacked"
cipher=""
if [[ "$DEEP_LEGENDS_KEY_MODE" == "private" ]]; then
  key_file="${RIOT_KEY_FILE:-$project_root/riot_key.local.txt}"
  if [[ ! -f "$key_file" ]]; then
    echo "Riot API key file not found: $key_file" >&2
    exit 1
  fi
  plain_key="$(tr -d '\r\n' < "$key_file")"
  [[ -n "$plain_key" ]] || { echo "Riot API key is empty" >&2; exit 1; }
  cipher="$(env GOCACHE="${GOCACHE:-$project_root/.gocache}" GOTMPDIR="${GOTMPDIR:-/private/tmp}" go run . -encrypt-riot-key "$plain_key")"
  [[ -n "$cipher" ]] || { echo "Failed to encrypt Riot API key" >&2; exit 1; }
else
  echo "Public build: no embedded Riot API key; KR queries require RIOT_API_KEY at runtime."
fi
source_fingerprint="$(node desktop/source-fingerprint.cjs)"
[[ "$source_fingerprint" =~ ^[0-9a-f]{12}$ ]] || { echo "Invalid source fingerprint" >&2; exit 1; }
GOCACHE="${GOCACHE:-$project_root/.gocache}" GOTMPDIR="${GOTMPDIR:-/private/tmp}" GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=false -trimpath \
  -ldflags="-s -w -H=windowsgui -buildid= -X main.version=$version -X main.buildFingerprint=$source_fingerprint -X main.riotAPIKey= -X main.riotAPIKeyCipher=$cipher" \
  -o desktop/backend/loot-service.exe .
node desktop/verify-build-fingerprint.cjs desktop/backend/loot-service.exe "$source_fingerprint"
if [[ "${SKIP_NPM_INSTALL:-0}" != "1" ]]; then
  (cd desktop && npm ci)
fi

discover_cached_electron_dist() {
  local electron_package="$project_root/desktop/node_modules/electron/package.json"
  local electron_version electron_zip_name cache_root candidate
  local -a cache_roots=()

  [[ -f "$electron_package" ]] || return 1
  command -v find >/dev/null 2>&1 || return 1
  command -v unzip >/dev/null 2>&1 || return 1
  electron_version="$(node -e 'process.stdout.write(require(process.argv[1]).version)' "$electron_package" 2>/dev/null || true)"
  [[ -n "$electron_version" ]] || return 1
  electron_zip_name="electron-v${electron_version}-win32-x64.zip"

  if [[ -n "${ELECTRON_CACHE:-}" ]]; then
    cache_roots+=("$ELECTRON_CACHE")
  fi
  case "$(uname -s)" in
    Darwin)
      [[ -n "${HOME:-}" ]] && cache_roots+=("$HOME/Library/Caches/electron")
      ;;
    Linux)
      if [[ -n "${XDG_CACHE_HOME:-}" ]]; then
        cache_roots+=("$XDG_CACHE_HOME/electron")
      elif [[ -n "${HOME:-}" ]]; then
        cache_roots+=("$HOME/.cache/electron")
      fi
      ;;
    MINGW*|MSYS*|CYGWIN*)
      [[ -n "${LOCALAPPDATA:-}" ]] && cache_roots+=("$LOCALAPPDATA/electron/Cache")
      ;;
  esac

  for cache_root in "${cache_roots[@]}"; do
    [[ -d "$cache_root" ]] || continue
    while IFS= read -r candidate; do
      [[ -f "$candidate" ]] || continue
      if ! unzip -tq "$candidate" >/dev/null 2>&1; then
        echo "Ignoring invalid cached Electron archive: $candidate" >&2
        continue
      fi
      printf '%s\n' "$candidate"
      return 0
    done < <(find "$cache_root" -type f -name "$electron_zip_name" -print 2>/dev/null || true)
  done
  return 1
}

resolve_electron_dist() {
  if [[ -n "${ELECTRON_DIST:-}" ]]; then
    printf '%s\n' "$ELECTRON_DIST"
    return 0
  fi
  [[ "${AUTO_ELECTRON_DIST:-1}" != "0" ]] || return 1
  discover_cached_electron_dist
}

report_electron_dist() {
  local electron_dist="$1"
  [[ -n "$electron_dist" ]] || return 0
  if [[ -n "${ELECTRON_DIST:-}" ]]; then
    echo "Using ELECTRON_DIST: $electron_dist"
  else
    echo "Reusing cached Electron distribution: $electron_dist"
  fi
}

(
  cd desktop
  export DEEP_LEGENDS_FINGERPRINT="$source_fingerprint"
  builder_common_args=(--config.win.signExecutable=false)
  electron_dist="$(resolve_electron_dist || true)"
  report_electron_dist "$electron_dist"
  trap 'rm -f "$project_root/desktop/uninstall-shell.exe"' EXIT
  GOCACHE="${GOCACHE:-$project_root/.gocache}" GOTMPDIR="${GOTMPDIR:-/tmp}" \
    node "$project_root/scripts/build-stage.cjs" uninstaller-shell node ../installer/build-shell.cjs --uninstall "$version"
  setup_args=("${builder_common_args[@]}")
  if [[ -n "$electron_dist" ]]; then setup_args+=(--config.electronDist="$electron_dist"); fi
  # beforePack selects level 3 compression; retain elapsed-time reports for NSIS.
  node "$project_root/scripts/build-stage.cjs" nsis npm run pack:win-setup -- "${setup_args[@]}"
  rm -f "$project_root/desktop/uninstall-shell.exe"
  # NSIS → embedded payload → Go shell → final artifact, before SHA256SUMS.
  GOCACHE="${GOCACHE:-$project_root/.gocache}" GOTMPDIR="${GOTMPDIR:-/tmp}" \
    node "$project_root/scripts/build-stage.cjs" installer-shell node ../installer/build-shell.cjs "$version" "$source_fingerprint"
  node verify-packaged-runtime.cjs ../dist/desktop/win-unpacked/resources/app.asar
  node verify-build-fingerprint.cjs ../dist/desktop/win-unpacked/resources/app.asar.unpacked/backend/loot-service.exe "$source_fingerprint"
  node release-build.cjs "$source_fingerprint"
  cd ../dist/desktop
  shasum -a 256 "Deep Legends Setup ${version}.exe" > SHA256SUMS.txt
  rm -rf win-unpacked
  echo "Setup build complete: Deep Legends Setup ${version}.exe"
)
