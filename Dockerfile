# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder
WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/gamebridge-panel \
    ./cmd/gamebridge-panel

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 gamebridge \
    && adduser -S -D -H -u 10001 -G gamebridge gamebridge \
    && mkdir -p /data \
    && chown -R gamebridge:gamebridge /data

COPY --from=builder /out/gamebridge-panel /usr/local/bin/gamebridge-panel

USER 10001:10001

ENV GAMEBRIDGE_PANEL_LISTEN=0.0.0.0:8088 \
    GAMEBRIDGE_PANEL_STATE=/data/panel-state.json \
    GAMEBRIDGE_PANEL_SESSION_KEY=/data/panel-session.key \
    GAMEBRIDGE_PANEL_MASTER_KEY=/data/panel-master.key \
    GAMEBRIDGE_COOKIE_SECURE=0

VOLUME ["/data"]
EXPOSE 8088

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8088/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/gamebridge-panel"]