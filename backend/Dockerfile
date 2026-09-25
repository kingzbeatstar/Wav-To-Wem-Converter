FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/wav-to-wem-api ./main.go

FROM debian:bookworm-slim
ENV DEBIAN_FRONTEND=noninteractive
ENV WINEDEBUG=-all
ENV HOME=/home/appuser
ENV WINEPREFIX=/home/appuser/.wine
ENV TMPDIR=/tmp

RUN dpkg --add-architecture i386 \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
       ca-certificates \
       curl \
       wine \
       wine64 \
       wine32 \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /opt/wav2wem /home/appuser/.wine \
    && curl -fsSL --retry 5 --retry-delay 2 \
       https://github.com/pas2k/wav2wem/releases/download/v0.1/wav2wem.exe \
       -o /opt/wav2wem/wav2wem.exe \
    && chmod 644 /opt/wav2wem/wav2wem.exe \
    && useradd --create-home --uid 10001 appuser \
    && chown -R appuser:appuser /opt/wav2wem /home/appuser

COPY --from=build /out/wav-to-wem-api /usr/local/bin/wav-to-wem-api

USER appuser
WORKDIR /home/appuser
EXPOSE 10000

ENTRYPOINT ["/usr/local/bin/wav-to-wem-api"]
