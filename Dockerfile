FROM scratch

ADD secrets-init /usr/local/bin/secrets-init

ENTRYPOINT ["/usr/local/bin/secrets-init"]