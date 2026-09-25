FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive
ENV WINEDEBUG=-all
ENV WINEPREFIX=/tmp/wineprefix

# Wine is used for the Windows wav2wem release. i386 support is installed
# because wav2wem's bundled oggenc2.exe may be 32-bit.
RUN dpkg --add-architecture i386 \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
       ca-certificates \
       curl \
       wine \
       wine32 \
       wine64 \
    && rm -rf /var/lib/apt/lists/*

# Official wav2wem Windows release (v0.1), as documented by the project.
RUN mkdir -p /opt/wav2wem \
    && curl -fL --retry 4 --retry-delay 2 \
       -o /opt/wav2wem/wav2wem.exe \
       https://github.com/pas2k/wav2wem/releases/download/v0.1/wav2wem.exe \
    && chmod 755 /opt/wav2wem/wav2wem.exe

# Initialize Wine once during image build so the first user request doesn't
# have to do setup. Ignore harmless Wine initialization output.
RUN mkdir -p /tmp/wineprefix \
    && wineboot --init || true

COPY --from=golang:1.24-bookworm /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}"

WORKDIR /app
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /usr/local/bin/wav2wem-api ./main.go

RUN useradd --create-home --uid 10001 appuser \
    && mkdir -p /tmp/wineprefix \
    && chown -R appuser:appuser /tmp/wineprefix /opt/wav2wem /app
USER appuser

EXPOSE 10000
ENTRYPOINT ["/usr/local/bin/wav2wem-api"]
