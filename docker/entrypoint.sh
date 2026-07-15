#!/bin/sh
# Container startup wrapper — runs INSIDE the Grafana container (bind-mounted read-only and set as the
# entrypoint by docker-compose.yaml). It does two dev-only prep steps, then hands off to the image's
# real entrypoint (/entrypoint.sh). Kept as a file (not inline YAML) so it stays readable and commented.
set -e

# 1. Fresh logs each start. GF_LOG_MODE writes to /var/log/grafana (bind-mounted to ./logs on the
#    host). Move any existing grafana* files into archive/ so each run starts with a clean grafana.log
#    and we never inherit a half-closed file handle from a crashed run ("FileLogWriter: file already
#    closed"). Rotation is disabled (GF_LOG_FILE_LOG_ROTATE=false) so this is the only log manager.
if [ -d /var/log/grafana ]; then
  mkdir -p /var/log/grafana/archive
  find /var/log/grafana -maxdepth 1 -type f -name "grafana*" -exec mv {} /var/log/grafana/archive/ \;
fi

# 2. Make the livereload <script> non-blocking. The managed .config/Dockerfile injects
#    `<script src="http://localhost:35729/livereload.js">` at end-of-body as a plain (parser-blocking)
#    tag. Grafana's own app bundles load with `defer`, so they run only after parsing finishes — but a
#    blocking script that never resolves (nothing listens on 35729 unless the webpack watcher is up, and
#    a forwarded-but-dead 35729 tunnel hangs instead of rejecting) stalls the parser forever → the app
#    never boots → endless bouncing logo. Adding `defer` lets parsing finish regardless, so Grafana boots
#    in every mode; livereload still connects and auto-reloads when the watcher is running. Idempotent:
#    a no-op once already patched, and never fatal (|| true) so a template change can't block startup.
INDEX_HTML=/usr/share/grafana/public/views/index.html
if [ -f "$INDEX_HTML" ]; then
  sed -i 's|<script src="http://localhost:35729/livereload.js">|<script defer src="http://localhost:35729/livereload.js">|' "$INDEX_HTML" || true
fi

exec /entrypoint.sh
