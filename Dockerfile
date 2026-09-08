# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN go build \
    -ldflags "\
    -X github.com/devprogrmer/GameBridge/internal/panel.version=${VERSION} \
    -X github.com/devprogrmer/GameBridge/internal/panel.commit=${COMMIT} \
    -X github.com/devprogrmer/GameBridge/internal/panel.buildTime=${BUILD_TIME}" \
    -o gamebridge ./cmd/gamebridge-panel


FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/gamebridge .

EXPOSE 8088

CMD ["./gamebridge"]
