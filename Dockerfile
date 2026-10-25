# Go 1.27.3 is the first 1.27 release whose crypto/x509 refuses the
# compromised eMudhra intermediates (CVE-2026-111943), whatever roots the
# system store still carries.
FROM golang:1.27.3@sha256:f20d83a83e18dbd2a4714b8ffd4b30b287b6243962dd75608e86abcc63ba4efc AS builder

ARG VERSION=development
WORKDIR /app
COPY . .
RUN VERSION="$VERSION" bash ./build.sh

FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a AS release

# ca-certificates 20261020 drops the eMudhra roots whose intermediates leaked
# their keys. The image ships whatever bundle exists at build time, so fail the
# build rather than ship one that still trusts them. Go reads the first bundle
# it finds and every file in its cert directories, so every one of Go's Linux
# locations that exists is checked.
# This is defense in depth: Go 1.27.3 refuses the intermediates regardless, and
# a runtime SSL_CERT_FILE/SSL_CERT_DIR is outside what a build can check.
# openssl is only needed for the check and leaves with the same layer. The
# bind mount needs BuildKit (the default builder; buildx in CI).
RUN --mount=type=bind,source=scripts/check-distrusted-certs.sh,target=/tmp/check-distrusted-certs.sh \
	apk add --no-cache --upgrade 'ca-certificates-bundle>=20261020' \
	&& apk add --no-cache --virtual .cert-check openssl \
	&& sh /tmp/check-distrusted-certs.sh $(find -L /etc/ssl/cert.pem /etc/ssl/ca-bundle.pem /etc/pki/tls/cacert.pem \
		/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem /etc/ssl/certs /etc/pki/tls/certs \
		-type f 2>/dev/null) \
	&& apk del --no-cache .cert-check

COPY --from=builder /app/bin/app .
USER 65534:65534
CMD ["./app"]
