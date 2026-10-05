# Architecture

Decisões técnicas da solução. Esta seção começa pelo domínio monetário; demais tópicos serão registrados conforme forem implementados.

## Money

### Representação

`Money` é um value object imutável em `internal/domain/money`.

- Valor interno: `int64` em **unidades mínimas** (centavos), escala fixa de **2**.
- Moeda: código ISO 4217 no formato `^[A-Z]{3}$` (sem catálogo completo de códigos).
- Não teve uso de `float32`/`float64` nem biblioteca decimal.

Motivo: escala 2 cobre o contrato do desafio (`"25.00"`), a aritmética fica explícita, e evita dependência extra.

### Limites

Com `int64` e escala 2, o intervalo representável é aproximadamente:

- mínimo: `-92233720368547758.08`
- máximo: `92233720368547758.07`

Overflow em parsing, soma, subtração e negação retorna `money.ErrOverflow`.

### Mapeamento

| Camada | Forma |
| --- | --- |
| API / JSON | `{"amount":"25.00","currency":"BRL"}` (strings) |
| Domínio | `minor int64` + `currency string` |
| Persistência | `BIGINT` (minor) + `CHAR(3)` (currency) |

### Entrada externa (`Parse`)

Aceita somente amount no formato rígido: dígitos + `.` + exatamente duas casas (`"25.00"`, `"0.00"`).

Rejeita, sem arredondar:

- vazio, sinais (`+`/`-`), `NaN`, `Infinity`
- notação científica
- `"25"`, `"25.0"`, `"25.001"`, `"25."` e similares
- moeda inválida (tamanho ≠ 3 ou não `A-Z`)

Não há normalização de formas equivalentes. O amount que entra no hash de idempotência futuro já precisa chegar canônico como `"XX.YY"`; HTTP e SQS compartilham a mesma regra via `Parse`.

### Valores negativos

- `Parse` / `UnmarshalJSON` (entrada externa): negativos são inválidos.
- Uso interno (`FromMinor`, `Sub`, `Neg`): negativos são permitidos para diferenças e cálculos.
- Saldo de carteira não-negativo é responsabilidade do agregado `Wallet` e das constraints do banco.

### Erros

Erros sentinela classificáveis com `errors.Is`:

- `ErrInvalidAmount`
- `ErrInvalidCurrency`
- `ErrCurrencyMismatch`
- `ErrOverflow`
- `ErrUninitialized` (zero-value `Money{}`)

### Serialização

`AmountString` e `MarshalJSON` sempre emitem duas casas decimais. `UnmarshalJSON` reutiliza `Parse`.

## Wallet

### Agregado

`Wallet` em `internal/domain/wallet` é a raiz do agregado financeiro.

- Campos: `id`, `playerId`, saldo (`money.Money`), `version`, `createdAt`, `updatedAt`.
- O saldo só muda por `Open` (saldo inicial), `Credit` e `Debit`.
- `WalletFromPersisted` / `LedgerEntryFromPersisted` reconstroem estado persistido sem reaplicar movimentos nem gerar ledger.
- IDs e timestamps são fornecidos pela camada de aplicação (o domínio não gera UUID).

### Abertura

| Saldo inicial | Efeito |
| --- | --- |
| `0.00` | Wallet com `version = 1`, sem lançamento de ledger |
| `> 0` | Wallet com `version = 1` + um `CREDIT` de abertura no ledger (`balanceBefore = 0`) |

O domínio wallet materializa saldo + ledger de abertura. `wallet.Open` (use case) com saldo > 0 também persiste `wager.Transaction` OPENING (`PROCESSED`) e outbox (`WagerTransactionProcessed` + `WalletBalanceChanged`) na mesma TX.

### Ledger

`LedgerEntry` é imutável. Cada `Credit`/`Debit` bem-sucedido produz exatamente um lançamento com:

- direção `CREDIT` ou `DEBIT`
- `balanceAfter = balanceBefore ± amount` (validado na construção)
- vínculo a `walletId` e `transactionId`

`LOSS` e rejeições não chamam `Credit`/`Debit`.

Movimentos exigem amount **> 0** e a mesma moeda da carteira. Débito com saldo insuficiente retorna `ErrInsufficientFunds` sem alterar o agregado.

