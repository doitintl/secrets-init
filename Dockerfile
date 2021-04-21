# syntax = docker/dockerfile:experimental

FROM --platform=${BUILDPLATFORM} golang:1.16-alpine as builder

# passed by buildkit
ARG TARGETOS
ARG TARGETARCH

# add CA certificates and TZ for local time
RUN apk --update add ca-certificates make git

# Create and change to the app directory.
RUN mkdir -p /go/src/app
WORKDIR /go/src/app

# Retrieve application dependencies.
# This allows the container build to reuse cached dependencies.
# Expecting to copy go.mod and if present go.sum.
COPY go.mod .
COPY go.sum .
RUN --mount=type=cache,target=/go/mod go mod download

# Copy local code to the container image.
COPY . .

# Build the binary.
RUN --mount=type=cache,target=/root/.cache/go-build TARGETOS=${TARGETOS} TARGETARCH=${TARGETARCH} make

# final image
# keep it FROM alpine - need to copy secrets-init to target container
FROM --platform=${TARGETPLATFORM} alpine:3.13

COPY --from=builder /go/src/app/.bin/secrets-init /usr/local/bin/secrets-init

CMD ["secrets-init", "--version"]
