#!/bin/sh
# Keep local reference modules and Git metadata outside FullPack formatting.
# Listing only: this command never rewrites source files.
set -eu
find . -type d \( -name .git -o -path ./11 -o -name vendor \) -prune -o \
  -type f -name '*.go' -print0 | xargs -0 gofmt -l
