# Como rodar o projeto

Guia prático: Postgres + Keycloak + API HTTP (Uber Fx).  
SQS / outbox / wagering HTTP ainda não estão nesta fase.

## Pré-requisitos

- Go (versão do `go.mod`)
- Docker + Docker Compose
- `psql`, `curl` e `jq` (úteis no passo a passo)

## 1. Subir dependências

```sh
docker compose up -d
docker compose exec postgres pg_isready -U jungle -d jungle
# Keycloak: aguarde o realm (pode levar ~1 min na 1ª subida)
curl -sf http://localhost:8081/realms/jungle >/dev/null && echo keycloak_ok
```

| Serviço | URL / credenciais |
| --- | --- |
| Postgres | `postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable` |
| Keycloak admin | http://localhost:8081 — `admin` / `admin` |
| Realm | `jungle` (troque no canto superior esquerdo do Admin) |

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
```

Sem `OIDC_ISSUER_URL` / `DATABASE_URL` no ambiente, a API **não sobe**.

## 3. Migrations (obrigatório antes da API / dos curls)

Cria `wallets`, `wallet_ledger_entries` e `wager_transactions` (+ coluna TTL de referência). Sem `000001`, auth pode passar e mesmo assim
`POST /wallets` responde `{"code":"internal_error"}` (relação inexistente no Postgres).

```sh
# preferível via Compose (não depende de psql no host):
docker compose exec -T postgres psql -U jungle -d jungle < migrations/000001_wallets_ledger.up.sql
docker compose exec -T postgres psql -U jungle -d jungle < migrations/000002_wager_transactions.up.sql
docker compose exec -T postgres psql -U jungle -d jungle < migrations/000003_wager_pending_reference_ttl.up.sql

# ou, com psql local + .env carregado:
psql "$DATABASE_URL" -f migrations/000001_wallets_ledger.up.sql
psql "$DATABASE_URL" -f migrations/000002_wager_transactions.up.sql
psql "$DATABASE_URL" -f migrations/000003_wager_pending_reference_ttl.up.sql
```

Confira:

```sh
docker compose exec -T postgres psql -U jungle -d jungle -c '\dt'
# deve listar wallets, wallet_ledger_entries e wager_transactions
```

Reverter (ordem inversa): `000003` → `000002` → `000001`.

O use case `wager.Process` já existe internamente; rota HTTP `/wagering` ainda não.

## 4. Subir a API

Opção A — script (carrega `.env` e roda):

```sh
./scripts/run-api.sh
```

Opção B — manual (no mesmo terminal onde rodou `source .env`):

```sh
go run ./cmd/server
```

Deixe esse processo rodando. Use **outro terminal** para os `curl` abaixo  
(lá o `$TOKEN` vive; a API não precisa do token no ambiente).

### 4.1 Token interno (`client_credentials`)

```sh
TOKEN=$(curl -s -X POST 'http://localhost:8081/realms/jungle/protocol/openid-connect/token' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d 'grant_type=client_credentials' \
  -d 'client_id=jungle-internal' \
  -d 'client_secret=jungle-internal-secret' | jq -r .access_token)

# obrigatório: tem que aparecer wallet-internal (senão a API devolve 403)
echo "$TOKEN" | cut -d. -f2 | python3 -c '
import sys, base64, json
s = sys.stdin.read().strip()
s += "=" * (-len(s) % 4)
print(json.dumps(json.loads(base64.urlsafe_b64decode(s)), indent=2))
'
```

No JSON decodificado, confira:

- `"aud"` contém `jungle-api`
- `"realm_access": { "roles": [ ..., "wallet-internal", ... ] }`

| Client | Secret | `/wallets*` |
| --- | --- | --- |
| `jungle-internal` | `jungle-internal-secret` | permitido (com role) |
| `provider-a` | `provider-a-secret` | **403** |

### 4.2 Chamadas

```sh
curl -s localhost:8080/health/live
curl -s localhost:8080/health/ready

curl -s -X POST localhost:8080/wallets \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"playerId":"0192f28f-5dc0-7d58-bdb2-814ad6a0f4a1","initialBalance":{"amount":"1000.00","currency":"BRL"}}'
```

Substitua `:id` pelo `id` da resposta:

```sh
curl -s localhost:8080/wallets/:id -H "Authorization: Bearer $TOKEN"
curl -s "localhost:8080/wallets/:id/ledger?limit=50" -H "Authorization: Bearer $TOKEN"
curl -s -X POST localhost:8080/wallets/:id/reconciliation -H "Authorization: Bearer $TOKEN"
```

Sem token → `401`. Token de `provider-a` → `403` em `/wallets*`.

| Resposta | Causa comum |
| --- | --- |
| `401` unauthorized | sem Bearer / token inválido ou expirado |
| `403` forbidden / `internal service role required` | token ok, mas sem role `wallet-internal` |
| `500` internal_error | migration não aplicada (`wallets` inexistente) — volte ao passo 3 |
| `409` conflict | já existe carteira para o mesmo `playerId` + moeda |

No log da API, tabela faltando aparece como `relation "wallets" does not exist`.

### 4.3 Se o decode do token NÃO tiver `wallet-internal`

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

## 5. Testes

```sh
set -a && source .env && set +a   # se for rodar integração com Postgres

# unitários (sem Docker)
go test ./internal/domain/... ./internal/auth/... ./internal/web/... ./internal/app/... -race

# integração Postgres
go test ./internal/database/ ./internal/usecase/wallet/ -v -count=1

go test ./... -race
go vet ./...
```

## O que ainda não roda

- LocalStack / filas SQS / outbox / `POST /wagering/transactions`
- Authz por `providerId` nas rotas de wagering (middleware `RequireProvider` já existe)

Decisões: `ARCHITECTURE.md`. Enunciado: `README.md`.
