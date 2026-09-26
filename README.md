# WAV to WEM Render V8

V8 fixes the latest failure:
`fatal error: bcryptprimitives.dll not found`

Cause:
The Windows wav2wem.exe v0.1 binary is a Go Windows executable whose runtime requires bcryptprimitives/ProcessPrng. Debian Bookworm's Wine 8.0 is too old for that API. Wine 8.13 added a bcryptprimitives ProcessPrng stub, and WineHQ stable 11.0 is used here.

V8 installs WineHQ stable instead of Debian's Wine 8.0 package.

## Repository layout

repo/
- render.yaml
- backend/
  - Dockerfile
  - go.mod
  - main.go

## Render service settings

Runtime: Docker
Root Directory: backend
Dockerfile Path: Dockerfile
Docker Build Context: .
Health Check Path: /health

After pushing V8, use:
Manual Deploy -> Clear build cache & deploy

## Expected startup logs

WINE SELF-TEST OK: wine-11.x...
WINE PREFIX INIT OK
WAV2WEM SELF-TEST OK: Windows binary starts under Wine
wav-to-wem API listening on :10000

The WAV2WEM self-test is deliberate: it actually starts wav2wem.exe, so a missing bcryptprimitives.dll is detected before any user upload.

## Test endpoints

GET /
GET /health
GET /diagnostics

The existing frontend does not need to change.
