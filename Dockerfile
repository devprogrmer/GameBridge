# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY . .

RUN go build -o gamebridge ./cmd

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/gamebridge .

EXPOSE 8080

CMD ["./gamebridge"]