`version` inicia em `1` e incrementa **somente** quando o saldo muda (`Credit`/`Debit`).

### Organização de pacotes

Inspirada em clean architecture (referência interna `pratico-ms-vEDA`), adaptada ao README:

```text
internal/
  domain/       # Money, Wallet, LedgerEntry (mantido; não "entity" — Money é VO)
  gateway/      # ports (interfaces de repositório + Querier/TxRunner/DB)
  database/     # adapters Postgres (pgx, SQL explícito) + fx.Module
  usecase/      # um pacote por módulo (ex.: wallet/)
  web/          # HTTP handlers + router (net/http ServeMux)
  auth/         # JWT/OIDC middleware (Keycloak JWKS)
  config/       # env (HTTP_ADDR, DATABASE_URL, OIDC_*)
  app/          # composição Fx dos módulos
pkg/            # idgen, clock
cmd/server/     # fx.New(app.Module).Run()
deploy/keycloak/# realm import
```

Wallet e `LedgerEntry` ficam no **mesmo** pacote `internal/domain/wallet`. O ledger não é agregado independente; nasce só com mudança de saldo. Persistência: duas tabelas e dois repos em `internal/database`.

### Concorrência

O README (§8) permite pessimista, otimista com retry, update atômico condicionado ou combinação. Coordenação **por carteira**; lock global é proibido.

#### Pessimista vs otimista (resumo)

- **Pessimista:** `SELECT … FOR UPDATE` bloqueia a linha da carteira antes de alterar; outros writers na mesma wallet esperam. Simples e adequado à disputa 100 vs 2×80.
- **Otimista:** lê sem lock e grava com `WHERE version = $antiga`; se 0 rows, houve conflito → retry. Melhor quando conflito é raro; sob contenção na mesma wallet gera muitos retries.
- **Update atômico:** `UPDATE … WHERE balance_minor >= $amount` como rede de segurança SQL — não substitui o agregado de domínio.

#### Escolha adotada

**Locking pessimista por carteira** (`SELECT … FROM wallets WHERE id = $1 FOR UPDATE`):

1. Lock apenas na linha da wallet alvo (carteiras distintas seguem em paralelo)
2. TX curta: lock → domínio (`Credit`/`Debit`) → `UPDATE` wallet + `INSERT` ledger → `COMMIT`
3. Campo `version`: incrementado no domínio a cada mudança de saldo; persistido para auditoria. O mecanismo primário anti lost-update entre writers é o `FOR UPDATE`, não retry otimista
4. Constraints no DB como fonte da verdade adicional (abaixo)

Não usamos motor de retry otimista nem fila/lock global por processo.

#### Atomicidade (obrigatório — não esquecer)

```text
BEGIN
  -- FOR UPDATE na wallet quando for alterar
  -- INSERT/UPDATE wallet
  -- INSERT ledger (sempre junto com mudança de saldo)
  -- INSERT/UPDATE wager_transaction (OPENING / Process)
  -- INSERT inbox (entrada SQS) / outbox_events (sempre na mesma TX)
COMMIT
```

Wallet + ledger + wager + inbox/outbox da mesma operação financeira **nunca** em commits separados.

#### Constraints (fonte da verdade no DB)

| Constraint | Onde | Por quê |
| --- | --- | --- |
| `UNIQUE (player_id, currency)` | `wallets` | uma carteira por jogador+moeda |
| `UNIQUE (wallet_id, transaction_id)` | `wallet_ledger_entries` | um lançamento por transação na carteira |
| `CHECK (balance_minor >= 0)` | `wallets` | impede saldo negativo no banco |
| Ledger append-only | DB trigger + repo | só `INSERT`; trigger impede `UPDATE`/`DELETE` (`000005`) |

### Persistência

| Tabela | Papel |
| --- | --- |
| `wallets` | estado atual (`balance_minor` BIGINT + currency, version) |
| `wallet_ledger_entries` | lançamentos imutáveis (append-only) |

Migrations em `migrations/`. Repos em `internal/database` (pgx, SQL explícito). Ambiente local: `docker compose up -d` (somente Postgres nesta fase).

Aplicar migrations (exemplo):

