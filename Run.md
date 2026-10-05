# Como rodar o projeto

Guia prático: Postgres + Keycloak + LocalStack (SQS) + API HTTP (Uber Fx) com workers de inbox/outbox.

## Pré-requisitos

- Go (versão do `go.mod`)
- Docker + Docker Compose
- `psql`, `curl`, `jq` e AWS CLI (úteis no passo a passo / smoke SQS)

## 1. Subir dependências

```sh
docker compose up -d
docker compose exec postgres pg_isready -U jungle -d jungle
# Keycloak: aguarde o realm (pode levar ~1 min na 1ª subida)
curl -sf http://localhost:8081/realms/jungle >/dev/null && echo keycloak_ok
# LocalStack: filas criadas por deploy/localstack/init-sqs.sh
curl -sf http://localhost:4566/_localstack/health >/dev/null && echo localstack_ok
aws --endpoint-url=http://localhost:4566 sqs list-queues
```

| Serviço | URL / credenciais |
| --- | --- |
| Postgres | `postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable` |
| Keycloak admin | http://localhost:8081 — `admin` / `admin` |
| Realm | `jungle` (troque no canto superior esquerdo do Admin) |
| LocalStack SQS | http://localhost:4566 |

```sh
docker compose down          # para
docker compose down -v       # para e apaga volume do Postgres (banco zera)
```

Depois de `down -v` (ou Postgres novo), as migrations do passo 3 são **obrigatórias** de novo.
Sem elas o `/health/ready` pode ficar ok, mas `POST /wallets` devolve `500`.

## 2. Variáveis de ambiente (obrigatório para a API)

A API **não** lê o arquivo `.env` sozinha. Ela só vê variáveis já exportadas no shell  
(`os.Getenv`). O `.env` é um atalho para carregar isso.

```sh
cp .env.example .env   # se ainda não existir
```

Conteúdo esperado (sem a palavra `export` no arquivo):

```env
DATABASE_URL=postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable
HTTP_ADDR=:8080
OIDC_ISSUER_URL=http://localhost:8081/realms/jungle
OIDC_AUDIENCE=jungle-api
INTERNAL_SERVICE_ROLE=wallet-internal
AWS_REGION=us-east-1
AWS_ENDPOINT_URL=http://localhost:4566
AWS_ACCESS_KEY_ID=test
AWS_SECRET_ACCESS_KEY=test
SQS_WAGER_QUEUE_URL=http://localhost:4566/000000000000/wager-transactions.fifo
SQS_WAGER_DLQ_URL=http://localhost:4566/000000000000/wager-transactions-dlq.fifo
SQS_DOMAIN_EVENTS_QUEUE_URL=http://localhost:4566/000000000000/domain-events.fifo
```

**Carregar no terminal atual** (faça isso sempre antes de `go run` / testes de integração):

```sh
set -a
source .env
set +a
```

Confira:

```sh
echo "$DATABASE_URL"
echo "$OIDC_ISSUER_URL"
echo "$SQS_WAGER_QUEUE_URL"
```

Sem `OIDC_ISSUER_URL` / `DATABASE_URL` / URLs SQS no ambiente, a API **não sobe**.

## 3. Migrations (obrigatório antes da API / dos curls)

Cria `wallets`, `wallet_ledger_entries`, `wager_transactions`, `outbox_events` e `inbox_messages`. Sem `000001`, auth pode passar e mesmo assim
`POST /wallets` responde `{"code":"internal_error"}` (relação inexistente no Postgres).

```sh
# preferível via Compose (não depende de psql no host):
docker compose exec -T postgres psql -U jungle -d jungle < migrations/000001_wallets_ledger.up.sql
docker compose exec -T postgres psql -U jungle -d jungle < migrations/000002_wager_transactions.up.sql
docker compose exec -T postgres psql -U jungle -d jungle < migrations/000003_wager_pending_reference_ttl.up.sql
docker compose exec -T postgres psql -U jungle -d jungle < migrations/000004_inbox_outbox.up.sql

# ou, com psql local + .env carregado:
psql "$DATABASE_URL" -f migrations/000001_wallets_ledger.up.sql
psql "$DATABASE_URL" -f migrations/000002_wager_transactions.up.sql
psql "$DATABASE_URL" -f migrations/000003_wager_pending_reference_ttl.up.sql
psql "$DATABASE_URL" -f migrations/000004_inbox_outbox.up.sql
```

Confira:

```sh
docker compose exec -T postgres psql -U jungle -d jungle -c '\dt'
# wallets, wallet_ledger_entries, wager_transactions, outbox_events, inbox_messages
```

