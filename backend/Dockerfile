# Render runs Linux containers. wav2wem embeds a Windows oggenc2.exe,
# so the converter is built for Windows and run through Wine.
FROM golang:1.24-bookworm AS api-build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath -ldflags="-s -w" \
    -o /wav2wem-api ./main.go

FROM golang:1.24-bookworm AS wav2wem-build
RUN CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
    go install github.com/pas2k/wav2wem/cmd/wav2wem@v0.1

FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive \
    WINEDEBUG=-all \
    WINEPREFIX=/home/appuser/.wine \
    HOME=/home/appuser

RUN dpkg --add-architecture i386 \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        wine \
        wine32 \
        wine64 \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --uid 10001 appuser \
    && mkdir -p /home/appuser/.wine \
    && chown -R appuser:appuser /home/appuser

COPY --from=api-build /wav2wem-api /usr/local/bin/wav2wem-api
COPY --from=wav2wem-build /go/bin/wav2wem.exe /usr/local/bin/wav2wem.exe
COPY entrypoint.sh /usr/local/bin/entrypoint.sh

RUN chown appuser:appuser /usr/local/bin/wav2wem.exe /usr/local/bin/wav2wem-api /usr/local/bin/entrypoint.sh \
    && chmod 0755 /usr/local/bin/entrypoint.sh

USER appuser
EXPOSE 10000
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]

