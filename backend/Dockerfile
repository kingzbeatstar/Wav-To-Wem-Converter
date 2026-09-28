FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/wav-to-wem-api ./main.go

FROM debian:bookworm-slim
ENV DEBIAN_FRONTEND=noninteractive

# Wine 8.0 from Debian Bookworm is too old for the Windows Go runtime
# used by wav2wem.exe v0.1. Wine 8.13 added bcryptprimitives/ProcessPrng
# support, so use the current WineHQ stable package instead.
RUN dpkg --add-architecture i386 \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
       ca-certificates \
       curl \
       gnupg \
    && mkdir -p /etc/apt/keyrings \
    && curl -fsSL https://dl.winehq.org/wine-builds/winehq.key \
       | gpg --dearmor --yes -o /etc/apt/keyrings/winehq-archive.key \
    && curl -fsSL https://dl.winehq.org/wine-builds/debian/dists/bookworm/winehq-bookworm.sources \
       -o /etc/apt/sources.list.d/winehq-bookworm.sources \
    && apt-get update \
    && apt-get install -y --install-recommends winehq-stable \
    && rm -rf /var/lib/apt/lists/*

RUN useradd --create-home --uid 10001 appuser \
    && mkdir -p /opt/wav2wem /home/appuser/.wine \
    && curl -fsSL --retry 5 --retry-delay 2 \
       https://github.com/pas2k/wav2wem/releases/download/v0.1/wav2wem.exe \
       -o /opt/wav2wem/wav2wem.exe \
    && chmod 755 /opt/wav2wem/wav2wem.exe \
    && chown -R appuser:appuser /opt/wav2wem /home/appuser

COPY --from=build /out/wav-to-wem-api /usr/local/bin/wav-to-wem-api

ENV HOME=/home/appuser
ENV WINEPREFIX=/home/appuser/.wine
ENV WINEARCH=win64
ENV WINEDEBUG=-all
ENV TMPDIR=/tmp

USER appuser
WORKDIR /home/appuser
EXPOSE 10000
ENTRYPOINT ["/usr/local/bin/wav-to-wem-api"]
