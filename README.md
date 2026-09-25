# WAV to WEM Render Backend V6

This version fixes the Wine failure seen in V5:

`wine: '/tmp' is not owned by you, refusing to create a configuration directory there`

V6 forces Wine to use an app-owned HOME and WINEPREFIX:

- `HOME=/home/appuser`
- `WINEPREFIX=/home/appuser/.wine`
- `TMPDIR=/tmp`

The API runs the official Windows `wav2wem.exe` v0.1 through Wine.

## Repo layout

```text
repo/
├── render.yaml
└── backend/
    ├── Dockerfile
    ├── go.mod
    └── main.go
```

## Render settings

- Runtime: Docker
- Root Directory: `backend`
- Dockerfile Path: `Dockerfile`
- Docker Build Context: `.`
- Health Check Path: `/health`

After pushing V6, use **Manual Deploy → Clear build cache & deploy**.

## Useful endpoints

- `/` — service/version information
- `/health` — health check
- `/diagnostics` — Wine/converter information
- `/convert` — multipart field `file`
