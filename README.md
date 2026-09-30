# WAV & YouTube to WEM — V14 Extended

V14 addresses the three reported YouTube problems:

- Search no longer redirects to YouTube. It calls the Render backend and renders results in the page.
- Search results are filtered to videos that allow embedding when the YouTube Data API key is configured.
- The embedded player uses YouTube's documented embed URL with `enablejsapi=1`, `playsinline=1`, and the current page origin. A fallback link appears if a video cannot be embedded.
- Direct-link lookup does not require the Data API key; it uses YouTube oEmbed metadata.
- WAV -> WEM remains the proven 200 MB Wine 11 / wav2wem pipeline.

## IMPORTANT: enable integrated search

For search to return actual YouTube results inside your page, you need one Render environment variable:

`YOUTUBE_API_KEY=YOUR_GOOGLE_CLOUD_API_KEY`

In Google Cloud, enable **YouTube Data API v3** for the project, create an API key, then in Render:

1. Open `wav-to-wem-converter`.
2. Open **Environment**.
3. Add `YOUTUBE_API_KEY` and paste the key as its value.
4. Save the environment changes.
5. Deploy the latest commit.

The key stays server-side. It is never placed in the HTML.

V14 does not intentionally fall back to an external YouTube search page; without the key it shows a configuration message on the same page instead.

## Direct YouTube link

Paste a normal `youtube.com/watch?v=...`, `youtu.be/...`, `/shorts/...`, `/embed/...`, or `/live/...` URL. The page resolves metadata without leaving the site, then embeds the video.

## Video playback

Some YouTube videos cannot be embedded by their owners. Those can still be identified through the link workflow, but the player may refuse playback. Integrated search filters for embeddable videos using the Data API.

## About the WAV -> WEM step

V14 keeps the working conversion pipeline. Selecting a YouTube video identifies the track; the conversion step accepts a WAV you are authorized to use and produces WEM.

## Render settings

- Runtime: Docker
- Root Directory: backend
- Dockerfile Path: Dockerfile
- Docker Build Context: .
- Health Check Path: /health
- Plan: free

## Website

Replace your current `wav-to-wem.html` with the V14 version.