```sh
docker compose up -d
psql "$DATABASE_URL" -f migrations/000001_wallets_ledger.up.sql
```

### Erros

- `ErrInvalidWallet` / `ErrInvalidPlayer`
- `ErrInsufficientFunds`
- `ErrInvalidMovement` / `ErrInvalidLedger`
- `ErrUninitialized`
- reutiliza `money.ErrCurrencyMismatch` quando a moeda do movimento diverge
- gateway/database: `ErrConflict`, `ErrNotFound`
- usecase: `ErrConflict`, `ErrNotFound`, `ErrInvalidInput`

## Application — use cases de carteira

Módulo `internal/usecase/wallet` — um `UseCase` com métodos:

| Método | Papel |
| --- | --- |
| `Open` | cria wallet (+ ledger se saldo > 0) na mesma TX |
| `Get` | leitura por id |
| `ListLedger` | ledger com cursor opaco `(created_at, id)` |
| `Reconcile` | saldo armazenado vs ΣCREDIT−ΣDEBIT; não altera saldo |

`Open` com saldo positivo persiste `WagerTransaction OPENING` + ledger + outbox (`WagerTransactionProcessed` + `WalletBalanceChanged`) na mesma TX. Saldo zero: só a carteira.

## HTTP + Uber Fx

Composição em `internal/app` via `fx.Module`s: `config` → `auth` → `database` → `usecase/wallet` + `usecase/wager` → `messaging` → `web`.

`fx.Lifecycle`:
- `OnStart`: HTTP server + loops SQS consumer / outbox publisher / pending-ref
- `OnStop`: cancela workers, `Shutdown` do HTTP + `DB.Close`

Rotas (`net/http` ServeMux):

| Método | Path | Auth |
| --- | --- | --- |
| `POST` | `/wallets` | JWT + role `wallet-internal` |
| `GET` | `/wallets/{walletId}` | JWT + role `wallet-internal` |
| `GET` | `/wallets/{walletId}/ledger` | JWT + role `wallet-internal` |
| `POST` | `/wallets/{walletId}/reconciliation` | JWT + role `wallet-internal` |
| `POST` | `/wagering/transactions` | JWT + claim `provider_id` (handler: body == claim) |
| `GET` | `/wagering/transactions/{transactionId}` | JWT + claim; tx de outro provedor → **403** |
| `GET` | `/providers/{providerId}/wagering/transactions/{externalTransactionId}` | `ProtectProviderPath` (claim == path) |
| `GET` | `/health/live` | público |
| `GET` | `/health/ready` | público (ping Postgres + `GetQueueAttributes` na fila de entrada) |

Erros de use case → HTTP: `400` invalid, `404` not found, `409` conflict, `500` demais.  
Auth → `401` unauthorized, `403` forbidden.  
POST wagering: `REJECTED` / `PENDING_REFERENCE` / replay → **200** com `status` (e `failureCode` se houver) no body.

## WagerTransaction

### Agregado

`Transaction` em `internal/domain/wager` é o agregado de negócio do README (`WagerTransaction`).

- Estado encapsulado + getters; erros sentinela (`errors.Is`); sem Fx/HTTP/pgx.
- IDs e timestamps vêm da application layer.
- Dinheiro só via `money.Money` (`amount_minor` + `currency` no SQL).
- Criação ≠ reidratação: `NewOpening` / `NewExternal` validam regras; `FromPersisted` reconstrói sem transição.

| Construtor | Origem | Status inicial |
| --- | --- | --- |
| `NewOpening` | `INTERNAL` | `PROCESSED` (README §9) |
| `NewExternal` | `EXTERNAL` | `PENDING` |
| `FromPersisted` | qualquer | preserva |

`NewExternal` rejeita `OPENING`. REFUND/ROLLBACK exigem `referenceExternalTransactionID`.

### Kinds e amounts

| Kind | Origem | Amount |
| --- | --- | --- |
| `OPENING` | INTERNAL | ≥ 0 |
| `BET` / `WIN` / `REFUND` / `ROLLBACK` | EXTERNAL | > 0 |
| `LOSS` | EXTERNAL | `0.00` |

Movimento de carteira (CREDIT/DEBIT) e resolução cruzada de referência ficam no use case — o domínio só guarda estado e FSM.

