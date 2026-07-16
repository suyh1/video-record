FROM node:26.5.0-bookworm-slim AS web-build

WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN VITE_EMBED_OUT_DIR=/out npm run build

FROM golang:1.26.5-bookworm AS go-build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY --from=web-build /out/ ./internal/assets/dist/
RUN test -f internal/assets/dist/index.html && \
    CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" \
    go build -tags production -trimpath -ldflags='-s -w -buildid=' -o /out/video-record ./cmd/server
RUN mkdir -p /out/data && chmod 0700 /out/data && chown 65532:65532 /out/data

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=go-build /out/video-record /video-record
COPY --from=go-build --chown=65532:65532 /out/data/ /data/

ENV APP_ENV=production \
    APP_PORT=8080 \
    DATA_DIR=/data

USER 65532:65532
EXPOSE 8080
VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/video-record", "healthcheck"]

ENTRYPOINT ["/video-record"]
