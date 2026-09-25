FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod main.go ./
RUN go install github.com/pas2k/wav2wem/cmd/wav2wem@latest
RUN go build -trimpath -ldflags="-s -w" -o /wav2wem-api ./main.go

FROM debian:bookworm-slim
RUN useradd --create-home --uid 10001 appuser
COPY --from=build /go/bin/wav2wem /usr/local/bin/wav2wem
COPY --from=build /wav2wem-api /usr/local/bin/wav2wem-api
USER appuser
EXPOSE 10000
ENTRYPOINT ["/usr/local/bin/wav2wem-api"]