Reverter (ordem inversa): `000004` → `000003` → `000002` → `000001`.

## 4. Smoke HTTP — dois terminais (ordem fixa)

Use **sempre o mesmo Terminal B** para todos os `curl` (variáveis `$TOKEN`, `$WID`, `$PID` não passam de um shell para outro).

| Client | Secret | `/wallets*` | `/wagering*` |
| --- | --- | --- | --- |
| `jungle-internal` | `jungle-internal-secret` | ok (role) | **403** (sem `provider_id`) |
| `provider-a` | `provider-a-secret` | **403** | ok (`provider_id`) |

### Terminal A — sobe a API e deixa rodando

```sh
cd /home/daniel/Desktop/git/jungle-gaming-test

docker compose up -d
curl -sf http://localhost:8081/realms/jungle >/dev/null && echo keycloak_ok

# se banco novo / down -v: rode as 3 migrations do passo 3

./scripts/run-api.sh
# espere: [Fx] RUNNING  (sem "address already in use")
```

Não feche este terminal.

---

### Terminal B — requests (cole bloco a bloco, na ordem)

**B1 — health**

```sh
cd /home/daniel/Desktop/git/jungle-gaming-test

curl -s localhost:8080/health/live
curl -s localhost:8080/health/ready
# {"status":"ok"} / {"status":"ready"}
```

**B2 — token interno + abrir carteira**

```sh
TOKEN=$(curl -s -X POST 'http://localhost:8081/realms/jungle/protocol/openid-connect/token' \
  -d 'grant_type=client_credentials' \
  -d 'client_id=jungle-internal' \
  -d 'client_secret=jungle-internal-secret' | jq -r .access_token)

# playerId único a cada teste (evita 409 se já abriu antes)
PLAYER_ID="0192f28f-5dc0-7d58-bdb2-$(date +%s | tail -c 13)"

RESP=$(curl -s -X POST localhost:8080/wallets \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"playerId\":\"$PLAYER_ID\",\"initialBalance\":{\"amount\":\"1000.00\",\"currency\":\"BRL\"}}")

echo "$RESP" | jq

WID=$(echo "$RESP" | jq -r .id)
PID=$(echo "$RESP" | jq -r .playerId)

echo "WID=$WID"
echo "PID=$PID"
# os dois precisam ser UUID/string não vazia — se WID=null, a abertura falhou
```

**B3 — ler carteira / ledger / reconciliação (ainda com `$TOKEN` interno)**

```sh
curl -s "localhost:8080/wallets/$WID" -H "Authorization: Bearer $TOKEN" | jq
curl -s "localhost:8080/wallets/$WID/ledger?limit=50" -H "Authorization: Bearer $TOKEN" | jq
curl -s -X POST "localhost:8080/wallets/$WID/reconciliation" -H "Authorization: Bearer $TOKEN" | jq
```

**B4 — token do provedor + BET**

```sh
PROVIDER_TOKEN=$(curl -s -X POST 'http://localhost:8081/realms/jungle/protocol/openid-connect/token' \
  -d 'grant_type=client_credentials' \
  -d 'client_id=provider-a' \
  -d 'client_secret=provider-a-secret' | jq -r .access_token)

# confira: WID e PID ainda preenchidos neste mesmo terminal
echo "WID=$WID PID=$PID"

EXT_ID="transaction-$(date +%s)"

BET=$(curl -s -X POST localhost:8080/wagering/transactions \
  -H "Authorization: Bearer $PROVIDER_TOKEN" \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: provider-a:$EXT_ID" \
  -d "{
    \"providerId\":\"provider-a\",
    \"externalTransactionId\":\"$EXT_ID\",
    \"playerId\":\"$PID\",
    \"walletId\":\"$WID\",
    \"roundId\":\"round-987\",
    \"gameId\":\"fortune-chimp\",
    \"kind\":\"BET\",
    \"money\":{\"amount\":\"25.00\",\"currency\":\"BRL\"}
  }")

echo "$BET" | jq
TID=$(echo "$BET" | jq -r .transactionId)
echo "TID=$TID EXT_ID=$EXT_ID"
```

Esperado no BET: `"status":"PROCESSED"`, `"balance":{"amount":"975.00",...}`, `idempotentReplay: false`.

**B5 — consultar wager + saldo depois da aposta**

```sh
curl -s "localhost:8080/wagering/transactions/$TID" \
  -H "Authorization: Bearer $PROVIDER_TOKEN" | jq

curl -s "localhost:8080/providers/provider-a/wagering/transactions/$EXT_ID" \
  -H "Authorization: Bearer $PROVIDER_TOKEN" | jq

curl -s "localhost:8080/wallets/$WID" \
  -H "Authorization: Bearer $TOKEN" | jq
# balance amount deve ser 975.00
```

