#!/bin/sh
set -eu

umask 077

mkdir -p /app/data
chown grok2api:grok2api /app/data

quality_guard_dir=/var/lib/grok2api-quality-guard
mkdir -p "${quality_guard_dir}"
chown grok2api:grok2api "${quality_guard_dir}"
chmod 0700 "${quality_guard_dir}"

config_source=${GROK2API_CONFIG_SOURCE:-/run/grok2api/config.yaml}
if [ ! -f "${config_source}" ]; then
  if [ -e "${config_source}" ] || [ "${config_source}" != /run/grok2api/config.yaml ]; then
    echo "config source is missing or is not a file: ${config_source}" >&2
    exit 1
  fi
  config_source=/app/data/config.yaml
  if [ ! -f "${config_source}" ]; then
    if [ -e /app/data/backend.db ]; then
      echo "existing database requires its original config and encryption key; mount config.yaml to /run/grok2api/config.yaml" >&2
      exit 1
    fi
    /app/grok2api init-config --config "${config_source}"
  fi
fi

cp "${config_source}" /app/config.yaml
chown grok2api:grok2api /app/config.yaml
chmod 0600 /app/config.yaml

exec su-exec grok2api:grok2api "$@"
