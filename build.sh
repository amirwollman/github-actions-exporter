#!/usr/bin/env bash

set -x

# Release builds run on a detached HEAD at the tag, where `git branch` would
# report "(HEAD", so prefer the exact tag and fall back to the branch name.
VERSION=`git describe --tags --exact-match 2>/dev/null || git rev-parse --abbrev-ref HEAD`

CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-X 'main.version=$VERSION'"  -o bin/app
