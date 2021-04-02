FROM golang:1.15-alpine as builder

WORKDIR /app
COPY . .

RUN go mod download
RUN go build -o /app/secrets-init
# ADD secrets-init /usr/local/bin/secrets-init

FROM alpine:latest
COPY --from=builder /app/secrets-init /usr/local/bin/secrets-init

CMD ["secrets-init", "--version"]