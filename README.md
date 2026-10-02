# WAV / YouTube → WEM — V22 Beatstar-Fix

V22 is specifically for **Beatstar: Touch Your Music**.

V21 correctly normalized the source audio to 44.1 kHz / stereo / PCM16, but that only fixed the WAV side. The remaining mismatch was inside the generated Wwise Vorbis `.wem` header.

## Root cause addressed in V22

The `wav2wem` implementation used by the service writes the modern Wwise Vorbis `fmt` structure, but for stereo its channel-configuration/subtype field is left as `0`.

Beatstar music media uses the normal Wwise stereo channel configuration:

`0x3102` = stereo / 2.0

V22 therefore performs a strict Beatstar compatibility pass *after* wav2wem creates the WEM:

1. Verify RIFF/WAVE.
2. Verify Wwise Vorbis codec `0xFFFF`.
3. Verify modern `fmt` size `0x42`.
4. Verify 2 channels.
5. Verify 44,100 Hz.
6. Verify the Vorbis extended-format fields.
7. Patch the Wwise channel config to `0x3102`.
8. Re-read the value from disk and verify it.
9. Require a non-empty `data` chunk and non-zero sample count.

The patch changes only the Wwise stereo channel-layout field. It does not blindly copy file-specific hashes, offsets, sample counts, packet sizes, or loop metadata from another song.

## Audio pipeline

YouTube / uploaded WAV
→ FFmpeg clean PCM16 stereo 44.1 kHz
→ wav2wem / Wwise Vorbis
→ **Beatstar WEM header validation + stereo config 0x3102**
→ `.wem`

The working V20 YouTube downloader, cookies, Deno, PO-token provider and fallback routes are unchanged.

## Upgrade from V21

Replace the entire `backend` folder with V22:

- `.dockerignore`
- `Dockerfile`
- `entrypoint.sh`
- `go.mod`
- `main.go`

Keep your existing Render environment:
- `YOUTUBE_API_KEY`
- `YOUTUBE_USER_AGENT` if configured
- Secret File `youtube-cookies.txt`

Then use:

**Render → Manual Deploy → Clear build cache & deploy**

The included HTML only changes the displayed version label; the Beatstar playback fix is server-side.

## Diagnostics

`/diagnostics` should show:

- `version: v22-beatstar-fix`
- `beatstar_channel_config: 0x3102 (stereo)`
- `beatstar_wem_profile: Wwise Vorbis / fmt 0x42 / 2ch / 44100 Hz`
- the existing Wine, wav2wem, FFmpeg, yt-dlp, Deno and bgutil tests passing.
