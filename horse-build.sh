#!/usr/bin/env bash
# Build the horse branch and put it inside /Applications/Neru.app (the cask's
# bundle), signed with bekh's Apple Development cert so the Accessibility
# grant survives rebuilds (an ad hoc signature pins it to the binary's hash).
set -euo pipefail
cd "$(dirname "$0")"

app=/Applications/Neru.app
id=com.y3owk1n.neru
cert="Apple Development: n3xstranger@gmail.com (6M637ZXCXR)"
version=$(git describe --tags --always --dirty)-horse
pkg=github.com/y3owk1n/neru/internal/buildinfo

CGO_ENABLED=1 go build -trimpath \
  -ldflags="-s -w -X $pkg.Version=$version -X $pkg.GitCommit=$(git rev-parse --short HEAD) -X $pkg.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o bin/neru ./cmd/neru

running=0
pgrep -x neru >/dev/null && running=1
[[ $running == 1 ]] && { osascript -e 'tell application "Neru" to quit' 2>/dev/null || true; sleep 1; pkill -x neru || true; }

cp bin/neru "$app/Contents/MacOS/neru"
codesign -f -s "$cert" --identifier "$id" "$app"
codesign -v "$app" && echo "signed: $version"

[[ $running == 1 ]] && open -a Neru
