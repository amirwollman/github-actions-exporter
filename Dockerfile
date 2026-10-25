# Go 1.27.3 is the first 1.27 release whose crypto/x509 refuses the
# compromised eMudhra intermediates (CVE-2026-111943), whatever roots the
# system store still carries.
FROM golang:1.27.3@sha256:f20d83a83e18dbd2a4714b8ffd4b30b287b6243962dd75608e86abcc63ba4efc AS builder

WORKDIR /app
COPY . .
RUN bash ./build.sh

FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a AS release

# ca-certificates 20261020 drops the eMudhra roots whose intermediates leaked
# their keys. The image ships whatever bundle exists at build time, so fail the
# build rather than ship one that still trusts them. Go reads the first bundle
# it finds and every file in /etc/ssl/certs, so all of them are checked.
# openssl is only needed for the check and leaves with the same layer. The
# bind mount needs BuildKit (the default builder; buildx in CI).
RUN --mount=type=bind,source=scripts/check-distrusted-certs.sh,target=/tmp/check-distrusted-certs.sh \
	apk add --no-cache --upgrade 'ca-certificates-bundle>=20261020' \
	&& apk add --no-cache --virtual .cert-check openssl \
	&& sh /tmp/check-distrusted-certs.sh /etc/ssl/cert.pem /etc/ssl/certs/* \
	&& apk del --no-cache .cert-check

COPY --from=builder /app/bin/app .
USER 65534:65534
CMD ["./app"]