### FSM

```text
PENDING → PENDING_REFERENCE | PROCESSED | REJECTED | FAILED
PENDING_REFERENCE → PROCESSED | REJECTED | FAILED
PROCESSED | REJECTED | FAILED  (terminal → ErrTerminalStatus)
```

Métodos (value receiver → novo estado): `AwaitReference(until, now)`, `MarkProcessed`, `MarkRejected`, `MarkFailed`, `ResolveReference`.  
`AwaitReference` grava `pendingReferenceUntil`; estados terminais zeram o campo.

### Failure codes (estáveis)

`INSUFFICIENT_FUNDS`, `REVERSAL_INSUFFICIENT_FUNDS`, `REFERENCE_NOT_FOUND`, `REFERENCE_NOT_PROCESSED`, `DUPLICATE_REVERSAL`, `INVALID_AMOUNT`, `INVALID_KIND`, `INVALID_TRANSITION`.

Conflito de idempotência → `usecase.ErrConflict` (não é failure code na tx).

### Persistência (`migrations/000002` + `000003`)

Tabela `wager_transactions` com CHECKs de origin/kind/status/amount e:

- INTERNAL ⇒ `kind = OPENING` e colunas externas NULL
- EXTERNAL ⇒ `kind <> OPENING` e campos de provedor NOT NULL
- `UNIQUE (wallet_id) WHERE kind = OPENING` — um crédito inicial por carteira
- `UNIQUE (provider_id, external_transaction_id)` e `UNIQUE (idempotency_key)` só em EXTERNAL
- FK `wallet_id → wallets(id)`
- `pending_reference_until` (000003) + índice parcial para o worker de referências

Port `gateway.WagerRepository` / impl `database.WagerRepo` (Insert/Update/lookups/`ListPendingReference`/`ListPendingReferenceDue`).

### Use case (`internal/usecase/wager`)

`processInTx` é o núcleo compartilhado por HTTP (`Process`) e SQS (`ProcessFromQueue`):

1. Idempotência por chave + `payloadHash` (replay ou conflito)
2. Unicidade `(providerId, externalTransactionId)`
3. `GetByIDForUpdate` na wallet
4. BET debit / WIN credit / LOSS sem ledger / REFUND·ROLLBACK com ref
5. Ref ausente → `PENDING_REFERENCE` com TTL **15m** (`PendingReferenceTTL`)
6. Commit atômico: wager + saldo + ledger + **outbox** (+ **inbox** no caminho SQS)

`ResumePendingReference` retoma ou expira (`REFERENCE_NOT_FOUND`) pendências; o worker `PendingRef` faz poll periódico.

#### Hash canônico do payload

`CanonicalPayloadHash`: SHA-256 hex de JSON com chaves ordenadas. Campos: `providerId`, `externalTransactionId`, `playerId`, `walletId`, `roundId`, `gameId`, `kind`, `amount`, `currency`, e `referenceExternalTransactionId` quando presente. Exclui `Idempotency-Key` e metadados de transporte.

#### HTTP wagering

Handler em `internal/web/wager_handler.go` chama `CanonicalPayloadHash` + `Process` / `Get` / `GetByExternal`.  
Header `Idempotency-Key` obrigatório no POST (não é substituído pelo servidor).

Auth: `Authenticate` nas rotas genéricas; `ProtectProviderPath` na rota com `{providerId}` no path (reusa `RequireProvider`). Isolamento restante (body/tx) no handler.

## Inbox + Outbox + SQS

### Schemas (`migrations/000004`)

| Tabela | Papel |
| --- | --- |
| `outbox_events` | snapshot imutável do envelope; `published_at NULL` = pendente; claim com `locked_at`/`locked_by` |
| `inbox_messages` | dedup de transporte `(consumer_name, message_id)` + `payload_hash`; `completed_at` após commit |

Nunca se publica na fila antes do commit da TX de domínio.

### Eventos (`internal/domain/event`)

Envelope: `eventId`, `eventType`, `aggregateId`, `correlationId`, `occurredAt` (UTC RFC3339), `version`, `data` (money em `"XX.YY"`).

