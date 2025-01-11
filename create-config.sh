#!/usr/bin/env bash
set -eu -o pipefail

encryption_key=$(openssl rand -base64 24 | tr -dc 'a-zA-Z0-9' | head -c 32)
echo "secretKey=${encryption_key}" > .secrets
