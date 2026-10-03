# Como rodar o projeto

Guia prático: Postgres + Keycloak + API HTTP (Uber Fx).  
SQS / outbox / wagering HTTP ainda não estão nesta fase.

## Pré-requisitos

- Go (versão do `go.mod`)
- Docker + Docker Compose
- `psql` e `curl` (opcional)

## 1. Subir dependências

```sh
docker compose up -d
docker compose exec postgres pg_isready -U jungle -d jungle
# Keycloak: aguarde health (import do realm jungle)
curl -sf http://localhost:8081/realms/jungle >/dev/null && echo keycloak_ok
```

| Serviço | URL / credenciais |
| --- | --- |
| Postgres | `postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable` |
| Keycloak admin | http://localhost:8081 — `admin` / `admin` |
| Realm | `jungle` |

```sh
docker compose down          # para
docker compose down -v       # zera volumes
```

## 2. Variáveis de ambiente

```sh
cp .env.example .env
export DATABASE_URL='postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable'
export HTTP_ADDR=':8080'
export OIDC_ISSUER_URL='http://localhost:8081/realms/jungle'
export OIDC_AUDIENCE='jungle-api'
export INTERNAL_SERVICE_ROLE='wallet-internal'
```

## 3. Migrations

```sh
psql "$DATABASE_URL" -f migrations/000001_wallets_ledger.up.sql
# ou:
docker compose exec -T postgres psql -U jungle -d jungle < migrations/000001_wallets_ledger.up.sql
```

## 4. Subir a API

```sh
go run ./cmd/server
```

### Token interno (`client_credentials`)

```sh
TOKEN=$(curl -s -X POST 'http://localhost:8081/realms/jungle/protocol/openid-connect/token' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d 'grant_type=client_credentials' \
  -d 'client_id=jungle-internal' \
  -d 'client_secret=jungle-internal-secret' | jq -r .access_token)
```

Client de provedor (não acessa `/wallets*` — deve retornar 403):

| Client | Secret |
| --- | --- |
| `jungle-internal` | `jungle-internal-secret` |
| `provider-a` | `provider-a-secret` |

### Exemplos autenticados

```sh
curl -s localhost:8080/health/live

curl -s -X POST localhost:8080/wallets \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"playerId":"0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1","initialBalance":{"amount":"1000.00","currency":"BRL"}}'

curl -s localhost:8080/wallets/:id -H "Authorization: Bearer $TOKEN"
curl -s "localhost:8080/wallets/:id/ledger?limit=50" -H "Authorization: Bearer $TOKEN"
curl -s -X POST localhost:8080/wallets/:id/reconciliation -H "Authorization: Bearer $TOKEN"
```

Sem token → `401`. Token de `provider-a` → `403` em `/wallets*`.

Se o service account não receber a role no import, no Admin UI:  
Clients → `jungle-internal` → Service account roles → Assign `wallet-internal`.

## 5. Testes

```sh
# unitários (sem Docker)
go test ./internal/domain/... ./internal/auth/... ./internal/web/... ./internal/app/... -race

# integração Postgres
export DATABASE_URL='postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable'
go test ./internal/database/ ./internal/usecase/wallet/ -v -count=1

go test ./... -race
go vet ./...
```

## O que ainda não roda

- LocalStack / filas SQS / outbox / `POST /wagering/transactions`
- Authz por `providerId` nas rotas de wagering (middleware `RequireProvider` já existe)

Decisões: `ARCHITECTURE.md`. Enunciado: `README.md`.
