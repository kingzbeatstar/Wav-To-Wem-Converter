# WAV & YouTube to WEM Converter — V13 Extended

V13 Extended keeps the proven V8 Wine 11 / Windows wav2wem backend and fixes the YouTube API-key problem in the UI.

## What changed

- WAV -> WEM remains the same working pipeline.
- 200 MB WAV limit is enforced by the frontend and backend.
- YouTube direct-link lookup works without `YOUTUBE_API_KEY` by using YouTube's oEmbed metadata endpoint.
- YouTube search no longer fails when `YOUTUBE_API_KEY` is missing.
  - Without a key, it opens YouTube's official search page so the user can choose a video and paste its URL into the link tab.
  - With a key, the backend uses the official YouTube Data API for integrated result cards.
- Selected videos show an embedded YouTube preview.
- The WEM step still uses an uploaded/authorized WAV; V13 does not scrape YouTube or automatically download/separate audiovisual content.

## Optional integrated search

If you want the search results to appear inside the page instead of opening YouTube, create a YouTube Data API key and add it in Render as:

`YOUTUBE_API_KEY=<your key>`

The key is server-side and is not placed in the HTML.

## Render settings

- Runtime: Docker
- Root Directory: backend
- Dockerfile Path: Dockerfile
- Docker Build Context: .
- Health Check Path: /health

The Dockerfile is the working Wine 11 setup from V8.

## Website

Replace your current `wav-to-wem.html` with the V13 Extended file.

## YouTube policy note

YouTube's current developer policies prohibit scraping YouTube applications and require API clients to access YouTube API data only through documented YouTube API services. V13 therefore uses the official oEmbed/YouTube player for link/preview and the official Data API only when a key is configured.
