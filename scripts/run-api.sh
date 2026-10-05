#!/usr/bin/env bash
# Carrega .env e sobe a API. Uso: ./scripts/run-api.sh
set -euo pipefail
cd "$(dirname "$0")/.."

if [[ ! -f .env ]]; then
  echo "arquivo .env não encontrado. Rode: cp .env.example .env" >&2
  exit 1
fi

set -a
# shellcheck disable=SC1091
source .env
set +a

: "${DATABASE_URL:?DATABASE_URL ausente no .env}"
: "${OIDC_ISSUER_URL:?OIDC_ISSUER_URL ausente no .env}"
: "${SQS_WAGER_QUEUE_URL:?SQS_WAGER_QUEUE_URL ausente no .env}"
: "${SQS_WAGER_DLQ_URL:?SQS_WAGER_DLQ_URL ausente no .env}"
: "${SQS_DOMAIN_EVENTS_QUEUE_URL:?SQS_DOMAIN_EVENTS_QUEUE_URL ausente no .env}"

exec go run ./cmd/server
