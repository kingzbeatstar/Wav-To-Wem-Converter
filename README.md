# WAV to WEM — Render V3

This version runs the official Windows `wav2wem.exe` release through Wine inside the Linux Render container. The whole Windows converter runs under Wine, so its bundled `oggenc2.exe` is also executed by Wine instead of Linux trying to execute the `.exe` directly.

## Deploy

1. Replace your backend repository contents with this `backend/` folder and `render.yaml`.
2. Commit and push to the GitHub branch connected to Render.
3. Render service settings:
   - Runtime / Language: Docker
   - Root Directory: `backend`
   - Dockerfile Path: `Dockerfile`
   - Docker Build Context: `.`
   - Health Check Path: `/health`
4. Deploy/redeploy.
5. Open `https://YOUR-SERVICE.onrender.com/health` and confirm `{"ok":true}`.
6. Keep your website's API URL pointed at `https://YOUR-SERVICE.onrender.com/convert`.
7. Test `Wont Stop Sync.wav`.

The Docker build downloads `wav2wem.exe` v0.1 from the upstream GitHub release during the Render build.

## If conversion fails

Open Render → service → Logs and copy the lines beginning with:

`wav2wem command output:`

or

`wav2wem error:`

The API also returns a JSON `detail` field on conversion failure so the frontend can expose the real error while testing.
