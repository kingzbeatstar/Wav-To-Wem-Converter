# WAV & YouTube to WEM — V15 Extended

V15 changes the YouTube workflow to the requested one-click pipeline:

YouTube link/search -> download audio -> create WAV -> convert WAV -> WEM -> download WEM

There is **no user WAV upload step in the YouTube mode**.

## What changed

- Keeps the working WAV -> WEM endpoint and 200 MB WAV limit.
- Adds `/youtube/convert` to start a server-side conversion job.
- Adds `/youtube/job?id=...` for progress polling.
- Adds `/youtube/download?id=...` for the finished WEM.
- Uses `yt-dlp` to download the selected video's best available audio and FFmpeg to convert it to WAV.
- Sends the generated WAV through the existing `wav2wem.exe` + Wine 11 WEM pipeline.
- Runs YouTube work asynchronously so the browser is not stuck on one long HTTP request.
- Frontend shows stages: download/extract audio -> WAV ready -> WEM conversion -> complete.
- Search remains in-page using the existing YouTube Data API key.
- Direct YouTube links still work using oEmbed metadata and do not require a Data API key.
- The selected video remains previewable in the embedded YouTube player.

## Current downloader runtime

The container pins:
- yt-dlp 2026.08.19
- Deno 2.9.7

Current yt-dlp documentation says full YouTube support requires a supported external JavaScript runtime and yt-dlp's EJS components; Deno is the recommended runtime. See the official yt-dlp EJS documentation.

## Render

Keep:
- Runtime: Docker
- Root Directory: backend
- Dockerfile Path: Dockerfile
- Docker Build Context: .
- Health Check Path: /health
- Plan: free

After pushing V15, use **Manual Deploy -> Clear build cache & deploy** so the new yt-dlp/Deno dependencies are definitely installed.

## Environment

Keep your existing:

`YOUTUBE_API_KEY=...`

No additional YouTube environment variable is required by V15.

## Website

Replace the current `wav-to-wem.html` with the V15 Extended file.

The frontend continues to call:

`https://wav-to-wem-converter.onrender.com`

## YouTube flow

1. Open YouTube -> WEM.
2. Paste a YouTube link OR search for the song.
3. Pick the exact video.
4. Preview it on the page.
5. Check the authorization confirmation.
6. Click **Convert to WAV + WEM**.
7. V15 downloads the audio server-side, creates a WAV, converts the WAV to WEM, and automatically downloads the WEM.

## Important usage note

Only use the YouTube conversion path for audio/video you are authorized to download and use. YouTube's Terms restrict downloading and automated access except where specifically permitted, by permission, or by applicable law. Using the official YouTube Data API for search also does not grant download rights.

V15 does not bypass DRM or other access controls. If YouTube refuses a server-side download, the job reports the failure instead of attempting to circumvent the restriction.

## Diagnostics

After deploy:

`https://wav-to-wem-converter.onrender.com/diagnostics`

Look for:
- `version: v15-extended`
- `yt_dlp_self_test: true`
- `deno_self_test: true`
- `wav2wem_self_test: true`
- `youtube_api_configured: true`

