# WAV to WEM Render V10 — Beatstar Vorbis High test

V10 keeps the working V8 WineHQ + Windows wav2wem backend and the V9 200 MB limit, but changes the Vorbis VBR quality from `-q 4` to `-q 5`.

Evidence from the uploaded files:
- Source WAV: 16-bit PCM, stereo, 44.1 kHz.
- Source PCM frame count: 7,339,569.
- Generated WEM PCM frame field: 7,339,569 (exact match).
- Generated WEM average bitrate: 128 kbps.
- Known-good Beatstar WEM average bitrate: about 191.9 kbps.
- Beatstar conversion guides instruct using Wwise `Vorbis Quality High`.

This is a targeted compatibility test. It is not a claim that wav2wem quality 5 is byte-identical to Wwise's proprietary `Vorbis Quality High` preset.

Render settings:
- Runtime: Docker
- Root Directory: backend
- Dockerfile Path: Dockerfile
- Docker Build Context: .
- Health Check Path: /health

The included `wav-to-wem.html` is the 200 MB version and can replace the current InfinityFree page.
