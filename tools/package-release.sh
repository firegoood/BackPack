#!/bin/sh
# Package only explicit build/legal artifacts, never the checkout/reference.
set -eu
test "$#" = 2
binary=$1
archive=$2
test -f "$binary"
stage=$(mktemp -d)
trap 'rm -rf -- "$stage"' EXIT HUP INT TERM
mkdir -p "$stage/licenses/waticmp"
cp "$binary" "$stage/fullpack"
cp LICENSE NOTICE TRADEMARK.md "$stage/"
cp internal/tunnel/waticmp/engine/LICENSE "$stage/licenses/waticmp/MIT.txt"
cp internal/tunnel/waticmp/engine/ORIGIN.json "$stage/licenses/waticmp/ORIGIN.json"
tar -czf "$archive" -C "$stage" fullpack LICENSE NOTICE TRADEMARK.md licenses