**B6 — checagens rápidas de auth (opcional)**

```sh
# 401 sem token
curl -si localhost:8080/wallets | head -1

# 403: provedor em /wallets*
curl -si -X POST localhost:8080/wallets \
  -H "Authorization: Bearer $PROVIDER_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"playerId":"x","initialBalance":{"amount":"0.00","currency":"BRL"}}' | head -1

# replay: mesmo Idempotency-Key + mesmo body → idempotentReplay true
curl -s -X POST localhost:8080/wagering/transactions \
  -H "Authorization: Bearer $PROVIDER_TOKEN" \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: provider-a:$EXT_ID" \
  -d "{
    \"providerId\":\"provider-a\",
    \"externalTransactionId\":\"$EXT_ID\",
    \"playerId\":\"$PID\",
    \"walletId\":\"$WID\",
    \"roundId\":\"round-987\",
    \"gameId\":\"fortune-chimp\",
    \"kind\":\"BET\",
    \"money\":{\"amount\":\"25.00\",\"currency\":\"BRL\"}
  }" | jq
```

| Resposta | Causa comum |
| --- | --- |
| `401` | sem Bearer / token inválido ou expirado |
| `403` em `/wallets*` | token sem role `wallet-internal` (ex.: provedor) |
| `403` em `/wagering*` | token sem `provider_id` (ex.: interno) |
| `400` `player/wallet/round/game` | `$PID` ou `$WID` vazios (mudou de terminal) |
| `500` | migration não aplicada |
| `409` | mesma carteira (player+moeda) ou conflito de idempotência |

### Se o token interno NÃO tiver `wallet-internal`

O realm antigo no container não pega mudança do JSON sozinho. Recrie o Keycloak:

```sh
docker compose rm -sf keycloak
docker compose up -d keycloak
curl -sf http://localhost:8081/realms/jungle >/dev/null && echo keycloak_ok
```

Depois pegue um **TOKEN novo** e decode de novo.

Ajuste manual no Admin (realm `jungle`):

1. Clients → `jungle-internal` → **Service account roles** → Assign `wallet-internal`
2. Clients → `jungle-internal` → **Client scopes** → Default → garanta `roles` e `jungle-audience`

## 5. Publicar wager via SQS (smoke)

Com a API rodando (consumer ativo) e `$WID` / `$PID` do passo B2:

```sh
set -a && source .env && set +a

MSG_ID="msg-$(date +%s)"
EXT_ID="sqs-tx-$(date +%s)"
BODY=$(jq -n \
  --arg mid "$MSG_ID" \
  --arg ext "$EXT_ID" \
  --arg pid "$PID" \
  --arg wid "$WID" \
  '{
    messageId: $mid,
    type: "WagerTransactionRequested",
    occurredAt: "2026-10-04T12:00:00.000Z",
    data: {
      providerId: "provider-a",
      externalTransactionId: $ext,
      idempotencyKey: ("provider-a:" + $ext),
      playerId: $pid,
      walletId: $wid,
      roundId: "round-sqs",
      gameId: "fortune-chimp",
      kind: "BET",
      money: { amount: "10.00", currency: "BRL" }
    }
  }')

aws --endpoint-url="$AWS_ENDPOINT_URL" sqs send-message \
  --queue-url "$SQS_WAGER_QUEUE_URL" \
  --message-body "$BODY" \
  --message-group-id "$WID" \
  --message-deduplication-id "provider-a:$EXT_ID"

# após alguns segundos: saldo debitado; eventos em domain-events.fifo
aws --endpoint-url="$AWS_ENDPOINT_URL" sqs receive-message \
  --queue-url "$SQS_DOMAIN_EVENTS_QUEUE_URL" \
  --max-number-of-messages 5 \
  --wait-time-seconds 5 | jq
```

HTTP curls do passo 4 permanecem inalterados (mesmo `Process`).

## 6. Testes

```sh
set -a && source .env && set +a

# unitários (sem Docker)
go test ./internal/domain/... ./internal/auth/... ./internal/web/... ./internal/app/... -race

# integração Postgres (inbox/outbox/use cases)
go test ./internal/database/ ./internal/usecase/wallet/ ./internal/usecase/wager/ -v -count=1

# integração LocalStack (skip se sem endpoint)
go test ./internal/messaging/ -v -count=1

go test ./... -race
go vet ./...
```

Decisões: `ARCHITECTURE.md`. Enunciado: `README.md`.
