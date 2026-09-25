# WAV to WEM Render Backend V5

This version deliberately does NOT build wav2wem from Go source.

It downloads the upstream Windows `wav2wem.exe` v0.1 release and runs that complete Windows program under Wine. This lets wav2wem launch its embedded `oggenc2.exe` using the Windows process model instead of Linux trying to execute a `.exe` directly.

## Repository layout

```text
repo/
├── render.yaml
└── backend/
    ├── Dockerfile
    ├── go.mod
    └── main.go
```

## Render settings

Use:

- Runtime: Docker
- Root Directory: `backend`
- Dockerfile Path: `Dockerfile`
- Docker Build Context: `.`
- Health Check Path: `/health`

## Test

After deployment:

- `/health` should return `{"ok":true}`
- `/diagnostics` should show `converter_present: true` and `wine_available: true`
- POST a WAV as multipart field `file` to `/convert`

The API converts at quality `-q 4` and returns `<input-name>.wem`.

The Windows converter is downloaded during the Docker build from the upstream v0.1 release:
https://github.com/pas2k/wav2wem/releases/download/v0.1/wav2wem.exe
