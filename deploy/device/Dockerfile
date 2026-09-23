ARG ALPINE_IMAGE=alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6
FROM ${ALPINE_IMAGE}

ARG SPACEWAVE_REVISION=unknown
LABEL org.opencontainers.image.title="Spacewave Device" \
      org.opencontainers.image.revision="${SPACEWAVE_REVISION}"

RUN apk add --no-cache \
      bash \
      ca-certificates \
      docker-cli \
      fuse3 \
      git \
      openssh-client \
    && addgroup -S spacewave \
    && adduser -S -G spacewave -h /var/lib/spacewave spacewave \
    && mkdir -p /run/spacewave /var/lib/spacewave \
    && chown -R spacewave:spacewave /run/spacewave /var/lib/spacewave

COPY --chmod=0755 spacewave /usr/local/bin/spacewave

USER spacewave
VOLUME ["/var/lib/spacewave", "/run/spacewave"]
ENTRYPOINT ["/usr/local/bin/spacewave"]
CMD ["--state-path", "/var/lib/spacewave", "--socket-path", "/run/spacewave/spacewave.sock", "serve", "--idle-timeout", "0"]
