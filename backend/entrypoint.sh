#!/bin/sh
set -eu

# Initialize the Wine prefix once when the service starts.
# The converter itself is then launched by the Go API for each request.
wineboot --init >/dev/null 2>&1 || true

exec /usr/local/bin/wav2wem-api
