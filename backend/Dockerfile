# Build a Linux API that uses the upstream wav2wem library, but patches only
# its internal oggenc2 launch so Linux invokes the embedded Windows encoder
# through Wine.

FROM golang:1.24-bookworm AS build

RUN apt-get update \
    && apt-get install -y --no-install-recommends git python3 ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Pin the source to the upstream v0.1 release instead of building whatever
# happens to be on main today.
RUN git clone --depth 1 --branch v0.1 https://github.com/pas2k/wav2wem.git /opt/wav2wem

# IMPORTANT: upstream encoder.go uses os/exec to launch the embedded
# oggenc2.exe directly. On Linux that produces "exec format error".
# Change ONLY that launch to: wine <embedded-oggenc2.exe> <args...>
RUN python3 - <<'PY'
from pathlib import Path
p = Path('/opt/wav2wem/encoder.go')
s = p.read_text()
old = 'cmd := exec.Command(exe, args...)'
new = 'cmd := exec.Command("wine", append([]string{exe}, args...)...)'
if old not in s:
    raise SystemExit('Expected wav2wem encoder.go line was not found; refusing to build.')
p.write_text(s.replace(old, new, 1))
PY

# Build the API against the patched local wav2wem source.
COPY go.mod main.go ./
RUN go mod edit -replace=github.com/pas2k/wav2wem=/opt/wav2wem \
    && go mod tidy \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/wav-to-wem-api ./main.go

FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive
ENV WINEDEBUG=-all
ENV WINEPREFIX=/tmp/wineprefix

RUN dpkg --add-architecture i386 \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
       ca-certificates \
       wine \
       wine32 \
       wine64 \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --uid 10001 appuser \
    && mkdir -p /tmp/wineprefix \
    && chown -R appuser:appuser /tmp/wineprefix

COPY --from=build /out/wav-to-wem-api /usr/local/bin/wav-to-wem-api

USER appuser
WORKDIR /home/appuser

EXPOSE 10000
ENTRYPOINT ["/usr/local/bin/wav-to-wem-api"]
