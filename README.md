# WAV / YouTube → WEM — V20 Final

V20 fixes the `Requested format is not available` / `YouTube did not provide a usable audio format` failure seen in V19.

## What changed

V19 forced:
`youtube:player-client=web_embedded,mweb`

V20 removes `web_embedded` from the download route and uses an audio-specific format selector:
`bestaudio/best`

The backend now tries these routes automatically, only falling back when the previous route fails with a format/403 compatibility error:

1. `default,mweb`
2. `mweb`
3. `web_safari`
4. yt-dlp automatic/default selection

The PO-token provider, Deno, browser cookies, request pacing, FFmpeg, Wine 11 and wav2wem pipeline remain enabled.

V20 also checks whether a valid WAV was actually produced before throwing away an otherwise usable result.

## Upgrade

Replace the entire `backend` folder from V19 with the V20 `backend` folder:

- `.dockerignore`
- `Dockerfile`
- `entrypoint.sh`
- `go.mod`
- `main.go`

You do **not** have to recreate your Render variables.

Keep:
- `YOUTUBE_API_KEY`
- `YOUTUBE_USER_AGENT` if you use it
- Render Secret File `youtube-cookies.txt`

Then:
1. Commit/push V20.
2. Render -> Manual Deploy.
3. Choose **Clear build cache & deploy**.

The website HTML does not require a functional change for this fix, but the V20 copy is included.

## Diagnostics

After deploy, `/diagnostics` should report:
- version: `v20-final`
- YouTube API configured: true
- YouTube cookies present: true
- yt-dlp self test: true
- Deno self test: true
- bgutil self test: true
- wav2wem self test: true

## Pipeline

YouTube link/search
→ best available audio stream
→ compatibility fallback only when needed
→ FFmpeg / yt-dlp WAV extraction
→ Wine + wav2wem
→ WEM download

Use only media you are permitted to process.
