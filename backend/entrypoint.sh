#!/bin/sh
set -eu

export HOME=/home/appuser
export WINEPREFIX=/home/appuser/.wine
export WINEARCH=win64
export WINEDEBUG=-all
export TMPDIR=/tmp

# Start the local Proof-of-Origin token provider used by yt-dlp's YouTube plugin.
cd /opt/bgutil-ytdlp-pot-provider/server
/opt/yt-dlp/deno run \
  --allow-env --allow-net --allow-ffi=. --allow-read=. \
  src/main.ts --port 4416 --host 127.0.0.1 \
  > /tmp/bgutil-provider.log 2>&1 &
BGUTIL_PID=$!

cleanup() {
  kill "$BGUTIL_PID" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# Wait for the provider to accept requests before starting the public API.
for i in $(seq 1 60); do
  if curl -fsS http://127.0.0.1:4416/ping >/dev/null 2>&1; then
    echo "BGUTIL POT PROVIDER READY"
    break
  fi
  sleep 1
done

if ! curl -fsS http://127.0.0.1:4416/ping >/dev/null 2>&1; then
  echo "BGUTIL POT PROVIDER FAILED TO START" >&2
  cat /tmp/bgutil-provider.log >&2 || true
  exit 1
fi

exec /usr/local/bin/wav-to-wem-api
