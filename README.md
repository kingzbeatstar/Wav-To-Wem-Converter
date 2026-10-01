# WAV & YouTube to WEM — V19 Group-Fix

V17 addresses the specific YouTube error:

`Sign in to confirm you're not a bot. Use --cookies-from-browser or --cookies`

## What changed

- Keeps the V16 PO-token provider, Deno, FFmpeg, Wine 11 and wav2wem pipeline.
- Adds optional authenticated YouTube cookies from a **Render Secret File**.
- Adds gentler yt-dlp request pacing to reduce rate-limit pressure.
- Never sends cookie contents to the browser.
- Never writes cookie contents to application logs.
- Adds `youtube_cookies_present` to `/diagnostics`.
- Adds `.dockerignore` rules so cookie files are not accidentally committed/copied into the image.
- Adds the app user to group 1000 so Render Docker services can read runtime secret files.

## Important reality

YouTube can challenge datacenter IP addresses such as Render. There is no code-only flag that guarantees anonymous server-side downloading will work indefinitely.

V17 supports the authenticated-session method recommended by yt-dlp for the exact bot-confirmation error, but YouTube can still expire cookies, rate-limit the account/IP, or refuse a stream.

## Render setup

Keep your existing:
`YOUTUBE_API_KEY`

Then add a Render Secret File:

1. Render dashboard -> `wav-to-wem-converter`
2. Environment
3. Secret Files -> Add Secret File
4. Filename: `youtube-cookies.txt`
5. Paste a fresh Netscape-format cookie file
6. Save Changes / redeploy

At runtime Render exposes it at:
`/etc/secrets/youtube-cookies.txt`

The V17 backend automatically detects and uses it.

### Cookie safety

Cookies are account credentials.

- Do **not** commit the cookie file to GitHub.
- Do **not** upload it to InfinityFree.
- Do **not** paste it into ChatGPT or public logs.
- Prefer a separate browser profile / dedicated YouTube account rather than your primary Google account.
- yt-dlp warns that using an account can lead to temporary or permanent account restrictions.
- Refresh the secret if YouTube expires the session.

If you use a matching browser User-Agent, optionally add this Render environment variable:

`YOUTUBE_USER_AGENT=<the user agent from the browser profile that produced the cookies>`

This is optional.

## Deploy

Replace:
- `backend/main.go`
- `backend/Dockerfile`

Add:
- `backend/.dockerignore`

You can also replace `wav-to-wem.html` for the V17 version label.

Keep:
- `backend/entrypoint.sh`
- `backend/go.mod`
- `render.yaml`

Then use:
**Manual Deploy -> Clear build cache & deploy**

## Diagnostics

Open:
`https://wav-to-wem-converter.onrender.com/diagnostics`

Look for:
- `"version": "v19-groupfix"`
- `"youtube_api_configured": true`
- `"youtube_cookies_present": true`
- `"bgutil_self_test": true`
- `"yt_dlp_self_test": true`
- `"wav2wem_self_test": true`

## Pipeline

YouTube link/search
-> server-side authenticated yt-dlp session when cookie secret is present
-> FFmpeg WAV
-> Wine + wav2wem
-> WEM download


## V18 Render build fix

This version addresses curl exit code 6 during the Render Docker build.

Changes:
- External downloads are separate Docker layers.
- curl uses IPv4 and retries transient failures up to 10 times.
- `--retry-all-errors` covers transient DNS/connectivity failures.
- GitHub cloning uses HTTP/1.1.
- Existing V17 YouTube cookies, PO-token provider, Deno, FFmpeg, Wine and WEM conversion behavior are preserved.

### Upgrade
Replace the entire `backend` folder with V18. Keep your existing Render environment variables and `youtube-cookies.txt` Secret File. Then choose **Manual Deploy -> Clear build cache & deploy**.


## V19 group fix

V18's build failure was caused by:

`usermod -a -G 1000 appuser`

`usermod` requires the target group to exist. Some Docker base images do not contain a group with GID 1000, so it exits with code 6.

V19 now:
1. Checks whether GID 1000 already exists.
2. Reuses its real group name if present.
3. Otherwise creates `rendersecrets` with GID 1000.
4. Adds `appuser` to that valid group.

All V18 network retry fixes and V17 YouTube cookie/token support remain unchanged.

Upgrade: replace the entire `backend` folder, push to GitHub, then on Render use **Manual Deploy -> Clear build cache & deploy**.