| `event_type` | Quando |
| --- | --- |
| `WagerTransactionProcessed` | PROCESSED (incl. LOSS e OPENING) |
| `WagerTransactionRejected` | REJECTED |
| `WalletBalanceChanged` | saldo mudou (Open>0, BET/WIN/REFUND/ROLLBACK) |
| `WagerTransactionPendingReference` | entrou em PENDING_REFERENCE |

### Filas (LocalStack)

| Fila | Uso |
| --- | --- |
| `wager-transactions.fifo` | entrada (consumer) |
| `wager-transactions-dlq.fifo` | DLQ + redrive (`maxReceiveCount=5`) |
| `domain-events.fifo` | destino do publisher de outbox |

Contrato de transporte (entrada):

- `MessageGroupId` = `walletId` (serializa por carteira)
- `MessageDeduplicationId` = `idempotencyKey` (dedup de transporte; **não** substitui inbox/idempotência financeira)

Saída (publisher): `MessageGroupId` = `aggregateId`; `MessageDeduplicationId` = `eventId` (republicações preservam o id).

### Workers (`internal/messaging`, mesmo processo Fx)

| Worker | Comportamento |
| --- | --- |
| Consumer | long poll → `ProcessFromQueue` (inbox + `processInTx` + outbox) → delete pós-commit; inválido/permanente → DLQ + delete; transitório → visibility |
| Publisher | `ClaimBatch` (`FOR UPDATE SKIP LOCKED`) → publish JSON → `MarkPublished`; falha → `MarkRetry` com backoff; reclaim se lock antigo (>30s) |
| PendingRef | lista `PENDING_REFERENCE` → `ResumePendingReference` |

`ProcessFromQueue`: inbox completed → replay (ack); hash mismatch → `ErrPermanent` (DLQ); `PENDING_REFERENCE` completa a inbox (worker de refs continua).

### Política REFUND + ROLLBACK (README §7)

- Duplicata do **mesmo kind** (dois REFUND ou dois ROLLBACK da mesma referência) → `DUPLICATE_REVERSAL`.
- Sobre a **mesma BET**: REFUND e ROLLBACK são mutuamente exclusivos — ambos creditam o débito original; o segundo em `PROCESSED` é rejeitado com `DUPLICATE_REVERSAL`.
- `ROLLBACK` de um `WIN` ou de um `REFUND` já processado debita (desfaz o crédito); não conflita com a política acima porque a referência não é a BET.

### Limitações / interpretações

- Métricas: contadores in-process em `GET /metrics` (status, replays, conflitos, DLQ, outbox, divergência de reconcile); logs JSON via `slog` no boot.
- Status `FAILED` existe no domínio para falha de infra; o caminho feliz atual usa `REJECTED` / `PENDING_REFERENCE` / `PROCESSED`. Falhas transitórias de I/O propagam erro sem marcar a tx.
- Worker de `PENDING_REFERENCE` usa TTL 15m + backoff exponencial por id em memória (reinício reprocessa imediatamente).
- Tracing OTel e load test ficam fora (diferenciais).

## Autenticação e autorização

### IdP

**Keycloak** no Docker Compose (`deploy/keycloak/realm-jungle.json`), fluxo `client_credentials` entre serviços. A API **não** emite tokens nem cadastra senhas.

Validação: `github.com/coreos/go-oidc/v3` — discovery do issuer + JWKS, verificação de assinatura/`exp`/`iss`/`aud`. Boot falha se `OIDC_ISSUER_URL` estiver ausente ou o IdP inacessível (fail-fast).

### Modelo

| Ator | Client | Claims / roles | Acesso |
| --- | --- | --- | --- |
| Serviço interno | `jungle-internal` | realm role `wallet-internal`; `aud` inclui `jungle-api` | `/wallets*` |
| Provedor | `provider-a` | claim `provider_id`; sem role interna | `/wagering*` e `/providers/{providerId}/wagering/*`; **403** em wallet |
| Anônimo | — | — | só health |

`Principal` no `context` (`subject`, `roles`, `providerId`) — uso em logs e authz futura (`RequireProvider`). Domínio permanece sem dependência de auth.

### Por que wallet = interno

README §2: operações de carteira restritas ao serviço interno; provedores só acessam as próprias transações de wagering. Abertura/leitura/reconciliação de carteira não são APIs de provedor.
