# syntax=docker/dockerfile:1

FROM golang:1.23-alpine AS builder

WORKDIR /build

# Download Go modules
COPY go.mod .
RUN go mod download
RUN go mod verify

# Transfer source code
COPY *.go .
COPY *.html .

# Build
RUN CGO_ENABLED=0 go build -trimpath -o /dist/app

# Test
FROM builder AS run-test-stage
RUN go test -v ./...

FROM alpine AS build-release-stage

RUN apk add --no-cache ffmpeg

WORKDIR /app

COPY --from=builder /dist .
ENTRYPOINT ["./app"]
