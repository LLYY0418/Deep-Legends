#!/usr/bin/env bash
set -euo pipefail

project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$project_root"
version="${1:-0.11.2}"
find "$project_root/dist/desktop" -maxdepth 1 -type f \( -name 'Deep Legends*.exe' -o -name 'Deep Legends*.zip' -o -name 'SHA256SUMS.txt' \) -delete 2>/dev/null || true
rm -rf "$project_root/dist/desktop/win-unpacked"
key_file="${RIOT_KEY_FILE:-$project_root/riot_key.local.txt}"
if [[ ! -f "$key_file" ]]; then
  echo "Riot API key file not found: $key_file" >&2
  exit 1
fi
plain_key="$(tr -d '\r\n' < "$key_file")"
[[ -n "$plain_key" ]] || { echo "Riot API key is empty" >&2; exit 1; }
cipher="$(env GOCACHE="${GOCACHE:-$project_root/.gocache}" GOTMPDIR="${GOTMPDIR:-/private/tmp}" go run . -encrypt-riot-key "$plain_key")"
[[ -n "$cipher" ]] || { echo "Failed to encrypt Riot API key" >&2; exit 1; }
source_fingerprint="$(node desktop/source-fingerprint.cjs)"
[[ "$source_fingerprint" =~ ^[0-9a-f]{12}$ ]] || { echo "Invalid source fingerprint" >&2; exit 1; }
GOCACHE="${GOCACHE:-$project_root/.gocache}" GOTMPDIR="${GOTMPDIR:-/private/tmp}" GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -a -buildvcs=false -trimpath \
  -ldflags="-s -w -H=windowsgui -buildid= -X main.version=$version -X main.buildFingerprint=$source_fingerprint -X main.riotAPIKeyCipher=$cipher" \
  -o desktop/backend/loot-service.exe .
node desktop/verify-build-fingerprint.cjs desktop/backend/loot-service.exe "$source_fingerprint"
if [[ "${SKIP_NPM_INSTALL:-0}" != "1" ]]; then
  (cd desktop && npm ci)
fi
# 临时只打便携版：pack:win-setup（NSIS 安装包）在部分本机环境下更容易失败，
# 一旦失败，脚本前面 `set -euo pipefail` 会让整条 && 链条直接中止——用户看到的
# 只是构建失败退出，dist/desktop 里可能连便携版都没留下，容易被误认成"构建没问题、
# 只是安装包缺了"。加这个开关后 SKIP_SETUP=1 只打便携版，问题排除后再去掉。
skip_setup="${SKIP_SETUP:-0}"
builder_args=()
if [[ -n "${ELECTRON_DIST:-}" ]]; then builder_args+=(--config.electronDist="$ELECTRON_DIST"); fi
(
  cd desktop
  node apply-portable-template.cjs >/dev/null
  export DEEP_LEGENDS_FINGERPRINT="$source_fingerprint"
  builder_args+=(--config.win.signAndEditExecutable=false --config.win.signExecutable=false)
  npm run pack:win -- "${builder_args[@]}"
  if [[ "$skip_setup" != "1" ]]; then
    npm run pack:win-setup -- "${builder_args[@]}"
  fi
  node verify-build-fingerprint.cjs ../dist/desktop/win-unpacked/resources/app.asar.unpacked/backend/loot-service.exe "$source_fingerprint"
  cd ../dist/desktop/win-unpacked
  zip -qrFS "../Deep Legends ${DEEP_LEGENDS_FINGERPRINT}.zip" .
  cd ..
  artifacts=("Deep Legends ${DEEP_LEGENDS_FINGERPRINT}.exe" "Deep Legends ${DEEP_LEGENDS_FINGERPRINT}.zip")
  if [[ "$skip_setup" != "1" ]]; then
    artifacts+=("Deep Legends Setup ${DEEP_LEGENDS_FINGERPRINT}.exe")
  fi
  shasum -a 256 "${artifacts[@]}" > SHA256SUMS.txt
)
if [[ "$skip_setup" == "1" ]]; then
  echo "SKIP_SETUP=1：本次只打了便携版（Deep Legends ${source_fingerprint}.exe），安装包未生成。" >&2
fi
