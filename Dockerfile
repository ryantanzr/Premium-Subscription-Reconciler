FROM buildpack-deps:bookworm-scm AS builder
FROM golang:1.21-bookworm AS build

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o reconciler main.go

FROM debian:bookworm-slim
WORKDIR /root/
COPY --from=build /app/reconciler .

CMD ["./reconciler"]