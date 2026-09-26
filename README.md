# WAV to WEM Render V7

This version fixes the Wine `/tmp` ownership issue from V6.

The important change is in `main.go`: inherited HOME/WINEPREFIX/TMPDIR values are removed before the correct values are added. This avoids duplicate environment variables causing Wine to continue seeing HOME=/tmp.

## Layout

repo/
- render.yaml
- backend/
  - Dockerfile
  - go.mod
  - main.go

## Render

Use:
- Runtime: Docker
- Root Directory: backend
- Dockerfile Path: Dockerfile
- Docker Build Context: .
- Health Check Path: /health

After pushing, use **Manual Deploy -> Clear build cache & deploy**.

## Expected startup logs

WINE SELF-TEST OK: wine-8.0
WINE PREFIX INIT OK
wav-to-wem API listening on :10000

If prefix initialization fails, the service exits instead of silently starting.

## Test

GET /health
GET /diagnostics

Then POST the WAV to /convert from the existing frontend.

The frontend does not need to change.
