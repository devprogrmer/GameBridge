# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY . .

RUN go build -o gamebridge ./cmd/gamebridge-panel


FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/gamebridge .

EXPOSE 8080

CMD ["./gamebridge"]