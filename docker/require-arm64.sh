#!/bin/sh
set -eu

image_arch=$(apk --print-arch)
machine_arch=$(uname -m)

case "${image_arch}:${machine_arch}" in
  aarch64:aarch64|aarch64:arm64) ;;
  *)
    echo "ARM64 required: image=${image_arch}, machine=${machine_arch}" >&2
    exit 1
    ;;
esac

if [ "$#" -gt 0 ]; then
  exec "$@"
fi
