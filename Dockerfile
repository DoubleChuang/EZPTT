# syntax=docker/dockerfile:1.7

# ── Stage 1: cross-compile ──────────────────────────────────────────────
# Runs on the build host's architecture and cross-compiles for the target,
# so an arm64/armv7 image does not need QEMU to run the Go compiler.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG VERSION=dev

WORKDIR /src

# Dependencies first: this layer is cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/ezptt ./cmd/ezptt

# ── Stage 2: runtime ────────────────────────────────────────────────────
FROM alpine:3.22

# ca-certificates is not optional: the login travels over TLS and is verified.
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 10001 -s /sbin/nologin ezptt

COPY --from=build /out/ezptt /usr/local/bin/ezptt

USER ezptt
WORKDIR /data

# Cron runs on local time; keep the container's clock in Taipei time.
# The account path is NOT baked in: WORKDIR is /data, so the default
# "PttConfig.csv" resolves to /data/PttConfig.csv, and ACCOUNTS_FILE from
# env_file can point elsewhere (e.g. PttConfig.csv.enc) without being
# overridden by a prefixed EZPTT_ variable.
ENV TZ=Asia/Taipei

ENTRYPOINT ["/usr/local/bin/ezptt"]
