# WAV to WEM Render V11 — Beatstar/Wwise compatibility profile

V11 keeps the working WineHQ + Windows `wav2wem.exe` backend and the 200 MB upload limit, but applies a targeted Beatstar/Wwise output profile.

## V11 changes

- Windows `wav2wem.exe` v0.1 runs through WineHQ stable.
- Vorbis quality changed to `-q 6`.
- Post-processing applies the observed values from the known-good Beatstar reference WEM:
  - stereo layout: `0x00003102`
  - decode allocation: `16080`
  - x64 decode allocation: `16560`
  - codebook UID: `0xD54BA8E8`
  - nominal average bytes/sec: `23992` (~192 kbps)
- Upload limit remains 200 MiB.

These header values are based on the supplied known-good Beatstar WEM. They are a compatibility test and are not claimed to be byte-identical to proprietary Wwise authoring output.

## Repository layout

repo/
- `render.yaml`
- `backend/`
  - `Dockerfile`
  - `go.mod`
  - `main.go`
- `wav-to-wem.html`

## Render settings

- Runtime: Docker
- Root Directory: `backend`
- Dockerfile Path: `Dockerfile`
- Docker Build Context: `.`
- Health Check Path: `/health`

After pushing V11, deploy the backend. Using **Manual Deploy -> Clear build cache & deploy** is safe if you want a completely fresh image.

## Expected startup / diagnostics

`/health` should report version `v11`.

`/diagnostics` should report the V11 converter and Beatstar profile values, including Vorbis quality `6`.

## Website

Replace the existing `wav-to-wem.html` with the included version. The API endpoint does not change.
