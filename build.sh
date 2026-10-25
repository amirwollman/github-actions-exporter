#!/usr/bin/env bash

set -x

# Release builds run on a detached HEAD at the tag, where `git branch` would
# report "(HEAD", so prefer the exact image tag (not a chart-v* tag on the same
# commit) and fall back to the short commit hash.
VERSION=`git describe --tags --exact-match --match 'v[0-9]*' 2>/dev/null || git rev-parse --short HEAD`

CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-X 'main.version=$VERSION'"  -o bin/app
