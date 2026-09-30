FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/wav-to-wem-api ./main.go

FROM debian:bookworm-slim
ENV DEBIAN_FRONTEND=noninteractive

# WineHQ stable is used because wav2wem.exe is a Windows Go binary.
RUN dpkg --add-architecture i386 \
    && apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl gnupg \
    && mkdir -p /etc/apt/keyrings \
    && curl -fsSL https://dl.winehq.org/wine-builds/winehq.key \
       | gpg --dearmor --yes -o /etc/apt/keyrings/winehq-archive.key \
    && curl -fsSL https://dl.winehq.org/wine-builds/debian/dists/bookworm/winehq-bookworm.sources \
       -o /etc/apt/sources.list.d/winehq-bookworm.sources \
    && apt-get update \
    && apt-get install -y --install-recommends winehq-stable ffmpeg unzip \
    && rm -rf /var/lib/apt/lists/*

# yt-dlp currently requires an external JavaScript runtime for full YouTube support.
# Use the official standalone Linux binary plus Deno and the bundled EJS components.
ARG YTDLP_VERSION=2026.08.19
ARG DENO_VERSION=2.9.7
RUN useradd --create-home --uid 10001 appuser \
    && mkdir -p /opt/wav2wem /opt/yt-dlp /home/appuser/.wine \
    && curl -fsSL --retry 5 --retry-delay 2 \
       https://github.com/pas2k/wav2wem/releases/download/v0.1/wav2wem.exe \
       -o /opt/wav2wem/wav2wem.exe \
    && chmod 755 /opt/wav2wem/wav2wem.exe \
    && curl -fsSL --retry 5 --retry-delay 2 \
       https://github.com/yt-dlp/yt-dlp/releases/download/${YTDLP_VERSION}/yt-dlp_linux \
       -o /opt/yt-dlp/yt-dlp \
    && chmod 755 /opt/yt-dlp/yt-dlp \
    && curl -fsSL --retry 5 --retry-delay 2 \
       https://github.com/denoland/deno/releases/download/v${DENO_VERSION}/deno-x86_64-unknown-linux-gnu.zip \
       -o /tmp/deno.zip \
    && unzip -q /tmp/deno.zip -d /opt/yt-dlp \
    && rm -f /tmp/deno.zip \
    && rm -rf /var/lib/apt/lists/* \
    && chown -R appuser:appuser /opt/wav2wem /opt/yt-dlp /home/appuser

COPY --from=build /out/wav-to-wem-api /usr/local/bin/wav-to-wem-api

ENV HOME=/home/appuser
ENV WINEPREFIX=/home/appuser/.wine
ENV WINEARCH=win64
ENV WINEDEBUG=-all
ENV TMPDIR=/tmp
ENV PATH=/opt/yt-dlp:$PATH

USER appuser
WORKDIR /home/appuser
EXPOSE 10000
ENTRYPOINT ["/usr/local/bin/wav-to-wem-api"]
