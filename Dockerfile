# syntax=docker/dockerfile:1

FROM node:24-bookworm-slim AS web-builder
WORKDIR /src

COPY web/package.json web/package-lock.json ./web/
RUN cd web && npm ci

COPY web ./web
COPY internal/web ./internal/web
RUN cd web && npm run build

FROM golang:1.25.12-bookworm AS go-builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web-builder /src/internal/web/dist ./internal/web/dist

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
	-ldflags="-s -w -X github.com/tinoosan/agen8/pkg/buildinfo.Version=${VERSION} -X github.com/tinoosan/agen8/pkg/buildinfo.Commit=${COMMIT} -X github.com/tinoosan/agen8/pkg/buildinfo.BuildDate=${BUILD_DATE}" \
	-o /out/agen8 ./cmd/agen8

FROM alpine:3.22.5 AS runtime

RUN apk add --no-cache ca-certificates git \
	&& addgroup -S -g 10001 agen8 \
	&& adduser -S -D -H -u 10001 -G agen8 -s /sbin/nologin agen8 \
	&& mkdir -p /data \
	&& chown agen8:agen8 /data

COPY --from=go-builder /out/agen8 /usr/local/bin/agen8

ENV AGEN8_DATA_DIR=/data
WORKDIR /data
VOLUME ["/data"]
EXPOSE 7777

USER agen8
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
	CMD ["/usr/local/bin/agen8", "healthcheck", "--url", "http://127.0.0.1:7777/healthz", "--timeout", "2s"]

ENTRYPOINT ["/usr/local/bin/agen8"]
CMD ["daemon", "start", "--listener", "http", "--http-addr", "0.0.0.0:7777", "--data-dir", "/data"]
