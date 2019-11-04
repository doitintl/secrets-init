FROM alpine:3.10

ADD secrets-init /usr/local/bin/secrets-init

CMD ["secrets-init", "--version"]