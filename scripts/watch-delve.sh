#!/bin/sh
# "Debug always on": keep delve attached to the plugin process on :2345, re-attaching whenever the
# plugin (re)starts — e.g. after a reload restarts Grafana. Runs on the HOST; execs delve inside the
# Grafana container (which is delve-ready via docker-compose-base.yaml: SYS_PTRACE + unconfined +
# published :2345). VS Code attaches to :2345 (launch.json "Backend: attach to delve").
#
# A running debug session ends when the plugin process is replaced by a reload (it's a new process);
# just re-attach in VS Code — :2345 comes back automatically.
DLV="$(command -v dlv)"
if [ -z "$DLV" ]; then
  echo "delve (dlv) not found on PATH — it's baked into the dev container image; rebuild the container"
  exit 1
fi

echo "delve watcher started; keeping :2345 attached to the plugin (Ctrl+C to stop)"
while true; do
  # Ensure delve is present in the container (survives container recreation).
  if ! docker compose exec -T grafana test -f /tmp/dlv 2>/dev/null; then
    docker compose cp "$DLV" grafana:/tmp/dlv >/dev/null 2>&1 || { sleep 2; continue; }
  fi
  # Attach (blocks while the plugin runs); returns when the plugin exits → loop re-attaches.
  docker compose exec -T grafana sh /root/meshiq-platform-datasource/scripts/attach-delve.sh || true
  sleep 1
done
