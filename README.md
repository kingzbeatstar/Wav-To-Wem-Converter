# WAV & YouTube to WEM — V16 Extended

V16 keeps the working V15 YouTube -> WAV -> WEM pipeline, but fixes the current YouTube server-side download failure by adding the current yt-dlp Proof-of-Origin token provider and a newer yt-dlp nightly build.

## Pipeline

YouTube link/search -> select video -> download authorized audio -> WAV -> wav2wem/Wine 11 -> WEM -> automatic download.

There is no user-WAV upload in YouTube mode.

## What changed from V15

- yt-dlp nightly 2026.09.16.232951 instead of the older stable binary.
- bgutil-ytdlp-pot-provider 2.0.0 is installed as a yt-dlp plugin.
- The provider runs locally inside the same Render container on 127.0.0.1:4416.
- yt-dlp is told to use the embeddable client first and mweb as a token-backed fallback.
- The provider URL is explicitly supplied to yt-dlp.
- Startup performs a provider self-test and the API diagnostics endpoint reports it.
- The 200 MB WAV upload path and proven WAV -> WEM path remain unchanged.

The yt-dlp project currently documents that YouTube may require Proof-of-Origin tokens and recommends a PO Token provider plugin. The current provider project's 2.0.0 release is the latest release and includes security fixes; it binds locally by default. See the sources below.

## Render settings

Runtime: Docker
Root Directory: backend
Dockerfile Path: Dockerfile
Docker Build Context: .
Health Check Path: /health

After pushing V16, use **Manual Deploy -> Clear build cache & deploy** because the image now includes new yt-dlp/plugin/provider layers.

## Environment

Keep:
`YOUTUBE_API_KEY=...`

No new secret is required for the PO-token provider.

## Diagnostics

After deploy:
`https://wav-to-wem-converter.onrender.com/diagnostics`

Look for:
- version: v16-extended
- yt_dlp_self_test: true
- deno_self_test: true
- bgutil_self_test: true
- wav2wem_self_test: true
- youtube_api_configured: true

## YouTube usage

Use this conversion path only for audio/video you are authorized to download and use. The provider and yt-dlp do not grant permission to download content.

Providing a PO token does not guarantee that YouTube will allow a particular server-side request; the upstream provider explicitly warns that it may not eliminate 403s or bot checks. If YouTube blocks a request on the Render IP, V16 reports the failure rather than attempting to defeat additional access controls.

## Files

- wav-to-wem.html — replace the current website page
- backend/Dockerfile — replace current Dockerfile
- backend/entrypoint.sh — add this new file
- backend/main.go — replace current backend
- backend/go.mod
- render.yaml
