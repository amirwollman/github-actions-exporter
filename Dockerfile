# Go 1.27.3 is the first 1.27 release whose crypto/x509 refuses the
# compromised eMudhra intermediates (CVE-2026-111943), whatever roots the
# system store still carries.
FROM golang:1.27.3@sha256:f20d83a83e18dbd2a4714b8ffd4b30b287b6243962dd75608e86abcc63ba4efc AS builder

WORKDIR /app
COPY . .
RUN bash ./build.sh

# ca-certificates 20261020 drops the eMudhra roots whose intermediates leaked
# their keys. The image ships whatever bundle exists at build time, so fail the
# build rather than ship one that still trusts them.
FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a AS certs
RUN apk add --no-cache --upgrade 'ca-certificates>=20261020' openssl
COPY scripts/check-distrusted-roots.sh /usr/local/bin/
RUN sh /usr/local/bin/check-distrusted-roots.sh /etc/ssl/certs/ca-certificates.crt

FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a AS release
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /app/bin/app .
USER 65534:65534
CMD ["./app"]
