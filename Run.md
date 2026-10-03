# Como rodar o projeto

Guia prático do estado atual: domínio + gateway + database + use cases de carteira.  
Ainda **não** há servidor HTTP / Keycloak / SQS.

## Pré-requisitos

- Go (versão do `go.mod`)
- Docker + Docker Compose
- `psql` (opcional, para aplicar migrations manualmente)

## 1. Subir o Postgres

Na raiz do repositório:

```sh
docker compose up -d
```

Espere o healthcheck (porta `5432`). Conferência rápida:

```sh
docker compose ps
docker compose exec postgres pg_isready -U jungle -d jungle
```

Credenciais locais (ver também `.env.example`):

| Item | Valor |
| --- | --- |
| User | `jungle` |
| Password | `jungle` |
| Database | `jungle` |
| URL | `postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable` |

Parar / limpar:

```sh
docker compose down          # para os containers
docker compose down -v       # para e apaga o volume (zera o banco)
```

## 2. Variável de ambiente

```sh
cp .env.example .env
# ou só exportar:
export DATABASE_URL='postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable'
```

## 3. Aplicar migrations

Com `psql`:

```sh
psql "$DATABASE_URL" -f migrations/000001_wallets_ledger.up.sql
```

Reverter:

```sh
psql "$DATABASE_URL" -f migrations/000001_wallets_ledger.down.sql
```

Sem `psql`, via Docker:

```sh
docker compose exec -T postgres psql -U jungle -d jungle < migrations/000001_wallets_ledger.up.sql
```

## 4. Testes

Somente domínio (não precisa de Docker):

```sh
go test ./internal/domain/... -race
```

Repos + use cases (precisa do Compose + `DATABASE_URL`):

```sh
export DATABASE_URL='postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable'
go test ./internal/database/ ./internal/usecase/wallet/ -v -count=1
```

Sem `DATABASE_URL`, os testes de integração são **pulados** (`Skip`).

Tudo que já existe:

```sh
go test ./... -race
go vet ./...
```

## 5. Fluxo mínimo recomendado

```sh
docker compose up -d
export DATABASE_URL='postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable'
docker compose exec -T postgres psql -U jungle -d jungle < migrations/000001_wallets_ledger.up.sql
go test ./internal/domain/... ./internal/database/ ./internal/usecase/wallet/ -race -count=1
```

## O que ainda não roda

- `cmd/server` — app HTTP ainda não implementada
- Keycloak / LocalStack / filas SQS — fora desta fase

Decisões de arquitetura: ver `ARCHITECTURE.md`.  
Enunciado do desafio: ver `README.md`.
