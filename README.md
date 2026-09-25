# WAV to WEM Converter — Render V4

This version fixes the specific `exec format error` seen in the previous deployments.

## Architecture

Linux API → patched `wav2wem` library → Wine → embedded `oggenc2.exe` → Wwise Vorbis `.wem`

The upstream `wav2wem` source launches its embedded `oggenc2.exe` with `os/exec`. On Linux that produces:

`fork/exec .../oggenc2.exe: exec format error`

V4 patches that one call so Linux launches the embedded encoder through Wine.

## Render layout

The GitHub repository should look like:

```text
repo/
├── render.yaml
└── backend/
    ├── Dockerfile
    ├── go.mod
    └── main.go
```

Render service settings:

- Runtime: Docker
- Root Directory: `backend`
- Dockerfile Path: `Dockerfile`
- Docker Build Context: `.`
- Health Check Path: `/health`
- Plan: Free

The `render.yaml` already contains these settings if Render is using the Blueprint.

## Test after deployment

1. Open `/health` — expect `{"ok":true}`.
2. Open `/diagnostics` — expect `wine_available: true`.
3. Test the existing website; it should continue posting to `/convert`.

For a failed conversion, the API now returns the underlying `wav2wem` error in JSON and logs it in Render.

## Notes

The upstream project documents modern Wwise Vorbis output and embeds `oggenc2.exe` plus its FLAC DLL. The build is pinned to the v0.1 source tag.
