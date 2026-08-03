FROM alpine:3.21

ARG TIGERBEETLE_VERSION=0.17.9

RUN apk add --no-cache ca-certificates netcat-openbsd unzip wget \
  && wget -q "https://github.com/tigerbeetle/tigerbeetle/releases/download/${TIGERBEETLE_VERSION}/tigerbeetle-x86_64-linux.zip" -O /tmp/tigerbeetle.zip \
  && unzip -q /tmp/tigerbeetle.zip -d /usr/local/bin \
  && chmod +x /usr/local/bin/tigerbeetle \
  && rm /tmp/tigerbeetle.zip

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh \
  && addgroup -S tigerbeetle \
  && adduser -S -G tigerbeetle tigerbeetle \
  && mkdir -p /data \
  && chown -R tigerbeetle:tigerbeetle /data /usr/local/bin/entrypoint.sh

USER tigerbeetle
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
