# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN go build \
    -trimpath \
    -ldflags "\
    -X github.com/devprogrmer/GameBridge/internal/panel.version=${VERSION} \
    -X github.com/devprogrmer/GameBridge/internal/panel.commit=${COMMIT} \
    -X github.com/devprogrmer/GameBridge/internal/panel.buildTime=${BUILD_TIME}" \
    -o gamebridge ./cmd/gamebridge-panel


FROM alpine:latest

RUN addgroup -S gamebridge && \
    adduser -S gamebridge -G gamebridge

WORKDIR /app

COPY --from=builder /app/gamebridge .

RUN chown -R gamebridge:gamebridge /app

USER gamebridge

EXPOSE 8088

HEALTHCHECK --interval=30s --timeout=3s \
    CMD wget -qO- http://127.0.0.1:8088/healthz || exit 1

ENTRYPOINT ["./gamebridge"]
