#!/bin/sh
# Run Grafana's plugin validator locally, the same checks the catalog review runs (osv-scanner,
# gosec, govulncheck, metadata, ...). Expects a fresh build in dist/ ("Build: all" task).
#
# The source code is passed as a copy of the working tree (tracked + untracked, minus gitignored
# files like node_modules and dist), so uncommitted changes are checked too, like a fresh clone
# of them would be.
set -e

cd "$(dirname "$0")/.."

if [ ! -f dist/plugin.json ]; then
  echo "dist/plugin.json not found: run the \"Build: all\" task first"
  exit 1
fi

PLUGIN_ID=$(jq -r .id dist/plugin.json)
VERSION=$(jq -r .info.version dist/plugin.json)
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

# Archive layout the catalog expects: <plugin-id>/... at the zip root.
cp -r dist "$WORK/$PLUGIN_ID"
(cd "$WORK" && zip -qr "$PLUGIN_ID-$VERSION.zip" "$PLUGIN_ID")

mkdir "$WORK/source"
git ls-files -z --cached --others --exclude-standard | xargs -0 cp --parents -t "$WORK/source"

docker run --pull=always --rm \
  -v "$WORK/$PLUGIN_ID-$VERSION.zip:/archive.zip" \
  -v "$WORK/source:/source_code" \
  grafana/plugin-validator-cli -sourceCodeUri file:///source_code /archive.zip
