#!/usr/bin/env bash

set -x

# The workflow passes the version in (the Dockerfile's VERSION build arg), so
# the image reports the tag that triggered the build and git isn't needed.
VERSION="${VERSION:-development}"

CGO_ENABLED=0 GOOS=linux go build -a -ldflags="-X 'main.version=$VERSION'" -o bin/app
