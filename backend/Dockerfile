FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/wav-to-wem-api ./main.go

FROM debian:bookworm-slim
ENV DEBIAN_FRONTEND=noninteractive

RUN dpkg --add-architecture i386 \
    && apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl gnupg git ffmpeg unzip \
    && mkdir -p /etc/apt/keyrings \
    && curl -fsSL https://dl.winehq.org/wine-builds/winehq.key \
       | gpg --dearmor --yes -o /etc/apt/keyrings/winehq-archive.key \
    && curl -fsSL https://dl.winehq.org/wine-builds/debian/dists/bookworm/winehq-bookworm.sources \
       -o /etc/apt/sources.list.d/winehq-bookworm.sources \
    && apt-get update \
    && apt-get install -y --install-recommends winehq-stable \
    && rm -rf /var/lib/apt/lists/*

ARG YTDLP_VERSION=2026.09.16.232951
ARG DENO_VERSION=2.9.7
ARG BGUTIL_VERSION=2.0.0

RUN useradd --create-home --uid 10001 appuser \
    && usermod -a -G 1000 appuser \
    && mkdir -p /opt/wav2wem /opt/yt-dlp /opt/bgutil-ytdlp-pot-provider /home/appuser/.wine

# Separate network downloads so Render shows exactly which dependency failed.
# --retry-all-errors also retries transient DNS/connectivity failures.
RUN curl --ipv4 -fL --retry 10 --retry-all-errors --retry-delay 3 \
       https://github.com/pas2k/wav2wem/releases/download/v0.1/wav2wem.exe \
       -o /opt/wav2wem/wav2wem.exe \
    && chmod 755 /opt/wav2wem/wav2wem.exe

RUN curl --ipv4 -fL --retry 10 --retry-all-errors --retry-delay 3 \
       "https://github.com/yt-dlp/yt-dlp-nightly-builds/releases/download/${YTDLP_VERSION}/yt-dlp_linux" \
       -o /opt/yt-dlp/yt-dlp \
    && chmod 755 /opt/yt-dlp/yt-dlp

RUN curl --ipv4 -fL --retry 10 --retry-all-errors --retry-delay 3 \
       "https://github.com/denoland/deno/releases/download/v${DENO_VERSION}/deno-x86_64-unknown-linux-gnu.zip" \
       -o /tmp/deno.zip \
    && unzip -q /tmp/deno.zip -d /opt/yt-dlp \
    && rm -f /tmp/deno.zip

RUN git -c http.version=HTTP/1.1 clone --depth 1 --branch "${BGUTIL_VERSION}" \
       https://github.com/Brainicism/bgutil-ytdlp-pot-provider.git \
       /opt/bgutil-ytdlp-pot-provider \
    && cd /opt/bgutil-ytdlp-pot-provider/server \
    && /opt/yt-dlp/deno install --allow-scripts=npm:canvas --frozen \
    && mkdir -p /opt/yt-dlp/yt-dlp-plugins/bgutil-ytdlp-pot-provider \
    && cp -a /opt/bgutil-ytdlp-pot-provider/plugin/. /opt/yt-dlp/yt-dlp-plugins/bgutil-ytdlp-pot-provider/

RUN chown -R appuser:appuser \
       /opt/wav2wem /opt/yt-dlp /opt/bgutil-ytdlp-pot-provider /home/appuser

COPY --from=build /out/wav-to-wem-api /usr/local/bin/wav-to-wem-api
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod 755 /usr/local/bin/entrypoint.sh

ENV HOME=/home/appuser
ENV WINEPREFIX=/home/appuser/.wine
ENV WINEARCH=win64
ENV WINEDEBUG=-all
ENV TMPDIR=/tmp
ENV PATH=/opt/yt-dlp:$PATH

USER appuser
WORKDIR /home/appuser
EXPOSE 10000
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
