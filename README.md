# WAV & YouTube to WEM Converter — V12 Extended

V12 Extended keeps the proven V8 WAV -> WEM backend and adds YouTube discovery:
- Add a YouTube video link and look up the exact video.
- Search YouTube for a song and choose the correct result using thumbnail/title/channel/duration.
- After selection, upload the authorized WAV for that video and use the existing WEM conversion.

## Important YouTube limitation

The YouTube Data API can be used for search and video metadata, but YouTube's current Developer Policies prohibit API clients from allowing users to download or separate audio tracks from YouTube videos. This build therefore does NOT implement automatic YouTube audio downloading.

## Google Cloud / YouTube API setup

1. Create/select a Google Cloud project.
2. Enable **YouTube Data API v3**.
3. Create an API key.
4. In Render -> your service -> Environment, add:
   `YOUTUBE_API_KEY=<your-key>`
5. Redeploy the service.

The key remains on the server and is never placed in the HTML.

## Files

- `wav-to-wem.html` — replace your current site page.
- `backend/Dockerfile` — proven Wine 11 / wav2wem setup.
- `backend/main.go` — WEM API plus `/youtube/search` and `/youtube/video`.
- `backend/go.mod`
- `render.yaml`

## Render settings

Keep:
- Runtime: Docker
- Root Directory: `backend`
- Dockerfile Path: `Dockerfile`
- Docker Build Context: `.`
- Health Check Path: `/health`

## Endpoints

- `GET /health`
- `GET /diagnostics`
- `POST /convert`
- `GET /youtube/search?q=...`
- `GET /youtube/video?url=...`

## 200 MB WAV limit

The frontend and backend both use a 200 MB limit.

## Note

Search requests to YouTube's `search.list` endpoint consume API quota, so the UI intentionally searches only on user action instead of firing requests for every keystroke.
