# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN go build \
    -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o gamebridge ./cmd/gamebridge-panel


FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/gamebridge .

EXPOSE 8088

CMD ["./gamebridge"]
