# WAV to WEM Render V9 — 200 MB upload limit

This package keeps the working V8 backend and raises the upload limit from 50 MB to 200 MB.

Updated:
- Backend: 200 MiB hard upload limit
- Frontend: 200 MB client-side validation and displayed limit
- Fixed the body allowance expression to `maxUpload + (1 << 20)`

## Repository layout

repo/
- render.yaml
- backend/
  - Dockerfile
  - go.mod
  - main.go
- wav-to-wem.html

## Render

Keep the same working service settings:
- Runtime: Docker
- Root Directory: backend
- Dockerfile Path: Dockerfile
- Docker Build Context: .
- Health Check Path: /health

After pushing, deploy the new backend. A cache clear is not normally necessary because only application source changes, but it is safe to use **Clear build cache & deploy** if you want a completely fresh image.

## Website

Replace the existing `wav-to-wem.html` with the included version.

The API endpoint does not change.

## Note

Render's current Free web service has 512 MB RAM. A 200 MB WAV can therefore be a heavy workload, depending on the WAV and Wine/converter memory usage. The 200 MB limit is supported by the application code, but very large files may still fail if the service runs out of memory.
