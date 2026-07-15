#!/bin/sh
# Attach delve (copied to /tmp/dlv by scripts/watch-delve.sh ("Debug: delve watcher" task)) to the running plugin
# process, headless on :2345, so VS Code can attach remotely. Runs INSIDE the Grafana container.
#
# The plain Grafana container is already delve-ready (docker-compose-base.yaml grants SYS_PTRACE +
# unconfined seccomp/apparmor and publishes 2345), so no special image is needed — just the binary.
set -e

DLV=/tmp/dlv

# Pick the NEWEST plugin process (highest PID). Grafana runs one plugin process, but a stale one
# can linger after a reload; the live one is the most recently started. Attaching to the wrong
# (older) one is exactly why breakpoints don't hit.
PID=$(for d in /proc/[0-9]*; do
  grep -qa gpx_platform "$d/cmdline" 2>/dev/null && echo "${d#/proc/}"
done | sort -n | tail -1)

if [ -z "$PID" ]; then
  echo "plugin process (gpx_platform) not found — is Grafana up and the datasource added/queried?"
  exit 1
fi

echo "attaching delve to PID $PID, listening on :2345 (build with 'mage build:debug' for good stepping)"
exec "$DLV" attach "$PID" --headless --listen=:2345 --api-version=2 --accept-multiclient --continue
