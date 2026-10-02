# WAV / YouTube → WEM — V21 Audio-Fix

V21 keeps the working V20 YouTube download pipeline and fixes the audio handoff before `wav2wem`.

## Why V20 could make a WEM that is silent in Beatstar

YouTube commonly supplies 48 kHz Opus/AAC audio. Uploaded WAV files can also be float, 24/32-bit PCM, mono, multichannel, or use other layouts.

The converter could technically accept the WAV and produce a `.wem`, but that does not guarantee that Beatstar's expected playback path will like the resulting media profile.

The known-good Beatstar reference used during development is stereo at 44.1 kHz.

## V21 audio pipeline

Every input now goes through this exact normalization before WEM conversion:

YouTube / uploaded WAV
→ FFmpeg
→ **PCM signed 16-bit little-endian**
→ **stereo**
→ **44,100 Hz**
→ metadata stripped
→ ffprobe verification
→ silence check
→ Wine + wav2wem
→ WEM

This applies to BOTH:
- normal WAV → WEM
- YouTube → WAV → WEM

V21 does not change the working V20 YouTube client/cookie/PO-token fallback system.

## Upgrade

Replace your entire current `backend` folder with the V21 `backend` folder.

Keep your existing Render settings:
- `YOUTUBE_API_KEY`
- `YOUTUBE_USER_AGENT` if configured
- Secret File `youtube-cookies.txt`

Then:
**Render → Manual Deploy → Clear build cache & deploy**

The included HTML only updates the version label; the important fix is in `backend/main.go`.

## Diagnostics

After deploy, `/diagnostics` should show:
- `version: v21-audiofix`
- `ffmpeg_present: true`
- `ffprobe_present: true`
- the existing yt-dlp / Deno / bgutil / wav2wem checks passing

## Safety check

If FFmpeg somehow renders an all-silent WAV, V21 now stops with a clear error instead of generating a silent `.wem`.
