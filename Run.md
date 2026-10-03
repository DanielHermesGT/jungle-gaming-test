# Como rodar o projeto

Guia prático: domínio + database + use cases de carteira + HTTP (Uber Fx).  
Auth Keycloak e SQS ainda **não** estão nesta fase.

## Pré-requisitos

- Go (versão do `go.mod`)
- Docker + Docker Compose
- `psql` (opcional, para aplicar migrations manualmente)

## 1. Subir o Postgres

```sh
docker compose up -d
docker compose exec postgres pg_isready -U jungle -d jungle
```

| Item | Valor |
| --- | --- |
| User | `jungle` |
| Password | `jungle` |
| Database | `jungle` |
| URL | `postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable` |

```sh
docker compose down          # para
docker compose down -v       # para e apaga o volume
```

## 2. Variáveis de ambiente

```sh
cp .env.example .env
export DATABASE_URL='postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable'
export HTTP_ADDR=':8080'
```

## 3. Migrations

```sh
psql "$DATABASE_URL" -f migrations/000001_wallets_ledger.up.sql
# ou:
docker compose exec -T postgres psql -U jungle -d jungle < migrations/000001_wallets_ledger.up.sql
```

Reverter: `migrations/000001_wallets_ledger.down.sql`.

## 4. Subir a API

```sh
go run ./cmd/server
```

### Exemplos (sem auth nesta fase)

```sh
# health
curl -s localhost:8080/health/live
curl -s localhost:8080/health/ready

# abrir carteira
curl -s -X POST localhost:8080/wallets \
  -H 'Content-Type: application/json' \
  -d '{"playerId":"0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1","initialBalance":{"amount":"1000.00","currency":"BRL"}}'

# ler / ledger / reconciliação (substitua :id)
curl -s localhost:8080/wallets/:id
curl -s 'localhost:8080/wallets/:id/ledger?limit=50'
curl -s -X POST localhost:8080/wallets/:id/reconciliation
```

## 5. Testes

```sh
# domínio + handlers (sem Docker)
go test ./internal/domain/... ./internal/web/... ./internal/app/... -race

# integração Postgres
export DATABASE_URL='postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable'
go test ./internal/database/ ./internal/usecase/wallet/ -v -count=1

go test ./... -race
go vet ./...
```

Sem `DATABASE_URL`, testes de integração fazem `Skip`.

## O que ainda não roda

- Keycloak / JWT nas rotas de negócio (TODO — eliminatório do README)
- LocalStack / filas SQS / outbox / wagering HTTP

Decisões: `ARCHITECTURE.md`. Enunciado: `README.md`.
