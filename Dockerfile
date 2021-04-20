# syntax = docker/dockerfile:experimental

FROM --platform=${BUILDPLATFORM} golang:1.16-alpine as builder

# add CA certificates and TZ for local time
RUN apk --update add ca-certificates tzdata make git

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
RUN make

# final image
FROM scratch

# copy the binary to the production image from the builder stage.
COPY --from=builder /go/src/app/.bin/secrets-init /secrets-init
# copy certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
# copy timezone settings
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

ENTRYPOINT ["/secrets-init"]
CMD ["--version"]
