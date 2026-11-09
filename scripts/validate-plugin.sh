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

# The build manifest lists every .go file under the plugin folder, gitignored ones too (only
# node_modules is skipped), but the source copy leaves gitignored files out. A stray .go file (for
# example in a leftover npx cache) fails the validator's go-manifest check, so catch it here.
MISSING=$(cut -d: -f2- dist/go_plugin_build_manifest | while read -r f; do
  [ -f "$WORK/source/$f" ] || echo "  $f"
done)
if [ -n "$MISSING" ]; then
  echo "dist/go_plugin_build_manifest lists files that are not in the source code:"
  echo "$MISSING"
  echo "Remove them (or the folder they are in) and run the \"Build: all\" task again."
  exit 1
fi

docker run --pull=always --rm \
  -v "$WORK/$PLUGIN_ID-$VERSION.zip:/archive.zip" \
  -v "$WORK/source:/source_code" \
  grafana/plugin-validator-cli -sourceCodeUri file:///source_code /archive.zip
