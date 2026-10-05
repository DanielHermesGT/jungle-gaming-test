# jungle-gaming-test

Serviço Go (Uber Fx) para carteiras e operações de wagering com PostgreSQL, Keycloak e SQS (LocalStack): HTTP + consumidor, inbox/outbox transacional e workers no mesmo processo.

Enunciado do desafio: [CHALLENGE.md](CHALLENGE.md). Decisões: [ARCHITECTURE.md](ARCHITECTURE.md). Runbook detalhado: [Run.md](Run.md).

## Pré-requisitos

- Docker + Docker Compose
- Go (versão em `go.mod`) para testes / `go run` local
- `curl`, `jq` (e AWS CLI opcional para smoke SQS)

## Subir tudo

```sh
cp .env.example .env   # se ainda não existir
docker compose up --build -d
```

Sobe Postgres, Keycloak, LocalStack (filas FIFO+DLQ) e a API (migrations aplicadas no entrypoint).

| Serviço | URL |
| --- | --- |
| API | http://localhost:8080 |
| Keycloak | http://localhost:8081 (`admin` / `admin`, realm `jungle`) |
| Postgres | `postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable` |
| LocalStack SQS | http://localhost:4566 |

Aguarde Keycloak (~1 min na 1ª vez) e confira:

```sh
curl -sf http://localhost:8080/health/live
curl -sf http://localhost:8080/health/ready
curl -sf http://localhost:8080/metrics
curl -sf http://localhost:8081/realms/jungle >/dev/null && echo keycloak_ok
```

## Variáveis de ambiente

Ver [`.env.example`](.env.example). Obrigatórias para a API: `DATABASE_URL`, `OIDC_ISSUER_URL`, `SQS_WAGER_QUEUE_URL`, `SQS_WAGER_DLQ_URL`, `SQS_DOMAIN_EVENTS_QUEUE_URL`.

No Compose, a API usa `network_mode: host` (Linux) para o issuer OIDC (`localhost:8081`) bater com o JWT emitido pelo Keycloak.

## Migrations

Versionadas em `migrations/` (`000001` … `000005`). No container da API o entrypoint aplica as pendentes via tabela `schema_migrations`.

Manual (host):

```sh
docker compose exec -T postgres psql -U jungle -d jungle < migrations/000001_wallets_ledger.up.sql
# … até 000005_ledger_immutable.up.sql
```

Reversão (ordem inversa dos `.down.sql`):

```sh
docker compose exec -T postgres psql -U jungle -d jungle < migrations/000005_ledger_immutable.down.sql
# … até 000001
```

## Filas SQS (LocalStack)

Criadas por [`deploy/localstack/init-sqs.sh`](deploy/localstack/init-sqs.sh):

- `wager-transactions.fifo` (+ DLQ com redrive)
- `domain-events.fifo` (saída do publisher de outbox)

`MessageGroupId` = `walletId`; `MessageDeduplicationId` = `idempotencyKey` (transporte; não substitui inbox/idempotência financeira).

## Identidades de teste (Keycloak)

| Client | Secret | Uso |
| --- | --- | --- |
| `jungle-internal` | `jungle-internal-secret` | `/wallets*` (role `wallet-internal`) |
| `provider-a` | `provider-a-secret` | `/wagering*` (claim `provider_id`) |

```sh
TOKEN=$(curl -s -X POST 'http://localhost:8081/realms/jungle/protocol/openid-connect/token' \
  -d 'grant_type=client_credentials' \
  -d 'client_id=jungle-internal' \
  -d 'client_secret=jungle-internal-secret' | jq -r .access_token)

curl -s -X POST localhost:8080/wallets \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"playerId":"player-demo","initialBalance":{"amount":"100.00","currency":"BRL"}}' | jq
```

Fluxos HTTP completos (BET, auth negada, etc.): [Run.md](Run.md).

## API só no host (sem container)

```sh
docker compose up -d postgres keycloak localstack
set -a && source .env && set +a
# aplique migrations se banco novo
./scripts/run-api.sh
```

## Testes

```sh
set -a && source .env && set +a   # integração com Postgres/LocalStack

go test ./...
go test -race ./...
go vet ./...
go fmt ./...
```

Integração §13 (Postgres + opcional Keycloak/SQS) vive em `internal/usecase/...` e `internal/integration/...` — skip automático se a infra não estiver no ar.

## Encerrar

```sh
docker compose down          # para
docker compose down -v       # apaga volume do Postgres
```
